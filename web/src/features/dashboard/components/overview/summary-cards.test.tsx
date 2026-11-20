// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SummaryCards } from './summary-cards'

type Breakdown = { total: { quota: number; count: number; token_used: number } } | undefined

const mocks = vi.hoisted(() => ({
  current: undefined as Breakdown,
  previous: undefined as Breakdown,
  errors: undefined as number | undefined,
  lowBalance: null as null | { reason: 'runway'; balanceQuota: number; days: number },
  last7DaysQuota: 0 as number | undefined,
  currentStart: 0,
}))

// Stable t, as in spend-breakdown-card.test.tsx.
const i18n = vi.hoisted(() => ({
  t: (key: string, options?: Record<string, unknown>) =>
    options
      ? Object.entries(options).reduce(
          (text, [name, value]) => text.replace(`{{${name}}}`, String(value)),
          key,
        )
      : key,
  i18n: { language: 'en', resolvedLanguage: 'en' },
}))

vi.mock('react-i18next', () => ({ useTranslation: () => i18n }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('./use-overview-usage', () => ({
  useCurrentMinute: () => new Date(2026, 10, 19, 12, 0, 0),
  // The current range is told apart from the previous one by its start.
  useSelfBreakdown: (range: { start_timestamp: number } | null) => ({
    data: range?.start_timestamp === mocks.currentStart ? mocks.current : mocks.previous,
    isError: false,
  }),
  useSelfErrorCount: () => ({ data: mocks.errors }),
  useSpendKpiQueries: () => ({ last7DaysQuota: mocks.last7DaysQuota }),
}))
vi.mock('./low-balance-banner', () => ({ useLowBalance: () => mocks.lowBalance }))
vi.mock('@/stores/auth-store', () => {
  const state = () => ({ auth: { user: { id: 1, role: 1, quota: 500000 }, session: { sid: 's' } } })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) => selector(state()),
      { getState: state },
    ),
  }
})

const range = {
  start: new Date(2026, 10, 19, 0, 0, 0),
  end: new Date(2026, 10, 19, 12, 0, 0),
}

beforeEach(() => {
  mocks.currentStart = Math.floor(range.start.getTime() / 1000)
  mocks.current = { total: { quota: 300000, count: 90, token_used: 0 } }
  mocks.previous = { total: { quota: 200000, count: 60, token_used: 0 } }
  mocks.errors = 10
  mocks.lowBalance = null
  mocks.last7DaysQuota = 700000
})

afterEach(() => {
  cleanup()
})

describe('SummaryCards', () => {
  it('shows balance, spend and requests for the selected range', () => {
    render(<SummaryCards range={range} />)

    expect(screen.getByText('Credit remaining')).toBeTruthy()
    expect(screen.getByText('Spend in this period')).toBeTruthy()
    // 90 successful + 10 failed requests.
    expect(screen.getByText('100')).toBeTruthy()
    expect(screen.queryByText('Spend today')).toBeNull()
    expect(screen.queryByText('Spend this month')).toBeNull()
  })

  it('compares spend and requests with the previous period', () => {
    render(<SummaryCards range={range} />)

    // 300,000 vs 200,000 quota and 90 vs 60 successful requests: both +50%.
    const badges = screen.getAllByText(/^▲ 50/)
    expect(badges).toHaveLength(2)
    // Spending more is not shown as good or bad news.
    expect(badges[0].className).toContain('text-muted-foreground')
  })

  it('flags an error rate above 5%', () => {
    render(<SummaryCards range={range} />)

    const rate = screen.getByText(/^Error rate 10/)
    expect(rate.className).toContain('text-destructive')
    expect(rate.textContent).toContain('10 errors')
  })

  it('shows the runway from recent spending and no separate low-balance label', () => {
    render(<SummaryCards range={range} />)

    // 500,000 balance at 100,000 per day.
    expect(screen.getByText('Enough for about 5 days at your recent spending')).toBeTruthy()
    expect(screen.queryByText('Low balance')).toBeNull()
    expect(screen.getByText('Top up').closest('a')?.getAttribute('href')).toBe('/wallet')
  })

  it('hides the runway without recent spending and never shows pending data as zero', () => {
    mocks.last7DaysQuota = 0
    mocks.current = undefined
    mocks.errors = undefined
    render(<SummaryCards range={range} />)

    expect(screen.queryByText(/at your recent spending/)).toBeNull()
    expect(screen.getAllByText('—')).toHaveLength(2)
  })

  it('leaves the runway and Top up to the banner while it shows', () => {
    mocks.lowBalance = { reason: 'runway', balanceQuota: 500000, days: 2 }
    render(<SummaryCards range={range} />)

    expect(screen.queryByText(/at your recent spending/)).toBeNull()
    expect(screen.queryByText('Top up')).toBeNull()
    expect(screen.getByText('Credit remaining')).toBeTruthy()
  })
})
