// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { XPikiQuotaPage, XPikiRequestLogsPage } from './index'
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
  key_options: [{ value: 'channel:10', label: 'Kênh: 9router', key_masked: '' }, { value: 'token_id:7', label: 'Khóa #7: test', key_masked: '' }],
  model_options: ['gpt-6.1-sol'],
  user_options: ['alice', 'bob'],
}
const report = {
  keys: [], total_remaining: '', total_unlimited: false, total_incomplete: false, no_model_price_keys: [],
  model_profiles: Array.from({ length: 5 }, (_, index) => ({
    labels: [`Key ${index + 1}`], group_ratio: '1', astra_price_vnd_per_1m: '1040',
    models: [], price_basis: '', cache_price_basis: '',
  })),
}
const clients: QueryClient[] = []
function renderWithQuery(component: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  clients.push(client)
  render(<QueryClientProvider client={client}>{component}</QueryClientProvider>)
}
// Request logs moved to their own /request-logs page.
function mountLogs() {
  renderWithQuery(<XPikiRequestLogsPage />)
}
// Quota check page keeps the balance and price-audit cards.
function mountQuota() {
  renderWithQuery(<XPikiQuotaPage />)
}
beforeEach(() => {
  // Portals from the previous render would otherwise stay in document.body
  // and make dropdown options from another test match first.
  cleanup()
  vi.clearAllMocks()
  // Drop one-shot fixtures left over from the previous test; vitest keeps
  // mockResolvedValueOnce queues across tests without reset.
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
  // Close any open Base UI popup so its dismiss handler finishes before the
  // next test installs a fresh document body.
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
  cleanup()
  clients.splice(0).forEach(client => client.clear())
  vi.unstubAllGlobals()
})

