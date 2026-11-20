import { getDashboardChartColors } from '../../lib/charts'

export const OTHER_COLOR = '#94a3b8'
const PALETTE_SIZE = 10

// Colors for the drawn series, in order. A model's color comes from its name,
// not its rank, so it usually stays the same across ranges; when that color is
// already taken by an earlier series, the next free one is used so two models
// never share a color. "Other" is always neutral gray.
export function modelColors(models: string[], otherLabel: string): string[] {
  const palette = getDashboardChartColors(PALETTE_SIZE)
  const used = new Set<number>()
  return models.map((model) => {
    if (model === otherLabel) return OTHER_COLOR
    let hash = 0
    for (const char of model) hash = (hash * 31 + char.charCodeAt(0)) >>> 0
    let index = hash % palette.length
    for (let step = 0; step < palette.length && used.has(index); step++) {
      index = (index + 1) % palette.length
    }
    used.add(index)
    return palette[index] ?? OTHER_COLOR
  })
}
