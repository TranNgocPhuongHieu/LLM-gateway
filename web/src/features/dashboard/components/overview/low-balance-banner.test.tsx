import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { LowBalanceBanner } from './low-balance-banner'

const state = vi.hoisted(() => ({
  quota: 0,
  threshold: 500000,
  last7DaysQuota: 0,
}))
const i18n = vi.hoisted(() => ({
  t: (key: string, options?: Record<string, unknown>) =>
    options
      ? Object.entries(options).reduce(
          (text, [name, value]) => text.replace(`{{${name}}}`, String(value)),
          key,
        )
      : key,
}))

vi.mock('react-i18next', () => ({ useTranslation: () => i18n }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('@/lib/currency', () => ({
  formatQuotaWithCredit: (quota: number) => `${quota}q`,
}))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (value: unknown) => unknown) =>
    selector({ auth: { user: { id: 1, quota: state.quota } } }),
}))
vi.mock('./use-overview-usage', () => ({
  useCurrentMinute: () => new Date('2026-11-19T10:00:00'),
  useWarningThreshold: () => ({ data: state.threshold }),
  useSpendKpiQueries: () => ({ last7DaysQuota: state.last7DaysQuota }),
}))

afterEach(() => cleanup())

describe('LowBalanceBanner', () => {
  it('stays hidden when balance is above threshold and runway is healthy', () => {
    Object.assign(state, { quota: 5_000_000, threshold: 500_000, last7DaysQuota: 700_000 })
    const { container } = render(<LowBalanceBanner />)
    expect(container.textContent).toBe('')
  })

  it('stays hidden below the threshold when nothing was spent recently', () => {
    // New account holding only its signup credit.
    Object.assign(state, { quota: 96_154, threshold: 500_000, last7DaysQuota: 0 })
    const { container } = render(<LowBalanceBanner />)
    expect(container.textContent).toBe('')
  })

  it('explains the threshold when an active customer is below it', () => {
    Object.assign(state, { quota: 400_000, threshold: 500_000, last7DaysQuota: 70_000 })
    render(<LowBalanceBanner />)
    expect(screen.getByText('Balance 400000q · warning threshold 500000q')).toBeTruthy()
  })

  it('explains the short runway when above threshold but spending fast', () => {
    // 7-day spend 2,100,000 → 300,000 per day; balance 600,000 → 2 days.
    Object.assign(state, { quota: 600_000, threshold: 500_000, last7DaysQuota: 2_100_000 })
    render(<LowBalanceBanner />)
    expect(
      screen.getByText('Balance 600000q · about 2 days left at your recent spending'),
    ).toBeTruthy()
    expect(screen.getByText('Top up').closest('a')?.getAttribute('href')).toBe('/wallet')
  })
})
