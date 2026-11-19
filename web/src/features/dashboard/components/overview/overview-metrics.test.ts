import { describe, expect, it } from 'vitest'

import {
  averageDailyQuota,
  buildDailyCostSeries,
  computeSpendKpis,
  getBreakdownRows,
  getHealthLevel,
  percentChange,
  projectRunOut,
  sharePercent,
} from './overview-metrics'
import {
  parseLocalDateKey,
  resolveCustomRange,
  resolveMonthComparison,
  resolvePresetRange,
  splitUnixRange,
  toLocalDateKey,
} from './overview-time-range'

// Local-time constructors keep these tests independent of the runner's TZ.
const local = (y: number, m: number, d: number, h = 0, min = 0) =>
  new Date(y, m - 1, d, h, min, 0, 0)

describe('spend KPIs', () => {
  it('computes month change against the same elapsed period last month', () => {
    expect(percentChange(150, 100)).toBeCloseTo(50)
    expect(percentChange(50, 100)).toBeCloseTo(-50)
  })

  it('hides the percentage when last month had no spend', () => {
    expect(percentChange(150, 0)).toBeNull()
    expect(percentChange(0, 0)).toBeNull()
  })

  it('averages the trailing 7 days for the daily rate', () => {
    expect(averageDailyQuota(700, 7)).toBe(100)
    expect(averageDailyQuota(0)).toBe(0)
  })

  it('projects the run-out date from balance divided by daily average', () => {
    const now = local(2026, 11, 19, 10)
    const projection = projectRunOut(1000, 100, now)
    expect(projection.kind).toBe('date')
    if (projection.kind !== 'date') return
    expect(projection.days).toBeCloseTo(10)
    expect(projection.date.getTime()).toBe(now.getTime() + 10 * 24 * 3600 * 1000)
  })

  it('reports no recent usage when the 7-day average is zero', () => {
    expect(projectRunOut(1000, 0, local(2026, 11, 19))).toEqual({ kind: 'no-usage' })
  })

  it('reports depleted balance before any usage projection', () => {
    expect(projectRunOut(0, 100, local(2026, 11, 19))).toEqual({ kind: 'depleted' })
  })

  it('derives health from runway in days', () => {
    expect(getHealthLevel(0, 0)).toBe('critical')
    expect(getHealthLevel(1000, 0)).toBe('healthy')
    expect(getHealthLevel(200, 100)).toBe('critical') // 2 days
    expect(getHealthLevel(1000, 100)).toBe('caution') // 10 days
    expect(getHealthLevel(3000, 100)).toBe('healthy') // 30 days
  })

  it('combines inputs into KPI values', () => {
    const kpis = computeSpendKpis({
      balanceQuota: 1000, // 10 days of runway at 100/day
      todayQuota: 40,
      monthQuota: 300,
      lastMonthSamePeriodQuota: 200,
      last7DaysQuota: 700,
      now: local(2026, 11, 19, 12),
    })
    expect(kpis.monthChangePct).toBeCloseTo(50)
    expect(kpis.avgDailyQuota).toBe(100)
    expect(kpis.health).toBe('caution')
    expect(kpis.runOut.kind).toBe('date')
  })

  it('returns null change when last month is empty', () => {
    const kpis = computeSpendKpis({
      balanceQuota: 100,
      todayQuota: 0,
      monthQuota: 10,
      lastMonthSamePeriodQuota: 0,
      last7DaysQuota: 0,
      now: local(2026, 11, 2),
    })
    expect(kpis.monthChangePct).toBeNull()
    expect(kpis.runOut).toEqual({ kind: 'no-usage' })
  })
})

