import { describe, expect, it } from 'vitest'

import { uncachedInputTokens } from './format'

describe('uncachedInputTokens', () => {
  it('removes cache reads from cache-inclusive OpenAI-style totals', () => {
    // GPT through 9router: prompt_tokens 160,298 includes 152,704 cached.
    expect(uncachedInputTokens(160298, { cache_tokens: 152704 })).toBe(7594)
  })

  it('keeps Anthropic-semantic input, which already excludes cache reads', () => {
    expect(
      uncachedInputTokens(308, { cache_tokens: 2449, usage_semantic: 'anthropic' })
    ).toBe(308)
    expect(uncachedInputTokens(308, { cache_tokens: 2449, claude: true })).toBe(308)
  })

  it('shows the full prompt without cache data and never goes negative', () => {
    expect(uncachedInputTokens(2496, {})).toBe(2496)
    expect(uncachedInputTokens(2496, null)).toBe(2496)
    expect(uncachedInputTokens(100, { cache_tokens: 150 })).toBe(0)
    expect(uncachedInputTokens(undefined, { cache_tokens: 10 })).toBe(0)
  })
})
