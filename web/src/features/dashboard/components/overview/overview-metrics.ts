import type {
  QuotaDataItem,
  SelfBreakdownData,
  SelfBreakdownGroupBy,
  SelfBreakdownItem,
} from '../../types'
import {
  DAY_MS,
  toLocalDateKey,
  type TimeRange,
} from './overview-time-range'

// Pure calculations behind the Overview cards. Quota stays in quota units
// here; callers convert to credits with the shared formatter/helper.

export const AVERAGE_WINDOW_DAYS = 7
export const LOW_RUNWAY_DAYS = 3
export const CAUTION_RUNWAY_DAYS = 14

export type HealthLevel = 'healthy' | 'caution' | 'critical'

// Null when the previous period is empty: a percentage is meaningless there.
export function percentChange(current: number, previous: number): number | null {
  if (!(previous > 0)) return null
  return ((current - previous) / previous) * 100
}

export function averageDailyQuota(
  quotaOverWindow: number,
  windowDays = AVERAGE_WINDOW_DAYS
): number {
  return windowDays > 0 ? quotaOverWindow / windowDays : 0
}

export function getHealthLevel(
  balanceQuota: number,
  avgDailyQuota: number
): HealthLevel {
  if (balanceQuota <= 0) return 'critical'
  if (avgDailyQuota <= 0) return 'healthy'
  const runwayDays = balanceQuota / avgDailyQuota
  if (runwayDays < LOW_RUNWAY_DAYS) return 'critical'
  if (runwayDays < CAUTION_RUNWAY_DAYS) return 'caution'
  return 'healthy'
}

export type RunOutProjection =
  | { kind: 'depleted' }
  | { kind: 'no-usage' }
  | { kind: 'date'; date: Date; days: number }

export function projectRunOut(
  balanceQuota: number,
  avgDailyQuota: number,
  now: Date
): RunOutProjection {
  if (balanceQuota <= 0) return { kind: 'depleted' }
  if (!(avgDailyQuota > 0)) return { kind: 'no-usage' }
  const days = balanceQuota / avgDailyQuota
  return {
    kind: 'date',
    days,
    date: new Date(now.getTime() + days * DAY_MS),
  }
}

export function isBelowWarningThreshold(
  balanceQuota: number,
  thresholdQuota: number
): boolean {
  return balanceQuota < thresholdQuota
}

export interface SpendKpiInput {
  balanceQuota: number
  todayQuota: number
  monthQuota: number
  lastMonthSamePeriodQuota: number
  last7DaysQuota: number
  now: Date
}

export interface SpendKpis {
  monthChangePct: number | null
  avgDailyQuota: number
  health: HealthLevel
  runOut: RunOutProjection
}

export function computeSpendKpis(input: SpendKpiInput): SpendKpis {
  const avgDailyQuota = averageDailyQuota(input.last7DaysQuota)
  return {
    monthChangePct: percentChange(
      input.monthQuota,
      input.lastMonthSamePeriodQuota
    ),
    avgDailyQuota,
    health: getHealthLevel(input.balanceQuota, avgDailyQuota),
    runOut: projectRunOut(input.balanceQuota, avgDailyQuota, input.now),
  }
}

export function sharePercent(quota: number, totalQuota: number): number {
  return totalQuota > 0 ? (quota / totalQuota) * 100 : 0
}

// ----------------------------------------------------------------------------
// Breakdown table rows
// ----------------------------------------------------------------------------

export interface BreakdownRow {
  key: string
  label: string
  requests: number
  tokens: number
  quota: number
  share: number
}

export function getBreakdownRows(
  data: SelfBreakdownData | undefined,
  groupBy: SelfBreakdownGroupBy,
  deletedKeyLabel: (keyId: string) => string
): BreakdownRow[] {
  const items = data?.items ?? []
  const total = Number(data?.total.quota ?? 0)
  return items.map((item) => ({
    key: item.key,
    label: resolveBreakdownLabel(item, groupBy, deletedKeyLabel),
    requests: Number(item.count) || 0,
    tokens: Number(item.token_used) || 0,
    quota: Number(item.quota) || 0,
    share: sharePercent(Number(item.quota) || 0, total),
  }))
}

