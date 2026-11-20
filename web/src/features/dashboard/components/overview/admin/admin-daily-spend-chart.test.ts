import { describe, expect, it } from 'vitest'

import { modelColors } from './admin-daily-spend-chart'

describe('modelColors', () => {
  it('never gives two drawn models the same color', () => {
    // These two names hash to the same palette slot.
    const models = ['claude-opus-5-5', 'gpt-6.2-astra', 'a', 'b', 'c', 'Other']
    const colors = modelColors(models, 'Other')
    const modelOnly = colors.slice(0, -1)
    expect(new Set(modelOnly).size).toBe(modelOnly.length)
    expect(colors.at(-1)).toBe('#94a3b8')
  })

  it('keeps the first model on its name-based color', () => {
    const alone = modelColors(['claude-opus-5-5'], 'Other')[0]
    expect(modelColors(['claude-opus-5-5', 'gpt-6.2-astra'], 'Other')[0]).toBe(alone)
    // The colliding name only moves when the slot is taken.
    expect(modelColors(['gpt-6.2-astra'], 'Other')[0]).toBe(alone)
  })
})
