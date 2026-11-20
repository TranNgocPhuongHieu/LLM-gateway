// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { useSelfErrorCount } from './use-overview-usage'

const mocks = vi.hoisted(() => ({ getUserLogs: vi.fn() }))
vi.mock('@/features/usage-logs/api', () => ({ getUserLogs: mocks.getUserLogs }))
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector: (value: unknown) => unknown) =>
    selector({ auth: { user: { id: 1 }, session: { sid: 's' } } }),
}))

function wrapper(props: { children: React.ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{props.children}</QueryClientProvider>
}

describe('useSelfErrorCount', () => {
  it('reads the exact error total from one log page, beyond the 5,000-row summary cap', async () => {
    mocks.getUserLogs.mockResolvedValue({ success: true, data: { items: [], total: 12_345 } })
    const range = { start_timestamp: 1000, end_timestamp: 5000 }
    const { result } = renderHook(() => useSelfErrorCount(range), { wrapper })

    await waitFor(() => expect(result.current.data).toBe(12_345))
    expect(mocks.getUserLogs).toHaveBeenCalledWith({
      start_timestamp: 1000,
      end_timestamp: 5000,
      type: 5,
      p: 1,
      page_size: 1,
    })
  })

  it('fails instead of reporting zero when the total is missing', async () => {
    mocks.getUserLogs.mockResolvedValue({ success: true, data: { items: [] } })
    const { result } = renderHook(
      () => useSelfErrorCount({ start_timestamp: 1, end_timestamp: 2 }),
      { wrapper },
    )
    await waitFor(() => expect(result.current.isError).toBe(true))
  })
})
