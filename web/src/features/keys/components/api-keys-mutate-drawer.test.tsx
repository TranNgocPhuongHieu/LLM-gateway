import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { ApiKeysMutateDrawer } from './api-keys-mutate-drawer'

const mocks = vi.hoisted(() => ({
  createApiKey: vi.fn(),
  updateApiKey: vi.fn(),
  getApiKey: vi.fn(),
  getTokenAutoGroups: vi.fn(),
  getUserModels: vi.fn(),
  getUserGroups: vi.fn(),
  triggerRefresh: vi.fn(),
}))
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
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { default_use_auto_group: false }, loading: false }),
}))
vi.mock('@/lib/api', () => ({
  getUserModels: mocks.getUserModels,
  getUserGroups: mocks.getUserGroups,
}))
vi.mock('../api', () => ({
  createApiKey: mocks.createApiKey,
  updateApiKey: mocks.updateApiKey,
  getApiKey: mocks.getApiKey,
  getTokenAutoGroups: mocks.getTokenAutoGroups,
}))
vi.mock('./api-keys-provider', () => ({
  useApiKeys: () => ({ triggerRefresh: mocks.triggerRefresh }),
}))

// jsdom has no matchMedia; the Select component reads it when it opens.
beforeAll(() => {
  if (!window.matchMedia) {
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: (query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }),
    })
  }
})

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

function renderDrawer() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  mocks.getUserModels.mockResolvedValue({ success: true, data: ['gpt-4o'] })
  mocks.getUserGroups.mockResolvedValue({
    success: true,
    data: { default: { desc: 'Default', ratio: 1 } },
  })
  mocks.getTokenAutoGroups.mockResolvedValue({
    success: true,
    data: { groups: [], max_count: 5, can_create_token_packages: false },
  })
  mocks.createApiKey.mockResolvedValue({ success: true, data: {} })
  return render(
    <QueryClientProvider client={client}>
      <ApiKeysMutateDrawer open onOpenChange={() => {}} />
    </QueryClientProvider>,
  )
}

describe('ApiKeysMutateDrawer', () => {
  it('keeps the Advanced group and quota-mode controls collapsed by default', async () => {
    renderDrawer()
    await screen.findByLabelText('Name')
    expect(screen.queryByText('Quota mode')).toBeNull()
    expect(screen.queryByText('Cross-group retry')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /Advanced Settings/ }))
    expect(await screen.findByText('Quota mode')).toBeTruthy()
    expect(screen.getByText('Group')).toBeTruthy()
  })

  it('submits the same create payload as before the layout change', async () => {
    renderDrawer()
    const name = await screen.findByLabelText('Name')
    const save = screen.getByRole('button', { name: 'Save changes' })
    await waitFor(() => expect((save as HTMLButtonElement).disabled).toBe(false))

    fireEvent.change(name, { target: { value: 'my-key' } })
    fireEvent.click(save)

    await waitFor(() => expect(mocks.createApiKey).toHaveBeenCalledTimes(1))
    expect(mocks.createApiKey.mock.calls[0][0]).toEqual({
      name: 'my-key',
      quota_mode: 'billing',
      remain_quota: 0,
      expired_time: -1,
      unlimited_quota: true,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
      group: '',
      auto_groups: [],
      cross_group_retry: false,
    })
  })
})
