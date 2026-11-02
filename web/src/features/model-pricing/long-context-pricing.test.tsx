// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { PriceManagementView } from './components/price-management-view'
import { saveModelPricing } from './api'
import { readLongContextPricing } from './bulk-pricing'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('./model-pricing-panel', () => ({ ModelPricingPanel: () => null }))
vi.mock('./components/price-sync-dialog', () => ({ PriceSyncDialog: () => null }))
vi.mock('@/features/system-settings/components/form-navigation-guard', () => ({ FormNavigationGuard: () => null }))
vi.mock('@/lib/api', () => ({ api: { get: vi.fn().mockResolvedValue({ data: { success: true, data: { items: [], version: 'import-version' } } }) } }))
vi.mock('./api', () => {
  const scope = { unique_models: ['fixture-model'], channel_models: [] }
  const pricing = { entries: [{ model_name: 'fixture-model', version: 'version-1', configured: { 'billing_setting.billing_expr': 'tier("base", p * 1 + c * 2 + cr * 0.5 + cc * 0)' } }], empty_version: '' }
  return {
    useChannelPricingScope: () => ({ data: scope, isLoading: false, isError: false, refetch: vi.fn() }),
    useModelPricing: () => ({ data: pricing, isLoading: false, isError: false, refetch: vi.fn() }),
    saveModelPricing: vi.fn().mockResolvedValue({}),
  }
})
vi.mock('@/components/layout/components/section-page-layout', () => ({
  SectionPageLayout: Object.assign(({ children }: { children: React.ReactNode }) => <div>{children}</div>, {
    Title: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    Actions: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    Content: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  }),
}))

let client: QueryClient
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }) })
  Element.prototype.scrollIntoView = vi.fn()
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
})
afterEach(() => { cleanup(); client.clear(); vi.unstubAllGlobals() })

it('previews an adjustable context rule and only persists on Save selling prices', async () => {
  const user = userEvent.setup()
  render(<QueryClientProvider client={client}><PriceManagementView /></QueryClientProvider>)
  await user.click(await screen.findByRole('button', { name: 'Long context pricing' }))
  expect(saveModelPricing).not.toHaveBeenCalled()
  await user.click(screen.getByRole('switch', { name: 'Enable long context pricing' }))
  const threshold = screen.getByRole('textbox', { name: 'Input context threshold (tokens)' })
  await user.clear(threshold)
  await user.type(threshold, '300000')
  await user.click(screen.getByRole('button', { name: 'Apply' }))
  expect(saveModelPricing).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Save selling prices' }))
  await waitFor(() => expect(saveModelPricing).toHaveBeenCalledTimes(1))
  const changes = vi.mocked(saveModelPricing).mock.calls[0][0]
  expect(changes).toHaveLength(1)
  expect(changes[0].model_name).toBe('fixture-model')
  expect(changes[0].expected_version).toBe('version-1')
  const rule = readLongContextPricing(String(changes[0].pricing['billing_setting.billing_expr']))
  expect(rule?.policy).toEqual({ threshold: 300000, input: 2, output: 2, cacheRead: 2, cacheWrite: 2 })
})
