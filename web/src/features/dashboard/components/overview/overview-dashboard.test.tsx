import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { OverviewDashboard } from './overview-dashboard'
import type { RangeState, TimeRange } from './overview-time-range'

const mocks = vi.hoisted(() => {
  const now = new Date(2026, 10, 19, 12, 30)
  return {
    now,
    role: 1,
    rangeState: undefined as RangeState | undefined,
    dailyRange: undefined as TimeRange | null | undefined,
    getApiKeys: vi.fn(),
    getUserModels: vi.fn(),
  }
})

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
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock('@/components/layout', () => {
  const Pass = ({ children }: { children?: React.ReactNode }) => <div>{children}</div>
  return { SectionPageLayout: Object.assign(Pass, { Title: Pass, Actions: Pass, Content: Pass }) }
})
vi.mock('@/components/page-transition', () => {
  const Pass = ({ children }: { children?: React.ReactNode }) => <div>{children}</div>
  return { CardStaggerContainer: Pass, CardStaggerItem: Pass }
})
vi.mock('@/stores/auth-store', () => {
  const state = () => ({
    auth: {
      user: { id: 1, role: mocks.role, quota: 0, used_quota: 0, request_count: 0 },
      session: { sid: 's' },
    },
  })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) =>
        selector(state()),
      { getState: state }
    ),
  }
})
vi.mock('@/features/keys/api', () => ({
  getApiKeys: mocks.getApiKeys,
  fetchTokenKey: vi.fn(),
}))
vi.mock('@/lib/api', () => ({ getUserModels: mocks.getUserModels }))
vi.mock('../../hooks/use-status-data', () => ({
  useApiInfo: () => ({ items: [] }),
}))
vi.mock('./use-overview-usage', () => ({
  useCurrentMinute: () => mocks.now,
}))
vi.mock('./recent-usage-panel', () => ({
  RecentUsagePanel: (props: { range?: { start_timestamp: number } | null; limit?: number }) => (
    <div>
      recent-usage {props.range ? `from-${props.range.start_timestamp}` : 'all-time'} limit-{props.limit}
    </div>
  ),
  useDashboardBalance: () => undefined,
}))
vi.mock('./summary-cards', () => ({
  SummaryCards: (props: { range: TimeRange | null }) => (
    <div>customer-summary-cards {props.range ? 'with-range' : 'no-range'}</div>
  ),
}))
vi.mock('./low-balance-banner', () => ({
  LowBalanceBanner: () => <div>low-balance</div>,
}))
vi.mock('./daily-cost-chart', () => ({
  DailyCostChart: (props: { range: TimeRange | null }) => {
    mocks.dailyRange = props.range
    return <div>customer-daily-cost</div>
  },
}))
vi.mock('./spend-breakdown-card', () => ({ SpendBreakdownCard: () => null }))
vi.mock('./recent-errors-card', () => ({ RecentErrorsCard: () => null }))
vi.mock('./overview-range-picker', () => ({
  OverviewRangePicker: (props: { value: RangeState }) => {
    mocks.rangeState = props.value
    return <div>range-picker</div>
  },
}))
vi.mock('./admin/admin-overview', () => ({
  AdminOverview: () => <div>admin-overview</div>,
}))

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <OverviewDashboard />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  mocks.rangeState = undefined
  mocks.dailyRange = undefined
  vi.clearAllMocks()
})

describe('OverviewDashboard default range', () => {
  it('opens on today and resolves it from local midnight to now', () => {
    mocks.role = 1
    mocks.getApiKeys.mockResolvedValue({ success: true, data: { items: [] } })
    mocks.getUserModels.mockResolvedValue({ success: true, data: [] })
    renderPage()

    expect(mocks.rangeState?.preset).toBe('today')
    expect(mocks.dailyRange?.start.getTime()).toBe(new Date(2026, 10, 19).getTime())
    expect(mocks.dailyRange?.end.getTime()).toBe(mocks.now.getTime())
  })
})

describe('OverviewDashboard for customers', () => {
  it('keeps the customer view with setup guide and self summary cards', async () => {
    mocks.role = 1
    mocks.getApiKeys.mockResolvedValue({ success: true, data: { items: [] } })
    mocks.getUserModels.mockResolvedValue({ success: true, data: [] })
    renderPage()

    // Every panel follows the shared range picker (Today by default).
    expect(screen.getByText('customer-summary-cards with-range')).toBeTruthy()
    expect(screen.getByText('low-balance')).toBeTruthy()
    expect(screen.getByText('customer-daily-cost')).toBeTruthy()
    expect(screen.getByText(/^recent-usage from-\d+ limit-5$/)).toBeTruthy()
    expect(screen.queryByText('admin-overview')).toBeNull()
    expect(await screen.findByText('Setup guide')).toBeTruthy()
  })
})

describe('OverviewDashboard for super admins', () => {
  it('renders AdminOverview instead of customer content', async () => {
    mocks.role = 100
    mocks.getApiKeys.mockResolvedValue({ success: true, data: { items: [] } })
    mocks.getUserModels.mockResolvedValue({ success: true, data: [] })
    renderPage()

    expect(screen.getByText('admin-overview')).toBeTruthy()
    expect(screen.getByText('range-picker')).toBeTruthy()
    expect(screen.queryByText('customer-summary-cards')).toBeNull()
    expect(screen.queryByText('low-balance')).toBeNull()
    expect(screen.queryByText('customer-daily-cost')).toBeNull()
    expect(screen.queryByText('Setup guide')).toBeNull()
    expect(screen.queryByText('Get started')).toBeNull()
    expect(screen.queryByText('Recommended actions')).toBeNull()
    expect(screen.queryByText('recent-usage')).toBeNull()

    // Customer-only setup queries never run for admins.
    await waitFor(() => expect(mocks.getUserModels).not.toHaveBeenCalled())
    expect(mocks.getApiKeys).not.toHaveBeenCalled()
  })
})
