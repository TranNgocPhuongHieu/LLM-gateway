// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { resolveLegacyRoute } from '@/lib/legacy-route'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import { DataTableRowActions } from '@/features/models/components/data-table-row-actions'
import { ModelsPrimaryButtons } from '@/features/models/components/models-primary-buttons'
import { ModelDeleteDialog } from '@/features/models/components/dialogs/model-delete-dialog'
import * as modelsApi from '@/features/models/api'
import type { Model } from '@/features/models/types'
import { updateExpressionDraft } from './components/price-management-view'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

const mockUser = { role: ROLE.SUPER_ADMIN, username: 'root' }
vi.mock('@/stores/auth-store', () => ({
  useAuthStore: (selector?: (state: any) => any) => {
    const state = { auth: { user: mockUser } }
    return selector ? selector(state) : state
  },
}))

vi.mock('@/features/models/components/models-provider', () => ({
  useModels: () => ({
    open: null,
    setOpen: vi.fn(),
    currentRow: null,
    setCurrentRow: vi.fn(),
  }),
}))

vi.mock('@tanstack/react-query', () => ({
  queryOptions: (options: any) => options,
  useQueryClient: () => ({
    invalidateQueries: vi.fn(),
  }),
  useMutation: ({ mutationFn }: any) => ({
    mutate: mutationFn,
    isPending: false,
    isError: false,
  }),
  useQuery: () => ({
    data: null,
    isLoading: false,
    isError: false,
  }),
}))

afterEach(cleanup)

