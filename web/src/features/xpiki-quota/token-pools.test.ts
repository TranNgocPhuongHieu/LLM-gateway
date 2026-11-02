import { describe, expect, it } from 'vitest'
import type { XPikiQuotaKeyResult } from './api'
import { formatPoolTokens, referenceTokenPools } from './token-pools'

function key(reference_model: string, remaining_tokens: string | undefined, unlimited = false) {
  return { reference_model, used_tokens: '0', limit_tokens: '1,000', remaining_tokens, unlimited } as XPikiQuotaKeyResult
}

describe('reference token pool totals', () => {
  it('never adds Astra tokens to Opus tokens', () => {
    const pools = referenceTokenPools({ keys: [key('gpt-6-astra', '10'), key('gpt-6-astra', '20'), key('claude-opus-5-5', '11')] })
    expect(pools.map((pool) => pool.remaining)).toEqual([30, 11])
  })
  it('keeps missing token counts unknown rather than falling back to quota', () => {
    const pools = referenceTokenPools({ keys: [key('gpt-6-astra', '10'), key('gpt-6-astra', undefined)] })
    expect(pools[0].remaining).toBeNull()
    expect(formatPoolTokens(pools[0].remaining)).toBe('—')
  })
  it('does not apply an unlimited GPT balance to Claude', () => {
    const pools = referenceTokenPools({ keys: [key('gpt-6-astra', '0', true), key('claude-opus-5-5', '11')] })
    expect(pools.map((pool) => pool.unlimited)).toEqual([true, false])
  })
  it('preserves explicit zero and parses formatted token counts', () => {
    const pools = referenceTokenPools({ keys: [key('gpt-6-astra', '0'), key('gpt-6-astra', '1,000')] })
    expect(pools[0].remaining).toBe(1000)
    expect(formatPoolTokens(0)).toBe('0')
  })
})
