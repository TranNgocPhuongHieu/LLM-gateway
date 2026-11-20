import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { AdminUsageTrackingCard } from './admin-usage-tracking-card'

const mocks = vi.hoisted(() => ({
  flow: vi.fn(),
}))

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
vi.mock('@/features/dashboard/api', () => ({ getFlowQuotaDates: mocks.flow }))
vi.mock('@/stores/auth-store', () => {
  const state = () => ({ auth: { user: { id: 1, role: 100 } } })
  return {
    useAuthStore: Object.assign(
      (selector: (value: ReturnType<typeof state>) => unknown) =>
        selector(state()),
      { getState: state }
    ),
  }
})

const range = { start: new Date(2026, 10, 1), end: new Date(2026, 10, 20, 12) }

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AdminUsageTrackingCard range={range} excludeInternal />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  // 500,000 quota = 1 credit in USD display mode.
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, quotaDisplayType: 'USD' },
  })
  mocks.flow.mockResolvedValue({
    success: true,
    data: [
      { user_id: 7, username: 'alice', token_id: 3, token_name: 'prod', model_name: 'gpt-4o', count: 40, quota: 37_458_114 },
      { user_id: 8, username: 'bob', token_id: 4, token_name: 'dev', model_name: 'gpt-4o-mini', count: 5, quota: 250_000 },
    ],
  })
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('AdminUsageTrackingCard', () => {
  it('shows credits, never raw quota units, in totals, bars and the table', async () => {
    renderCard()

    // 37,458,114 + 250,000 = 37,708,114 quota = 75.42 credits
    expect(await screen.findByText('75.42 credit')).toBeTruthy()
    expect(screen.getAllByText('74.92 credit').length).toBeGreaterThan(0)
    expect(screen.getAllByText('0.5 credit').length).toBeGreaterThan(0)
    expect(screen.queryByText(/37,458,114/)).toBeNull()
    expect(screen.queryByText(/37,708,114/)).toBeNull()
  })

  it('follows the page range and hides internal accounts', async () => {
    renderCard()

    await screen.findByText('75.42 credit')
    expect(mocks.flow).toHaveBeenCalledWith(
      {
        start_timestamp: Math.floor(range.start.getTime() / 1000),
        end_timestamp: Math.floor(range.end.getTime() / 1000),
        exclude_internal: true,
      },
      true
    )
    expect(screen.queryByText(/days/)).toBeNull()
  })

  it('totals every row, not only the rows shown in the top list', async () => {
    mocks.flow.mockResolvedValue({
      success: true,
      data: Array.from({ length: 12 }, (_, index) => ({
        user_id: index + 1,
        username: `user-${index + 1}`,
        token_id: index + 1,
        model_name: 'gpt-4o',
        count: 1,
        quota: 500_000,
      })),
    })
    renderCard()

    // 12 rows x 1 credit, although only the top 10 users are drawn.
    expect(await screen.findByText('12 credit')).toBeTruthy()
    expect(screen.getByText('12')).toBeTruthy()
    expect(screen.queryByText('user-11')).toBeNull()
  })
})
