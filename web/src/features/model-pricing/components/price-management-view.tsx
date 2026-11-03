/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  Banknote,
  Coins,
  Radio,
  RefreshCw,
  Save,
  Sliders,
  Tag,
} from 'lucide-react'
import { Fragment, useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog as AppDialog } from '@/components/dialog'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout/components/section-page-layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  useChannelPricingScope,
  useModelPricing,
  saveModelPricing,
  type ModelPricingChange,
} from '@/features/model-pricing/api'
import { ModelPricingPanel } from '@/features/model-pricing/model-pricing-panel'
import { FormNavigationGuard } from '@/features/system-settings/components/form-navigation-guard'
import { api } from '@/lib/api'
import { resolveAppLocale } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'

import { PriceSyncDialog } from './price-sync-dialog'
import { calculateBulkPrices, modelFamily, priceFields, type BulkPriceMode, type PriceField } from '../bulk-pricing'
import { applyLongContextPricing, canConfigureLongContext, defaultLongContextPricing, readLongContextPricing, validateLongContextPricing, type LongContextPricing } from '../bulk-pricing'
import { Switch } from '@/components/ui/switch'

const VND_EXCHANGE_RATE = 26000.0

export interface DeclaredImportPriceItem {
  id: string
  channel_id?: number
  provider: string
  model: string
  inputVnd: number
  cacheReadVnd: number
  outputVnd: number
  note?: string
}

export interface ModelSellingPriceDraft {
  model: string
  hasPricing: boolean
  inputVnd: number
  cacheReadVnd: number
  outputVnd: number
  cacheWriteVnd?: number
  isOpusOrSol?: boolean
  isMultiTier?: boolean
  rawExpr?: string
  longContext?: LongContextPricing | null
}

export interface ChannelImportDraft {
  channel_id: number
  channel_name: string
  channel_type: number
  model: string
  hasPricing: boolean
  inputVnd: number
  cacheReadVnd: number
  outputVnd: number
  note?: string
}

const OPUS_MODELS = [
  'claude-opus-4-6',
  'claude-opus-4-7',
  'claude-opus-4-8',
  'claude-opus-5',
  'claude-opus-5-5',
]

const SOL_MODELS = ['gpt-6.1-sol']

function PriceVndInput(props: {
  valueVnd: number | undefined
  hasPricing: boolean
  placeholder?: string
  onChangeVnd: (vnd: number) => void
  className?: string
  'aria-label'?: string
}) {
  const [draft, setDraft] = useState<{
    text: string
    lastVnd: number | undefined
  } | null>(null)
  const decimalSeparator = resolveAppLocale().startsWith('vi') ? ',' : '.'

  const displayString =
    props.hasPricing && props.valueVnd !== undefined
      ? String(Math.round((props.valueVnd / 1000) * 1000000) / 1000000).replace(
          '.',
          decimalSeparator
        )
      : ''

  const isEditing = draft !== null && draft.lastVnd === props.valueVnd
  const text = isEditing ? draft.text : displayString

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const nextText = e.target.value
    const normalized = nextText.trim().replace(',', '.')
    if (normalized === '') {
      setDraft({ text: nextText, lastVnd: 0 })
      props.onChangeVnd(0)
      return
    }
    const parsed = Number(normalized)
    if (Number.isFinite(parsed) && parsed >= 0) {
      const nextVnd = Math.round(parsed * 1000000) / 1000
      setDraft({ text: nextText, lastVnd: nextVnd })
      props.onChangeVnd(nextVnd)
      return
    }
    setDraft({ text: nextText, lastVnd: props.valueVnd })
  }

  const handleBlur = () => {
    setDraft(null)
  }

  return (
    <Input
      type='text'
      inputMode='decimal'
      value={text}
      placeholder={props.placeholder}
      onChange={handleChange}
      onBlur={handleBlur}
      aria-label={props['aria-label']}
      className={props.className}
    />
  )
}

