import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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
vi.mock('../admin-usage-tracking-card', () => ({
  AdminUsageTrackingCard: (props: { excludeInternal?: boolean }) => (
    <div>usage-tracking exclude={String(props.excludeInternal)}</div>
  ),
}))
vi.mock('../recent-usage-panel', () => ({
  RecentUsagePanel: (props: {
    range?: { start_timestamp: number } | null
    limit?: number
  }) => (
    <div>
      recent-usage from={props.range?.start_timestamp ?? 'all'} limit=
      {props.limit}
    </div>
  ),
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
    // Credited quota includes bonus and old USDT snapshots: 1,950 credits at
    // 500,000 quota per credit. The KPI must use paid_vnd instead.
    credited_quota: 975_000_000,
    paid_vnd: 1_700_000, // 1,700 credits
    bonus_vnd: 250_000, // 250 credits
    count: 12,
    pending_count: 3,
    by_method: [
      { method: 'sepay', currency: 'VND', amount: 1_200_000, count: 10, paid_vnd: 1_200_000, bonus_vnd: 200_000 },
      { method: 'usdt', currency: 'USDT', amount: 50, count: 2, paid_vnd: 500_000, bonus_vnd: 50_000 },
    ],
  },
  users: {
    new_users: 40,
    new_google_users: 9,
    signup_quota_granted: 5_000_000, // 10 credits
    active_users: 77,
  },
  requests: { total: 2000, errors: 50 },
  usage: { consumed_quota: 50_000_000 },
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
  window.localStorage.clear()
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

    // Revenue is the money paid, not the credited quota (which includes bonus).
    expect(await screen.findByText('Top-up revenue')).toBeTruthy()
    expect(screen.getByText('1,700 credit')).toBeTruthy()
    expect(screen.queryByText('1,950 credit')).toBeNull()
    expect(screen.queryByText(/975,000,000/)).toBeNull()
    expect(screen.getByText('+ 250 credit bonus granted')).toBeTruthy()
    expect(screen.getByText('SePay 1,200,000 VND')).toBeTruthy()
    expect(screen.getByText('USDT 50')).toBeTruthy()

    expect(screen.getByText('≈ 1,700,000 VND · 12 orders')).toBeTruthy()

    // Pending top-ups are a prominent banner linking to the review page.
    const banner = screen.getByText('3 top-up orders need review').closest('a')
    expect(banner?.getAttribute('href')).toBe('/topup-management')

    expect(screen.getByText('Credits customers used')).toBeTruthy()
    expect(screen.getByText('100 credit')).toBeTruthy()

    expect(screen.getByText('77')).toBeTruthy()
    expect(screen.getByText('40 new signups (9 via Google)')).toBeTruthy()
    expect(screen.getByText('Granted 10 credit at signup')).toBeTruthy()

    expect(screen.getByText('2,000')).toBeTruthy()
    expect(screen.getByText(/Error rate 2\.5%/).className).not.toContain('text-destructive')

    expect(mocks.summary).toHaveBeenCalledWith({
      start_timestamp: Math.floor(range.start.getTime() / 1000),
      end_timestamp: Math.floor(range.end.getTime() / 1000),
      exclude_internal: true,
    })
  })

  it('hides internal accounts by default and remembers turning it off', async () => {
    const { unmount } = renderOverview()

    expect(await screen.findByText('usage-tracking exclude=true')).toBeTruthy()
    expect(mocks.errorSummary).toHaveBeenCalledWith(
      expect.objectContaining({ exclude_internal: true })
    )
    expect(mocks.quotaDates).toHaveBeenCalledWith(
      expect.objectContaining({ exclude_internal: true }),
      true
    )

    fireEvent.click(screen.getByRole('switch'))

    expect(await screen.findByText('usage-tracking exclude=false')).toBeTruthy()
    await waitFor(() =>
      expect(mocks.summary).toHaveBeenLastCalledWith(
        expect.objectContaining({ exclude_internal: false })
      )
    )
    expect(window.localStorage.getItem('admin-overview:exclude-internal')).toBe('false')

    unmount()
    renderOverview()
    expect(await screen.findByText('usage-tracking exclude=false')).toBeTruthy()
  })

  it('passes the page range to the recent requests panel', async () => {
    renderOverview()

    expect(
      await screen.findByText(
        `recent-usage from=${Math.floor(range.start.getTime() / 1000)} limit=5`
      )
    ).toBeTruthy()
  })

  it('states how many errors are grouped when the summary is truncated', async () => {
    mocks.errorSummary.mockResolvedValue({
      success: true,
      data: {
        total: 5000,
        truncated: true,
        categories: [
          { category: 'insufficient_balance', count: 5000, last_at: 1_700_000_000, model_name: 'gpt-4o' },
        ],
      },
    })
    renderOverview()

    expect(
      await screen.findByText(
        'Counts cover only the 5,000 most recent errors in this period; older errors are not grouped here.'
      )
    ).toBeTruthy()
  })

  it('shows top-up revenue as VND / 1,000 credits with a custom display currency', async () => {
    // Production display: CUSTOM currency at 26,000. Quota values go through
    // the currency rate, but order VND must stay 1 credit = 1,000 VND.
    useSystemConfigStore.getState().setConfig({
      currency: { ...DEFAULT_CURRENCY_CONFIG },
    })
    renderOverview()

    expect(await screen.findByText('1,700 credit')).toBeTruthy()
    expect(screen.getByText('+ 250 credit bonus granted')).toBeTruthy()
    expect(screen.queryByText(/44,200/)).toBeNull()
  })

  it('labels a rise from an almost empty previous period as new', async () => {
    const currentStart = Math.floor(range.start.getTime() / 1000)
    mocks.summary.mockImplementation(async (params: { start_timestamp: number }) => ({
      success: true,
      data:
        params.start_timestamp === currentStart
          ? summaryData
          : { ...summaryData, requests: { total: 10, errors: 0 } },
    }))
    renderOverview()

    // 2,000 vs 10 requests = +19,900%: no percentage is shown.
    expect(await screen.findByText('New')).toBeTruthy()
    expect(screen.queryByText(/999%|19,900/)).toBeNull()
  })

  it('compares each KPI with the previous period and flags a high error rate', async () => {
    const currentStart = Math.floor(range.start.getTime() / 1000)
    mocks.summary.mockImplementation(async (params: { start_timestamp: number }) => ({
      success: true,
      data:
        params.start_timestamp === currentStart
          ? { ...summaryData, requests: { total: 2000, errors: 120 } }
          : {
              ...summaryData,
              topup: { ...summaryData.topup, paid_vnd: 2_000_000 },
              users: { ...summaryData.users, active_users: 70 },
              requests: { total: 1600, errors: 10 },
              usage: { consumed_quota: 0 },
            },
    }))
    renderOverview()

    // 1.7M vs 2M paid, 77 vs 70 active, 2,000 vs 1,600 requests.
    expect(await screen.findByText('▼ 15%')).toBeTruthy()
    expect(screen.getByText('▲ 10%')).toBeTruthy()
    expect(screen.getByText('▲ 25%')).toBeTruthy()
    // No consumption in the previous period: no percentage.
    expect(screen.getAllByText(/[▲▼]/)).toHaveLength(3)
    // The previous window is the same clock window one day earlier.
    expect(mocks.summary).toHaveBeenCalledWith(
      expect.objectContaining({
        start_timestamp: currentStart - 24 * 60 * 60,
        end_timestamp: Math.floor(range.end.getTime() / 1000) - 24 * 60 * 60,
      })
    )
    // 120 / 2,000 = 6% is above the 5% alert threshold.
    expect(screen.getByText(/Error rate 6%/).className).toContain('text-destructive')
  })

  it('hides the pending banner when no top-ups are waiting', async () => {
    mocks.summary.mockResolvedValue({
      success: true,
      data: { ...summaryData, topup: { ...summaryData.topup, pending_count: 0 } },
    })
    renderOverview()

    expect(await screen.findByText('1,700 credit')).toBeTruthy()
    expect(screen.queryByText(/need review/)).toBeNull()
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

  it('drops the duplicate distribution chart and the models link', async () => {
    renderOverview()

    expect(await screen.findByText('Credits consumed')).toBeTruthy()
    expect(screen.queryByText('View model call analytics on the Models tab')).toBeNull()
    expect(screen.queryByText('Quota Distribution')).toBeNull()
    expect(screen.getByText(/^usage-tracking/)).toBeTruthy()
    expect(screen.getByText(/^recent-usage/)).toBeTruthy()
  })
})
