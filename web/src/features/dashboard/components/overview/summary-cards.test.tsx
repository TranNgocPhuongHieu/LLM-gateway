// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SummaryCards } from './summary-cards'

const mocks = vi.hoisted(() => ({
  spend: {
    todayQuota: 250000,
    monthQuota: 1500000,
    lastMonthSamePeriodQuota: 1000000,
    last7DaysQuota: 700000,
    isError: false,
  },
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
  useSpendKpiQueries: () => mocks.spend,
}))
vi.mock('@/stores/auth-store', () => {
  const state = () => ({ auth: { user: { id: 1, role: 1, quota: 500000 }, session: { sid: 's' } } })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) => selector(state()),
      { getState: state },
    ),
  }
})

afterEach(() => {
  cleanup()
})

describe('SummaryCards', () => {
  it('renders the credit, today and month cards', () => {
    render(<SummaryCards />)

    expect(screen.getByText('Credit remaining')).toBeTruthy()
    expect(screen.getByText('Spend today')).toBeTruthy()
    expect(screen.getByText('Spend this month')).toBeTruthy()
  })

  it('no longer renders the projected run-out card for any user', () => {
    render(<SummaryCards />)

    expect(screen.queryByText('Projected run-out')).toBeNull()
    expect(screen.queryByText('Balance depleted')).toBeNull()
    expect(screen.queryByText('No recent usage')).toBeNull()
  })
})
