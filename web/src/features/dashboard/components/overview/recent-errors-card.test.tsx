import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RecentErrorsCard } from './recent-errors-card'

const mocks = vi.hoisted(() => ({
  summary: vi.fn(),
  toast: vi.fn(),
}))
// Stable t: react-i18next must not return a new function on each render.
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
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('sonner', () => ({ toast: mocks.toast }))
vi.mock('../../api', () => ({ getSelfErrorSummary: mocks.summary }))
vi.mock('@/stores/auth-store', () => {
  const state = () => ({ auth: { user: { id: 1 }, session: { sid: 's' } } })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) => selector(state()),
      { getState: state },
    ),
  }
})

const range = { start: new Date(2026, 10, 13), end: new Date(2026, 10, 19) }

function renderCard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(
    <QueryClientProvider client={client}>
      <RecentErrorsCard range={range} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('RecentErrorsCard', () => {
  it('renders one row per category with its count and fix hint', async () => {
    mocks.summary.mockResolvedValue({
      success: true,
      data: {
        total: 3,
        truncated: false,
        categories: [
          { category: 'insufficient_balance', count: 2, last_at: 1_700_000_000, model_name: 'gpt-4o' },
          { category: 'model_unavailable', count: 1, last_at: 1_700_000_100, model_name: 'claude-x' },
        ],
      },
    })
    renderCard()

    expect(await screen.findByText('Insufficient balance')).toBeTruthy()
    expect(screen.getByText('Top up your wallet').closest('a')?.getAttribute('href')).toBe('/wallet')
    expect(screen.getByText('Model unavailable')).toBeTruthy()
    expect(screen.getByText('Check the model name on the Pricing page').closest('a')?.getAttribute('href')).toBe('/pricing')
    expect(screen.getByText('2')).toBeTruthy()
    expect(mocks.summary).toHaveBeenCalledWith(
      expect.objectContaining({
        start_timestamp: Math.floor(range.start.getTime() / 1000),
        end_timestamp: Math.floor(range.end.getTime() / 1000),
      }),
    )
  })

  it('shows the empty state when no errors were recorded', async () => {
    mocks.summary.mockResolvedValue({
      success: true,
      data: { total: 0, truncated: false, categories: [] },
    })
    renderCard()
    expect(await screen.findByText('No errors in this period')).toBeTruthy()
  })

  it('shows a quiet inline error without a toast when the endpoint fails', async () => {
    mocks.summary.mockRejectedValue(new Error('404 not found'))
    renderCard()

    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.getByText('Could not load recent errors.')).toBeTruthy()
    expect(screen.queryByText('No errors in this period')).toBeNull()
    expect(mocks.toast).not.toHaveBeenCalled()
  })

  it('treats a success:false envelope as an error, not as an empty list', async () => {
    mocks.summary.mockResolvedValue({ success: false, message: 'span too long' })
    renderCard()
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.queryByText('No errors in this period')).toBeNull()
  })
})