function parseExpressionToDraft(model: string, expr: string): ModelSellingPriceDraft {
  const longContext = readLongContextPricing(expr)
  let inputCoeff = 0
  let cacheReadCoeff = 0
  let outputCoeff = 0
  let cacheWriteCoeff = 0
  let hasPricing = false

  if (expr && expr.trim()) {
    const pMatch = expr.match(/\bp\s*\*\s*([0-9.]+)/)
    if (pMatch) {
      inputCoeff = parseFloat(pMatch[1])
      hasPricing = true
    }

    const cMatch = expr.match(/\bc\s*\*\s*([0-9.]+)/)
    if (cMatch) {
      outputCoeff = parseFloat(cMatch[1])
      hasPricing = true
    }

    const crMatch = expr.match(/\bcr\s*\*\s*([0-9.]+)/)
    if (crMatch) {
      cacheReadCoeff = parseFloat(crMatch[1])
      hasPricing = true
    }

    const ccMatch = expr.match(/\bcc\s*\*\s*([0-9.]+)/)
    if (ccMatch) cacheWriteCoeff = parseFloat(ccMatch[1])
  }

  const isOpus = OPUS_MODELS.includes(model)
  const isSol = SOL_MODELS.includes(model)
  const isMultiTier = !longContext && (expr?.match(/\btier\s*\(/g) || []).length > 1

  return {
    model,
    hasPricing,
    inputVnd: hasPricing ? Math.round(inputCoeff * VND_EXCHANGE_RATE) : 0,
    cacheReadVnd: hasPricing ? Math.round(cacheReadCoeff * VND_EXCHANGE_RATE) : 0,
    outputVnd: hasPricing ? Math.round(outputCoeff * VND_EXCHANGE_RATE) : 0,
    cacheWriteVnd: cacheWriteCoeff > 0 ? Math.round(cacheWriteCoeff * VND_EXCHANGE_RATE) : undefined,
    isOpusOrSol: isOpus || isSol,
    isMultiTier,
    rawExpr: expr,
    longContext: longContext?.policy,
  }
}

export function updateExpressionDraft(origExpr: string, draft: ModelSellingPriceDraft): string {
  const existing = readLongContextPricing(origExpr)
  if (existing) {
    const base = updateExpressionDraft(existing.base, { ...draft, longContext: undefined })
    const policy = draft.longContext === undefined ? existing.policy : draft.longContext
    return policy ? applyLongContextPricing(base, policy) : base
  }
  const expressionValue = (field: string, valueVnd: number) => {
    const original = origExpr.match(
      new RegExp(`\\b${field}\\s*\\*\\s*([0-9.]+)`)
    )
    if (
      original &&
      Math.round(Number(original[1]) * VND_EXCHANGE_RATE) === valueVnd
    ) {
      return original[1]
    }
    return (valueVnd / VND_EXCHANGE_RATE).toFixed(16)
  }

  const pVal = expressionValue('p', draft.inputVnd)
  const cVal = expressionValue('c', draft.outputVnd)
  const crVal = expressionValue('cr', draft.cacheReadVnd)

  if (!origExpr) {
    let expr = `tier("base", p * ${pVal} + c * ${cVal} + cr * ${crVal}`
    if (draft.cacheWriteVnd !== undefined) {
      const ccVal = expressionValue('cc', draft.cacheWriteVnd)
      expr += ` + cc * ${ccVal}`
    } else if (draft.model.includes('opus')) {
      expr += ` + cc * 0.013`
    } else if (draft.model.includes('sol')) {
      expr += ` + cc * 0.008`
    }
    expr += ')'
    return draft.longContext ? applyLongContextPricing(expr, draft.longContext) : expr
  }

  // Preserve other tiers and non-edited components
  let updated = origExpr
  if (/\bp\s*\*\s*[0-9.]+/.test(updated)) {
    updated = updated.replace(/\bp\s*\*\s*[0-9.]+/, `p * ${pVal}`)
  } else {
    updated = updated.replace(/tier\("base",\s*/, `tier("base", p * ${pVal} + `)
  }

  if (/\bc\s*\*\s*[0-9.]+/.test(updated)) {
    updated = updated.replace(/\bc\s*\*\s*[0-9.]+/, `c * ${cVal}`)
  } else {
    updated = updated.replace(/tier\("base",\s*/, `tier("base", c * ${cVal} + `)
  }

  if (/\bcr\s*\*\s*[0-9.]+/.test(updated)) {
    updated = updated.replace(/\bcr\s*\*\s*[0-9.]+/, `cr * ${crVal}`)
  } else if (draft.cacheReadVnd > 0) {
    updated = updated.replace(/tier\("base",\s*/, `tier("base", cr * ${crVal} + `)
  }

  if (draft.cacheWriteVnd !== undefined) {
    const ccVal = expressionValue('cc', draft.cacheWriteVnd)
    if (/\bcc\s*\*\s*[0-9.]+/.test(updated)) {
      updated = updated.replace(/\bcc\s*\*\s*[0-9.]+/, `cc * ${ccVal}`)
    } else if (draft.cacheWriteVnd > 0) {
      updated = updated.replace(/tier\("base",\s*/, `tier("base", cc * ${ccVal} + `)
    }
  }

  return draft.longContext ? applyLongContextPricing(updated, draft.longContext) : updated
}

export function PriceManagementView() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [activeTab, setActiveTab] = useState<'selling' | 'import'>('selling')
  const [modelSearch, setModelSearch] = useState('')
  const [familyFilter, setFamilyFilter] = useState('all')
  const [selectedModels, setSelectedModels] = useState<Set<string>>(new Set())
  const [bulkMode, setBulkMode] = useState<BulkPriceMode>('ratio')
  const [bulkFields, setBulkFields] = useState<Partial<Record<PriceField, string>>>({})
  const [contextTargets, setContextTargets] = useState<string[]>([])
  const [contextEnabled, setContextEnabled] = useState(true)
  const [contextFields, setContextFields] = useState<Record<keyof LongContextPricing, string>>({
    threshold: '272000', input: '2', output: '2', cacheRead: '2', cacheWrite: '2',
  })
  const [contextError, setContextError] = useState(false)

  // Scope: Only models configured in system channels (including disabled channels)
  const {
    data: channelScope,
    isLoading: isScopeLoading,
    isError: isScopeError,
    refetch: refetchScope,
  } = useChannelPricingScope()

  const uniqueModels = useMemo(
    () => channelScope?.unique_models ?? [],
    [channelScope]
  )
  const channelModels = useMemo(
    () => channelScope?.channel_models ?? [],
    [channelScope]
  )

  // Only load model pricing configuration for uniqueModels when they exist; never call without parameters
  const {
    data: pricingConfig,
    isLoading: isPricingLoading,
    isError: isPricingError,
    refetch: refetchModelPricing,
  } = useModelPricing(uniqueModels, uniqueModels.length > 0)

  // Load declared import prices snapshot
  const {
    data: importPricingSnapshot,
    isLoading: isImportLoading,
    isError: isImportError,
    refetch: refetchImportPricing,
  } = useQuery({
    queryKey: ['declared-import-pricing'],
    queryFn: async () => {
      const res = await api.get<{
        success: boolean
        data: { items: DeclaredImportPriceItem[]; version: string }
      }>('/api/option/import_pricing')
      if (!res.data?.success) throw new Error('Failed to load declared import pricing')
      return res.data.data
    },
    staleTime: 30000,
  })

  // Local state for editing selling prices
  const [sellingDrafts, setSellingDrafts] = useState<Record<string, ModelSellingPriceDraft>>({})
  const [modifiedModelNames, setModifiedModelNames] = useState<Set<string>>(new Set())
  const [isSellingDirty, setIsSellingDirty] = useState(false)

  // Local state for editing channel import prices (mapped per channel_id:model)
  const [importDrafts, setImportDrafts] = useState<Record<string, ChannelImportDraft>>({})
  const [modifiedImportKeys, setModifiedImportKeys] = useState<Set<string>>(new Set())
  const [isImportDirty, setIsImportDirty] = useState(false)

  const [syncPricingDialogOpen, setSyncPricingDialogOpen] = useState(false)
  const [advancedModelName, setAdvancedModelName] = useState<string | null>(null)

  // Synchronize selling price options into local drafts strictly for configured channel models
  useEffect(() => {
    if (!channelScope) return
    if (uniqueModels.length === 0) {
      setSellingDrafts({})
      setModifiedModelNames(new Set())
      setIsSellingDirty(false)
      return
    }
    if (!pricingConfig?.entries) return

    const drafts: Record<string, ModelSellingPriceDraft> = {}
    for (const model of uniqueModels) {
      const entry = pricingConfig.entries.find((e) => e.model_name === model)
      const expr = (entry?.configured?.['billing_setting.billing_expr'] ||
        entry?.effective?.['billing_setting.billing_expr'] ||
        '') as string
      drafts[model] = parseExpressionToDraft(model, expr)
    }

    setSellingDrafts(drafts)
    setModifiedModelNames(new Set())
    setIsSellingDirty(false)
  }, [channelScope, uniqueModels, pricingConfig])

  // Synchronize declared import prices into local drafts strictly for configured (channel, model) pairs
  useEffect(() => {
    if (!channelScope || !importPricingSnapshot) return
    if (channelModels.length === 0) {
      setImportDrafts({})
      setModifiedImportKeys(new Set())
      setIsImportDirty(false)
      return
    }

    const drafts: Record<string, ChannelImportDraft> = {}
    for (const ch of channelModels) {
      const key = `${ch.channel_id}:${ch.model}`
      const matched = importPricingSnapshot.items.find(
        (item) =>
          (item.channel_id === ch.channel_id || (!item.channel_id && item.provider === ch.channel_name)) &&
          item.model === ch.model
      )
      if (matched) {
        drafts[key] = {
          channel_id: ch.channel_id,
          channel_name: ch.channel_name,
          channel_type: ch.channel_type,
          model: ch.model,
          hasPricing: true,
          inputVnd: matched.inputVnd,
          cacheReadVnd: matched.cacheReadVnd,
          outputVnd: matched.outputVnd,
          note: matched.note || '',
        }
      } else {
        drafts[key] = {
          channel_id: ch.channel_id,
          channel_name: ch.channel_name,
          channel_type: ch.channel_type,
          model: ch.model,
          hasPricing: false,
          inputVnd: 0,
          cacheReadVnd: 0,
          outputVnd: 0,
          note: '',
        }
      }
    }
    setImportDrafts(drafts)
    setModifiedImportKeys(new Set())
    setIsImportDirty(false)
  }, [channelScope, channelModels, importPricingSnapshot])

  // Mutation to save Selling Prices: ONLY applies dirty models, preserving other models and tiers
  const saveSellingMutation = useMutation({
    mutationFn: async () => {
      if (modifiedModelNames.size === 0) return
      const changes: ModelPricingChange[] = []
      for (const model of modifiedModelNames) {
        const draft = sellingDrafts[model]
        if (!draft) continue
        const entry = pricingConfig?.entries.find((e) => e.model_name === model)
        const expected_version = entry?.version ?? pricingConfig?.empty_version ?? ''
        const origExpr = (entry?.configured?.['billing_setting.billing_expr'] ||
          entry?.effective?.['billing_setting.billing_expr'] ||
          '') as string
        const updatedExpr = updateExpressionDraft(origExpr, draft)
        const pricing: Record<string, any> = {
          ...(entry?.configured ? entry.configured : {}),
          'billing_setting.billing_mode': 'tiered_expr',
          'billing_setting.billing_expr': updatedExpr,
        }
        changes.push({
          model_name: model,
          expected_version,
          pricing,
        })
      }
      await saveModelPricing(changes)
    },
    onSuccess: () => {
      refetchModelPricing()
      queryClient.invalidateQueries({ queryKey: ['pricing'] })
      queryClient.invalidateQueries({ queryKey: ['model-pricing-config'] })
      queryClient.invalidateQueries({ queryKey: ['models'] })
      setModifiedModelNames(new Set())
      setIsSellingDirty(false)
      toast.success(t('Selling prices saved successfully.'))
    },
    onError: (err) =>
      handleServerError(
        err,
        t(
          'Failed to save selling prices. Session may be outdated or conflicted, please reload the page.'
        )
      ),
  })

  // Mutation to save Import Prices: merges dirty channel-model items while preserving unconfigured/historical items
  const saveImportMutation = useMutation({
    mutationFn: async () => {
      if (modifiedImportKeys.size === 0) return
      const existingItems = [...(importPricingSnapshot?.items ?? [])]
      const updatedMap = new Map<string, DeclaredImportPriceItem>()

      for (const item of existingItems) {
        const key = item.channel_id ? `${item.channel_id}:${item.model}` : `${item.provider}:${item.model}`
        updatedMap.set(key, { ...item })
      }

      for (const key of modifiedImportKeys) {
        const draft = importDrafts[key]
        if (!draft) continue
        const existing = updatedMap.get(key)
        updatedMap.set(key, {
          id: existing?.id || `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
          channel_id: draft.channel_id,
          provider: draft.channel_name,
          model: draft.model,
          inputVnd: draft.inputVnd,
          cacheReadVnd: draft.cacheReadVnd,
          outputVnd: draft.outputVnd,
          note: draft.note || '',
        })
      }

      const itemsToSave = Array.from(updatedMap.values())
      const res = await api.put('/api/option/import_pricing', {
        items: itemsToSave,
        expected_version: importPricingSnapshot?.version ?? '',
      })
      if (!res.data?.success) throw new Error(res.data?.message || 'Failed to update import price')
      return res.data
    },
    onSuccess: () => {
      refetchImportPricing()
      queryClient.invalidateQueries({ queryKey: ['declared-import-pricing'] })
      setModifiedImportKeys(new Set())
      setIsImportDirty(false)
      toast.success(t('Purchase prices saved successfully.'))
    },
    onError: (err) =>
      handleServerError(
        err,
        t(
          'Failed to save purchase prices. Session may be outdated or conflicted, please reload the page.'
        )
      ),
  })

  const handleSellingPriceChange = (
    model: string,
    field: 'inputVnd' | 'cacheReadVnd' | 'outputVnd' | 'cacheWriteVnd',
    value: number
  ) => {
    setSellingDrafts((prev) => {
      const current = prev[model]
      if (!current) return prev
      return {
        ...prev,
        [model]: {
          ...current,
          hasPricing: true,
          [field]: Math.max(0, value),
        },
      }
    })
    setModifiedModelNames((prev) => new Set(prev).add(model))
    setIsSellingDirty(true)
  }

  const openContextPricing = (models: string[]) => {
    if (models.some(model => !sellingDrafts[model]?.hasPricing)) {
      toast.error(t('Configure base selling prices first'))
      return
    }
    if (models.some(model => {
      const draft = sellingDrafts[model]
      const expr = updateExpressionDraft(draft.rawExpr || '', draft)
      return !canConfigureLongContext(readLongContextPricing(expr)?.base ?? expr)
    })) {
      toast.error(t('Use the advanced editor for custom expressions'))
      return
    }
    const policy = models.length === 1 ? sellingDrafts[models[0]].longContext : undefined
    setContextEnabled(models.length > 1 || Boolean(policy))
    const values = policy ?? defaultLongContextPricing
    setContextFields(Object.fromEntries(Object.entries(values).map(([key, value]) => [key, String(value)])) as Record<keyof LongContextPricing, string>)
    setContextError(false)
    setContextTargets(models)
  }

  const applyContextPricing = () => {
    const policy = Object.fromEntries(Object.entries(contextFields).map(([key, value]) => [key, Number(value.replace(',', '.'))])) as LongContextPricing
    try {
      if (contextEnabled) validateLongContextPricing(policy)
      for (const model of contextTargets) {
        updateExpressionDraft(sellingDrafts[model].rawExpr || '', { ...sellingDrafts[model], longContext: contextEnabled ? policy : null })
      }
    } catch {
      setContextError(true)
      return
    }
    setSellingDrafts(previous => {
      const next = { ...previous }
      for (const model of contextTargets) next[model] = { ...next[model], longContext: contextEnabled ? policy : null }
      return next
    })
    setModifiedModelNames(previous => new Set([...previous, ...contextTargets]))
    setIsSellingDirty(true)
    setContextTargets([])
  }

  const handleImportPriceChange = (
    key: string,
    field: 'inputVnd' | 'cacheReadVnd' | 'outputVnd' | 'note',
    value: number | string
  ) => {
    setImportDrafts((prev) => {
      const current = prev[key]
      if (!current) return prev
      return {
        ...prev,
        [key]: {
          ...current,
          hasPricing: true,
          [field]: field === 'note' ? (value as string) : Math.max(0, Number(value) || 0),
        },
      }
    })
    setModifiedImportKeys((prev) => new Set(prev).add(key))
    setIsImportDirty(true)
  }

  // Sorted list of models for selling prices: Opus first, Sol second, then alphabetical
  const sortedSellingModels = useMemo(() => {
    const list = Object.values(sellingDrafts)
    return list.sort((a, b) => {
      const aOpus = OPUS_MODELS.includes(a.model)
      const bOpus = OPUS_MODELS.includes(b.model)
      if (aOpus && !bOpus) return -1
      if (!aOpus && bOpus) return 1

      const aSol = SOL_MODELS.includes(a.model)
      const bSol = SOL_MODELS.includes(b.model)
      if (aSol && !bSol) return -1
      if (!aSol && bSol) return 1

      return a.model.localeCompare(b.model)
    })
  }, [sellingDrafts])

  // Sorted list of channel-model items for purchase prices
  const sortedChannelImportRows = useMemo(() => {
    const list = Object.values(importDrafts)
    return list.sort((a, b) => {
      if (a.channel_id !== b.channel_id) return a.channel_id - b.channel_id
      return a.model.localeCompare(b.model)
    })
  }, [importDrafts])

  const visibleSellingModels = sortedSellingModels
    .filter((item) => item.model.toLowerCase().includes(modelSearch.toLowerCase()) &&
      (familyFilter === 'all' || modelFamily(item.model) === familyFilter))
    .sort((a, b) => modelFamily(a.model).localeCompare(modelFamily(b.model)) || a.model.localeCompare(b.model))
  const visibleImportRows = sortedChannelImportRows.filter((item) =>
    item.model.toLowerCase().includes(modelSearch.toLowerCase()) &&
    (familyFilter === 'all' || modelFamily(item.model) === familyFilter))
  const visibleNames = [...new Set((activeTab === 'selling' ? visibleSellingModels : visibleImportRows).map((item) => item.model))]
  const selectedVisible = visibleNames.filter((model) => selectedModels.has(model))
  const toggleModels = (names: string[], checked: boolean) => setSelectedModels((previous) => {
    const next = new Set(previous)
    for (const name of names) {
      if (checked) next.add(name)
      else next.delete(name)
    }
    return next
  })
  const applyBulk = () => {
    try {
      // Calculate the complete preview first so invalid selections cannot partly apply.
      if (activeTab === 'selling') {
        const changes = selectedVisible.map((model) => {
          const draft = sellingDrafts[model]
          if (draft.isMultiTier || (bulkMode !== 'value' && !draft.hasPricing && !bulkFields.inputVnd)) {
            throw new Error('Use the advanced editor for unconfigured or multi-tier prices')
          }
          return { model, values: calculateBulkPrices(draft, bulkMode, bulkFields) }
        })
        for (const change of changes) {
          for (const field of priceFields) {
            const value = change.values[field]
            if (value !== undefined && value !== sellingDrafts[change.model][field]) {
              handleSellingPriceChange(change.model, field, value)
            }
          }
        }
      } else {
        const { cacheWriteVnd: _unsupported, ...fields } = bulkFields
        const changes = visibleImportRows.filter((row) => selectedModels.has(row.model)).map((row) => ({
          key: `${row.channel_id}:${row.model}`,
          values: calculateBulkPrices(row, bulkMode, fields),
        }))
        for (const change of changes) {
          for (const field of ['inputVnd', 'outputVnd', 'cacheReadVnd'] as const) {
            const value = change.values[field]
            if (value !== undefined && value !== importDrafts[change.key][field]) {
              handleImportPriceChange(change.key, field, value)
            }
          }
        }
      }
    } catch {
      toast.error(t('Invalid bulk adjustment. Check values and use Advanced Editor for multi-tier models.'))
    }
  }

  const isLoading = isScopeLoading || (uniqueModels.length > 0 && isPricingLoading) || isImportLoading

  if (isLoading) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Price Management')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <LoadingState message={t('Loading pricing data...')} />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  if (isScopeError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Price Management')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <ErrorState
            title={t('Failed to load channel pricing scope')}
            onRetry={() => refetchScope()}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  if (isPricingError && uniqueModels.length > 0) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Price Management')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <ErrorState
            title={t('Failed to load pricing data')}
            onRetry={() => refetchModelPricing()}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  if (isImportError) {
    return (
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Price Management')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <ErrorState
            title={t('Failed to load pricing data')}
            onRetry={() => refetchImportPricing()}
          />
        </SectionPageLayout.Content>
      </SectionPageLayout>
    )
  }

  const isEmpty = uniqueModels.length === 0

  return (
    <>
      <FormNavigationGuard when={isSellingDirty || isImportDirty} />

      <SectionPageLayout stackActionsOnMobile>
        <SectionPageLayout.Title>
          <div className='flex items-center gap-2'>
            <Coins className='size-5 text-primary' />
            <span>{t('Price Management')}</span>
          </div>
        </SectionPageLayout.Title>

        <SectionPageLayout.Actions>
          <div className='flex items-center gap-2'>
            {activeTab === 'selling' && (
              <>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setSyncPricingDialogOpen(true)}
                  className='gap-1.5'
                  disabled={isEmpty}
                >
                  <RefreshCw className='size-3.5' />
                  <span>{t('Sync pricing')}</span>
                </Button>
                <Button
                  size='sm'
                  disabled={!isSellingDirty || saveSellingMutation.isPending}
                  onClick={() => saveSellingMutation.mutate()}
                  className='gap-1.5'
                >
                  <Save className='size-3.5' />
                  <span>
                    {saveSellingMutation.isPending ? t('Saving...') : t('Save selling prices')}
                  </span>
                </Button>
              </>
            )}

            {activeTab === 'import' && (
              <Button
                size='sm'
                disabled={!isImportDirty || saveImportMutation.isPending}
                onClick={() => saveImportMutation.mutate()}
                className='gap-1.5'
              >
                <Save className='size-3.5' />
                <span>
                  {saveImportMutation.isPending ? t('Saving...') : t('Save purchase prices')}
                </span>
              </Button>
            )}
          </div>
        </SectionPageLayout.Actions>

        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <Card className='border shadow-sm'>
              <CardHeader className='pb-4'>
                <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                  <div>
                    <CardTitle className='text-base font-semibold'>
                      {activeTab === 'selling' ? t('Selling Prices') : t('Purchase Prices')}
                    </CardTitle>
                    <CardDescription className='mt-1 text-xs'>
                      {t(
                        'View and adjust Selling Prices and Purchase Prices independently. Unit: credit / 1,000,000 tokens (1 credit = 1,000 VNĐ).'
                      )}
                    </CardDescription>
                  </div>
                  <Tabs
                    value={activeTab}
                    onValueChange={(val) => setActiveTab(val as 'selling' | 'import')}
                    className='w-auto'
                  >
                    <TabsList className='grid grid-cols-2'>
                      <TabsTrigger value='selling' className='gap-1.5 text-xs'>
                        <Tag className='size-3.5' />
                        <span>{t('Selling Prices')}</span>
                      </TabsTrigger>
                      <TabsTrigger value='import' className='gap-1.5 text-xs'>
                        <Banknote className='size-3.5' />
                        <span>{t('Purchase Prices')}</span>
                      </TabsTrigger>
                    </TabsList>
                  </Tabs>
                </div>
              </CardHeader>

              <CardContent>
                {!isEmpty && (
                  <div className='mb-4 space-y-3'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <Input aria-label={t('Search models')} placeholder={t('Search models')} value={modelSearch} onChange={(event) => setModelSearch(event.target.value)} className='max-w-64' />
                      {['all', 'opus', 'fable', 'haiku', 'gpt', 'other'].map((family) => (
                        <Button key={family} size='sm' variant={familyFilter === family ? 'default' : 'outline'} onClick={() => setFamilyFilter(family)}>
                          {family === 'all' ? t('All') : family === 'other' ? t('Other') : family.toUpperCase()}
                        </Button>
                      ))}
                      <Button variant='outline' size='sm' onClick={() => toggleModels(visibleNames, selectedVisible.length !== visibleNames.length)}>{t('Select visible models')}</Button>
                    </div>
                    {selectedVisible.length > 0 && (
                      <div className='space-y-3 rounded-lg border border-primary/30 bg-primary/5 p-4'>
                        <p className='text-sm font-semibold'>{t('Bulk adjustment')} · {selectedVisible.length} {t('Models')}</p>
                        <div className='flex flex-wrap gap-2'>
                          {(['ratio', 'percent', 'value'] as const).map((mode) => (
                            <Button key={mode} size='sm' variant={bulkMode === mode ? 'default' : 'outline'} onClick={() => { setBulkMode(mode); setBulkFields({}) }}>
                              {t(mode === 'ratio' ? 'Relative to input' : mode === 'percent' ? 'Increase / decrease %' : 'Set value')}
                            </Button>
                          ))}
                        </div>
                        <div className='flex flex-wrap items-end gap-3'>
                          {priceFields.filter((field) => activeTab === 'selling' || field !== 'cacheWriteVnd').map((field) => (
                            <label key={field} className='grid gap-1 text-xs'>
                              {t(field === 'inputVnd' ? 'Input' : field === 'outputVnd' ? 'Output' : field === 'cacheReadVnd' ? 'Cache-read' : 'Cache-write')}
                              <Input type='text' inputMode='decimal' className='w-32' placeholder={t('Keep unchanged')} value={bulkFields[field] ?? ''} onChange={(event) => setBulkFields((previous) => ({ ...previous, [field]: event.target.value }))} />
                            </label>
                          ))}
                          <Button size='sm' disabled={!Object.values(bulkFields).some((value) => value?.trim())} onClick={applyBulk}>{t('Apply to selected models')}</Button>
                          <Button size='sm' variant='ghost' onClick={() => setSelectedModels(new Set())}>{t('Clear selection')}</Button>
                          {activeTab === 'selling' && <Button size='sm' variant='outline' onClick={() => openContextPricing(selectedVisible)}>{t('Long context pricing')}</Button>}
                        </div>
                        <p className='text-muted-foreground text-xs'>
                          {t('Preview only. Changes are not saved until you press Save.')}
                          {activeTab === 'import' && ` ${t('Applies to all declared channels for the selected visible models.')}`}
                                  {bulkMode === 'ratio' && ` ${t('Input is an absolute price in credit; other fields are multiples of Input.')}`}
                        </p>
                      </div>
                    )}
                    {(isSellingDirty || isImportDirty) && (
                      <p className='text-primary text-xs' role='status'>{t('Unsaved changes')}: {activeTab === 'selling' ? modifiedModelNames.size : modifiedImportKeys.size}</p>
                    )}
                  </div>
                )}
                {isEmpty ? (
                  <div className='flex flex-col items-center justify-center p-12 text-center border border-dashed rounded-lg bg-muted/10'>
                    <Coins className='size-10 text-muted-foreground mb-3 opacity-40' />
                    <h3 className='text-sm font-semibold text-foreground'>
                      {t('No models declared in channels')}
                    </h3>
                    <p className='text-xs text-muted-foreground mt-1 max-w-sm'>
                      {t('Add models in channel settings to manage pricing.')}
                    </p>
                    <Button
                      variant='outline'
                      size='sm'
                      className='mt-4 text-xs gap-1.5'
                      onClick={() => navigate({ to: '/channels' })}
                    >
                      <Radio className='size-3.5' />
                      <span>{t('Go to Channels')}</span>
                    </Button>
                  </div>
                ) : activeTab === 'selling' ? (
                  <div className='space-y-4'>
                    <Alert className='bg-muted/40 py-2.5'>
                      <AlertDescription className='text-xs leading-relaxed'>
                        <strong>{t('Selling price note:')}</strong>{' '}
                        {t(
                          'This is the SELLING PRICE FOR CUSTOMERS. The price displayed in the Models tab and deducted from LLM Gateway wallets uses this configuration. When saved, new prices take effect immediately for new requests.'
                        )}
                      </AlertDescription>
                    </Alert>

                    <div className='flex items-center justify-between text-xs text-muted-foreground'>
                      <div>
                        {t('Total models:')}{' '}
                        <span className='font-mono font-medium text-foreground'>
                          {sortedSellingModels.length}
                        </span>
                      </div>
                      <div className='font-mono text-[11px] text-muted-foreground'>
                          {t('Unit: credit / 1,000,000 tokens')}
                      </div>
                    </div>

                    <div className='rounded-md border overflow-x-auto'>
                      <Table>
                        <TableHeader>
                          <TableRow className='bg-muted/30 text-xs'>
                            <TableHead className='w-10'><Checkbox aria-label={t('Select visible models')} checked={selectedVisible.length > 0 && selectedVisible.length === visibleNames.length} indeterminate={selectedVisible.length > 0 && selectedVisible.length < visibleNames.length} onCheckedChange={(checked) => toggleModels(visibleNames, checked)} /></TableHead>
                            <TableHead className='w-[220px] font-medium'>{t('Model')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Input (credit / 1M)')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Cache-read (credit / 1M)')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Output (credit / 1M)')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Cache-write (credit / 1M)')}</TableHead>
                            <TableHead className='text-center w-[80px] font-medium'>{t('Actions')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {visibleSellingModels.map((item, index) => (
                            <Fragment key={item.model}>
                              {(index === 0 || modelFamily(visibleSellingModels[index - 1].model) !== modelFamily(item.model)) && (
                                <TableRow className='bg-muted/40'>
                                  <TableCell colSpan={7}>
                                    <label className='flex items-center gap-3 font-medium'>
                                      <Checkbox aria-label={`${t('Select group')} ${modelFamily(item.model)}`} checked={visibleNames.filter((name) => modelFamily(name) === modelFamily(item.model)).every((name) => selectedModels.has(name))} onCheckedChange={(checked) => toggleModels(visibleNames.filter((name) => modelFamily(name) === modelFamily(item.model)), checked)} />
                                      {modelFamily(item.model).toUpperCase()}
                                    </label>
                                  </TableCell>
                                </TableRow>
                              )}
                            <TableRow className={modifiedModelNames.has(item.model) ? 'bg-primary/10 text-xs' : 'text-xs hover:bg-muted/20'}>
                              <TableCell><Checkbox aria-label={`${t('Select model')} ${item.model}`} checked={selectedModels.has(item.model)} onCheckedChange={(checked) => toggleModels([item.model], checked)} /></TableCell>
                              <TableCell className='font-mono font-medium py-2.5'>
                                <div className='flex items-center gap-1.5 flex-wrap'>
                                  <span>{item.model}</span>
                                  {item.longContext && <Badge variant='outline' className='text-[10px]'>{t('Long context')} &gt; {item.longContext.threshold.toLocaleString()}</Badge>}
                                  {!item.hasPricing && (
                                    <Badge
                                      variant='outline'
                                      className='text-[10px] px-1 py-0 h-4 border-amber-500 text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/30'
                                    >
                                      {t('Not configured')}
                                    </Badge>
                                  )}
                                  {OPUS_MODELS.includes(item.model) && (
                                    <Badge
                                      variant='outline'
                                      className='text-[10px] px-1 py-0 h-4 border-amber-500 text-amber-600 dark:text-amber-400'
                                    >
                                      Opus
                                    </Badge>
                                  )}
                                  {SOL_MODELS.includes(item.model) && (
                                    <Badge
                                      variant='outline'
                                      className='text-[10px] px-1 py-0 h-4 border-blue-500 text-blue-600 dark:text-blue-400'
                                    >
                                      Sol
                                    </Badge>
                                  )}
                                  {item.isMultiTier && (
                                    <Badge
                                      variant='secondary'
                                      className='text-[10px] px-1 py-0 h-4'
                                      title={t('Multi-tier model: table edits apply to base tier.')}
                                    >
                                      {t('Multi-tier')}
                                    </Badge>
                                  )}
                                </div>
                              </TableCell>
                              <TableCell className='text-right py-2.5'>
                                <PriceVndInput
                                  valueVnd={item.inputVnd}
                                  hasPricing={item.hasPricing}
                                  placeholder={item.hasPricing ? '0' : t('Not configured')}
                                  onChangeVnd={(vnd) =>
                                    handleSellingPriceChange(item.model, 'inputVnd', vnd)
                                  }
                                  className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                  aria-label={`${t('Input (credit / 1M)')} ${item.model}`}
                                />
                              </TableCell>
                              <TableCell className='text-right py-2.5'>
                                <PriceVndInput
                                  valueVnd={item.cacheReadVnd}
                                  hasPricing={item.hasPricing}
                                  placeholder={item.hasPricing ? '0' : t('Not configured')}
                                  onChangeVnd={(vnd) =>
                                    handleSellingPriceChange(item.model, 'cacheReadVnd', vnd)
                                  }
                                  className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                  aria-label={`${t('Cache-read (credit / 1M)')} ${item.model}`}
                                />
                              </TableCell>
                              <TableCell className='text-right py-2.5'>
                                <PriceVndInput
                                  valueVnd={item.outputVnd}
                                  hasPricing={item.hasPricing}
                                  placeholder={item.hasPricing ? '0' : t('Not configured')}
                                  onChangeVnd={(vnd) =>
                                    handleSellingPriceChange(item.model, 'outputVnd', vnd)
                                  }
                                  className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                  aria-label={`${t('Output (credit / 1M)')} ${item.model}`}
                                />
                              </TableCell>
                              <TableCell className='text-right py-2.5'>
                                <PriceVndInput
                                  valueVnd={item.cacheWriteVnd}
                                  hasPricing={item.cacheWriteVnd !== undefined}
                                  placeholder={t('Keep unchanged')}
                                  onChangeVnd={(vnd) =>
                                    handleSellingPriceChange(item.model, 'cacheWriteVnd', vnd)
                                  }
                                  className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                  aria-label={`${t('Cache-write (credit / 1M)')} ${item.model}`}
                                />
                              </TableCell>
                              <TableCell className='text-center py-2.5'>
                                <Button variant='outline' size='sm' className='h-7 px-2 text-xs' onClick={() => openContextPricing([item.model])}>{t('Long context pricing')}</Button>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => setAdvancedModelName(item.model)}
                                  className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground'
                                  title={t('Advanced Editor')}
                                >
                                  <Sliders className='size-3.5' />
                                </Button>
                              </TableCell>
                            </TableRow>
                            </Fragment>
                          ))}
                        </TableBody>
                      </Table>
                    </div>
                  </div>
                ) : (
                  <div className='space-y-4'>
                    <Alert className='bg-muted/40 py-2.5'>
                      <AlertDescription className='text-xs leading-relaxed'>
                        <strong>{t('Purchase price note:')}</strong>{' '}
                        {t(
                          'Purchase prices are declared by Channel and Model for cost management. This is independent from actual upstream reconciliation costs and does not alter customer selling prices.'
                        )}
                      </AlertDescription>
                    </Alert>

                    <div className='flex items-center justify-between text-xs text-muted-foreground'>
                      <div>
                        {t('Declared channel model items:')}{' '}
                        <span className='font-mono font-medium text-foreground'>
                          {sortedChannelImportRows.length}
                        </span>
                      </div>
                      <div className='font-mono text-[11px] text-muted-foreground'>
                        {t('Unit: credit / 1,000,000 tokens')}
                      </div>
                    </div>

                    <div className='rounded-md border overflow-x-auto'>
                      <Table>
                        <TableHeader>
                          <TableRow className='bg-muted/30 text-xs'>
                            <TableHead className='font-medium'>{t('Channel')}</TableHead>
                            <TableHead className='font-medium'>{t('Model')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Input (credit / 1M)')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Cache-read (credit / 1M)')}</TableHead>
                            <TableHead className='text-right font-medium'>{t('Output (credit / 1M)')}</TableHead>
                            <TableHead className='font-medium'>{t('Note')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {visibleImportRows.map((row) => {
                            const key = `${row.channel_id}:${row.model}`
                            return (
                              <TableRow key={key} className='text-xs hover:bg-muted/20'>
                                <TableCell className='font-medium py-2.5'>
                                  <div className='flex items-center gap-1.5'>
                                    <span className='text-muted-foreground font-mono text-[11px]'>
                                      #{row.channel_id}
                                    </span>
                                    <span>{row.channel_name}</span>
                                  </div>
                                </TableCell>
                                <TableCell className='font-mono py-2.5'>
                                  <div className='flex items-center gap-1.5'>
                                    <span>{row.model}</span>
                                    {!row.hasPricing && (
                                      <Badge
                                        variant='outline'
                                        className='text-[10px] px-1 py-0 h-4 border-amber-500 text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/30'
                                      >
                                        {t('Not configured')}
                                      </Badge>
                                    )}
                                  </div>
                                </TableCell>
                                <TableCell className='text-right py-2.5'>
                                  <PriceVndInput
                                    valueVnd={row.inputVnd}
                                    hasPricing={row.hasPricing}
                                    placeholder={row.hasPricing ? '0' : t('Not configured')}
                                    onChangeVnd={(vnd) =>
                                      handleImportPriceChange(key, 'inputVnd', vnd)
                                    }
                                    className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                    aria-label={`${t('Input (credit / 1M)')} ${row.model}`}
                                  />
                                </TableCell>
                                <TableCell className='text-right py-2.5'>
                                  <PriceVndInput
                                    valueVnd={row.cacheReadVnd}
                                    hasPricing={row.hasPricing}
                                    placeholder={row.hasPricing ? '0' : t('Not configured')}
                                    onChangeVnd={(vnd) =>
                                      handleImportPriceChange(key, 'cacheReadVnd', vnd)
                                    }
                                    className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                    aria-label={`${t('Cache-read (credit / 1M)')} ${row.model}`}
                                  />
                                </TableCell>
                                <TableCell className='text-right py-2.5'>
                                  <PriceVndInput
                                    valueVnd={row.outputVnd}
                                    hasPricing={row.hasPricing}
                                    placeholder={row.hasPricing ? '0' : t('Not configured')}
                                    onChangeVnd={(vnd) =>
                                      handleImportPriceChange(key, 'outputVnd', vnd)
                                    }
                                    className='h-7 w-28 ml-auto text-right font-mono text-xs'
                                    aria-label={`${t('Output (credit / 1M)')} ${row.model}`}
                                  />
                                </TableCell>
                                <TableCell className='py-2.5'>
                                  <Input
                                    type='text'
                                    value={row.note || ''}
                                    placeholder={t('Optional note...')}
                                    onChange={(e) =>
                                      handleImportPriceChange(key, 'note', e.target.value)
                                    }
                                    className='h-7 w-36 text-xs'
                                  />
                                </TableCell>
                              </TableRow>
                            )
                          })}
                        </TableBody>
                      </Table>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <PriceSyncDialog
        open={syncPricingDialogOpen}
        onOpenChange={setSyncPricingDialogOpen}
      />

      <AppDialog
        open={contextTargets.length > 0}
        onOpenChange={open => { if (!open) setContextTargets([]) }}
        title={t('Long context pricing')}
        description={contextTargets.join(', ')}
        footer={<Button onClick={applyContextPricing}>{t('Apply')}</Button>}
      >
        <div className='space-y-4'>
          <div className='flex items-center justify-between gap-3 text-sm'>
            {t('Enable long context pricing')}
            <Switch checked={contextEnabled} onCheckedChange={setContextEnabled} aria-label={t('Enable long context pricing')} />
          </div>
          <p className='text-muted-foreground text-xs'>{t('Context includes cached input, not output. Above the threshold, multipliers apply to the entire request. Changes only affect new requests after saving selling prices.')}</p>
          <div className='grid grid-cols-2 gap-3'>
            {(Object.keys(contextFields) as (keyof LongContextPricing)[]).map(field => (
              <label key={field} className='grid gap-1 text-xs'>
                {t(field === 'threshold' ? 'Input context threshold (tokens)' : field === 'input' ? 'Input multiplier' : field === 'output' ? 'Output multiplier' : field === 'cacheRead' ? 'Cache-read multiplier' : 'Cache-write multiplier')}
                <Input disabled={!contextEnabled} value={contextFields[field]} inputMode={field === 'threshold' ? 'numeric' : 'decimal'} onChange={event => setContextFields(previous => ({ ...previous, [field]: event.target.value }))} />
              </label>
            ))}
          </div>
          {contextError && <p role='alert' className='text-destructive text-xs'>{t('Threshold must be a positive integer; multipliers must be finite numbers of at least 1.')}</p>}
          <p className='text-muted-foreground text-xs'>{t('Preview only. Changes are not saved until you press Save.')}</p>
        </div>
      </AppDialog>

      {/* Advanced pricing dialog for deep editing */}
      {advancedModelName && (
        <AppDialog
          open={Boolean(advancedModelName)}
          onOpenChange={(v) => !v && setAdvancedModelName(null)}
          title={t('Advanced Pricing: {{model}}', { model: advancedModelName })}
          description={t('Configure tier expressions and multi-lane pricing for {{model}}', {
            model: advancedModelName,
          })}
          contentClassName='sm:max-w-4xl'
          contentHeight='min(85vh, 850px)'
          bodyClassName='h-full overflow-y-auto'
        >
          <ModelPricingPanel
            modelName={advancedModelName}
            onSaveSuccess={() => {
              refetchModelPricing()
              queryClient.invalidateQueries({ queryKey: ['pricing'] })
              queryClient.invalidateQueries({ queryKey: ['models'] })
              setAdvancedModelName(null)
            }}
          />
        </AppDialog>
      )}
    </>
  )
}