describe('time ranges (local time)', () => {
  it('starts Today at local midnight', () => {
    const now = local(2026, 11, 19, 15, 30)
    const range = resolvePresetRange('today', now)
    expect(range.start.getTime()).toBe(local(2026, 11, 19).getTime())
    expect(range.end.getTime()).toBe(now.getTime())
  })

  it('includes today in 7-day and 30-day windows', () => {
    const now = local(2026, 11, 19, 9)
    expect(toLocalDateKey(resolvePresetRange('7d', now).start)).toBe('2026-11-13')
    expect(toLocalDateKey(resolvePresetRange('30d', now).start)).toBe('2026-10-21')
  })

  it('compares This month with the same elapsed period of last month', () => {
    const now = local(2026, 3, 31, 12)
    const { current, previous } = resolveMonthComparison(now)
    expect(current.start.getTime()).toBe(local(2026, 3, 1).getTime())
    // February is shorter, so the previous window is clamped to March 1.
    expect(previous.start.getTime()).toBe(local(2026, 2, 1).getTime())
    expect(previous.end.getTime()).toBeLessThanOrEqual(local(2026, 3, 1).getTime())
  })

  it('rejects custom ranges longer than 31 days and reversed dates', () => {
    const now = local(2026, 12, 1)
    expect(resolveCustomRange('2026-10-31', '2026-12-01', now).error).toBe('too_long')
    expect(resolveCustomRange('2026-11-10', '2026-11-01', now).error).toBe('order')
    expect(resolveCustomRange('2026-02-30', '2026-03-01', now).error).toBe('invalid')
    expect(resolveCustomRange('2026-12-05', '2026-12-06', now).error).toBe('future')
  })

  it('accepts exactly 31 inclusive days and caps the end at now', () => {
    const now = local(2026, 12, 15, 8)
    const ok = resolveCustomRange('2026-11-15', '2026-12-15', now)
    expect(ok.error).toBeNull()
    expect(ok.range?.end.getTime()).toBe(now.getTime())
  })

  it('parses only real calendar dates', () => {
    expect(parseLocalDateKey('2026-02-28')).not.toBeNull()
    expect(parseLocalDateKey('2026-02-29')).toBeNull()
    expect(parseLocalDateKey('26-02-28')).toBeNull()
  })

  it('splits long ranges into non-overlapping chunks within the span limit', () => {
    const chunks = splitUnixRange(
      { start_timestamp: 1000, end_timestamp: 1000 + 2_592_000 + 500 },
      2_592_000,
    )
    expect(chunks).toHaveLength(2)
    expect(chunks[0].end_timestamp - chunks[0].start_timestamp + 1).toBeLessThanOrEqual(2_592_000)
    expect(chunks[1].start_timestamp).toBe(chunks[0].end_timestamp + 1)
    expect(chunks[1].end_timestamp).toBe(1000 + 2_592_000 + 500)
  })
})

describe('breakdown rows', () => {
  it('labels deleted keys and computes share of total', () => {
    const rows = getBreakdownRows(
      {
        total: { quota: 400, count: 3, token_used: 90 },
        items: [
          { key: '7', label: 'Prod', quota: 300, count: 2, token_used: 60 },
          { key: '9', label: '', quota: 100, count: 1, token_used: 30 },
        ],
      },
      'token',
      (id) => `Key #${id} (deleted)`,
    )
    expect(rows.map((row) => row.label)).toEqual(['Prod', 'Key #9 (deleted)'])
    expect(rows.map((row) => row.share)).toEqual([75, 25])
    expect(rows[0].requests).toBe(2)
    expect(rows[0].tokens).toBe(60)
  })

  it('does not relabel empty model names as deleted keys', () => {
    const rows = getBreakdownRows(
      {
        total: { quota: 10, count: 1, token_used: 1 },
        items: [{ key: 'gpt', label: '', quota: 10, count: 1, token_used: 1 }],
      },
      'model',
      () => 'deleted',
    )
    expect(rows[0].label).toBe('gpt')
  })

  it('returns zero share when the total is zero', () => {
    expect(sharePercent(0, 0)).toBe(0)
    expect(sharePercent(5, 0)).toBe(0)
  })
})

describe('daily cost series', () => {
  it('stacks the top models and folds the rest into Other, with zero days kept', () => {
    const range = {
      start: local(2026, 11, 1),
      end: local(2026, 11, 3, 23, 59),
    }
    const hour = (day: number, h: number) => Math.floor(local(2026, 11, day, h).getTime() / 1000)
    const rows = [
      { created_at: hour(1, 10), model_name: 'a', quota: 500000 },
      { created_at: hour(1, 11), model_name: 'b', quota: 250000 },
      { created_at: hour(2, 9), model_name: 'a', quota: 500000 },
      { created_at: hour(2, 9), model_name: 'c', quota: 100000 },
      { created_at: hour(2, 9), model_name: 'd', quota: 50000 },
      { created_at: hour(2, 9), model_name: 'e', quota: 25000 },
      { created_at: hour(2, 9), model_name: 'f', quota: 10000 },
      // Outside the selected days: must be ignored.
      { created_at: hour(9, 9), model_name: 'a', quota: 999999 },
    ]
    const series = buildDailyCostSeries(rows, range, {
      topN: 2,
      otherLabel: 'Other',
      unknownModelLabel: '-',
      toCredits: (quota) => quota / 500000,
    })

    expect(series.days).toEqual(['2026-11-01', '2026-11-02', '2026-11-03'])
    expect(series.series).toEqual(['a', 'b', 'Other'])
    const cell = (day: string, name: string) =>
      series.points.find((p) => p.day === day && p.series === name)?.credits
    expect(cell('2026-11-01', 'a')).toBeCloseTo(1)
    expect(cell('2026-11-01', 'b')).toBeCloseTo(0.5)
    expect(cell('2026-11-02', 'a')).toBeCloseTo(1)
    expect(cell('2026-11-02', 'b')).toBe(0)
    expect(cell('2026-11-02', 'Other')).toBeCloseTo((100000 + 50000 + 25000 + 10000) / 500000)
    expect(cell('2026-11-03', 'a')).toBe(0)
    expect(series.totalCredits).toBeCloseTo((500000 * 2 + 250000 + 185000) / 500000)
  })
})