describe('XPiki dashboard interactions', () => {
  it('retains the final channel and ordered fallback trail for recovered requests', async () => {
    vi.mocked(fetchXPikiLogs).mockResolvedValueOnce({ ...logs, items: [{
      log_id: 1, created_at: '2026-10-20T08:00:00Z', type: 'chat/completions',
      model_slug: 'gpt-6.1-sol', status: 'success', request_outcome: 'retry_recovered',
      input_tokens: 100, output_tokens: 20, cache_read_tokens: 0, cache_write_tokens: 0,
      credit_cost: 0, cost_vnd: '0', duration_ms: 1000, request_id: 'fixture-request',
      ip: '', key_label: 'fixture-key', channel_id: 2, channel_name: 'fixture-fallback',
      channel_trail: ['fixture-primary', 'fixture-fallback'], retried: true,
      total_attempts: 2, failed_attempts: 1,
    }] })
    mountLogs()
    expect(await screen.findByText('Fallback: fixture-primary → fixture-fallback')).toBeTruthy()
    expect(screen.getByText('Success')).toBeTruthy()
    expect(screen.getByText('Fallback successful')).toBeTruthy()
    expect(screen.queryByText('Request completed')).toBeNull()
    expect(screen.queryByText('Retry recovered')).toBeNull()
    expect(screen.getByText(/#2.*fixture-fallback/)).toBeTruthy()
  })
  it('shows the matched fallback channel when only the XPiki attribution fields are available', async () => {
    vi.mocked(fetchXPikiLogs).mockResolvedValueOnce({ ...logs, items: [{
      log_id: 2, created_at: '2026-10-20T08:00:00Z', type: 'chat/completions',
      model_slug: 'gpt-6.1-sol', status: 'success', request_outcome: 'success',
      input_tokens: 100, output_tokens: 20, cache_read_tokens: 0, cache_write_tokens: 0,
      credit_cost: 0, cost_vnd: '0', duration_ms: 1000, request_id: 'fallback-request',
      ip: '', key_label: 'fixture-key', channel_id: 1, channel_name: 'fixture-primary',
      fallback_channel_id: 2, fallback_channel_name: 'fixture-fallback',
    }] })
    mountLogs()
    expect(await screen.findByText('Fallback: fixture-primary → fixture-fallback')).toBeTruthy()
    expect(screen.getByText('Success')).toBeTruthy()
    expect(screen.queryByText('Request completed')).toBeNull()
  })
  it('shows only the failed status instead of a redundant request-outcome badge', async () => {
    vi.mocked(fetchXPikiLogs).mockResolvedValueOnce({ ...logs, items: [{
      log_id: 3, created_at: '2026-10-20T08:00:00Z', type: 'chat/completions',
      model_slug: 'gpt-6.1-sol', status: 'failed', request_outcome: 'failed_final',
      input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0,
      credit_cost: 0, cost_vnd: '0', duration_ms: 1000, request_id: 'failed-request',
      ip: '', key_label: 'fixture-key', error_code: 'upstream failed',
    }] })
    mountLogs()
    expect(await screen.findByText('Failed')).toBeTruthy()
    expect(screen.queryByText('Final request failure')).toBeNull()
  })
  it('shows an unknown user safely when the log has no username', async () => {
    vi.mocked(fetchXPikiLogs).mockResolvedValueOnce({
      ...logs,
      items: [{
        log_id: 9, created_at: '2026-10-20T08:00:00Z', type: 'chat/completions',
        model_slug: 'gpt-6.1-sol', status: 'success', request_outcome: 'success',
        input_tokens: 1, output_tokens: 1, cache_read_tokens: 0, cache_write_tokens: 0,
        credit_cost: 0, cost_vnd: '0', duration_ms: 10, request_id: 'no-user',
        ip: '', key_label: 'fixture-key',
      }],
    })
    mountLogs()
    expect(await screen.findByText('Không xác định')).toBeTruthy()
  })

  it('shows failed fallback separately from a recovered request', async () => {
    vi.mocked(fetchXPikiLogs).mockResolvedValueOnce({ ...logs, items: [{
      log_id: 11, created_at: '2026-10-20T08:00:00Z', type: 'chat/completions',
      model_slug: 'gpt-6.1-sol', status: 'failed', request_outcome: 'failed_final',
      input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0,
      credit_cost: 0, cost_vnd: '0', duration_ms: 10, request_id: 'failed-fallback',
      ip: '', key_label: 'fixture-key', retried: true,
      channel_trail: ['primary ✗', 'secondary ✗'], total_attempts: 2, failed_attempts: 2,
    }] })
    mountLogs()
    expect(await screen.findByText('Fallback exhausted')).toBeTruthy()
    expect(screen.getByText('Failed')).toBeTruthy()
    expect(screen.getByText('Fallback: primary ✗ → secondary ✗')).toBeTruthy()
    expect(screen.queryByText('Fallback successful')).toBeNull()
  })

  it('renders one collapsed price card for five keys and audits only when opened', async () => {
    localStorage.setItem('newapi_xpiki_quota_cache_actual_tariffs_v6', JSON.stringify({ report, timestamp: Date.now() }))
    mountQuota()
    expect(screen.getAllByText('Giá theo token đã đối soát')).toHaveLength(1)
    expect(checkXPikiQuota).not.toHaveBeenCalled()
    await userEvent.click(screen.getAllByRole('button', { name: 'Show' })[0])
    await waitFor(() => expect(checkXPikiQuota).toHaveBeenCalledWith([], true))
    await userEvent.click(screen.getByRole('combobox', { name: 'Key đối soát giá' }))
    await userEvent.click(await screen.findByRole('option', { name: 'Key 5' }))
    expect(screen.getByRole('combobox', { name: 'Key đối soát giá' }).textContent).toContain('Key 5')
    // Close the price-audit popup; a lingering Base UI dismiss handler would
    // swallow the first interaction of the next test.
    await userEvent.keyboard('{Escape}')
    await waitFor(() =>
      expect(screen.queryByRole('combobox', { name: 'Key đối soát giá' })?.getAttribute('aria-expanded')).not.toBe('true')
    )
  })
})
