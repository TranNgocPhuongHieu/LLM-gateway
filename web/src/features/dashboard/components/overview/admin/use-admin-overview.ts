import { useQuery } from '@tanstack/react-query'

import { requireServerSuccess } from '@/lib/server-error-message'
import { useAuthStore } from '@/stores/auth-store'

import {
  getAdminErrorSummary,
  getAdminSummary,
  getUserQuotaDates,
} from '../../../api'
import type {
  AdminSummaryData,
  QuotaDataItem,
  SelfErrorSummaryData,
} from '../../../types'
import {
  BREAKDOWN_MAX_SPAN_SECONDS,
  SELF_DATA_MAX_SPAN_SECONDS,
  clampRangeSpan,
  splitUnixRange,
  type UnixRange,
} from '../overview-time-range'

const STALE_TIME_MS = 60 * 1000

function useAdminScope() {
  const userId = useAuthStore((state) => state.auth.user?.id)
  const sessionId = useAuthStore((state) => state.auth.session?.sid)
  return { userId, sessionId }
}

// System-wide KPIs for the super admin Overview. excludeInternal hides admin
// and root accounts from the usage figures (see the backend summary).
export function useAdminSummary(
  range: UnixRange | null,
  excludeInternal = false
) {
  const { userId, sessionId } = useAdminScope()
  return useQuery({
    queryKey: [
      'dashboard',
      'admin-overview',
      'summary',
      userId,
      sessionId,
      range?.start_timestamp,
      range?.end_timestamp,
      excludeInternal,
    ],
    enabled: Boolean(userId && range),
    queryFn: async (): Promise<AdminSummaryData> => {
      if (!range) throw new Error('Missing range')
      const bounded = clampRangeSpan(range, BREAKDOWN_MAX_SPAN_SECONDS)
      const result = requireServerSuccess(
        await getAdminSummary({ ...bounded, exclude_internal: excludeInternal })
      )
      if (!result.data) throw new Error('Missing admin summary data')
      return result.data
    },
    staleTime: STALE_TIME_MS,
    retry: false,
    meta: { errorToast: false },
  })
}

// System-wide error categories for the super admin Overview.
export function useAdminErrorSummary(
  range: UnixRange | null,
  excludeInternal = false
) {
  const { userId, sessionId } = useAdminScope()
  return useQuery({
    queryKey: [
      'dashboard',
      'admin-overview',
      'error-summary',
      userId,
      sessionId,
      range?.start_timestamp,
      range?.end_timestamp,
      excludeInternal,
    ],
    enabled: Boolean(userId && range),
    queryFn: async (): Promise<SelfErrorSummaryData> => {
      if (!range) throw new Error('Missing range')
      const bounded = clampRangeSpan(range, BREAKDOWN_MAX_SPAN_SECONDS)
      const result = requireServerSuccess(
        await getAdminErrorSummary({
          ...bounded,
          exclude_internal: excludeInternal,
        })
      )
      return result.data ?? { total: 0, truncated: false, categories: [] }
    },
    staleTime: STALE_TIME_MS,
    retry: false,
    meta: { errorToast: false },
  })
}

// System-wide hourly rows (all users) used by the daily spend and distribution
// charts. Each call is capped at the /api/data span limit, so longer ranges are
// fetched in consecutive non-overlapping chunks.
export function useAdminHourlyUsage(
  range: UnixRange | null,
  excludeInternal = false
) {
  const { userId, sessionId } = useAdminScope()
  return useQuery({
    queryKey: [
      'dashboard',
      'admin-overview',
      'hourly',
      userId,
      sessionId,
      range?.start_timestamp,
      range?.end_timestamp,
      excludeInternal,
    ],
    enabled: Boolean(userId && range),
    queryFn: async (): Promise<QuotaDataItem[]> => {
      if (!range) return []
      const rows: QuotaDataItem[] = []
      for (const chunk of splitUnixRange(range, SELF_DATA_MAX_SPAN_SECONDS)) {
        const result = requireServerSuccess(
          await getUserQuotaDates(
            {
              ...chunk,
              default_time: 'hour',
              exclude_internal: excludeInternal,
            },
            true
          )
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
