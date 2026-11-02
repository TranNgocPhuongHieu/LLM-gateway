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
import { Users, Loader2, KeyRound } from 'lucide-react'
import { useMemo, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useChartTheme } from '@/lib/use-chart-theme'
import {
  getFlowQuotaDates,
  getUserQuotaDataByUsers,
} from '@/features/dashboard/api'
import {
  TIME_GRANULARITY_OPTIONS,
  TIME_RANGE_PRESETS,
} from '@/features/dashboard/constants'
import {
  getDefaultDays,
  saveGranularity,
  processUserChartData,
} from '@/features/dashboard/lib'
import type {
  ProcessedUserChartData,
  UserChartsFilters,
} from '@/features/dashboard/types'
import { getRollingDateRange, type TimeGranularity } from '@/lib/time'
import { VChart, VCHART_OPTION, type ISpec } from '@/lib/vchart'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const USER_CHARTS: {
  value: string
  labelKey: string
  specKey: keyof ProcessedUserChartData
}[] = [
  {
    value: 'rank',
    labelKey: 'User Consumption Ranking',
    specKey: 'spec_user_rank',
  },
  {
    value: 'trend',
    labelKey: 'User Consumption Trend',
    specKey: 'spec_user_trend',
  },
]

const TOP_USER_LIMIT_OPTIONS = [5, 10, 20, 50]

interface UserChartsProps {
  filters: UserChartsFilters
  onFiltersChange: (filters: UserChartsFilters) => void
}

