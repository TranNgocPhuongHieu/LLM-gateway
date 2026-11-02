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
import {
  ChevronDown,
  ChevronUp,
  Copy,
  GripVertical,
  Plus,
  Search,
  Trash2,
} from 'lucide-react'
import {
  type DragEvent,
  type KeyboardEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { JsonCodeEditor } from '@/components/json-code-editor'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'


import { ParamOverrideCondition, ParamOverrideOperation, ParamOverrideEditorDialogProps, OPERATION_MODE_OPTIONS, OPERATION_MODE_LABEL_MAP, CONDITION_MODE_OPTIONS, MODE_META, MODE_DESCRIPTIONS, SYNC_TARGET_TYPE_OPTIONS, LEGACY_TEMPLATE, OPERATION_TEMPLATE, TEMPLATE_PRESET_CONFIG, parseLooseValue, verifyJSON, createDefaultCondition, normalizeOperation, createDefaultOperation, reorderOperations, isOperationBlank, getOperationSummary, getModeTagTailwind, getModePathLabel, getModePathPlaceholder, getModeFromLabel, getModeFromPlaceholder, getModeToLabel, getModeToPlaceholder, getModeValueLabel, getModeValuePlaceholder, parseSyncTargetSpec, buildSyncTargetSpec, ReturnErrorDraft, parseReturnErrorDraft, buildReturnErrorValueText, PruneRule, PruneObjectsDraft, normalizePruneRule, parsePruneObjectsDraft, buildPruneObjectsValueText, parseInitialState, buildOperationsJson } from './param-override-types-utils';
// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export function ParamOverrideEditorDialog(
  props: ParamOverrideEditorDialogProps
) {
  const { t } = useTranslation()

  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')
  const [visualMode, setVisualMode] = useState<'operations' | 'legacy'>(
    'operations'
  )
  const [legacyValue, setLegacyValue] = useState('')
  const [operations, setOperations] = useState<ParamOverrideOperation[]>([
    createDefaultOperation(),
  ])
  const [jsonText, setJsonText] = useState('')
  const [jsonError, setJsonError] = useState('')
  const [operationSearch, setOperationSearch] = useState('')
  const [selectedOperationId, setSelectedOperationId] = useState('')
  const [expandedConditions, setExpandedConditions] = useState<
    Record<string, boolean>
  >({})
  const [draggedOperationId, setDraggedOperationId] = useState('')
  const [dragOverOperationId, setDragOverOperationId] = useState('')
  const [dragOverPosition, setDragOverPosition] = useState<'before' | 'after'>(
    'before'
  )
  const [templatePresetKey, setTemplatePresetKey] =
    useState('operations_default')

  // Initialize state when dialog opens
  useEffect(() => {
    if (!props.open) return
    const state = parseInitialState(props.value)
    setEditMode(state.editMode)
    setVisualMode(state.visualMode)
    setLegacyValue(state.legacyValue)
    setOperations(state.operations)
    setJsonText(state.jsonText)
    setJsonError(state.jsonError)
    setOperationSearch('')
    setSelectedOperationId(state.operations[0]?.id || '')
    setExpandedConditions({})
    setDraggedOperationId('')
    setDragOverOperationId('')
    setDragOverPosition('before')
    if (state.visualMode === 'legacy') {
      setTemplatePresetKey('legacy_default')
    } else {
      setTemplatePresetKey('operations_default')
    }
  }, [props.open, props.value])

  // Keep selectedOperationId valid
  useEffect(() => {
    if (operations.length === 0) {
      setSelectedOperationId('')
      return
    }
    if (!operations.some((o) => o.id === selectedOperationId)) {
      setSelectedOperationId(operations[0].id)
    }
  }, [operations, selectedOperationId])

  // Template preset options filtered by group
  const templatePresetOptions = useMemo(
    () =>
      Object.entries(TEMPLATE_PRESET_CONFIG).map(([value, config]) => ({
        value,
        label: config.label,
      })),
    []
  )

  const operationCount = useMemo(
    () => operations.filter((o) => !isOperationBlank(o)).length,
    [operations]
  )

  const filteredOperations = useMemo(() => {
    const keyword = operationSearch.trim().toLowerCase()
    if (!keyword) return operations
    return operations.filter((op) => {
      const searchableText = [
        op.description,
        op.mode,
        op.path,
        op.from,
        op.to,
        op.value_text,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return searchableText.includes(keyword)
    })
  }, [operationSearch, operations])

  const selectedOperation = useMemo(
    () => operations.find((o) => o.id === selectedOperationId),
    [operations, selectedOperationId]
  )

  const selectedOperationIndex = useMemo(
    () => operations.findIndex((o) => o.id === selectedOperationId),
    [operations, selectedOperationId]
  )

  const returnErrorDraft = useMemo(() => {
    if (!selectedOperation || selectedOperation.mode !== 'return_error') {
      return null
    }
    return parseReturnErrorDraft(selectedOperation.value_text)
  }, [selectedOperation])

  const pruneObjectsDraft = useMemo(() => {
    if (!selectedOperation || selectedOperation.mode !== 'prune_objects') {
      return null
    }
    return parsePruneObjectsDraft(selectedOperation.value_text)
  }, [selectedOperation])

  const topOperationModes = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const op of operations) {
      const mode = op.mode || 'set'
      counts[mode] = (counts[mode] || 0) + 1
    }
    return Object.entries(counts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 4)
  }, [operations])

  // ---------------------------------------------------------------------------
  // Operations
  // ---------------------------------------------------------------------------

  const updateOperation = useCallback(
    (operationId: string, patch: Partial<ParamOverrideOperation>) => {
      setOperations((prev) =>
        prev.map((o) => (o.id === operationId ? { ...o, ...patch } : o))
      )
    },
    []
  )

  const addOperation = useCallback(() => {
    const created = createDefaultOperation()
    setOperations((prev) => [...prev, created])
    setSelectedOperationId(created.id)
  }, [])

  const duplicateOperation = useCallback((operationId: string) => {
    let insertedId = ''
    setOperations((prev) => {
      const idx = prev.findIndex((o) => o.id === operationId)
      if (idx < 0) return prev
      const source = prev[idx]
      const cloned = normalizeOperation({
        description: source.description,
        path: source.path,
        mode: source.mode,
        value: parseLooseValue(source.value_text),
        keep_origin: source.keep_origin,
        from: source.from,
        to: source.to,
        logic: source.logic,
        conditions: source.conditions.map((c) => ({
          path: c.path,
          mode: c.mode,
          value: parseLooseValue(c.value_text),
          invert: c.invert,
          pass_missing_key: c.pass_missing_key,
        })),
      })
      insertedId = cloned.id
      const next = [...prev]
      next.splice(idx + 1, 0, cloned)
      return next
    })
    if (insertedId) setSelectedOperationId(insertedId)
  }, [])

  const removeOperation = useCallback((operationId: string) => {
    setOperations((prev) => {
      if (prev.length <= 1) return [createDefaultOperation()]
      return prev.filter((o) => o.id !== operationId)
    })
  }, [])

  // Conditions
  const addCondition = useCallback((operationId: string) => {
    const created = createDefaultCondition()
    setOperations((prev) =>
      prev.map((op) =>
        op.id === operationId
          ? { ...op, conditions: [...op.conditions, created] }
          : op
      )
    )
    setExpandedConditions((prev) => ({ ...prev, [created.id]: true }))
  }, [])

  const updateCondition = useCallback(
    (
      operationId: string,
      conditionId: string,
      patch: Partial<ParamOverrideCondition>
    ) => {
      setOperations((prev) =>
        prev.map((op) =>
          op.id === operationId
            ? {
                ...op,
                conditions: op.conditions.map((c) =>
                  c.id === conditionId ? { ...c, ...patch } : c
                ),
              }
            : op
        )
      )
    },
    []
  )

  const removeCondition = useCallback(
    (operationId: string, conditionId: string) => {
      setOperations((prev) =>
        prev.map((op) =>
          op.id === operationId
            ? {
                ...op,
                conditions: op.conditions.filter((c) => c.id !== conditionId),
              }
            : op
        )
      )
    },
    []
  )

  // return_error draft
  const updateReturnErrorDraft = useCallback(
    (operationId: string, draftPatch: Partial<ReturnErrorDraft>) => {
      setOperations((prev) =>
        prev.map((op) => {
          if (op.id !== operationId) return op
          const draft = parseReturnErrorDraft(op.value_text)
          const nextDraft = { ...draft, ...draftPatch }
          return {
            ...op,
            value_text: buildReturnErrorValueText(nextDraft),
          }
        })
      )
    },
    []
  )

  // prune_objects draft
  const updatePruneObjectsDraft = useCallback(
    (
      operationId: string,
      updater:
        | Partial<PruneObjectsDraft>
        | ((draft: PruneObjectsDraft) => PruneObjectsDraft)
    ) => {
      setOperations((prev) =>
        prev.map((op) => {
          if (op.id !== operationId) return op
          const draft = parsePruneObjectsDraft(op.value_text)
          const nextDraft =
            typeof updater === 'function'
              ? updater(draft)
              : { ...draft, ...updater }
          return {
            ...op,
            value_text: buildPruneObjectsValueText(nextDraft),
          }
        })
      )
    },
    []
  )

  const addPruneRule = useCallback(
    (operationId: string) => {
      updatePruneObjectsDraft(operationId, (draft) => ({
        ...draft,
        simpleMode: false,
        rules: [...draft.rules, normalizePruneRule({})],
      }))
    },
    [updatePruneObjectsDraft]
  )

  const updatePruneRule = useCallback(
    (operationId: string, ruleId: string, patch: Partial<PruneRule>) => {
      updatePruneObjectsDraft(operationId, (draft) => ({
        ...draft,
        rules: draft.rules.map((r) =>
          r.id === ruleId ? { ...r, ...patch } : r
        ),
      }))
    },
    [updatePruneObjectsDraft]
  )

  const removePruneRule = useCallback(
    (operationId: string, ruleId: string) => {
      updatePruneObjectsDraft(operationId, (draft) => ({
        ...draft,
        rules: draft.rules.filter((r) => r.id !== ruleId),
      }))
    },
    [updatePruneObjectsDraft]
  )

  // Drag and drop
  const resetDragState = useCallback(() => {
    setDraggedOperationId('')
    setDragOverOperationId('')
    setDragOverPosition('before')
  }, [])

  const handleDragStart = useCallback(
    (event: DragEvent, operationId: string) => {
      setDraggedOperationId(operationId)
      setSelectedOperationId(operationId)
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData('text/plain', operationId)
    },
    []
  )

  const handleDragOver = useCallback(
    (event: DragEvent, operationId: string) => {
      event.preventDefault()
      if (!draggedOperationId || draggedOperationId === operationId) return
      const rect = event.currentTarget.getBoundingClientRect()
      const position: 'before' | 'after' =
        event.clientY - rect.top > rect.height / 2 ? 'after' : 'before'
      setDragOverOperationId(operationId)
      setDragOverPosition(position)
      event.dataTransfer.dropEffect = 'move'
    },
    [draggedOperationId]
  )

  const handleDrop = useCallback(
    (event: DragEvent, operationId: string) => {
      event.preventDefault()
      const sourceId =
        draggedOperationId || event.dataTransfer.getData('text/plain')
      const position =
        dragOverOperationId === operationId ? dragOverPosition : 'before'
      if (sourceId && operationId && sourceId !== operationId) {
        setOperations((prev) =>
          reorderOperations(prev, sourceId, operationId, position)
        )
        setSelectedOperationId(sourceId)
      }
      resetDragState()
    },
    [draggedOperationId, dragOverOperationId, dragOverPosition, resetDragState]
  )

  // ---------------------------------------------------------------------------
  // Mode switching & templates
  // ---------------------------------------------------------------------------

  const buildVisualJson = useCallback((): string => {
    if (visualMode === 'legacy') {
      const trimmed = legacyValue.trim()
      if (!trimmed) return ''
      if (!verifyJSON(trimmed)) {
        throw new Error(t('Parameter override must be valid JSON format'))
      }
      const parsed = JSON.parse(trimmed) as unknown
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error(t('Legacy format must be a JSON object'))
      }
      return JSON.stringify(parsed, null, 2)
    }
    return buildOperationsJson(operations, { validate: true }, t)
  }, [legacyValue, operations, t, visualMode])

  const switchToJsonMode = useCallback(() => {
    if (editMode === 'json') return
    try {
      setJsonText(buildVisualJson())
      setJsonError('')
    } catch (error) {
      toast.error((error as Error).message)
      if (visualMode === 'legacy') {
        setJsonText(legacyValue)
      } else {
        setJsonText(buildOperationsJson(operations, { validate: false }, t))
      }
      setJsonError(
        (error as Error).message || t('Parameter configuration error')
      )
    }
    setEditMode('json')
  }, [buildVisualJson, editMode, legacyValue, operations, t, visualMode])

  const switchToVisualMode = useCallback(() => {
    if (editMode === 'visual') return
    const trimmed = jsonText.trim()
    if (!trimmed) {
      const fallback = createDefaultOperation()
      setVisualMode('operations')
      setOperations([fallback])
      setSelectedOperationId(fallback.id)
      setLegacyValue('')
      setJsonError('')
      setEditMode('visual')
      return
    }
    if (!verifyJSON(trimmed)) {
      toast.error(t('Parameter override must be valid JSON format'))
      return
    }
    const parsed = JSON.parse(trimmed) as Record<string, unknown>
    if (
      parsed &&
      typeof parsed === 'object' &&
      !Array.isArray(parsed) &&
      Array.isArray(parsed.operations)
    ) {
      const nextOps =
        (parsed.operations as Record<string, unknown>[]).length > 0
          ? (parsed.operations as Record<string, unknown>[]).map(
              normalizeOperation
            )
          : [createDefaultOperation()]
      setVisualMode('operations')
      setOperations(nextOps)
      setSelectedOperationId(nextOps[0]?.id || '')
      setLegacyValue('')
      setJsonError('')
      setEditMode('visual')
      setTemplatePresetKey('operations_default')
      return
    }
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      const fallback = createDefaultOperation()
      setVisualMode('legacy')
      setLegacyValue(JSON.stringify(parsed, null, 2))
      setOperations([fallback])
      setSelectedOperationId(fallback.id)
      setJsonError('')
      setEditMode('visual')
      setTemplatePresetKey('legacy_default')
      return
    }
    toast.error(t('Parameter override must be a valid JSON object'))
  }, [editMode, jsonText, t])

  const fillTemplate = useCallback(
    (mode: 'fill' | 'append') => {
      const preset =
        TEMPLATE_PRESET_CONFIG[templatePresetKey] ||
        TEMPLATE_PRESET_CONFIG.operations_default
      const payload = preset.payload as Record<string, unknown>

      if (preset.kind === 'legacy') {
        if (mode === 'append' && visualMode === 'legacy') {
          const trimmed = legacyValue.trim()
          let parsedCurrent: Record<string, unknown> = {}
          if (trimmed) {
            if (!verifyJSON(trimmed)) {
              toast.error(t('Current legacy JSON is invalid, cannot append'))
              return
            }
            parsedCurrent = JSON.parse(trimmed) as Record<string, unknown>
          }
          const merged = { ...payload, ...parsedCurrent }
          const text = JSON.stringify(merged, null, 2)
          setVisualMode('legacy')
          setLegacyValue(text)
          setOperations([createDefaultOperation()])
          setJsonText(text)
          setJsonError('')
          setEditMode('visual')
        } else {
          const text = JSON.stringify(payload, null, 2)
          setVisualMode('legacy')
          setLegacyValue(text)
          setOperations([createDefaultOperation()])
          setJsonText(text)
          setJsonError('')
          setEditMode('visual')
        }
        return
      }

      const operationsPayload = ((payload as Record<string, unknown>)
        .operations || []) as Record<string, unknown>[]

      if (mode === 'append') {
        const appended = operationsPayload.map(normalizeOperation)
        const existing =
          visualMode === 'operations'
            ? operations.filter((o) => !isOperationBlank(o))
            : []
        const nextOps = [...existing, ...appended]
        setVisualMode('operations')
        setOperations(nextOps.length > 0 ? nextOps : appended)
        setSelectedOperationId(nextOps[0]?.id || appended[0]?.id || '')
        setLegacyValue('')
        setJsonError('')
        setEditMode('visual')
        setJsonText('')
      } else {
        const nextOps = operationsPayload.map(normalizeOperation)
        const finalOps =
          nextOps.length > 0 ? nextOps : [createDefaultOperation()]
        setVisualMode('operations')
        setOperations(finalOps)
        setSelectedOperationId(finalOps[0]?.id || '')
        setJsonText(JSON.stringify({ operations: operationsPayload }, null, 2))
        setJsonError('')
        setEditMode('visual')
      }
    },
    [legacyValue, operations, templatePresetKey, visualMode, t]
  )

  const resetEditorState = useCallback(() => {
    const fallback = createDefaultOperation()
    setVisualMode('operations')
    setLegacyValue('')
    setOperations([fallback])
    setSelectedOperationId(fallback.id)
    setJsonText('')
    setJsonError('')
    setTemplatePresetKey('operations_default')
    setEditMode('visual')
  }, [])

  // JSON mode
  const handleJsonChange = useCallback(
    (nextValue: string) => {
      setJsonText(nextValue)
      const trimmed = nextValue.trim()
      if (!trimmed) {
        setJsonError('')
        return
      }
      setJsonError(verifyJSON(trimmed) ? '' : t('JSON format error'))
    },
    [t]
  )

  const visualValidationError = useMemo(() => {
    if (editMode !== 'visual') return ''
    try {
      buildVisualJson()
      return ''
    } catch (error) {
      return (error as Error)?.message || t('Parameter configuration error')
    }
  }, [buildVisualJson, editMode, t])

  // Save
  const handleSave = useCallback(() => {
    try {
      let result = ''
      if (editMode === 'json') {
        const trimmed = jsonText.trim()
        if (trimmed) {
          if (!verifyJSON(trimmed)) {
            throw new Error(t('Parameter override must be valid JSON format'))
          }
          result = JSON.stringify(JSON.parse(trimmed), null, 2)
        }
      } else {
        result = buildVisualJson()
      }
      props.onSave(result)
      props.onOpenChange(false)
    } catch (error) {
      toast.error((error as Error).message)
    }
  }, [buildVisualJson, editMode, jsonText, props, t])

  // Expand/collapse all conditions
  const expandAllConditions = useCallback(() => {
    if (!selectedOperation) return
    const map: Record<string, boolean> = {}
    for (const c of selectedOperation.conditions) map[c.id] = true
    setExpandedConditions((prev) => ({ ...prev, ...map }))
  }, [selectedOperation])

  const collapseAllConditions = useCallback(() => {
    if (!selectedOperation) return
    const map: Record<string, boolean> = {}
    for (const c of selectedOperation.conditions) map[c.id] = false
    setExpandedConditions((prev) => ({ ...prev, ...map }))
  }, [selectedOperation])

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Parameter Override')}
      description={t(
        'Create request parameter override rules with a visual editor or raw JSON.'
      )}
      contentClassName='flex max-h-[90vh] flex-col gap-0 p-0 sm:max-w-5xl'
      headerClassName='border-b px-6 py-4'
      footerClassName='border-t px-6 py-4'
      contentHeight='min(72vh, 720px)'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={handleSave}>
            {t('Save')}
          </Button>
        </>
      }
    >
      {/* Toolbar */}
      <div className='bg-muted/30 border-b px-4 py-3'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='text-muted-foreground text-xs font-medium'>
            {t('Mode')}
          </span>
          <Button
            type='button'
            variant={editMode === 'visual' ? 'default' : 'outline'}
            size='sm'
            onClick={switchToVisualMode}
          >
            {t('Visual')}
          </Button>
          <Button
            type='button'
            variant={editMode === 'json' ? 'default' : 'outline'}
            size='sm'
            onClick={switchToJsonMode}
          >
            {t('JSON Text')}
          </Button>

          <div className='bg-border mx-1 h-5 w-px' />

          <span className='text-muted-foreground text-xs font-medium'>
            {t('Template')}
          </span>
          <Combobox
            options={templatePresetOptions.map((o) => ({
              value: o.value,
              label: t(o.label),
            }))}
            value={templatePresetKey}
            onValueChange={(v) =>
              setTemplatePresetKey(v || 'operations_default')
            }
            className='h-8 w-[220px]'
          />
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => fillTemplate('fill')}
          >
            {t('Fill Template')}
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => fillTemplate('append')}
          >
            {t('Append Template')}
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={resetEditorState}
          >
            {t('Reset')}
          </Button>
        </div>
      </div>
      {/* Content */}
      <div className='min-h-0 flex-1 overflow-hidden'>
        {editMode === 'visual' &&
          (visualMode === 'legacy' ? (
            <div className='p-4'>
              <p className='text-muted-foreground mb-2 text-sm'>
                {t('Legacy Format (JSON Object)')}
              </p>
              <JsonCodeEditor
                value={legacyValue}
                onChange={setLegacyValue}
                placeholder={JSON.stringify(LEGACY_TEMPLATE, null, 2)}
                heightClassName='h-72 min-h-72 max-h-72'
                ariaLabel={t('Legacy Format (JSON Object)')}
              />
              <p className='text-muted-foreground mt-2 text-xs'>
                {t(
                  'Edit JSON object directly. Suitable for simple parameter overrides.'
                )}
              </p>
            </div>
          ) : (
            <div className='flex h-full'>
              {/* Left sidebar */}
              <div className='flex w-[280px] flex-shrink-0 flex-col border-r'>
                <div className='flex items-center justify-between border-b px-3 py-2'>
                  <div className='flex items-center gap-2'>
                    <span className='text-sm font-medium'>{t('Rules')}</span>
                    <Badge variant='secondary'>
                      {operationCount}/{operations.length}
                    </Badge>
                  </div>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    onClick={addOperation}
                  >
                    <Plus className='h-4 w-4' />
                  </Button>
                </div>

                {topOperationModes.length > 0 && (
                  <div className='flex flex-wrap gap-1 border-b px-3 py-2'>
                    {topOperationModes.map(([mode, count]) => (
                      <span
                        key={`mode_stat_${mode}`}
                        className={cn(
                          'inline-flex items-center rounded-md border px-1.5 py-0.5 text-[10px] font-medium',
                          getModeTagTailwind(mode)
                        )}
                      >
                        {t(OPERATION_MODE_LABEL_MAP[mode] || mode)} · {count}
                      </span>
                    ))}
                  </div>
                )}

                <div className='px-3 py-2'>
                  <div className='relative'>
                    <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-3.5 w-3.5' />
                    <Input
                      value={operationSearch}
                      onChange={(e) => setOperationSearch(e.target.value)}
                      placeholder={t('Search rules...')}
                      className='h-8 pl-8 text-xs'
                    />
                  </div>
                </div>

                <ScrollArea className='flex-1'>
                  <div className='flex flex-col gap-1 px-3 pb-3'>
                    {filteredOperations.length === 0 ? (
                      <p className='text-muted-foreground py-4 text-center text-xs'>
                        {t('No matching rules')}
                      </p>
                    ) : (
                      filteredOperations.map((operation) => {
                        const index = operations.findIndex(
                          (o) => o.id === operation.id
                        )
                        const isActive = operation.id === selectedOperationId
                        const isDragging = operation.id === draggedOperationId
                        const isDropTarget =
                          operation.id === dragOverOperationId &&
                          draggedOperationId !== '' &&
                          draggedOperationId !== operation.id
                        return (
                          <div
                            key={operation.id}
                            role='button'
                            tabIndex={0}
                            draggable={operations.length > 1}
                            onClick={() => setSelectedOperationId(operation.id)}
                            onDragStart={(e) =>
                              handleDragStart(e, operation.id)
                            }
                            onDragOver={(e) => handleDragOver(e, operation.id)}
                            onDrop={(e) => handleDrop(e, operation.id)}
                            onDragEnd={resetDragState}
                            onKeyDown={(e: KeyboardEvent) => {
                              if (e.key === 'Enter' || e.key === ' ') {
                                e.preventDefault()
                                setSelectedOperationId(operation.id)
                              }
                            }}
                            className={cn(
                              'cursor-pointer rounded-lg border p-2.5 transition-colors',
                              isActive
                                ? 'border-primary bg-primary/5'
                                : 'hover:bg-muted/50',
                              isDragging && 'opacity-50',
                              isDropTarget &&
                                dragOverPosition === 'before' &&
                                'border-t-primary border-t-2',
                              isDropTarget &&
                                dragOverPosition === 'after' &&
                                'border-b-primary border-b-2'
                            )}
                          >
                            <div className='flex items-start gap-2'>
                              <GripVertical
                                className={cn(
                                  'text-muted-foreground mt-0.5 h-3.5 w-3.5 flex-shrink-0',
                                  operations.length > 1
                                    ? 'cursor-grab'
                                    : 'cursor-default'
                                )}
                              />
                              <div className='min-w-0 flex-1'>
                                <div className='flex items-center justify-between gap-1'>
                                  <span className='text-xs font-semibold'>
                                    #{index + 1}
                                  </span>
                                  <Badge
                                    variant='outline'
                                    className='text-[10px]'
                                  >
                                    {operation.conditions.length}
                                  </Badge>
                                </div>
                                <p className='text-muted-foreground mt-0.5 line-clamp-1 text-[11px]'>
                                  {getOperationSummary(operation, index)}
                                </p>
                                {operation.description.trim() && (
                                  <p className='text-muted-foreground mt-0.5 line-clamp-2 text-[10px]'>
                                    {operation.description}
                                  </p>
                                )}
                                <span
                                  className={cn(
                                    'mt-1 inline-flex items-center rounded-md border px-1.5 py-0.5 text-[10px] font-medium',
                                    getModeTagTailwind(operation.mode || 'set')
                                  )}
                                >
                                  {t(
                                    OPERATION_MODE_LABEL_MAP[
                                      operation.mode || 'set'
                                    ] ||
                                      operation.mode ||
                                      'set'
                                  )}
                                </span>
                              </div>
                            </div>
                          </div>
                        )
                      })
                    )}
                  </div>
                </ScrollArea>
              </div>

              {/* Right panel - Rule editor */}
              <div className='flex min-w-0 flex-1 flex-col overflow-y-auto'>
                {selectedOperation ? (
                  <RuleEditor
                    operation={selectedOperation}
                    operationIndex={selectedOperationIndex}
                    operations={operations}
                    returnErrorDraft={returnErrorDraft}
                    pruneObjectsDraft={pruneObjectsDraft}
                    expandedConditions={expandedConditions}
                    setExpandedConditions={setExpandedConditions}
                    updateOperation={updateOperation}
                    duplicateOperation={duplicateOperation}
                    removeOperation={removeOperation}
                    addCondition={addCondition}
                    updateCondition={updateCondition}
                    removeCondition={removeCondition}
                    updateReturnErrorDraft={updateReturnErrorDraft}
                    updatePruneObjectsDraft={updatePruneObjectsDraft}
                    addPruneRule={addPruneRule}
                    updatePruneRule={updatePruneRule}
                    removePruneRule={removePruneRule}
                    expandAllConditions={expandAllConditions}
                    collapseAllConditions={collapseAllConditions}
                  />
                ) : (
                  <div className='flex flex-1 items-center justify-center'>
                    <p className='text-muted-foreground text-sm'>
                      {t('Select a rule to edit.')}
                    </p>
                  </div>
                )}

                {visualValidationError && (
                  <div className='border-t px-4 py-2'>
                    <p className='text-destructive text-xs'>
                      {visualValidationError}
                    </p>
                  </div>
                )}
              </div>
            </div>
          ))}
        {/* JSON mode */}
        {editMode !== 'visual' && (
          <div className='p-4'>
            <div className='mb-2 flex items-center gap-2'>
              <span className='text-muted-foreground text-xs'>
                {t('Advanced text editing')}
              </span>
            </div>
            <JsonCodeEditor
              value={jsonText}
              onChange={handleJsonChange}
              placeholder={JSON.stringify(OPERATION_TEMPLATE, null, 2)}
              heightClassName='h-[420px] min-h-[420px] max-h-[420px]'
              aria-invalid={Boolean(jsonError)}
              ariaLabel={t('Advanced text editing')}
            />
            <p className='text-muted-foreground mt-2 text-xs'>
              {t('Edit JSON text directly. Format will be validated on save.')}
            </p>
            {jsonError && (
              <p className='text-destructive mt-1 text-xs'>{jsonError}</p>
            )}
          </div>
        )}
      </div>
      {/* Footer */}
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// RuleEditor sub-component
// ---------------------------------------------------------------------------

