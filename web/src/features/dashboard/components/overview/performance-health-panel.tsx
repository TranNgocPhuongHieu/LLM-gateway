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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
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
const TOP_MODEL_LIMIT = 6

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

export function PerformanceHealthPanel() {
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

  const summary = useMemo(() => {
    return {
      avgLatencyMs: Math.round(
        simpleAverage(
          models,
          'avg_latency_ms',
          (v) => Number.isFinite(v) && v > 0
        )
      ),
      avgTps: simpleAverage(
        models,
        'avg_tps',
        (v) => Number.isFinite(v) && v > 0
      ),
      successRate: simpleAverage(models, 'success_rate', Number.isFinite),
    }
  }, [models])

  const topModels = useMemo(() => models.slice(0, TOP_MODEL_LIMIT), [models])
  const loading = metricsQuery.isLoading
  const hasData = models.length > 0

  return (
    <section className='bg-card h-full overflow-hidden rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex items-center justify-between gap-2'>
        <h3 className='text-sm font-semibold sm:text-base'>
          {t('Performance health')}
        </h3>
        <span className='text-muted-foreground text-xs'>
          {t('Performance metrics for the last 24 hours')}
        </span>
      </div>

      <div className='mt-4 space-y-4'>
        {metricsQuery.isError ? (
          <ErrorState
            title={t('Failed to load performance metrics')}
            description={
              metricsQuery.error instanceof Error
                ? metricsQuery.error.message
                : undefined
            }
            onRetry={() => metricsQuery.refetch()}
            className='min-h-[160px]'
          />
        ) : (
          <>
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
              <MetricCell
                label={t('Success rate')}
                value={formatUptimePct(summary.successRate)}
                loading={loading}
                valueClassName={getSuccessRateTextClass(summary.successRate)}
              />
              <MetricCell
                label={t('Average latency')}
                value={formatLatency(summary.avgLatencyMs)}
                loading={loading}
              />
              <MetricCell
                label={t('Throughput')}
                value={formatThroughput(summary.avgTps)}
                loading={loading}
              />
            </div>

            {loading ? (
              <div className='grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3'>
                {['m1', 'm2', 'm3'].map((key) => (
                  <Skeleton key={key} className='h-10 w-full rounded-xl' />
                ))}
              </div>
            ) : (
              hasData && (
                <div className='space-y-2.5'>
                  <span className='text-muted-foreground block text-xs'>
                    {t('Top models by traffic')}
                  </span>
                  <div className='grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3'>
                    {topModels.map((model) => (
                      <div
                        key={model.model_name}
                        className='bg-background/40 flex items-center justify-between gap-2 rounded-xl border px-3.5 py-2.5'
                      >
                        <span className='min-w-0 flex-1 truncate font-mono text-xs font-medium'>
                          {model.model_name}
                        </span>
                        <span
                          className={cn(
                            'shrink-0 font-mono text-xs font-semibold tabular-nums',
                            getSuccessRateTextClass(model.success_rate)
                          )}
                        >
                          {formatUptimePct(model.success_rate)}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )
            )}
          </>
        )}
      </div>
    </section>
  )
}

function MetricCell(props: {
  label: string
  value: string
  loading: boolean
  valueClassName?: string
}) {
  return (
    <div className='bg-background/40 rounded-xl border p-4'>
      <div className='text-muted-foreground truncate text-xs'>
        {props.label}
      </div>
      {props.loading ? (
        <Skeleton className='mt-2 h-7 w-24' />
      ) : (
        <div
          className={cn(
            'mt-2 text-xl font-bold tracking-tight tabular-nums sm:text-2xl',
            props.valueClassName
          )}
        >
          {props.value}
        </div>
      )}
    </div>
  )
}
