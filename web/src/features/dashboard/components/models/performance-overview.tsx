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
import { AlertCircle } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { getPerfMetricsSummary } from '@/features/performance-metrics/api'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import type { PerfModelSummary } from '@/features/performance-metrics/types'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

const PERFORMANCE_WINDOW_HOURS = 24

type WeightedMetric = 'avg_latency_ms' | 'avg_tps' | 'success_rate'

type PerformanceSummary = {
  totalRequests: number
  avgLatencyMs: number
  avgTps: number
  successRate: number
}

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

function buildPerformanceSummary(rows: PerfModelSummary[]): PerformanceSummary {
  return {
    totalRequests: rows.length,
    avgLatencyMs: Math.round(
      simpleAverage(
        rows,
        'avg_latency_ms',
        (value) => Number.isFinite(value) && value > 0
      )
    ),
    avgTps: simpleAverage(
      rows,
      'avg_tps',
      (value) => Number.isFinite(value) && value > 0
    ),
    successRate: simpleAverage(rows, 'success_rate', Number.isFinite),
  }
}

export function PerformanceOverview() {
  const { t } = useTranslation()
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
  const summary = useMemo(() => buildPerformanceSummary(models), [models])
  const loading = metricsQuery.isLoading

  if (metricsQuery.isError) {
    return (
      <div className='flex flex-wrap items-center justify-between gap-2 overflow-hidden rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-2.5 text-xs text-destructive sm:px-5 sm:py-3'>
        <div className='flex items-center gap-2'>
          <AlertCircle className='size-4' />
          <span>{t('Failed to load performance metrics')}</span>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => metricsQuery.refetch()}
          className='h-7 text-xs'
        >
          {t('Retry')}
        </Button>
      </div>
    )
  }

  return (
    <div className='bg-card overflow-hidden rounded-2xl border'>
      <div className='flex flex-wrap items-center gap-x-6 gap-y-2.5 px-4 py-3 sm:px-5 sm:py-3.5'>
        <span className='text-xs font-semibold whitespace-nowrap sm:text-sm'>
          {t('Performance health')}
        </span>

        {loading ? (
          <div className='flex flex-wrap items-center gap-x-6 gap-y-2'>
            {['success', 'latency', 'throughput'].map((key) => (
              <div key={key} className='flex items-center gap-1.5'>
                <Skeleton className='h-3.5 w-20' />
                <Skeleton className='h-4 w-14' />
              </div>
            ))}
          </div>
        ) : (
          <div className='flex flex-wrap items-center gap-x-6 gap-y-2'>
            <InlineMetric
              label={t('Success rate')}
              value={formatUptimePct(summary.successRate)}
              valueClassName={getSuccessRateTextClass(summary.successRate)}
            />
            <InlineMetric
              label={t('Average latency')}
              value={formatLatency(summary.avgLatencyMs)}
            />
            <InlineMetric
              label={t('Throughput')}
              value={formatThroughput(summary.avgTps)}
            />
          </div>
        )}
      </div>
    </div>
  )
}

function InlineMetric(props: {
  label: string
  value: string
  valueClassName?: string
}) {
  return (
    <div className='flex items-center gap-1.5 text-xs sm:text-sm'>
      <span className='text-muted-foreground'>{props.label}</span>
      <span
        className={cn(
          'font-semibold tabular-nums',
          props.valueClassName ?? 'text-foreground'
        )}
      >
        {props.value}
      </span>
    </div>
  )
}