// The backend returns an empty label for a deleted token; the UI replaces it.
export function resolveBreakdownLabel(
  item: SelfBreakdownItem,
  groupBy: SelfBreakdownGroupBy,
  deletedKeyLabel: (keyId: string) => string
): string {
  if (groupBy === 'token' && item.label === '') {
    return deletedKeyLabel(item.key)
  }
  return item.label || item.key || '-'
}

// ----------------------------------------------------------------------------
// Daily cost series (stacked by model)
// ----------------------------------------------------------------------------

export interface DailyCostPoint {
  day: string
  series: string
  credits: number
}

export interface DailyCostSeries {
  days: string[]
  // Ordered by total spend; the overflow bucket is always last.
  series: string[]
  points: DailyCostPoint[]
  totalCredits: number
}

export interface DailyCostOptions {
  topN: number
  otherLabel: string
  unknownModelLabel: string
  toCredits: (quota: number) => number
  // 'hour' buckets by local hour ("YYYY-MM-DD HH") instead of local day.
  granularity?: 'day' | 'hour'
}

export function listLocalDays(range: TimeRange): string[] {
  const days: string[] = []
  const cursor = new Date(range.start)
  cursor.setHours(0, 0, 0, 0)
  const last = toLocalDateKey(range.end)
  // Bounded loop: a range is at most ~31 days, the guard prevents runaway.
  for (let guard = 0; guard < 400; guard++) {
    const key = toLocalDateKey(cursor)
    days.push(key)
    if (key === last) break
    cursor.setDate(cursor.getDate() + 1)
  }
  return days
}

function toLocalHourKey(date: Date): string {
  return `${toLocalDateKey(date)} ${String(date.getHours()).padStart(2, '0')}`
}

// Local hour buckets from the range start to its end, both inclusive. Only
// hours inside the range are listed, so a "Today" chart never shows yesterday.
export function listLocalHours(range: TimeRange): string[] {
  const hours: string[] = []
  const cursor = new Date(range.start)
  cursor.setMinutes(0, 0, 0)
  const last = toLocalHourKey(range.end)
  // Bounded loop: hourly charts cover about one day; the guard prevents runaway.
  for (let guard = 0; guard < 800; guard++) {
    const key = toLocalHourKey(cursor)
    hours.push(key)
    if (key === last || cursor >= range.end) break
    cursor.setHours(cursor.getHours() + 1)
  }
  return hours
}

export function buildDailyCostSeries(
  rows: QuotaDataItem[],
  range: TimeRange,
  options: DailyCostOptions
): DailyCostSeries {
  const hourly = options.granularity === 'hour'
  const days = hourly ? listLocalHours(range) : listLocalDays(range)
  const bucketKey = hourly ? toLocalHourKey : toLocalDateKey
  const dayIndex = new Set(days)
  const perModel = new Map<string, number>()
  const perDayModel = new Map<string, number>()
  let totalQuota = 0

  for (const row of rows) {
    const quota = Number(row.quota) || 0
    if (quota === 0 || !Number.isFinite(Number(row.created_at))) continue
    const day = bucketKey(new Date(Number(row.created_at) * 1000))
    if (!dayIndex.has(day)) continue
    const model = row.model_name || options.unknownModelLabel
    perModel.set(model, (perModel.get(model) ?? 0) + quota)
    const cellKey = `${day}\u0000${model}`
    perDayModel.set(cellKey, (perDayModel.get(cellKey) ?? 0) + quota)
    totalQuota += quota
  }

  const ranked = [...perModel.entries()].sort((a, b) => b[1] - a[1])
  const top = new Set(ranked.slice(0, options.topN).map(([model]) => model))
  const hasOverflow = ranked.length > options.topN
  const series = ranked.slice(0, options.topN).map(([model]) => model)
  if (hasOverflow) series.push(options.otherLabel)

  const bucket = new Map<string, number>()
  for (const [cellKey, quota] of perDayModel) {
    const [day, model] = cellKey.split('\u0000')
    const seriesName = top.has(model) ? model : options.otherLabel
    const key = `${day}\u0000${seriesName}`
    bucket.set(key, (bucket.get(key) ?? 0) + quota)
  }

  const points: DailyCostPoint[] = []
  for (const day of days) {
    for (const seriesName of series) {
      points.push({
        day,
        series: seriesName,
        credits: options.toCredits(bucket.get(`${day}\u0000${seriesName}`) ?? 0),
      })
    }
  }

  return {
    days,
    series,
    points,
    totalCredits: options.toCredits(totalQuota),
  }
}
