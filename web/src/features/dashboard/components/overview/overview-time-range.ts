// Local-time ranges for the Overview page. Boundaries follow the browser's
// calendar day (not UTC), so "Today" starts at local midnight.

export const DAY_MS = 24 * 60 * 60 * 1000
export const CUSTOM_RANGE_MAX_DAYS = 31

// Server limits: /api/data/self accepts at most 30 days per call, while the
// breakdown and error summary endpoints accept up to 31 days.
export const SELF_DATA_MAX_SPAN_SECONDS = 2_592_000
export const BREAKDOWN_MAX_SPAN_SECONDS = 2_678_400

export type RangePreset = 'today' | '7d' | '30d' | 'month' | 'custom'

export interface TimeRange {
  start: Date
  end: Date
}

export interface UnixRange {
  start_timestamp: number
  end_timestamp: number
}

export interface RangeState {
  preset: RangePreset
  // YYYY-MM-DD in local time; only read when preset is 'custom'.
  customStart: string
  customEnd: string
}

export type RangeError = 'invalid' | 'order' | 'too_long' | 'future'

export function startOfLocalDay(date: Date): Date {
  const result = new Date(date)
  result.setHours(0, 0, 0, 0)
  return result
}

export function addLocalDays(date: Date, days: number): Date {
  const result = new Date(date)
  result.setDate(result.getDate() + days)
  return result
}

export function startOfLocalMonth(date: Date): Date {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}

export function toLocalDateKey(date: Date): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

// Parses YYYY-MM-DD as local midnight. Returns null for impossible dates such
// as 2026-02-30 so that overflow never silently shifts the range.
export function parseLocalDateKey(key: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2]) - 1
  const day = Number(match[3])
  const date = new Date(year, month, day)
  if (
    date.getFullYear() !== year ||
    date.getMonth() !== month ||
    date.getDate() !== day
  ) {
    return null
  }
  return date
}

export function toUnixRange(range: TimeRange): UnixRange {
  return {
    start_timestamp: Math.floor(range.start.getTime() / 1000),
    end_timestamp: Math.floor(range.end.getTime() / 1000),
  }
}

// Shrinks a range so its span never exceeds the server limit.
export function clampRangeSpan(
  range: UnixRange,
  maxSpanSeconds: number
): UnixRange {
  const maxEnd = range.start_timestamp + maxSpanSeconds
  return {
    start_timestamp: range.start_timestamp,
    end_timestamp: Math.min(range.end_timestamp, maxEnd),
  }
}

// Splits a range into consecutive chunks that each fit the span limit. Chunks
// do not overlap, so hourly rows are never counted twice.
export function splitUnixRange(
  range: UnixRange,
  maxSpanSeconds: number
): UnixRange[] {
  const chunks: UnixRange[] = []
  let start = range.start_timestamp
  while (start <= range.end_timestamp) {
    const end = Math.min(range.end_timestamp, start + maxSpanSeconds - 1)
    chunks.push({ start_timestamp: start, end_timestamp: end })
    start = end + 1
  }
  return chunks
}

export function resolvePresetRange(
  preset: Exclude<RangePreset, 'custom'>,
  now: Date
): TimeRange {
  switch (preset) {
    case 'today':
      return { start: startOfLocalDay(now), end: now }
    case '7d':
      return { start: startOfLocalDay(addLocalDays(now, -6)), end: now }
    case '30d':
      return { start: startOfLocalDay(addLocalDays(now, -29)), end: now }
    case 'month':
      return { start: startOfLocalMonth(now), end: now }
  }
}

// The comparison period for a range: the same clock window shifted back by
// the number of local days the range covers. Today 00:00-14:30 compares with
// yesterday 00:00-14:30; the last 7 days compare with the 7 days before.
export function previousComparableRange(range: TimeRange): TimeRange {
  const days =
    Math.round(
      (startOfLocalDay(range.end).getTime() -
        startOfLocalDay(range.start).getTime()) /
        DAY_MS
    ) + 1
  return {
    start: addLocalDays(range.start, -days),
    end: addLocalDays(range.end, -days),
  }
}

// True when the whole range falls on one local calendar day.
export function isSingleLocalDay(range: TimeRange): boolean {
  return toLocalDateKey(range.start) === toLocalDateKey(range.end)
}

// Validates a custom inclusive day range and returns it as a TimeRange whose
// end is the end of the last selected day, capped at `now`.
export function resolveCustomRange(
  startKey: string,
  endKey: string,
  now: Date
): { range: TimeRange; error: null } | { range: null; error: RangeError } {
  const start = parseLocalDateKey(startKey)
  const end = parseLocalDateKey(endKey)
  if (!start || !end) return { range: null, error: 'invalid' }
  if (end.getTime() < start.getTime()) return { range: null, error: 'order' }
  if (start.getTime() > now.getTime()) return { range: null, error: 'future' }

  const days = Math.round((end.getTime() - start.getTime()) / DAY_MS) + 1
  if (days > CUSTOM_RANGE_MAX_DAYS) return { range: null, error: 'too_long' }

  const endOfLastDay = new Date(addLocalDays(end, 1).getTime() - 1)
  return {
    range: { start, end: endOfLastDay.getTime() > now.getTime() ? now : endOfLastDay },
    error: null,
  }
}

export function resolveRangeState(
  state: RangeState,
  now: Date
): { range: TimeRange | null; error: RangeError | null } {
  if (state.preset !== 'custom') {
    return { range: resolvePresetRange(state.preset, now), error: null }
  }
  const result = resolveCustomRange(state.customStart, state.customEnd, now)
  return { range: result.range, error: result.error }
}

// Same elapsed part of the current month, compared with last month.
// The previous window is clamped to the start of this month so that e.g.
// March 31 does not spill into March 1 when February is shorter.
export function resolveMonthComparison(now: Date): {
  current: TimeRange
  previous: TimeRange
} {
  const thisMonthStart = startOfLocalMonth(now)
  const previousMonthStart = new Date(
    now.getFullYear(),
    now.getMonth() - 1,
    1
  )
  const elapsed = now.getTime() - thisMonthStart.getTime()
  const previousEnd = new Date(
    Math.min(previousMonthStart.getTime() + elapsed, thisMonthStart.getTime())
  )
  return {
    current: { start: thisMonthStart, end: now },
    previous: { start: previousMonthStart, end: previousEnd },
  }
}

export function resolveRollingRange(days: number, now: Date): TimeRange {
  return { start: new Date(now.getTime() - days * DAY_MS), end: now }
}
