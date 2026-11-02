/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatUptimePct,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type { PerfModelSummary } from '@/features/performance-metrics/types'
import { useStatus } from '@/hooks/use-status'
import { formatQuotaWithCredit } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

const PERFORMANCE_WINDOW_HOURS = 24

type HealthLevel = 'healthy' | 'caution' | 'critical'

const HEALTH_CONFIG: Record<HealthLevel, { labelKey: string }> = {
  healthy: {
    labelKey: 'Healthy',
  },
  caution: {
    labelKey: 'Caution',
  },
  critical: {
    labelKey: 'Low balance',
  },
}

function getHealthLevel(balance: number, dailyUsage: number): HealthLevel {
  if (balance <= 0) return 'critical'
  if (dailyUsage <= 0) return 'healthy'
  const runway = balance / dailyUsage
  if (runway < 3) return 'critical'
  if (runway < 14) return 'caution'
  return 'healthy'
}

function getRunwayDays(balance: number, dailyUsage: number): number | null {
  if (balance <= 0) return 0
  if (dailyUsage <= 0) return null
  return balance / dailyUsage
}

type WeightedMetric = 'avg_latency_ms' | 'avg_tps' | 'success_rate'

function simpleAverage(
  rows: PerfModelSummary[],
  metric: WeightedMetric,
  isValid: (value: number) => boolean
): number {
  let total = 0
  let count = 0
  for (const row of rows) {
    const value = Number(row[metric])
    if (!isValid(value)) continue
    total += value
    count++
  }
  return count > 0 ? total / count : Number.NaN
}

export function SummaryCards() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const { loading } = useStatus()

  const summaryTimeRange = useMemo(() => computeTimeRange(1), [])
  const remainQuota = Number(user?.quota ?? 0)
  const requestCount = Number(user?.request_count ?? 0)

  const usageTrendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'summary-sparklines',
      summaryTimeRange.start_timestamp,
      summaryTimeRange.end_timestamp,
    ],
    queryFn: async () =>
      requireServerSuccess(
        await getUserQuotaDates({
          start_timestamp: summaryTimeRange.start_timestamp,
          end_timestamp: summaryTimeRange.end_timestamp,
          default_time: 'hour',
        })
      ),
    staleTime: 60 * 1000,
  })

  const metricsQuery = useQuery({
    queryKey: ['perf-metrics-summary', PERFORMANCE_WINDOW_HOURS],
    queryFn: async () =>
      requireServerSuccess(
        await getPerfMetricsSummary(PERFORMANCE_WINDOW_HOURS)
      ),
    staleTime: 60 * 1000,
    retry: false,
    meta: { errorToast: false },
  })

  const models = useMemo(
    () =>
      Array.isArray(metricsQuery.data?.data?.models)
        ? metricsQuery.data.data.models
        : [],
    [metricsQuery.data]
  )

  const perfSummary = useMemo(() => {
    return {
      avgLatencyMs: Math.round(
        simpleAverage(
          models,
          'avg_latency_ms',
          (v) => Number.isFinite(v) && v > 0
        )
      ),
      successRate: simpleAverage(models, 'success_rate', Number.isFinite),
    }
  }, [models])

  const recentUsage = useMemo(
    () =>
      (usageTrendQuery.data?.data ?? []).reduce(
        (total, item) => total + (Number(item.quota) || 0),
        0
      ),
    [usageTrendQuery.data?.data]
  )

  const healthLevel = getHealthLevel(remainQuota, recentUsage)
  const healthCfg = HEALTH_CONFIG[healthLevel]
  const runwayDays = getRunwayDays(remainQuota, recentUsage)

  const remainCompactDisplay = formatQuotaWithCredit(remainQuota, {
    compact: true,
    abbreviate: false,
  })
  const remainFullDisplay = formatQuotaWithCredit(remainQuota, {
    abbreviate: false,
    digitsLarge: 0,
    digitsSmall: 2,
  })

  let runwayDisplay: string
  if (runwayDays !== null) {
    if (runwayDays < 1) {
      runwayDisplay = t('Less than 1 day left')
    } else if (runwayDays > 999) {
      runwayDisplay = `999+ ${t('days')}`
    } else {
      runwayDisplay = `~${formatNumber(Math.floor(runwayDays))} ${t('days')}`
    }
  } else if (remainQuota <= 0) {
    runwayDisplay = t('Balance depleted')
  } else {
    runwayDisplay = `999+ ${t('days')}`
  }

  const requestCountDisplay = loading ? '—' : formatNumber(requestCount)

  return (
    <div className='bg-card overflow-hidden rounded-2xl border px-5 py-4 shadow-xs sm:px-6 sm:py-5'>
      <div className='flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between'>
        <div className='min-w-0 flex-1'>
          <div className='flex items-center gap-2'>
            <span className='text-muted-foreground text-xs'>
              {t('Credit remaining')}
            </span>
            <span
              className={cn(
                'rounded-full px-2 py-0.5 text-[11px] font-medium',
                healthLevel === 'critical'
                  ? 'bg-red-500/15 text-red-400'
                  : healthLevel === 'caution'
                    ? 'bg-amber-500/15 text-amber-400'
                    : 'bg-emerald-500/15 text-emerald-400'
              )}
            >
              {t(healthCfg.labelKey)}
            </span>
          </div>
          <div className='mt-1.5 text-2xl font-bold tracking-tight tabular-nums sm:text-3xl'>
            {remainCompactDisplay}
          </div>
          <div className='text-muted-foreground mt-1 text-xs tabular-nums'>
            {remainFullDisplay} · {t('runway prefix')} {runwayDisplay}
          </div>
        </div>

        <div className='border-border/60 hidden h-14 border-l lg:block' />

        <div className='min-w-[8rem] lg:px-4'>
          <div className='text-muted-foreground text-xs'>
            {t('Total Count')}
          </div>
          <div className='mt-1.5 text-xl font-bold tracking-tight tabular-nums sm:text-2xl'>
            {requestCountDisplay}
          </div>
        </div>

        <div className='border-border/60 hidden h-14 border-l lg:block' />

        <div className='min-w-[9rem] lg:px-4'>
          <div className='text-muted-foreground text-xs leading-snug'>
            {t('Success rate')} · 24 {t('hours')}
          </div>
          <div
            className={cn(
              'mt-1.5 text-xl font-bold tracking-tight tabular-nums sm:text-2xl',
              getSuccessRateTextClass(perfSummary.successRate)
            )}
          >
            {metricsQuery.isLoading
              ? '—'
              : formatUptimePct(perfSummary.successRate)}
          </div>
        </div>

        <div className='border-border/60 hidden h-14 border-l lg:block' />

        <div className='flex items-center justify-between gap-4 lg:pl-4'>
          <div className='min-w-[9rem]'>
            <div className='text-muted-foreground text-xs leading-snug'>
              {t('Average latency')} · 24 {t('hours')}
            </div>
            <div className='mt-1.5 text-xl font-bold tracking-tight tabular-nums sm:text-2xl'>
              {metricsQuery.isLoading
                ? '—'
                : formatLatency(perfSummary.avgLatencyMs)}
            </div>
          </div>

          <Button
            className='h-9 shrink-0 rounded-xl bg-blue-600 px-4 text-xs font-semibold text-white hover:bg-blue-500'
            render={<Link to='/wallet' />}
          >
            {t('Open wallet')}
          </Button>
        </div>
      </div>
    </div>
  )
}
