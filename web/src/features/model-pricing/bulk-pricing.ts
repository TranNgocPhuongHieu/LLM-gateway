export type PriceField = 'inputVnd' | 'outputVnd' | 'cacheReadVnd' | 'cacheWriteVnd'

export type LongContextPricing = {
  threshold: number
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
}

export const defaultLongContextPricing: LongContextPricing = {
  threshold: 272000, input: 2, output: 2, cacheRead: 2, cacheWrite: 2,
}

const numberPattern = String.raw`(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?`
const priceTerm = new RegExp(String.raw`\b(p|c|cr|cc|cc1h)\s*\*\s*(${numberPattern})`, 'g')
const basePattern = new RegExp(String.raw`^tier\("base",\s*((?:p|c|cr|cc|cc1h)\s*\*\s*${numberPattern}(?:\s*\+\s*(?:p|c|cr|cc|cc1h)\s*\*\s*${numberPattern})*)\)$`)
const multiplierFields = { p: 'input', c: 'output', cr: 'cacheRead', cc: 'cacheWrite', cc1h: 'cacheWrite' } as const

export function validateLongContextPricing(policy: LongContextPricing): void {
  if (!Number.isSafeInteger(policy.threshold) || policy.threshold <= 0 ||
      [policy.input, policy.output, policy.cacheRead, policy.cacheWrite].some(value => !Number.isFinite(value) || value < 1)) {
    throw new Error('Invalid long context pricing')
  }
}

export function canConfigureLongContext(expression: string): boolean {
  return basePattern.test(expression.trim().replace(/^v1:/, '').trim())
}

export function applyLongContextPricing(base: string, policy: LongContextPricing): string {
  validateLongContextPricing(policy)
  const unversioned = base.trim().replace(/^v1:/, '').trim()
  if (!canConfigureLongContext(unversioned)) throw new Error('Use the advanced editor for custom expressions')
  const body = unversioned.match(basePattern)![1]
  const longBody = body.replace(priceTerm, (_term, variable: keyof typeof multiplierFields, coefficient: string) => {
    const value = Number(coefficient) * policy[multiplierFields[variable]]
    if (!Number.isFinite(value)) throw new Error('Invalid long context pricing')
    return `${variable} * (${coefficient} * ${policy[multiplierFields[variable]]})`
  })
  return `len <= ${policy.threshold} ? ${unversioned} : tier("long_context", ${longBody})`
}

export function readLongContextPricing(expression: string): { base: string; policy: LongContextPricing } | null {
  const match = expression.trim().replace(/^v1:/, '').trim().match(/^len\s*<=\s*(\d+)\s*\?\s*(tier\("base",[\s\S]*\))\s*:\s*tier\("long_context",\s*([\s\S]*)\)$/)
  if (!match || !canConfigureLongContext(match[2])) return null
  const policy = { ...defaultLongContextPricing, threshold: Number(match[1]) }
  const terms = new RegExp(String.raw`\b(p|c|cr|cc|cc1h)\s*\*\s*\(\s*${numberPattern}\s*\*\s*(${numberPattern})\s*\)`, 'g')
  for (const term of match[3].matchAll(terms)) {
    policy[multiplierFields[term[1] as keyof typeof multiplierFields]] = Number(term[2])
  }
  try {
    const generated = applyLongContextPricing(match[2], policy).replace(/\s+/g, '')
    if (generated !== expression.trim().replace(/^v1:/, '').replace(/\s+/g, '')) return null
  } catch { return null }
  return { base: match[2], policy }
}
export type BulkPriceMode = 'ratio' | 'percent' | 'value'
export type PriceValues = Partial<Record<PriceField, number>>
export const priceFields: PriceField[] = ['inputVnd', 'outputVnd', 'cacheReadVnd', 'cacheWriteVnd']

export function modelFamily(model: string): string {
  const name = model.toLowerCase()
  for (const family of ['opus', 'fable', 'haiku', 'gpt']) {
    if (name.includes(family)) return family
  }
  return 'other'
}

// Blank fields preserve the existing value; zero is an explicit price.
// Under the 1,000 VND = 1 $ convention:
// - Absolute values and base input in ratio mode are entered in $ and converted to VND (* 1,000).
// - Percentages and multiplier ratios are unitless numbers and are NOT multiplied by 1,000.
export function calculateBulkPrices(
  current: PriceValues,
  mode: BulkPriceMode,
  fields: Partial<Record<PriceField, string>>
): PriceValues {
  const result = { ...current }
  for (const field of priceFields) {
    const raw = fields[field]?.trim()
    if (!raw) continue
    const normalized = raw.replace(',', '.')
    const value = Number(normalized)
    if (!Number.isFinite(value) || (mode !== 'percent' && value < 0) || (mode === 'percent' && value < -100)) {
      throw new Error('Invalid price adjustment')
    }
    if (mode === 'percent') {
      if (current[field] === undefined) throw new Error('No current price')
      result[field] = current[field]! * (1 + value / 100)
    } else if (mode === 'ratio' && field !== 'inputVnd') {
      if (result.inputVnd === undefined) throw new Error('No input price')
      result[field] = result.inputVnd * value
    } else if (mode === 'ratio' && field === 'inputVnd') {
      result[field] = value * 1000
    } else {
      // mode === 'value': absolute price entered in $, stored as VND
      result[field] = value * 1000
    }
    if (!Number.isFinite(result[field]) || result[field]! < 0) {
      throw new Error('Invalid price adjustment')
    }
    result[field] = Math.round(result[field]! * 10000) / 10000
  }
  return result
}
