import type { XPikiQuotaReport } from './api'

export function referenceTokenPools(report: Pick<XPikiQuotaReport, 'keys'>) {
  return [
    { model: 'gpt-6-astra', label: 'GPT · Astra' },
    { model: 'claude-opus-5-5', label: 'Claude · Opus 5.5' },
  ].flatMap((pool) => {
    const keys = report.keys.filter((key) => key.reference_model === pool.model)
    if (!keys.length) return []
    const sum = (field: 'used_tokens' | 'limit_tokens' | 'remaining_tokens') => {
      const values = keys.map((key) => {
        const value = key[field]
        return value ? Number(value.replace(/,/g, '')) : NaN
      })
      if (values.some((value) => !Number.isFinite(value))) return null
      return values.reduce((total, value) => total + value, 0)
    }
    return [{ ...pool, used: sum('used_tokens'), limit: sum('limit_tokens'), remaining: sum('remaining_tokens'), unlimited: keys.some((key) => key.unlimited) }]
  })
}

export function formatPoolTokens(value: number | null) {
  return value === null ? '—' : new Intl.NumberFormat('vi-VN', { maximumFractionDigits: 0 }).format(value)
}
