// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { XPikiRequestLogsPage } from './index'
import { checkXPikiQuota, fetchXPikiLogs, updateChannelStatus } from './api'

vi.mock('./api', () => ({ checkXPikiQuota: vi.fn(), fetchXPikiLogs: vi.fn(), updateChannelStatus: vi.fn() }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('@/lib/handle-server-error', () => ({ handleServerError: vi.fn() }))
vi.mock('@/components/layout/components/section-page-layout', () => ({
  SectionPageLayout: Object.assign(({ children }: { children: React.ReactNode }) => <div>{children}</div>, {
    Title: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    Actions: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    Content: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  }),
}))

const logs = {
  items: [], total: 0, has_more: false,
  key_options: [
    { value: 'channel:10', label: 'Kênh: 9router', key_masked: '' },
    { value: 'token_id:7', label: 'Khóa #7: test', key_masked: '' },
  ],
  model_options: ['gpt-6.1-sol'],
  user_options: ['alice', 'bob'],
}
const report = {
  keys: [], total_remaining: '', total_unlimited: false, total_incomplete: false, no_model_price_keys: [],
  model_profiles: [],
}
const clients: QueryClient[] = []

function mountLogs() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  render(<QueryClientProvider client={client}><XPikiRequestLogsPage /></QueryClientProvider>)
}

beforeEach(() => {
  cleanup()
  vi.clearAllMocks()
  vi.mocked(fetchXPikiLogs).mockReset()
  vi.mocked(checkXPikiQuota).mockReset()
  vi.mocked(updateChannelStatus).mockReset()
  localStorage.clear()
  localStorage.setItem('newapi_xpiki_logs_preferences_v1', JSON.stringify({ isExpanded: true, pageSize: 20 }))
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }) })
  Element.prototype.scrollIntoView = vi.fn()
  vi.mocked(fetchXPikiLogs).mockResolvedValue(logs)
  vi.mocked(checkXPikiQuota).mockResolvedValue(report)
})
afterEach(() => {
  cleanup()
  clients.splice(0).forEach(client => client.clear())
  vi.unstubAllGlobals()
})

async function lastLogCall() {
  await waitFor(() => expect(fetchXPikiLogs).toHaveBeenCalled())
  return vi.mocked(fetchXPikiLogs).mock.lastCall?.[0]
}

describe('Request Logs filters', () => {
  it('changes page size and channel, keeping earlier filters during pending requests', async () => {
    mountLogs()
    await waitFor(() => expect(screen.getByRole('combobox', { name: 'Chọn model' })).toBeDefined())
    await userEvent.click(screen.getByRole('combobox', { name: 'Số dòng/trang' }))
    await userEvent.click(await screen.findByRole('option', { name: '50 dòng/trang' }))
    await waitFor(() => expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].page_size).toBe(50))

    await userEvent.click(screen.getByRole('combobox', { name: 'Lọc khóa API' }))
    expect(screen.queryByRole('option', { name: 'Kênh: 9router' })).toBeNull()
    await userEvent.click(await screen.findByRole('option', { name: 'Khóa #7: test' }))
    await waitFor(() => expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].key_filter).toBe('token_id:7'))

    let resolvePending!: (value: typeof logs) => void
    vi.mocked(fetchXPikiLogs).mockImplementationOnce(() => new Promise(resolve => { resolvePending = resolve }))
    expect(screen.queryByRole('button', { name: 'Yesterday' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Today' })).toBeDefined()
    await userEvent.click(screen.getByRole('combobox', { name: 'Lọc kênh' }))
    await userEvent.click(await screen.findByRole('option', { name: 'Kênh: 9router' }))
    await waitFor(() => expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].channel_filter).toBe('channel:10'))
    expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].key_filter).toBe('token_id:7')
    // Model values stay selectable while the previous query is pending.
    await userEvent.click(screen.getByRole('combobox', { name: 'Chọn model' }))
    await userEvent.click(await screen.findByRole('option', { name: 'gpt-6.1-sol' }))
    await waitFor(() => expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].model).toBe('gpt-6.1-sol'))
    expect(vi.mocked(fetchXPikiLogs).mock.calls.some(([, signal]) => signal?.aborted)).toBe(true)
    resolvePending(logs)
  })

  it('offers fallback successful inside the status filter', async () => {
    mountLogs()
    await lastLogCall()

    await userEvent.click(screen.getByRole('combobox', { name: 'Status filter' }))
    await userEvent.click(await screen.findByRole('option', { name: 'Fallback successful' }))

    await waitFor(() =>
      expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].status).toBe('retry_recovered')
    )
  })

  it('sends every selected user as an IN list', async () => {
    mountLogs()
    const userSelect = await waitFor(() =>
      screen.getByRole('combobox', { name: 'Chọn người dùng' })
    )
    await userEvent.click(userSelect)
    await userEvent.click(await screen.findByRole('option', { name: 'alice' }))
    await waitFor(() => expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].user).toBe('alice'))

    await userEvent.click(screen.getByRole('combobox', { name: 'Chọn người dùng' }))
    await userEvent.click(await screen.findByRole('option', { name: 'bob' }))
    await waitFor(() => expect(vi.mocked(fetchXPikiLogs).mock.lastCall?.[0].user).toBe('alice,bob'))
  })
})
