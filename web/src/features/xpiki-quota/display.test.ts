// @vitest-environment jsdom
import { readFileSync } from 'node:fs'
import { fileURLToPath, URL as NodeURL } from 'node:url'
import { QueryClient, QueryObserver, focusManager, onlineManager } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'

// Guard the page's presentation without importing its API/query side effects.
const source = readFileSync(fileURLToPath(new NodeURL('./index.tsx', import.meta.url)), 'utf8')
const modelCard = source.split('function ModelRatioTable(')[1].split('function ManageKeysDialog(')[0]
const logsCard = source.split('function XPikiLogsCard(')[1].split('export function XPikiQuotaPage(')[0]

describe('XPiki page presentation', () => {
  it('starts model price cards collapsed', () => {
    expect(modelCard).toContain('const [isExpanded, setIsExpanded] = useState(false)')
    expect(modelCard).toContain('{isExpanded && (')
  })
  it('keeps one price card and separates historical auditing from balance refresh', () => {
    expect(source).not.toContain('props.report.model_profiles.map((profile) => (')
    expect(source).toContain('<ModelRatioTable profiles={props.report.model_profiles}')
    expect(modelCard).toContain("aria-label={t('Key đối soát giá')}")
    expect(modelCard).toContain('enabled: isExpanded')
    expect(modelCard).toContain('checkXPikiQuota(props.customKeys, true)')
  })
  it('keeps the existing manual show/hide button', () => {
    expect(modelCard).toContain("isExpanded ? t('Hide') : t('Show')")
    expect(modelCard).toContain('e.stopPropagation()')
    expect(modelCard).toContain('setIsExpanded((prev) => !prev)')
  })
  it('does not render an XPiki IP column', () => {
    expect(source).not.toContain("id: 'ip'")
    expect(source).not.toContain('IP tại XPiki')
    expect(source).not.toContain('row.ip')
  })
  it('reads grouped customer requests, polling only while expanded', () => {
    for (const option of ['retry', 'refetchOnWindowFocus', 'refetchOnReconnect', 'refetchIntervalInBackground']) {
      expect(logsCard).toContain(`${option}: false`)
    }
    expect(logsCard).toContain('enabled: isExpanded')
    expect(logsCard).toContain('refetchInterval: isExpanded ? 10000 : false')
    expect(logsCard).toContain("source: 'newapi'")
    expect(logsCard).toContain("group_mode: 'grouped'")
    expect(logsCard).not.toContain("source: 'xpiki'")
  })
  it('shows key identity, username, fallback, tokens, cache and multi-select filters', () => {
    expect(logsCard).toContain('refetch()')
    expect(logsCard).toContain("t('Tải lại log')")
    expect(logsCard).toContain('isLoading: isPending')
    expect(logsCard).toContain('row.token_id')
    // Endpoint, duration and request ID columns were removed on purpose.
    expect(logsCard).not.toContain('row.request_path')
    expect(logsCard).not.toContain('duration_ms,')
    expect(logsCard).not.toContain("id: 'request_id'")
    // The administrator table identifies the requesting user and degrades
    // safely when a legacy row has no username.
    expect(logsCard).toContain("header: t('User')")
    expect(logsCard).toContain('row.username')
    expect(logsCard).toContain("t('Không xác định')")
    expect(logsCard).toContain("t('Fallback')")
    expect(logsCard).toContain("t('Token (input / output)'")
    expect(logsCard).toContain('row.input_tokens.toLocaleString()')
    expect(logsCard).toContain('text-amber-600 dark:text-amber-400')
    expect(logsCard).toContain('formatLogCost(row.token_cost?.total_vnd)')
    // Model and user filters accept several values and send them as a list.
    expect(logsCard).toContain("placeholder={t('Chọn model')}")
    expect(logsCard).toContain("placeholder={t('Chọn người dùng')}")
    expect(logsCard).toContain("userFilter.length ? userFilter.join(',') : undefined")
    expect(logsCard).toContain("modelFilter === 'all' ? undefined : modelFilter")
    expect(logsCard).toContain('userOptions.map((user) => ({ value: user, label: user }))')
    expect(source).not.toContain('token quy đổi ${')
  })

  it('keeps the request-log card out of the quota check page', () => {
    // The card moved to /request-logs so both pages stay independent.
    expect(source).toContain('export function XPikiRequestLogsPage()')
    const quotaPage = source.split('export function XPikiQuotaPage(')[1].split('export function XPikiRequestLogsPage(')[0]
    expect(quotaPage).not.toContain('<XPikiLogsCard')
  })

  it('does not trigger background quota checks or invalidate log queries', () => {
    expect(source).not.toContain('initialCheckStarted')
    expect(source).not.toContain('invalidateQueries')
  })
  it('polls every 10 seconds, fetches on changed filters and stops when collapsed', async () => {
    vi.useFakeTimers()
    const client = new QueryClient()
    client.mount()
    const fetcher = vi.fn(async () => ['log'])
    const options = {
      enabled: /enabled: isExpanded/.test(logsCard),
      retry: false as const,
      refetchInterval: /refetchInterval: isExpanded \? 10000/.test(logsCard) ? 10000 : false as const,
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: !/refetchOnWindowFocus: false/.test(logsCard),
      refetchOnReconnect: !/refetchOnReconnect: false/.test(logsCard),
      refetchOnMount: !/refetchOnMount: false/.test(logsCard),
    }
    const observer = new QueryObserver(client, { ...options, queryKey: ['customer-logs', 1], queryFn: fetcher })
    const unsubscribe = observer.subscribe(() => {})
    try {
      focusManager.setFocused(true)
      onlineManager.setOnline(true)
      await vi.advanceTimersByTimeAsync(1)
      expect(fetcher).toHaveBeenCalledTimes(1)
      expect(observer.getCurrentResult().isLoading).toBe(false)
      await vi.advanceTimersByTimeAsync(9998)
      expect(fetcher).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(2)
      expect(fetcher).toHaveBeenCalledTimes(2)
      await observer.refetch()
      expect(fetcher).toHaveBeenCalledTimes(3)
      observer.setOptions({ ...options, queryKey: ['customer-logs', 2], queryFn: fetcher })
      await vi.advanceTimersByTimeAsync(1)
      expect(fetcher).toHaveBeenCalledTimes(4)
      observer.setOptions({ ...options, enabled: false, refetchInterval: false, queryKey: ['customer-logs', 2], queryFn: fetcher })
      await vi.advanceTimersByTimeAsync(30000)
      expect(fetcher).toHaveBeenCalledTimes(4)
    } finally {
      unsubscribe()
      client.unmount()
      client.clear()
      focusManager.setFocused(undefined)
      onlineManager.setOnline(true)
      vi.useRealTimers()
    }
  })
})
