import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { AdminOverview } from './admin-overview'

const mocks = vi.hoisted(() => ({
  summary: vi.fn(),
  errorSummary: vi.fn(),
  quotaDates: vi.fn(),
}))

// Stable t: react-i18next must not return a new function on each render.
const i18n = vi.hoisted(() => ({
  t: (key: string, options?: Record<string, unknown>) =>
    options
      ? Object.entries(options).reduce(
          (text, [name, value]) => text.replace(`{{${name}}}`, String(value)),
          key
        )
      : key,
  i18n: { language: 'en' },
}))

vi.mock('react-i18next', () => ({ useTranslation: () => i18n }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('@/lib/vchart', () => ({ VChart: () => null, VCHART_OPTION: {} }))
vi.mock('@/lib/use-chart-theme', () => ({
  useChartTheme: () => ({ resolvedTheme: 'light', themeReady: true }),
}))
vi.mock('../../../api', () => ({
  getAdminSummary: mocks.summary,
  getAdminErrorSummary: mocks.errorSummary,
  getUserQuotaDates: mocks.quotaDates,
}))
vi.mock('../../models/consumption-distribution-chart', () => ({
  ConsumptionDistributionChart: () => <div>quota-distribution</div>,
}))
vi.mock('../admin-usage-tracking-card', () => ({
  AdminUsageTrackingCard: () => <div>usage-tracking</div>,
}))
vi.mock('../recent-usage-panel', () => ({
  RecentUsagePanel: () => <div>recent-usage</div>,
}))
vi.mock('@/stores/auth-store', () => {
  const state = () => ({ auth: { user: { id: 1 }, session: { sid: 's' } } })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) =>
        selector(state()),
      { getState: state }
    ),
  }
})

const range = { start: new Date(2026, 10, 19), end: new Date(2026, 10, 19, 12) }

const summaryData = {
  topup: {
    credited_quota: 975_000_000, // 1,950 credits at 500,000 quota per credit
    count: 12,
    pending_count: 3,
    by_method: [
      { method: 'sepay', currency: 'VND', amount: 1_200_000, count: 10 },
      { method: 'usdt', currency: 'USDT', amount: 50, count: 2 },
    ],
  },
  users: {
    new_users: 40,
    new_google_users: 9,
    signup_quota_granted: 5_000_000, // 10 credits
    active_users: 77,
  },
  requests: { total: 2000, errors: 50 },
}

function renderOverview() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AdminOverview range={range} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  // Credits are shown 1:1 with quota / 500,000 in USD display mode.
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
  mocks.summary.mockResolvedValue({ success: true, data: summaryData })
  mocks.errorSummary.mockResolvedValue({
    success: true,
    data: {
      total: 2,
      truncated: false,
      categories: [
        { category: 'insufficient_balance', count: 2, last_at: 1_700_000_000, model_name: 'gpt-4o' },
      ],
    },
  })
  mocks.quotaDates.mockResolvedValue({ success: true, data: [] })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('AdminOverview', () => {
  it('shows system-wide KPI cards with credits instead of raw quota', async () => {
    renderOverview()

    expect(await screen.findByText('1,950 credit')).toBeTruthy()
    expect(screen.queryByText(/975,000,000/)).toBeNull()
    expect(screen.getByText('≈ 1,950,000 VND')).toBeTruthy()
    expect(screen.getByText('SePay 1,200,000 VND')).toBeTruthy()
    expect(screen.getByText('USDT 50')).toBeTruthy()

    expect(screen.getByText('12')).toBeTruthy()
    const pending = screen.getByText('3 pending').closest('a')
    expect(pending?.getAttribute('href')).toBe('/topup-management')

    expect(screen.getByText('40')).toBeTruthy()
    expect(screen.getByText('9 via Google')).toBeTruthy()
    expect(screen.getByText('Granted 10 credit at signup')).toBeTruthy()

    expect(screen.getByText('77')).toBeTruthy()
    expect(screen.getByText('2,000')).toBeTruthy()
    expect(screen.getByText('Error rate 2.5%')).toBeTruthy()

    expect(mocks.summary).toHaveBeenCalledWith({
      start_timestamp: Math.floor(range.start.getTime() / 1000),
      end_timestamp: Math.floor(range.end.getTime() / 1000),
    })
  })

  it('hides the pending link when no top-ups are waiting', async () => {
    mocks.summary.mockResolvedValue({
      success: true,
      data: { ...summaryData, topup: { ...summaryData.topup, pending_count: 0 } },
    })
    renderOverview()

    expect(await screen.findByText('1,950 credit')).toBeTruthy()
    expect(screen.queryByText('0 pending')).toBeNull()
    expect(screen.queryByText(/pending/)).toBeNull()
  })

  it('shows a system summary error without customer content', async () => {
    mocks.summary.mockRejectedValue(new Error('500'))
    renderOverview()

    expect(await screen.findByText('Could not load system summary.')).toBeTruthy()
  })

  it('groups system-wide errors by cause without customer top-up hints', async () => {
    renderOverview()

    expect(await screen.findByText('Insufficient balance')).toBeTruthy()
    expect(screen.queryByText('Top up your wallet')).toBeNull()
    expect(mocks.errorSummary).toHaveBeenCalledWith(
      expect.objectContaining({
        start_timestamp: Math.floor(range.start.getTime() / 1000),
        end_timestamp: Math.floor(range.end.getTime() / 1000),
      })
    )
  })

  it('links to model analytics instead of rendering model charts', async () => {
    renderOverview()

    const link = await screen.findByText('View model call analytics on the Models tab')
    expect(link.closest('a')?.getAttribute('href')).toBe('/dashboard/$section')
    expect(screen.getByText('quota-distribution')).toBeTruthy()
    expect(screen.getByText('usage-tracking')).toBeTruthy()
    expect(screen.getByText('recent-usage')).toBeTruthy()
  })
})