type RuleEditorProps = {
  operation: ParamOverrideOperation
  operationIndex: number
  operations: ParamOverrideOperation[]
  returnErrorDraft: ReturnErrorDraft | null
  pruneObjectsDraft: PruneObjectsDraft | null
  expandedConditions: Record<string, boolean>
  setExpandedConditions: React.Dispatch<
    React.SetStateAction<Record<string, boolean>>
  >
  updateOperation: (
    operationId: string,
    patch: Partial<ParamOverrideOperation>
  ) => void
  duplicateOperation: (operationId: string) => void
  removeOperation: (operationId: string) => void
  addCondition: (operationId: string) => void
  updateCondition: (
    operationId: string,
    conditionId: string,
    patch: Partial<ParamOverrideCondition>
  ) => void
  removeCondition: (operationId: string, conditionId: string) => void
  updateReturnErrorDraft: (
    operationId: string,
    draftPatch: Partial<ReturnErrorDraft>
  ) => void
  updatePruneObjectsDraft: (
    operationId: string,
    updater:
      | Partial<PruneObjectsDraft>
      | ((draft: PruneObjectsDraft) => PruneObjectsDraft)
  ) => void
  addPruneRule: (operationId: string) => void
  updatePruneRule: (
    operationId: string,
    ruleId: string,
    patch: Partial<PruneRule>
  ) => void
  removePruneRule: (operationId: string, ruleId: string) => void
  expandAllConditions: () => void
  collapseAllConditions: () => void
}

