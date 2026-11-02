import { describe, expect, it } from 'vitest'

import { calculateBulkPrices, modelFamily, applyLongContextPricing, defaultLongContextPricing, readLongContextPricing } from './bulk-pricing'
import { updateExpressionDraft } from './components/price-management-view'
import { parseTiersFromExpr } from '@/features/pricing/lib/billing-expr'
import { evaluateBillingExpression } from '@/features/pricing/lib/billing-expression/runtime'

describe('bulk pricing preview', () => {
  const current = { inputVnd: 700, outputVnd: 3500, cacheReadVnd: 350 }

  it('applies ratios against the new input ($ -> VND) and leaves blanks unchanged', () => {
    // inputVnd entered as 1 $ (= 1000 VND), outputVnd entered as ratio 5x
    expect(calculateBulkPrices(current, 'ratio', { inputVnd: '1', outputVnd: '5', cacheReadVnd: '' }))
      .toEqual({ inputVnd: 1000, outputVnd: 5000, cacheReadVnd: 350 })
    expect(current.inputVnd).toBe(700)

    // Supports Vietnamese comma in ratio inputVnd
    expect(calculateBulkPrices(current, 'ratio', { inputVnd: '0,7', outputVnd: '5' }))
      .toEqual({ inputVnd: 700, outputVnd: 3500, cacheReadVnd: 350 })
  })

  it('supports increases, decreases, comma decimals and explicit zero prices', () => {
    expect(calculateBulkPrices(current, 'percent', { inputVnd: '10', outputVnd: '-100' }))
      .toEqual({ inputVnd: 770, outputVnd: 0, cacheReadVnd: 350 })

    // value mode: input in $, converted to VND (* 1000)
    expect(calculateBulkPrices(current, 'value', { inputVnd: '0.7', cacheReadVnd: '0' }))
      .toEqual({ inputVnd: 700, outputVnd: 3500, cacheReadVnd: 0 })
    expect(calculateBulkPrices(current, 'value', { inputVnd: '0,35' }).inputVnd).toBe(350)
  })

  it('rejects negative prices, non-finite prices, and missing ratio bases', () => {
    expect(() => calculateBulkPrices(current, 'value', { inputVnd: '-1' })).toThrow()
    expect(() => calculateBulkPrices(current, 'percent', { inputVnd: '-101' })).toThrow()
    expect(() => calculateBulkPrices(current, 'value', { inputVnd: 'Infinity' })).toThrow()
    expect(() => calculateBulkPrices({}, 'ratio', { outputVnd: '5' })).toThrow()
  })

  it('groups provider-prefixed models by family', () => {
    expect(modelFamily('provider/claude-opus-5')).toBe('opus')
    expect(modelFamily('provider/gpt-6.1-sol')).toBe('gpt')
    expect(modelFamily('unclassified')).toBe('other')
  })

  it('persists zero cache-write and adds missing cache coefficients', () => {
    const draft = { model: 'example', hasPricing: true, ...current, cacheWriteVnd: 0 }
    const expression = updateExpressionDraft('tier("base", p * 1 + c * 2 + cc * 3)', draft)
    expect(expression).toContain('cc * 0.0000000000000000')
    expect(expression).toContain(`cr * ${(350 / 26000).toFixed(16)}`)
    expect(updateExpressionDraft('tier("base", p * 1)', { ...draft, cacheWriteVnd: 100 }))
      .toContain(`cc * ${(100 / 26000).toFixed(16)}`)
    expect(updateExpressionDraft('', { ...draft, model: 'claude-opus-5' }))
      .toContain('cc * 0.0000000000000000')
  })

  it('round-trips adjustable context rules and exposes both prices publicly', () => {
    const base = 'tier("base", p * 0.7 + c * 3.5 + cr * 0.35 + cc * 0)'
    const policy = { ...defaultLongContextPricing, output: 1.5, cacheWrite: 3 }
    const expression = applyLongContextPricing(base, policy)
    expect(readLongContextPricing(expression)).toEqual({ base, policy })
    const tiers = parseTiersFromExpr(expression)
    expect(tiers).toHaveLength(2)
    expect(tiers[0].inputPrice).toBe(0.7)
    expect(tiers[1].inputPrice).toBe(1.4)
    expect(tiers[1].outputPrice).toBe(5.25)
    expect(tiers[1].conditions).toEqual([{ var: 'len', op: '>', value: 272000 }])
    for (const len of [271999, 272000, 272001]) {
      const tokens = { len, p: 100, cr: len - 100, c: 50, cc: 0 }
      const result = evaluateBillingExpression(expression, { tokens })
      expect(result.status).toBe('success')
      if (result.status === 'success') {
        expect(result.matchedTier).toBe(len > 272000 ? 'long_context' : 'base')
        expect(result.cost).toBeCloseTo(len > 272000 ? 100 * 1.4 + (len - 100) * 0.7 + 50 * 5.25 : 100 * 0.7 + (len - 100) * 0.35 + 50 * 3.5)
      }
    }
  })

  it('updates both rates when base prices change and removes the rule when disabled', () => {
    const base = 'tier("base", p * 0.026923076923076923 + c * 0.1346153846153846 + cr * 0.013461538461538462)'
    const expression = applyLongContextPricing(base, defaultLongContextPricing)
    const draft = { model: 'gpt-6.1-sol', hasPricing: true, ...current }
    expect(updateExpressionDraft(expression, draft)).toBe(expression)
    const changed = updateExpressionDraft(expression, { ...draft, inputVnd: 1400 })
    expect(readLongContextPricing(changed)?.policy).toEqual(defaultLongContextPricing)
    const tiers = parseTiersFromExpr(changed)
    expect(Number(tiers[1].inputPrice)).toBeCloseTo(Number(tiers[0].inputPrice) * 2)
    expect(updateExpressionDraft(expression, { ...draft, longContext: null })).toBe(base)
    expect((changed.match(/long_context/g) || [])).toHaveLength(1)
  })

  it('rejects invalid rules and preserves unsupported expressions', () => {
    const base = 'tier("base", p * 1 + c * 2)'
    for (const policy of [
      { ...defaultLongContextPricing, threshold: 0 },
      { ...defaultLongContextPricing, threshold: 1.5 },
      { ...defaultLongContextPricing, input: -2 },
      { ...defaultLongContextPricing, output: Infinity },
      { ...defaultLongContextPricing, cacheRead: NaN },
    ]) expect(() => applyLongContextPricing(base, policy)).toThrow()
    const custom = 'len <= 100 ? tier("custom", p * 1) : tier("large", p * 2)'
    expect(readLongContextPricing(custom)).toBeNull()
    expect(() => applyLongContextPricing(custom, defaultLongContextPricing)).toThrow()
    const generated = applyLongContextPricing(base, defaultLongContextPricing)
    expect(readLongContextPricing(generated.replace('p * (1 * 2)', 'p * (1 * 3)'))).not.toEqual({ base, policy: defaultLongContextPricing })
  })
})