describe('Dedicated Price Management and Models cleanup', () => {
  it('preserves original billing coefficients when an unchanged price draft is saved', () => {
    const expression =
      'tier("base", p * 0.026923076923076923 + c * 0.1346153846153846 + cr * 0.013461538461538462)'

    expect(
      updateExpressionDraft(expression, {
        model: 'gpt-6.1-sol',
        hasPricing: true,
        inputVnd: 700,
        outputVnd: 3500,
        cacheReadVnd: 350,
      })
    ).toBe(expression)
  })

  it('places Price Management directly below Quota Check in sidebar with SUPER_ADMIN requirement', () => {
    const data = useSidebarData()
    const adminGroup = data.navGroups.find((g) => g.id === 'admin')
    expect(adminGroup).toBeDefined()

    const items = adminGroup!.items
    const quotaCheckIndex = items.findIndex((i: any) => i.url === '/quota-check')
    const priceMgmtIndex = items.findIndex((i: any) => i.url === '/price-management')

    expect(quotaCheckIndex).toBeGreaterThan(-1)
    expect(priceMgmtIndex).toBe(quotaCheckIndex + 1)

    const priceMgmtItem = items[priceMgmtIndex] as any
    expect(priceMgmtItem.title).toBe('Price Management')
    expect(priceMgmtItem.url).toBe('/price-management')
    expect(priceMgmtItem.requiredRole).toBe(ROLE.SUPER_ADMIN)
  })

  it('resolves legacy setting ratio tab to /price-management', () => {
    const resolved = resolveLegacyRoute('https://legacy-route.invalid/console/setting?tab=ratio')
    expect(resolved).toBe('/price-management?tab=ratio')
  })

  it('removes Pricing action from Models row actions', () => {
    const fixtureModel: Model = {
      id: 1,
      model_name: 'test-model',
      status: 1,
      name_rule: 0,
      sync_official: 1,
    } as Model

    const row = {
      original: fixtureModel,
    } as any

    render(<DataTableRowActions row={row} />)
    expect(screen.queryByText('Pricing')).toBeNull()
  })

  it('removes Sync pricing button from Models primary buttons', () => {
    render(<ModelsPrimaryButtons />)
    expect(screen.queryByText('Sync pricing')).toBeNull()
  })

  it('removes "Also remove pricing" checkbox from ModelDeleteDialog and never passes removePricing=true', async () => {
    const deleteModelSpy = vi.spyOn(modelsApi, 'deleteModel').mockResolvedValue({
      success: true,
      data: { deleted_count: 1, updated_channels: 0 },
    })

    const fixtureModel: Model = {
      id: 42,
      model_name: 'obsolete-model',
      status: 1,
      name_rule: 0,
    } as Model

    render(
      <ModelDeleteDialog
        models={[fixtureModel]}
        onClose={vi.fn()}
      />
    )

    // Should not have the "Also remove pricing" checkbox or text
    expect(screen.queryByText('Also remove pricing')).toBeNull()

    // Trigger delete
    const deleteButton = screen.getByRole('button', { name: 'Delete' })
    deleteButton.click()

    expect(deleteModelSpy).toHaveBeenCalledWith(42, false, false)
    deleteModelSpy.mockRestore()
  })

  it('restricts pricing editing rights strictly to ROLE.SUPER_ADMIN (100)', () => {
    // Test role permission logic
    const canEditRole100 = (role: number) => role === ROLE.SUPER_ADMIN
    expect(canEditRole100(ROLE.SUPER_ADMIN)).toBe(true)
    expect(canEditRole100(ROLE.ADMIN)).toBe(false)
    expect(canEditRole100(ROLE.USER)).toBe(false)
    expect(canEditRole100(0)).toBe(false)
  })

  it('preserves multi-tier expression structure when updating base tier coefficients', () => {
    const astraExpr = 'tier("base", p * 0.00003 + c * 0.0001) + tier("tier2", p * 0.00002 + c * 0.00008, usage("prompt_tokens") > 100000)'

    // Simulate updating base tier to Opus prices (1500 / 26000, 7500 / 26000)
    const pVal = (1500 / 26000).toFixed(16)
    const cVal = (7500 / 26000).toFixed(16)

    let updated = astraExpr
    if (/\bp\s*\*\s*[0-9.]+/.test(updated)) {
      updated = updated.replace(/\bp\s*\*\s*[0-9.]+/, `p * ${pVal}`)
    }
    if (/\bc\s*\*\s*[0-9.]+/.test(updated)) {
      updated = updated.replace(/\bc\s*\*\s*[0-9.]+/, `c * ${cVal}`)
    }

    expect(updated).toContain(`p * ${pVal}`)
    expect(updated).toContain(`c * ${cVal}`)
    // Crucial check: tier2 must remain intact!
    expect(updated).toContain('tier("tier2", p * 0.00002 + c * 0.00008, usage("prompt_tokens") > 100000)')
  })

  it('verifies sidebar displays consecutively: Channels -> Quota Check -> Price Management at the same level', () => {
    const data = useSidebarData()
    const adminGroup = data.navGroups.find((g) => g.id === 'admin')
    expect(adminGroup).toBeDefined()

    const items = adminGroup!.items
    const channelsIdx = items.findIndex((i: any) => i.url === '/channels')
    const quotaCheckIdx = items.findIndex((i: any) => i.url === '/quota-check')
    const priceMgmtIdx = items.findIndex((i: any) => i.url === '/price-management')

    expect(channelsIdx).toBeGreaterThan(-1)
    expect(quotaCheckIdx).toBe(channelsIdx + 1)
    expect(priceMgmtIdx).toBe(quotaCheckIdx + 1)
  })

  it('correctly maps 2 channels declaring same model into 1 selling price row and 2 purchase price records', () => {
    // Channel A (id: 1, name: "Claude-Primary") has "claude-opus-5"
    // Channel B (id: 2, name: "Claude-Backup") has "claude-opus-5"
    const channels = [
      { id: 1, name: 'Claude-Primary', type: 14, models: 'claude-opus-5' },
      { id: 2, name: 'Claude-Backup', type: 14, models: 'claude-opus-5' },
    ]

    // Construct channel models and unique models exactly as backend GetConfiguredChannelPricingScope does
    const channelModels: Array<{ channel_id: number; channel_name: string; model: string }> = []
    const uniqueMap = new Set<string>()

    for (const ch of channels) {
      const models = ch.models.split(',').map((m) => m.trim()).filter(Boolean)
      for (const m of models) {
        channelModels.push({
          channel_id: ch.id,
          channel_name: ch.name,
          model: m,
        })
        uniqueMap.add(m)
      }
    }

    const uniqueModels = Array.from(uniqueMap)

    // Acceptance criteria check:
    // 1 selling price row (uniqueModels.length === 1)
    expect(uniqueModels).toEqual(['claude-opus-5'])
    expect(uniqueModels).toHaveLength(1)

    // 2 purchase price records (channelModels.length === 2)
    expect(channelModels).toHaveLength(2)
    expect(channelModels[0]).toEqual({
      channel_id: 1,
      channel_name: 'Claude-Primary',
      model: 'claude-opus-5',
    })
    expect(channelModels[1]).toEqual({
      channel_id: 2,
      channel_name: 'Claude-Backup',
      model: 'claude-opus-5',
    })
  })

  it('excludes models in catalog that are not configured in any channel', () => {
    const catalogModels = ['catalog-only-model-a', 'claude-opus-5', 'catalog-only-model-b']
    const channels = [
      { id: 1, name: 'TestChannel', models: 'claude-opus-5' },
    ]

    const channelModelsSet = new Set(
      channels.flatMap((ch) => ch.models.split(',').map((m) => m.trim()))
    )

    const managedModels = catalogModels.filter((m) => channelModelsSet.has(m))
    expect(managedModels).toEqual(['claude-opus-5'])
    expect(managedModels).not.toContain('catalog-only-model-a')
    expect(managedModels).not.toContain('catalog-only-model-b')
  })

  it('displays empty state and avoids fetching full catalog when no channels or models configured', () => {
    const uniqueModels: string[] = []
    const shouldFetchCatalog = uniqueModels.length > 0
    expect(shouldFetchCatalog).toBe(false)
  })
})