function RuleEditor(ruleEditorProps: RuleEditorProps) {
  const { t } = useTranslation()
  const operation = ruleEditorProps.operation
  const mode = operation.mode || 'set'
  const returnErrorDraft =
    mode === 'return_error' ? ruleEditorProps.returnErrorDraft : null
  const pruneObjectsDraft =
    mode === 'prune_objects' ? ruleEditorProps.pruneObjectsDraft : null
  const meta = MODE_META[mode] || MODE_META.set
  const conditions = operation.conditions
  const syncFromTarget =
    mode === 'sync_fields' ? parseSyncTargetSpec(operation.from) : null
  const syncToTarget =
    mode === 'sync_fields' ? parseSyncTargetSpec(operation.to) : null

  return (
    <ScrollArea className='flex-1'>
      <div className='space-y-4 p-4'>
        {/* Header */}
        <div className='flex items-center justify-between'>
          <div className='flex items-center gap-2'>
            <Badge variant='outline'>
              #{ruleEditorProps.operationIndex + 1}
            </Badge>
            <span className='text-muted-foreground line-clamp-1 text-xs'>
              {getOperationSummary(operation, ruleEditorProps.operationIndex)}
            </span>
          </div>
          <div className='flex items-center gap-1'>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => ruleEditorProps.duplicateOperation(operation.id)}
            >
              <Copy className='mr-1 h-3.5 w-3.5' />
              {t('Duplicate')}
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='text-destructive hover:text-destructive'
              onClick={() => ruleEditorProps.removeOperation(operation.id)}
            >
              <Trash2 className='mr-1 h-3.5 w-3.5' />
              {t('Delete')}
            </Button>
          </div>
        </div>

        {/* Operation type + path */}
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='space-y-1.5'>
            <label className='text-xs font-medium'>{t('Operation Type')}</label>
            <Combobox
              options={OPERATION_MODE_OPTIONS.map((o) => ({
                value: o.value,
                label: t(o.label),
              }))}
              value={mode}
              onValueChange={(nextMode) =>
                nextMode !== null &&
                ruleEditorProps.updateOperation(operation.id, {
                  mode: nextMode,
                })
              }
              className='h-9'
            />
          </div>
          {(meta.path || meta.pathOptional) && (
            <div className='space-y-1.5'>
              <label className='text-xs font-medium'>
                {t(getModePathLabel(mode))}
              </label>
              <Input
                value={operation.path}
                onChange={(e) =>
                  ruleEditorProps.updateOperation(operation.id, {
                    path: e.target.value,
                  })
                }
                placeholder={getModePathPlaceholder(mode)}
                className='h-9'
              />
            </div>
          )}
        </div>

        {/* Mode description */}
        {MODE_DESCRIPTIONS[mode] && (
          <p className='text-muted-foreground text-xs'>
            {t(MODE_DESCRIPTIONS[mode])}
          </p>
        )}

        {/* Description */}
        <div className='space-y-1.5'>
          <div className='flex items-center justify-between'>
            <label className='text-xs font-medium'>
              {t('Rule Description (optional)')}
            </label>
            <span className='text-muted-foreground text-[10px]'>
              {operation.description.length}/180
            </span>
          </div>
          <Input
            value={operation.description}
            onChange={(e) =>
              ruleEditorProps.updateOperation(operation.id, {
                description: e.target.value,
              })
            }
            placeholder={t(
              'e.g. Clean tool parameters to avoid upstream validation errors'
            )}
            maxLength={180}
            className='h-9'
          />
        </div>

        {/* Value section */}
        {meta.value && (
          <>
            {returnErrorDraft && (
              <ReturnErrorEditor
                operationId={operation.id}
                draft={returnErrorDraft}
                updateDraft={ruleEditorProps.updateReturnErrorDraft}
              />
            )}
            {pruneObjectsDraft && (
              <PruneObjectsEditor
                operationId={operation.id}
                draft={pruneObjectsDraft}
                updateDraft={ruleEditorProps.updatePruneObjectsDraft}
                addRule={ruleEditorProps.addPruneRule}
                updateRule={ruleEditorProps.updatePruneRule}
                removeRule={ruleEditorProps.removePruneRule}
              />
            )}
            {!returnErrorDraft && !pruneObjectsDraft && (
              <div className='space-y-1.5'>
                <div className='flex items-center justify-between'>
                  <label className='text-xs font-medium'>
                    {t(getModeValueLabel(mode))}
                  </label>
                  {operation.value_text.trim().startsWith('{') && (
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      className='text-muted-foreground h-auto px-1.5 py-0.5 text-xs'
                      onClick={() => {
                        try {
                          const parsed = JSON.parse(operation.value_text)
                          ruleEditorProps.updateOperation(operation.id, {
                            value_text: JSON.stringify(parsed, null, 2),
                          })
                        } catch {
                          /* not valid JSON */
                        }
                      }}
                    >
                      {t('Format')}
                    </Button>
                  )}
                </div>
                <Textarea
                  value={operation.value_text}
                  onChange={(e) =>
                    ruleEditorProps.updateOperation(operation.id, {
                      value_text: e.target.value,
                    })
                  }
                  placeholder={getModeValuePlaceholder(mode)}
                  rows={3}
                  className='max-h-[200px] resize-y overflow-y-auto font-mono text-xs'
                />
              </div>
            )}
          </>
        )}

        {/* keep_origin */}
        {meta.keepOrigin && (
          <div className='flex items-center justify-between rounded-lg border px-3 py-2'>
            <p className='text-sm font-medium'>
              {t('Keep original value (skip if target exists)')}
            </p>
            <Switch
              checked={operation.keep_origin}
              onCheckedChange={(checked) =>
                ruleEditorProps.updateOperation(operation.id, {
                  keep_origin: checked,
                })
              }
            />
          </div>
        )}

        {/* sync_fields */}
        {mode === 'sync_fields' && syncFromTarget && syncToTarget && (
          <SyncFieldsEditor
            operationId={operation.id}
            syncFromTarget={syncFromTarget}
            syncToTarget={syncToTarget}
            updateOperation={ruleEditorProps.updateOperation}
          />
        )}
        {(meta.from || meta.to !== undefined) && mode !== 'sync_fields' && (
          <div className='grid gap-3 sm:grid-cols-2'>
            {(meta.from || meta.to === false) && (
              <div className='space-y-1.5'>
                <label className='text-xs font-medium'>
                  {t(getModeFromLabel(mode))}
                </label>
                <Input
                  value={operation.from}
                  onChange={(e) =>
                    ruleEditorProps.updateOperation(operation.id, {
                      from: e.target.value,
                    })
                  }
                  placeholder={getModeFromPlaceholder(mode)}
                  className='h-9'
                />
              </div>
            )}
            {(meta.to || meta.to === false) && (
              <div className='space-y-1.5'>
                <label className='text-xs font-medium'>
                  {t(getModeToLabel(mode))}
                </label>
                <Input
                  value={operation.to}
                  onChange={(e) =>
                    ruleEditorProps.updateOperation(operation.id, {
                      to: e.target.value,
                    })
                  }
                  placeholder={getModeToPlaceholder(mode)}
                  className='h-9'
                />
              </div>
            )}
          </div>
        )}

        {/* Conditions */}
        <div className='rounded-lg border p-3'>
          <div className='mb-2 flex items-center justify-between'>
            <div className='flex items-center gap-2'>
              <span className='text-sm font-medium'>{t('Conditions')}</span>
              <Select
                items={[
                  { value: 'OR', label: t('Match Any (OR)') },
                  { value: 'AND', label: t('Match All (AND)') },
                ]}
                value={operation.logic || 'OR'}
                onValueChange={(v) =>
                  v !== null &&
                  ruleEditorProps.updateOperation(operation.id, {
                    logic: v,
                  })
                }
              >
                <SelectTrigger className='h-7 w-[120px] text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='OR'>{t('Match Any (OR)')}</SelectItem>
                    <SelectItem value='AND'>{t('Match All (AND)')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className='flex items-center gap-1'>
              {conditions.length > 0 && (
                <>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-7 text-xs'
                    onClick={ruleEditorProps.expandAllConditions}
                  >
                    <ChevronDown className='mr-1 h-3 w-3' />
                    {t('Expand All')}
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-7 text-xs'
                    onClick={ruleEditorProps.collapseAllConditions}
                  >
                    <ChevronUp className='mr-1 h-3 w-3' />
                    {t('Collapse All')}
                  </Button>
                </>
              )}
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 text-xs'
                onClick={() => ruleEditorProps.addCondition(operation.id)}
              >
                <Plus className='mr-1 h-3 w-3' />
                {t('Add Condition')}
              </Button>
            </div>
          </div>

          {conditions.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('When no conditions are set, the operation always executes.')}
            </p>
          ) : (
            <div className='space-y-2'>
              {conditions.map((condition, conditionIndex) => (
                <ConditionEditor
                  key={condition.id}
                  condition={condition}
                  conditionIndex={conditionIndex}
                  operationId={operation.id}
                  expanded={
                    ruleEditorProps.expandedConditions[condition.id] ?? false
                  }
                  onExpandedChange={(expanded) =>
                    ruleEditorProps.setExpandedConditions((prev) => ({
                      ...prev,
                      [condition.id]: expanded,
                    }))
                  }
                  updateCondition={ruleEditorProps.updateCondition}
                  removeCondition={ruleEditorProps.removeCondition}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </ScrollArea>
  )
}

// ---------------------------------------------------------------------------
// ConditionEditor
// ---------------------------------------------------------------------------

type ConditionEditorProps = {
  condition: ParamOverrideCondition
  conditionIndex: number
  operationId: string
  expanded: boolean
  onExpandedChange: (expanded: boolean) => void
  updateCondition: (
    operationId: string,
    conditionId: string,
    patch: Partial<ParamOverrideCondition>
  ) => void
  removeCondition: (operationId: string, conditionId: string) => void
}

function ConditionEditor(conditionEditorProps: ConditionEditorProps) {
  const { t } = useTranslation()
  const condition = conditionEditorProps.condition

  return (
    <Collapsible
      open={conditionEditorProps.expanded}
      onOpenChange={conditionEditorProps.onExpandedChange}
    >
      <div className='rounded-md border'>
        <CollapsibleTrigger className='hover:bg-muted/50 flex w-full items-center justify-between px-3 py-2'>
          <div className='flex items-center gap-2'>
            <Badge variant='outline' className='text-[10px]'>
              C{conditionEditorProps.conditionIndex + 1}
            </Badge>
            <span className='text-muted-foreground text-xs'>
              {condition.path || t('Path not set')}
            </span>
          </div>
          {conditionEditorProps.expanded ? (
            <ChevronUp className='text-muted-foreground h-3.5 w-3.5' />
          ) : (
            <ChevronDown className='text-muted-foreground h-3.5 w-3.5' />
          )}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className='space-y-3 border-t px-3 py-3'>
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-xs'>
                {t('Condition Settings')}
              </span>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='text-destructive hover:text-destructive h-7 text-xs'
                onClick={() =>
                  conditionEditorProps.removeCondition(
                    conditionEditorProps.operationId,
                    condition.id
                  )
                }
              >
                <Trash2 className='mr-1 h-3 w-3' />
                {t('Delete Condition')}
              </Button>
            </div>
            <div className='grid gap-2 sm:grid-cols-3'>
              <div className='space-y-1'>
                <label className='text-[10px] font-medium'>
                  {t('Field Path')}
                </label>
                <Input
                  value={condition.path}
                  onChange={(e) =>
                    conditionEditorProps.updateCondition(
                      conditionEditorProps.operationId,
                      condition.id,
                      { path: e.target.value }
                    )
                  }
                  placeholder='model'
                  className='h-8 text-xs'
                />
              </div>
              <div className='space-y-1'>
                <label className='text-[10px] font-medium'>
                  {t('Match Mode')}
                </label>
                <Combobox
                  options={CONDITION_MODE_OPTIONS.map((o) => ({
                    value: o.value,
                    label: t(o.label),
                  }))}
                  value={condition.mode}
                  onValueChange={(v) =>
                    v !== null &&
                    conditionEditorProps.updateCondition(
                      conditionEditorProps.operationId,
                      condition.id,
                      { mode: v }
                    )
                  }
                  className='h-8 text-xs'
                />
              </div>
              <div className='space-y-1'>
                <label className='text-[10px] font-medium'>
                  {t('Match Value')}
                </label>
                <Input
                  value={condition.value_text}
                  onChange={(e) =>
                    conditionEditorProps.updateCondition(
                      conditionEditorProps.operationId,
                      condition.id,
                      { value_text: e.target.value }
                    )
                  }
                  placeholder='gpt'
                  className='h-8 text-xs'
                />
              </div>
            </div>
            <div className='flex flex-wrap gap-4'>
              <label className='flex items-center gap-2 text-xs'>
                <Switch
                  checked={condition.invert}
                  onCheckedChange={(checked) =>
                    conditionEditorProps.updateCondition(
                      conditionEditorProps.operationId,
                      condition.id,
                      { invert: checked }
                    )
                  }
                />
                {t('Invert match')}
              </label>
              <label className='flex items-center gap-2 text-xs'>
                <Switch
                  checked={condition.pass_missing_key}
                  onCheckedChange={(checked) =>
                    conditionEditorProps.updateCondition(
                      conditionEditorProps.operationId,
                      condition.id,
                      { pass_missing_key: checked }
                    )
                  }
                />
                {t('Pass when key is missing')}
              </label>
            </div>
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

// ---------------------------------------------------------------------------
// ReturnErrorEditor
// ---------------------------------------------------------------------------

type ReturnErrorEditorProps = {
  operationId: string
  draft: ReturnErrorDraft
  updateDraft: (
    operationId: string,
    draftPatch: Partial<ReturnErrorDraft>
  ) => void
}

function ReturnErrorEditor(returnErrorEditorProps: ReturnErrorEditorProps) {
  const { t } = useTranslation()
  const draft = returnErrorEditorProps.draft

  return (
    <div className='rounded-lg border p-3'>
      <div className='mb-2 flex items-center justify-between'>
        <span className='text-sm font-medium'>
          {t('Custom Error Response')}
        </span>
        <div className='flex items-center gap-1'>
          <span className='text-muted-foreground text-xs'>{t('Mode')}</span>
          <Button
            type='button'
            variant={draft.simpleMode ? 'default' : 'outline'}
            size='sm'
            className='h-7 text-xs'
            onClick={() =>
              returnErrorEditorProps.updateDraft(
                returnErrorEditorProps.operationId,
                { simpleMode: true }
              )
            }
          >
            {t('Simple')}
          </Button>
          <Button
            type='button'
            variant={draft.simpleMode ? 'outline' : 'default'}
            size='sm'
            className='h-7 text-xs'
            onClick={() =>
              returnErrorEditorProps.updateDraft(
                returnErrorEditorProps.operationId,
                { simpleMode: false }
              )
            }
          >
            {t('Advanced')}
          </Button>
        </div>
      </div>

      <div className='space-y-1.5'>
        <Label required className='text-xs font-medium'>
          {t('Error Message')}
        </Label>
        <Textarea
          value={draft.message}
          onChange={(e) =>
            returnErrorEditorProps.updateDraft(
              returnErrorEditorProps.operationId,
              { message: e.target.value }
            )
          }
          placeholder={t('e.g. This request does not meet access policy')}
          rows={2}
          className='text-xs'
        />
      </div>

      {draft.simpleMode ? (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t(
            'Simple mode only returns message; status code and error type use system defaults.'
          )}
        </p>
      ) : (
        <>
          <div className='mt-3 grid gap-3 sm:grid-cols-3'>
            <div className='space-y-1'>
              <label className='text-xs font-medium'>{t('Status Code')}</label>
              <Input
                value={String(draft.statusCode ?? '')}
                onChange={(e) =>
                  returnErrorEditorProps.updateDraft(
                    returnErrorEditorProps.operationId,
                    { statusCode: Number.parseInt(e.target.value, 10) || 400 }
                  )
                }
                placeholder='400'
                className='h-8 text-xs'
              />
            </div>
            <div className='space-y-1'>
              <label className='text-xs font-medium'>
                {t('Error Code (optional)')}
              </label>
              <Input
                value={draft.code}
                onChange={(e) =>
                  returnErrorEditorProps.updateDraft(
                    returnErrorEditorProps.operationId,
                    { code: e.target.value }
                  )
                }
                placeholder='forced_bad_request'
                className='h-8 text-xs'
              />
            </div>
            <div className='space-y-1'>
              <label className='text-xs font-medium'>
                {t('Error Type (optional)')}
              </label>
              <Input
                value={draft.type}
                onChange={(e) =>
                  returnErrorEditorProps.updateDraft(
                    returnErrorEditorProps.operationId,
                    { type: e.target.value }
                  )
                }
                placeholder='invalid_request_error'
                className='h-8 text-xs'
              />
            </div>
          </div>
          <div className='mt-2 flex items-center gap-2'>
            <span className='text-muted-foreground text-xs'>
              {t('Retry Suggestion')}
            </span>
            <Button
              type='button'
              variant={draft.skipRetry ? 'default' : 'outline'}
              size='sm'
              className='h-7 text-xs'
              onClick={() =>
                returnErrorEditorProps.updateDraft(
                  returnErrorEditorProps.operationId,
                  { skipRetry: true }
                )
              }
            >
              {t('Stop Retry')}
            </Button>
            <Button
              type='button'
              variant={draft.skipRetry ? 'outline' : 'default'}
              size='sm'
              className='h-7 text-xs'
              onClick={() =>
                returnErrorEditorProps.updateDraft(
                  returnErrorEditorProps.operationId,
                  { skipRetry: false }
                )
              }
            >
              {t('Allow Retry')}
            </Button>
          </div>
          <div className='mt-2 flex flex-wrap gap-1'>
            {[
              {
                label: 'Bad Request',
                statusCode: 400,
                code: 'invalid_request',
                type: 'invalid_request_error',
              },
              {
                label: 'Unauthorized',
                statusCode: 401,
                code: 'unauthorized',
                type: 'authentication_error',
              },
              {
                label: 'Rate Limited',
                statusCode: 429,
                code: 'rate_limited',
                type: 'rate_limit_error',
              },
            ].map((preset) => (
              <Button
                key={preset.code}
                type='button'
                variant='outline'
                size='sm'
                className='h-6 text-[10px]'
                onClick={() =>
                  returnErrorEditorProps.updateDraft(
                    returnErrorEditorProps.operationId,
                    {
                      statusCode: preset.statusCode,
                      code: preset.code,
                      type: preset.type,
                    }
                  )
                }
              >
                {t(preset.label)}
              </Button>
            ))}
          </div>
        </>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// PruneObjectsEditor
// ---------------------------------------------------------------------------

type PruneObjectsEditorProps = {
  operationId: string
  draft: PruneObjectsDraft
  updateDraft: (
    operationId: string,
    updater:
      | Partial<PruneObjectsDraft>
      | ((draft: PruneObjectsDraft) => PruneObjectsDraft)
  ) => void
  addRule: (operationId: string) => void
  updateRule: (
    operationId: string,
    ruleId: string,
    patch: Partial<PruneRule>
  ) => void
  removeRule: (operationId: string, ruleId: string) => void
}

function PruneObjectsEditor(pruneObjectsEditorProps: PruneObjectsEditorProps) {
  const { t } = useTranslation()
  const draft = pruneObjectsEditorProps.draft

  return (
    <div className='rounded-lg border p-3'>
      <div className='mb-2 flex items-center justify-between'>
        <span className='text-sm font-medium'>{t('Object Prune Rules')}</span>
        <div className='flex items-center gap-1'>
          <span className='text-muted-foreground text-xs'>{t('Mode')}</span>
          <Button
            type='button'
            variant={draft.simpleMode ? 'default' : 'outline'}
            size='sm'
            className='h-7 text-xs'
            onClick={() =>
              pruneObjectsEditorProps.updateDraft(
                pruneObjectsEditorProps.operationId,
                { simpleMode: true }
              )
            }
          >
            {t('Simple')}
          </Button>
          <Button
            type='button'
            variant={draft.simpleMode ? 'outline' : 'default'}
            size='sm'
            className='h-7 text-xs'
            onClick={() =>
              pruneObjectsEditorProps.updateDraft(
                pruneObjectsEditorProps.operationId,
                { simpleMode: false }
              )
            }
          >
            {t('Advanced')}
          </Button>
        </div>
      </div>

      <div className='space-y-1.5'>
        <label className='text-xs font-medium'>{t('Type (common)')}</label>
        <Input
          value={draft.typeText}
          onChange={(e) =>
            pruneObjectsEditorProps.updateDraft(
              pruneObjectsEditorProps.operationId,
              { typeText: e.target.value }
            )
          }
          placeholder='redacted_thinking'
          className='h-8 text-xs'
        />
      </div>

      {draft.simpleMode ? (
        <p className='text-muted-foreground mt-2 text-xs'>
          {t('Simple mode: prune objects by type, e.g. redacted_thinking.')}
        </p>
      ) : (
        <>
          <div className='mt-3 grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1'>
              <label className='text-xs font-medium'>{t('Logic')}</label>
              <Select
                items={[
                  { value: 'AND', label: t('All Must Match (AND)') },
                  { value: 'OR', label: t('Any Match (OR)') },
                ]}
                value={draft.logic}
                onValueChange={(v) =>
                  pruneObjectsEditorProps.updateDraft(
                    pruneObjectsEditorProps.operationId,
                    { logic: v || 'AND' }
                  )
                }
              >
                <SelectTrigger className='h-8 text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='AND'>
                      {t('All Must Match (AND)')}
                    </SelectItem>
                    <SelectItem value='OR'>{t('Any Match (OR)')}</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className='space-y-1'>
              <label className='text-xs font-medium'>
                {t('Recursion Strategy')}
              </label>
              <div className='flex gap-1'>
                <Button
                  type='button'
                  variant={draft.recursive ? 'default' : 'outline'}
                  size='sm'
                  className='h-8 text-xs'
                  onClick={() =>
                    pruneObjectsEditorProps.updateDraft(
                      pruneObjectsEditorProps.operationId,
                      { recursive: true }
                    )
                  }
                >
                  {t('Recursive')}
                </Button>
                <Button
                  type='button'
                  variant={draft.recursive ? 'outline' : 'default'}
                  size='sm'
                  className='h-8 text-xs'
                  onClick={() =>
                    pruneObjectsEditorProps.updateDraft(
                      pruneObjectsEditorProps.operationId,
                      { recursive: false }
                    )
                  }
                >
                  {t('Current Level Only')}
                </Button>
              </div>
            </div>
          </div>

          <div className='bg-muted/30 mt-3 rounded-md border p-2'>
            <div className='mb-2 flex items-center justify-between'>
              <span className='text-xs font-medium'>
                {t('Additional Conditions')}
              </span>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 text-xs'
                onClick={() =>
                  pruneObjectsEditorProps.addRule(
                    pruneObjectsEditorProps.operationId
                  )
                }
              >
                <Plus className='mr-1 h-3 w-3' />
                {t('Add Condition')}
              </Button>
            </div>
            {draft.rules.length === 0 ? (
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Without additional conditions, only the type above is used for pruning.'
                )}
              </p>
            ) : (
              <div className='space-y-2'>
                {draft.rules.map((rule, ruleIndex) => (
                  <div
                    key={rule.id}
                    className='bg-background rounded-md border p-2'
                  >
                    <div className='mb-1 flex items-center justify-between'>
                      <Badge variant='outline' className='text-[10px]'>
                        R{ruleIndex + 1}
                      </Badge>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        className='text-destructive hover:text-destructive h-6 text-[10px]'
                        onClick={() =>
                          pruneObjectsEditorProps.removeRule(
                            pruneObjectsEditorProps.operationId,
                            rule.id
                          )
                        }
                      >
                        <Trash2 className='mr-1 h-3 w-3' />
                        {t('Delete')}
                      </Button>
                    </div>
                    <div className='grid gap-2 sm:grid-cols-3'>
                      <div className='space-y-0.5'>
                        <label className='text-[10px] font-medium'>
                          {t('Field Path')}
                        </label>
                        <Input
                          value={rule.path}
                          onChange={(e) =>
                            pruneObjectsEditorProps.updateRule(
                              pruneObjectsEditorProps.operationId,
                              rule.id,
                              { path: e.target.value }
                            )
                          }
                          placeholder='type'
                          className='h-7 text-xs'
                        />
                      </div>
                      <div className='space-y-0.5'>
                        <label className='text-[10px] font-medium'>
                          {t('Match Mode')}
                        </label>
                        <Combobox
                          options={CONDITION_MODE_OPTIONS.map((o) => ({
                            value: o.value,
                            label: t(o.label),
                          }))}
                          value={rule.mode}
                          onValueChange={(v) =>
                            v !== null &&
                            pruneObjectsEditorProps.updateRule(
                              pruneObjectsEditorProps.operationId,
                              rule.id,
                              { mode: v }
                            )
                          }
                          className='h-7 text-xs'
                        />
                      </div>
                      <div className='space-y-0.5'>
                        <label className='text-[10px] font-medium'>
                          {t('Match Value (optional)')}
                        </label>
                        <Input
                          value={rule.value_text}
                          onChange={(e) =>
                            pruneObjectsEditorProps.updateRule(
                              pruneObjectsEditorProps.operationId,
                              rule.id,
                              { value_text: e.target.value }
                            )
                          }
                          placeholder='redacted_thinking'
                          className='h-7 text-xs'
                        />
                      </div>
                    </div>
                    <div className='mt-1.5 flex flex-wrap gap-3'>
                      <label className='flex items-center gap-1.5 text-[10px]'>
                        <Switch
                          checked={rule.invert}
                          onCheckedChange={(checked) =>
                            pruneObjectsEditorProps.updateRule(
                              pruneObjectsEditorProps.operationId,
                              rule.id,
                              { invert: checked }
                            )
                          }
                        />
                        {t('Invert match')}
                      </label>
                      <label className='flex items-center gap-1.5 text-[10px]'>
                        <Switch
                          checked={rule.pass_missing_key}
                          onCheckedChange={(checked) =>
                            pruneObjectsEditorProps.updateRule(
                              pruneObjectsEditorProps.operationId,
                              rule.id,
                              { pass_missing_key: checked }
                            )
                          }
                        />
                        {t('Pass when key is missing')}
                      </label>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// SyncFieldsEditor
// ---------------------------------------------------------------------------

type SyncFieldsEditorProps = {
  operationId: string
  syncFromTarget: { type: string; key: string }
  syncToTarget: { type: string; key: string }
  updateOperation: (
    operationId: string,
    patch: Partial<ParamOverrideOperation>
  ) => void
}

function SyncFieldsEditor(syncFieldsEditorProps: SyncFieldsEditorProps) {
  const { t } = useTranslation()
  return (
    <div className='space-y-3'>
      <label className='text-xs font-medium'>{t('Sync Endpoints')}</label>
      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='space-y-1.5'>
          <label className='text-[10px] font-medium'>
            {t('Source Endpoint')}
          </label>
          <div className='flex gap-2'>
            <Select
              items={SYNC_TARGET_TYPE_OPTIONS.map((o) => ({
                value: o.value,
                label: t(o.label),
              }))}
              value={syncFieldsEditorProps.syncFromTarget.type || 'json'}
              onValueChange={(v) =>
                v !== null &&
                syncFieldsEditorProps.updateOperation(
                  syncFieldsEditorProps.operationId,
                  {
                    from: buildSyncTargetSpec(
                      v,
                      syncFieldsEditorProps.syncFromTarget.key
                    ),
                  }
                )
              }
            >
              <SelectTrigger className='h-8 w-[110px] text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {SYNC_TARGET_TYPE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {t(o.label)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              value={syncFieldsEditorProps.syncFromTarget.key}
              onChange={(e) =>
                syncFieldsEditorProps.updateOperation(
                  syncFieldsEditorProps.operationId,
                  {
                    from: buildSyncTargetSpec(
                      syncFieldsEditorProps.syncFromTarget.type,
                      e.target.value
                    ),
                  }
                )
              }
              placeholder='session_id'
              className='h-8 text-xs'
            />
          </div>
        </div>
        <div className='space-y-1.5'>
          <label className='text-[10px] font-medium'>
            {t('Target Endpoint')}
          </label>
          <div className='flex gap-2'>
            <Select
              items={SYNC_TARGET_TYPE_OPTIONS.map((o) => ({
                value: o.value,
                label: t(o.label),
              }))}
              value={syncFieldsEditorProps.syncToTarget.type || 'json'}
              onValueChange={(v) =>
                v !== null &&
                syncFieldsEditorProps.updateOperation(
                  syncFieldsEditorProps.operationId,
                  {
                    to: buildSyncTargetSpec(
                      v,
                      syncFieldsEditorProps.syncToTarget.key
                    ),
                  }
                )
              }
            >
              <SelectTrigger className='h-8 w-[110px] text-xs'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {SYNC_TARGET_TYPE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {t(o.label)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              value={syncFieldsEditorProps.syncToTarget.key}
              onChange={(e) =>
                syncFieldsEditorProps.updateOperation(
                  syncFieldsEditorProps.operationId,
                  {
                    to: buildSyncTargetSpec(
                      syncFieldsEditorProps.syncToTarget.type,
                      e.target.value
                    ),
                  }
                )
              }
              placeholder='prompt_cache_key'
              className='h-8 text-xs'
            />
          </div>
        </div>
      </div>
      <div className='flex flex-wrap gap-1'>
        {[
          {
            label: 'header:session_id -> json:prompt_cache_key',
            from: 'header:session_id',
            to: 'json:prompt_cache_key',
          },
          {
            label: 'json:prompt_cache_key -> header:session_id',
            from: 'json:prompt_cache_key',
            to: 'header:session_id',
          },
        ].map((preset) => (
          <Button
            key={preset.label}
            type='button'
            variant='outline'
            size='sm'
            className='h-6 text-[10px]'
            onClick={() =>
              syncFieldsEditorProps.updateOperation(
                syncFieldsEditorProps.operationId,
                { from: preset.from, to: preset.to }
              )
            }
          >
            {preset.label}
          </Button>
        ))}
      </div>
    </div>
  )
}
