import { z } from 'zod'
import type { XPikiQuotaModelRow } from './api'

const creditNumber = new Intl.NumberFormat('vi-VN', { maximumFractionDigits: 8 })
function formatVndAmountToCredit(value: string): string {
  const amount = Number(value)
  return Number.isFinite(amount) ? `${creditNumber.format(amount / 1000)} credit` : '—'
}

export function actualTariffRows(
  models: XPikiQuotaModelRow[]
): XPikiQuotaModelRow[] {
  return models.flatMap((model) => {
    const rows = (model.observed_tariffs || []).map((tariff) => ({
      ...model,
      effective_input_vnd_per_1m: tariff.input_vnd_per_1m,
      effective_output_vnd_per_1m: tariff.output_vnd_per_1m,
      effective_cache_read_vnd_per_1m: tariff.cache_read_vnd_per_1m,
      observed_multiplier: tariff.multiplier,
      verified_samples: tariff.samples,
      verified_at: tariff.latest_at,
    }))
    if (rows.length === 0 || model.unverified_samples > 0) {
      rows.push({
        ...model,
        effective_input_vnd_per_1m: '',
        effective_output_vnd_per_1m: '',
        effective_cache_read_vnd_per_1m: '',
        observed_multiplier: '',
        verified_samples: 0,
        verified_at: '',
      })
    }
    return rows
  })
}

export const xpikiTokenCostSchema = z.object({
  status: z.enum(['reconciled', 'confirmed_zero', 'unverified']),
  reference_model: z.string().optional(),
  multiplier: z.string().optional(),
  input_tokens: z.number(),
  output_tokens: z.number(),
  cache_read_tokens: z.number(),
  cache_write_tokens: z.number(),
  input_vnd_per_1m: z.string().optional(),
  output_vnd_per_1m: z.string().optional(),
  cache_read_vnd_per_1m: z.string().optional(),
  input_vnd: z.string().optional(),
  output_vnd: z.string().optional(),
  cache_read_vnd: z.string().optional(),
  rounding_vnd: z.string().optional(),
  total_vnd: z.string().optional(),
  cost_vnd: z.string().optional(),
  warning: z.string().optional(),
})

export type XPikiTokenCost = z.infer<typeof xpikiTokenCostSchema>

// Decimal strings avoid floating-point rounding at money boundaries.
// Under the 1,000 VND = 1 credit convention, VND amounts are converted to credit (/ 1000).
// Small costs (e.g. 0.01 VND -> 0.00001 $) are preserved without premature rounding to zero.
export function formatLogCost(value?: string): string {
  if (value === undefined || !/^\d+(?:\.\d+)?$/.test(value)) return '—'
  return formatVndAmountToCredit(value)
}

export function formatXPikiVND(value?: string, includeSymbol = true): string {
  if (value === undefined) return '—'
  const rawValue = value
    .trim()
    .replace(/\s*(?:₫|đ|vnd|\$)$/iu, '')
    .trim()
  const match = rawValue.match(/^(\d+|\d{1,3}(?:,\d{3})+)(?:\.(\d+))?$/)
  if (!match) return '—'
  const whole = match[1].replaceAll(',', '')
  const decimal = match[2] ? `${whole}.${match[2]}` : whole
  const result = formatVndAmountToCredit(decimal)
  return includeSymbol ? result : result.replace(/\scredit$/, '')
}

export function tokenCostExplanation(cost?: XPikiTokenCost): string {
  if (!cost || cost.status === 'unverified')
    return cost?.warning || 'Chưa đối soát được giá vốn theo token'
  if (cost.status === 'confirmed_zero') return 'XPiki xác nhận không thu phí'
  const money = (value?: string) =>
    value === undefined
      ? '—'
      : formatVndAmountToCredit(value)
  return [
    `Input không cache: ${cost.input_tokens.toLocaleString('vi-VN')} × ${money(cost.input_vnd_per_1m)} /1M = ${money(cost.input_vnd)}`,
    `Output: ${cost.output_tokens.toLocaleString('vi-VN')} × ${money(cost.output_vnd_per_1m)} /1M = ${money(cost.output_vnd)}`,
    `Cache-read: ${cost.cache_read_tokens.toLocaleString('vi-VN')} × ${money(cost.cache_read_vnd_per_1m)} /1M = ${money(cost.cache_read_vnd)}`,
    `Làm tròn biên lai: ${money(cost.rounding_vnd)}`,
    `Đã bao gồm hệ số XPiki ×${cost.multiplier}; không nhân lại`,
  ].join('\n')
}
