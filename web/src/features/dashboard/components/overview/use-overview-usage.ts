import { useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'

import { getUserProfile } from '@/features/profile/api'
import { normalizeUserSettings } from '@/features/profile/lib/user-settings'
import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  getSelfErrorSummary,
  getSelfUsageBreakdown,
  getUserQuotaDates,
} from '../../api'
import type {
  QuotaDataItem,
  SelfBreakdownData,
  SelfBreakdownGroupBy,
  SelfErrorSummaryData,
} from '../../types'
import {
  BREAKDOWN_MAX_SPAN_SECONDS,
  SELF_DATA_MAX_SPAN_SECONDS,
  clampRangeSpan,
  resolveMonthComparison,
  resolveRollingRange,
  startOfLocalDay,
  splitUnixRange,
  toUnixRange,
  type UnixRange,
} from './overview-time-range'
import { AVERAGE_WINDOW_DAYS } from './overview-metrics'

const STALE_TIME_MS = 60 * 1000
const MINUTE_MS = 60 * 1000

const EMPTY_BREAKDOWN: SelfBreakdownData = {
  total: { quota: 0, count: 0, token_used: 0 },
  items: [],
}

// Re-renders once a minute so that "now" based ranges move forward while the
// page stays open. Query keys use the resulting timestamps.
export function useCurrentMinute(): Date {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), MINUTE_MS)
    return () => window.clearInterval(id)
  }, [])
  return now
}

function useUserScope() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  return { userId, sessionId }
}

export function useSelfBreakdown(
  range: UnixRange | null,
  groupBy: SelfBreakdownGroupBy
) {
  const { userId, sessionId } = useUserScope()
  return useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'self-breakdown',
      userId,
      sessionId,
      groupBy,
      range?.start_timestamp,
      range?.end_timestamp,
    ],
    enabled: Boolean(userId && range),
    queryFn: async () => {
      if (!range) return EMPTY_BREAKDOWN
      const bounded = clampRangeSpan(range, BREAKDOWN_MAX_SPAN_SECONDS)
      const result = requireServerSuccess(
        await getSelfUsageBreakdown({ ...bounded, group_by: groupBy })
      )
      return result.data ?? EMPTY_BREAKDOWN
    },
    staleTime: STALE_TIME_MS,
    retry: false,
    meta: { errorToast: false },
  })
}

// Hourly rows for the chart; /api/data/self caps each call at 30 days, so
// longer ranges are fetched in consecutive non-overlapping chunks.
export function useSelfDailyUsage(range: UnixRange | null) {
  const { userId, sessionId } = useUserScope()
  return useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'self-hourly',
      userId,
      sessionId,
      range?.start_timestamp,
      range?.end_timestamp,
    ],
    enabled: Boolean(userId && range),
    queryFn: async (): Promise<QuotaDataItem[]> => {
      if (!range) return []
      const rows: QuotaDataItem[] = []
      for (const chunk of splitUnixRange(range, SELF_DATA_MAX_SPAN_SECONDS)) {
        const result = requireServerSuccess(
          await getUserQuotaDates({ ...chunk, default_time: 'hour' }, false)
        )
        rows.push(...(result.data ?? []))
      }
      return rows
    },
    staleTime: STALE_TIME_MS,
    retry: false,
    meta: { errorToast: false },
  })
}

export function useSelfErrorSummary(range: UnixRange | null) {
  const { userId, sessionId } = useUserScope()
  return useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'self-error-summary',
      userId,
      sessionId,
      range?.start_timestamp,
      range?.end_timestamp,
    ],
    enabled: Boolean(userId && range),
    queryFn: async (): Promise<SelfErrorSummaryData> => {
      if (!range) throw new Error('Missing range')
      const bounded = clampRangeSpan(range, BREAKDOWN_MAX_SPAN_SECONDS)
      const result = requireServerSuccess(
        await getSelfErrorSummary(bounded)
      )
      return result.data ?? { total: 0, truncated: false, categories: [] }
    },
    staleTime: STALE_TIME_MS,
    retry: false,
    meta: { errorToast: false },
  })
}

// Four model breakdowns feed the KPI row: today, this month, the same elapsed
// period last month, and the trailing 7 days used for run-out projection.
export function useSpendKpiQueries(now: Date) {
  const { userId, sessionId } = useUserScope()
  const periods = useMemo(() => {
    const month = resolveMonthComparison(now)
    return {
      today: toUnixRange({ start: startOfLocalDay(now), end: now }),
      month: toUnixRange(month.current),
      lastMonth: toUnixRange(month.previous),
      last7: toUnixRange(resolveRollingRange(AVERAGE_WINDOW_DAYS, now)),
    }
  }, [now])

  const makeQuery = (name: keyof typeof periods) => ({
    queryKey: [
      'dashboard',
      'overview',
      'spend-kpi',
      userId,
      sessionId,
      name,
      periods[name].start_timestamp,
      periods[name].end_timestamp,
    ],
    enabled: Boolean(userId),
    queryFn: async () => {
      const bounded = clampRangeSpan(periods[name], BREAKDOWN_MAX_SPAN_SECONDS)
      const result = requireServerSuccess(
        await getSelfUsageBreakdown({ ...bounded, group_by: 'model' })
      )
      return Number(result.data?.total.quota ?? 0)
    },
    staleTime: STALE_TIME_MS,
    refetchInterval: MINUTE_MS,
    retry: false,
    meta: { errorToast: false },
  })

  const today = useQuery(makeQuery('today'))
  const month = useQuery(makeQuery('month'))
  const lastMonth = useQuery(makeQuery('lastMonth'))
  const last7 = useQuery(makeQuery('last7'))
  const queries = [today, month, lastMonth, last7]

  return {
    todayQuota: today.data,
    monthQuota: month.data,
    lastMonthSamePeriodQuota: lastMonth.data,
    last7DaysQuota: last7.data,
    isPending: queries.some((query) => query.isPending),
    isError: queries.some((query) => query.isError),
  }
}

// Customer's own warning threshold (quota units). Falls back to the same
// default the notification settings use.
export function useWarningThreshold() {
  const { userId, sessionId } = useUserScope()
  return useQuery({
    queryKey: ['dashboard', 'overview', 'warning-threshold', userId, sessionId],
    enabled: Boolean(userId),
    queryFn: async () => {
      const response = await getUserProfile()
      requireServerSuccess(response)
      return normalizeUserSettings(response.data?.setting).quota_warning_threshold
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })
}