export function UserCharts(props: UserChartsProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const canViewCrossUserUsage = role >= ROLE.SUPER_ADMIN

  // The selection is owned by the dashboard parent so it persists across
  // sub-section switches; the rolling window is derived from the chosen range.
  const timeGranularity = props.filters.timeGranularity
  const selectedRange = props.filters.selectedRange
  const topUserLimit = props.filters.topUserLimit
  const onFiltersChange = props.onFiltersChange

  const timeRange = useMemo(() => {
    const { start, end } = getRollingDateRange(selectedRange)
    return {
      start_timestamp: Math.floor(start.getTime() / 1000),
      end_timestamp: Math.floor(end.getTime() / 1000),
    }
  }, [selectedRange])

  const handleRangeChange = useCallback(
    (days: number) => {
      onFiltersChange({ ...props.filters, selectedRange: days })
    },
    [onFiltersChange, props.filters]
  )

  const handleGranularityChange = useCallback(
    (g: TimeGranularity) => {
      saveGranularity(g)
      onFiltersChange({
        ...props.filters,
        timeGranularity: g,
        selectedRange: getDefaultDays(g),
      })
    },
    [onFiltersChange, props.filters]
  )

  const handleTopUserLimitChange = useCallback(
    (limit: number) => {
      onFiltersChange({ ...props.filters, topUserLimit: limit })
    },
    [onFiltersChange, props.filters]
  )

  const {
    data: userData,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: ['dashboard', 'user-quota', timeRange],
    queryFn: async () => {
      const res = await getUserQuotaDataByUsers(timeRange)
      if (!res?.success) {
        throw new Error(res?.message || 'Failed to load user consumption data')
      }
      return Array.isArray(res?.data) ? res.data : []
    },
    staleTime: 60_000,
    retry: 1,
    meta: { errorToast: false },
  })

  const flowQuery = useQuery({
    queryKey: ['dashboard', 'user-flow', timeRange],
    enabled: canViewCrossUserUsage,
    queryFn: async () => {
      const res = await getFlowQuotaDates(timeRange, true)
      if (!res?.success) {
        throw new Error(res?.message || 'Failed to load API key usage data')
      }
      return Array.isArray(res?.data) ? res.data : []
    },
    staleTime: 60_000,
    retry: 1,
    meta: { errorToast: false },
  })

  const hasData = Array.isArray(userData) && userData.length > 0
  const flowRows = useMemo(
    () => (flowQuery.data ?? []).slice(0, 100),
    [flowQuery.data]
  )

  const chartData = useMemo(
    () =>
      processUserChartData(
        isLoading || !hasData ? [] : userData,
        timeGranularity,
        t,
        topUserLimit
      ),
    [userData, isLoading, hasData, timeGranularity, t, topUserLimit]
  )

  const filterControls = (
    <div className='flex items-center gap-1.5 overflow-x-auto pb-1 sm:gap-2'>
      <Tabs
        value={String(selectedRange)}
        onValueChange={(value) => handleRangeChange(Number(value))}
        className='shrink-0'
      >
        <TabsList>
          {TIME_RANGE_PRESETS.map((preset) => (
            <TabsTrigger
              key={preset.days}
              value={String(preset.days)}
              className='px-2.5 text-xs'
            >
              {t(preset.label)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <Tabs
        value={timeGranularity}
        onValueChange={(value) =>
          handleGranularityChange(value as TimeGranularity)
        }
        className='shrink-0'
      >
        <TabsList>
          {TIME_GRANULARITY_OPTIONS.map((opt) => (
            <TabsTrigger
              key={opt.value}
              value={opt.value}
              className='px-2.5 text-xs'
            >
              {t(opt.label)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <Tabs
        value={String(topUserLimit)}
        onValueChange={(value) => handleTopUserLimitChange(Number(value))}
        className='shrink-0'
      >
        <TabsList>
          <span className='text-muted-foreground px-2 text-xs font-medium whitespace-nowrap'>
            {t('Top Users')}
          </span>
          {TOP_USER_LIMIT_OPTIONS.map((limit) => (
            <TabsTrigger
              key={limit}
              value={String(limit)}
              className='px-2.5 text-xs'
            >
              {t('Top {{count}}', { count: limit })}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {isLoading && (
        <Loader2 className='text-muted-foreground size-4 animate-spin' />
      )}
    </div>
  )

  if (isError) {
    return (
      <div className='space-y-3'>
        {filterControls}
        <ErrorState
          title={t('Failed to load user consumption data')}
          description={
            error instanceof Error
              ? error.message
              : t('An unexpected error occurred while loading charts.')
          }
          onRetry={() => refetch()}
          className='min-h-[250px] rounded-lg border'
        />
      </div>
    )
  }

  if (!isLoading && !hasData) {
    return (
      <div className='space-y-3'>
        {filterControls}
        <EmptyState
          icon={Users}
          title={t('No user consumption data')}
          description={t(
            'No consumption recorded for any users in the selected time range.'
          )}
          className='min-h-[250px] rounded-lg border'
        />
      </div>
    )
  }

  return (
    <div className='space-y-3'>
      {filterControls}

      <div className='grid gap-3'>
        {USER_CHARTS.map((chart) => {
          const spec = chartData[chart.specKey]

          return (
            <div
              key={chart.value}
              className='overflow-hidden rounded-lg border'
            >
              <div className='flex w-full items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
                <IconBadge tone='info' size='sm'>
                  <Users />
                </IconBadge>
                <div className='text-sm font-semibold'>{t(chart.labelKey)}</div>
              </div>

              <div className='h-[300px] p-1.5 sm:h-96 sm:p-2'>
                {isLoading ? (
                  <Skeleton className='h-full w-full' />
                ) : (
                  themeReady &&
                  spec && (
                    <VChart
                      key={`user-${chart.value}-${topUserLimit}-${resolvedTheme}`}
                      spec={
                        {
                          ...spec,
                          theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                          background: 'transparent',
                        } as ISpec
                      }
                      options={VCHART_OPTION}
                    />
                  )
                )}
              </div>
            </div>
          )
        })}
      </div>

      {canViewCrossUserUsage && (
        <section className='overflow-hidden rounded-lg border'>
          <div className='flex items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
            <IconBadge tone='info' size='sm'>
              <KeyRound />
            </IconBadge>
            <div>
              <div className='text-sm font-semibold'>
                {t('Usage by user, API key and model')}
              </div>
              <div className='text-muted-foreground text-xs'>
                {t('Only Super Admins can view cross-user usage details.')}
              </div>
            </div>
          </div>
          <div className='overflow-x-auto'>
            <table className='w-full min-w-[720px] text-sm'>
              <thead className='bg-muted/30 text-muted-foreground text-left text-xs'>
                <tr>
                  <th className='px-3 py-2 font-medium sm:px-5'>{t('User')}</th>
                  <th className='px-3 py-2 font-medium'>{t('API key')}</th>
                  <th className='px-3 py-2 font-medium'>{t('Model')}</th>
                  <th className='px-3 py-2 text-right font-medium'>{t('Requests')}</th>
                  <th className='px-3 py-2 text-right font-medium'>{t('Credits used')}</th>
                </tr>
              </thead>
              <tbody className='divide-border/60 divide-y'>
                {flowQuery.isLoading ? (
                  <tr><td colSpan={5} className='px-3 py-6 text-center sm:px-5'>{t('Loading')}</td></tr>
                ) : flowRows.length === 0 ? (
                  <tr><td colSpan={5} className='text-muted-foreground px-3 py-6 text-center sm:px-5'>{t('No API key usage in this range')}</td></tr>
                ) : (
                  flowRows.map((row, index) => (
                    <tr key={`${row.user_id ?? 'u'}-${row.token_id ?? 't'}-${row.model_name ?? 'm'}-${index}`} className='hover:bg-muted/20'>
                      <td className='px-3 py-2.5 sm:px-5'>{row.username || `#${row.user_id ?? '—'}`}</td>
                      <td className='text-muted-foreground px-3 py-2.5'>{row.token_name || (row.token_id ? `#${row.token_id}` : t('Unknown'))}</td>
                      <td className='px-3 py-2.5 font-mono text-xs'>{row.model_name || '—'}</td>
                      <td className='px-3 py-2.5 text-right tabular-nums'>{Number(row.count ?? 0).toLocaleString()}</td>
                      <td className='px-3 py-2.5 text-right tabular-nums'>{Number(row.quota ?? 0).toLocaleString()}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  )
}
