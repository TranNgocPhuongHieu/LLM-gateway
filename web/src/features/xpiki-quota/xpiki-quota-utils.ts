import type { XPikiQuotaKeyResult, XPikiQuotaReport } from './api'
import { formatPoolTokens, referenceTokenPools } from './token-pools'
import { formatXPikiVND } from './token-cost'

export const STORAGE_KEY_CACHE = 'newapi_xpiki_quota_cache_actual_tariffs_v6'
export const STORAGE_KEY_CUSTOM_KEYS = 'newapi_xpiki_quota_custom_keys_v1'
export const STORAGE_KEY_LOGS = 'newapi_xpiki_logs_preferences_v1'

export type CachedQuotaData = {
  report: XPikiQuotaReport
  timestamp: number
}

export type QuotaTableRow = {
  id: string
  key: string
  rawKey: XPikiQuotaKeyResult
  usedVnd: string
  usedAstra: string
  limitVnd: string
  limitAstra: string
  remainingVnd: string
  remainingAstra: string
  usedPercent: string
  statusText: string
  isTotal: boolean
}

export function quotaRows(
  report: XPikiQuotaReport,
  translate: (value: string) => string
): QuotaTableRow[] {
  const rows: QuotaTableRow[] = report.keys.map((key, index) => ({
    id: `key-${index + 1}`,
    key: key.name ? `${key.label} (${key.name})` : key.label,
    rawKey: key,
    usedVnd: formatXPikiVND(key.used_vnd),
    usedAstra: key.used_tokens || '—',
    limitVnd: key.unlimited ? translate('Unlimited') : formatXPikiVND(key.limit_vnd),
    limitAstra: key.unlimited ? translate('Unlimited') : (key.limit_tokens || '—'),
    remainingVnd: key.unlimited ? translate('Unlimited') : formatXPikiVND(key.remaining_vnd),
    remainingAstra: key.unlimited ? translate('Unlimited') : (key.remaining_tokens || '—'),
    usedPercent: key.used_percent,
    statusText: getStatusLabel(key, translate),
    isTotal: false,
  }))

  for (const pool of referenceTokenPools(report)) {
    rows.push({
      id: `total-${pool.model}`, key: `${translate('TOTAL')} ${pool.label}`, rawKey: { unlimited: pool.unlimited } as XPikiQuotaKeyResult,
      usedVnd: '', usedAstra: formatPoolTokens(pool.used), limitVnd: '',
      limitAstra: pool.unlimited ? translate('Unlimited') : formatPoolTokens(pool.limit),
      remainingVnd: '', remainingAstra: pool.unlimited ? translate('Unlimited') : formatPoolTokens(pool.remaining),
      usedPercent: '', statusText: '', isTotal: true,
    })
  }

  rows.push({
    id: 'total',
    key: `${translate('TOTAL')} credit`,
    rawKey: {} as XPikiQuotaKeyResult,
    usedVnd: '',
    usedAstra: '—',
    limitVnd: '',
    limitAstra: '—',
    remainingVnd: report.total_unlimited
      ? translate('Unlimited')
      : formatXPikiVND(report.total_remaining_vnd),
    remainingAstra: '—',
    usedPercent: '',
    statusText: '',
    isTotal: true,
  })
  return rows
}

export function getStatusLabel(
  key: XPikiQuotaKeyResult,
  translate: (value: string) => string
): string {
  if (key.error) return key.error
  if (!key.status || key.status.length === 0) return translate('OK')
  return key.status
    .map((s) => {
      switch (s) {
        case 'disabled':
          return translate('Disabled')
        case 'expired':
          return translate('Expired')
        case 'quota_exhausted':
          return translate('Quota exhausted')
        default:
          return translate('OK')
      }
    })
    .join(', ')
}

