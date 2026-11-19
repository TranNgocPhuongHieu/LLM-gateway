import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SpendBreakdownCard, formatShare } from './spend-breakdown-card'

const mocks = vi.hoisted(() => ({ breakdown: vi.fn() }))
// Stable t, as in binance-usdt-topup.test.tsx.
const i18n = vi.hoisted(() => ({
  t: (key: string, options?: Record<string, unknown>) =>
    options
      ? Object.entries(options).reduce(
          (text, [name, value]) => text.replace(`{{${name}}}`, String(value)),
          key,
        )
      : key,
  i18n: { language: 'en' },
}))

vi.mock('react-i18next', () => ({ useTranslation: () => i18n }))
vi.mock('../../api', () => ({ getSelfUsageBreakdown: mocks.breakdown }))
vi.mock('@/stores/auth-store', () => {
  const state = () => ({ auth: { user: { id: 1 }, session: { sid: 's' } } })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) => selector(state()),
      { getState: state },
    ),
  }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

const modelData = {
  success: true,
  data: {
    total: { quota: 400000, count: 3, token_used: 90 },
    items: [{ key: 'gpt-4o', label: 'gpt-4o', quota: 400000, count: 3, token_used: 90 }],
  },
}
const tokenData = {
  success: true,
  data: {
    total: { quota: 400000, count: 3, token_used: 90 },
    items: [
      { key: '7', label: 'Production', quota: 300000, count: 2, token_used: 60 },
      { key: '9', label: '', quota: 100000, count: 1, token_used: 30 },
    ],
  },
}

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(
    <QueryClientProvider client={client}>
      <SpendBreakdownCard range={{ start: new Date(2026, 10, 1), end: new Date(2026, 10, 19) }} />
    </QueryClientProvider>,
  )
}

describe('SpendBreakdownCard', () => {
  it('labels a deleted key and computes each share of the total', async () => {
    mocks.breakdown.mockImplementation(async (params: { group_by: string }) =>
      params.group_by === 'token' ? tokenData : modelData,
    )
    renderCard()

    expect(await screen.findByText('gpt-4o')).toBeTruthy()
    fireEvent.click(screen.getByRole('tab', { name: 'By API key' }))

    expect(await screen.findByText('Production')).toBeTruthy()
    expect(screen.getByText('Key #9 (deleted)')).toBeTruthy()
    expect(screen.getByText('75.0%')).toBeTruthy()
    expect(screen.getByText('25.0%')).toBeTruthy()
    expect(mocks.breakdown).toHaveBeenCalledWith(expect.objectContaining({ group_by: 'token' }))
  })

  it('shows zero shares without dividing by zero when nothing was spent', async () => {
    mocks.breakdown.mockResolvedValue({
      success: true,
      data: {
        total: { quota: 0, count: 0, token_used: 0 },
        items: [{ key: 'gpt-4o', label: 'gpt-4o', quota: 0, count: 0, token_used: 0 }],
      },
    })
    renderCard()
    expect(await screen.findByText('0.0%')).toBeTruthy()
  })

  it('shows a quiet error message when the breakdown request fails', async () => {
    mocks.breakdown.mockRejectedValue(new Error('network'))
    renderCard()
    expect(await screen.findByText('Could not load usage. Try again later.')).toBeTruthy()
  })
})

describe('formatShare', () => {
  it('uses the UI locale decimal separator', () => {
    expect(formatShare(49.94, 'vi')).toBe('49,9%')
    expect(formatShare(49.94, 'en')).toBe('49.9%')
  })
})
