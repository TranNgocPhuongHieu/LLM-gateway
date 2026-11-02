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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { getUserQuotaDates } from '@/features/dashboard/api'
import { useModelStatCardsConfig } from '@/features/dashboard/hooks/use-dashboard-config'
import {
  buildQueryParams,
  calculateDashboardStats,
  getDefaultDays,
} from '@/features/dashboard/lib'
import type {
  QuotaDataItem,
  DashboardFilters,
} from '@/features/dashboard/types'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCredit } from '@/lib/currency'
import { formatCompactNumber, formatNumber } from '@/lib/format'
import { computeTimeRange } from '@/lib/time'
import { cn } from '@/lib/utils'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

interface LogStatCardsProps {
  filters?: DashboardFilters
  onDataUpdate?: (data: QuotaDataItem[], loading: boolean) => void
}

const MAX_INLINE_STAT_CHARS = 9

function formatStatNumber(value: number, locale: Intl.LocalesArgument) {
  const fullValue = formatNumber(value, locale)
  const displayValue =
    fullValue.length > MAX_INLINE_STAT_CHARS
      ? formatCompactNumber(value, locale)
      : fullValue

  return {
    displayValue,
    fullValue,
  }
}

export function LogStatCards(props: LogStatCardsProps) {
  const { i18n } = useTranslation()
  const statCardsConfig = useModelStatCardsConfig()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = !!(user?.role && user.role >= ROLE.SUPER_ADMIN)
  const [stats, setStats] = useState<{
    totalQuota: number
    totalCount: number
    totalTokens: number
  } | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const [timeRangeMinutes, setTimeRangeMinutes] = useState(0)

  const { filters, onDataUpdate } = props

  useEffect(() => {
    const abortController = new AbortController()
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)

    setError(false)
    onDataUpdate?.([], true)

    const timeRange = computeTimeRange(
      getDefaultDays(filters?.time_granularity),
      filters?.start_timestamp,
      filters?.end_timestamp
    )
    const timeDiff = (timeRange.end_timestamp - timeRange.start_timestamp) / 60
    setTimeRangeMinutes(timeDiff)

    void getUserQuotaDates(buildQueryParams(timeRange, filters), isAdmin)
      .then((res) => {
        if (abortController.signal.aborted) return
        const data = res?.data || []
        setStats(calculateDashboardStats(data))
        onDataUpdate?.(data, false)
      })
      .catch(() => {
        if (abortController.signal.aborted) return
        setStats(null)
        setError(true)
        onDataUpdate?.([], false)
      })
      .finally(() => {
        if (!abortController.signal.aborted) {
          setLoading(false)
        }
      })

    return () => {
      abortController.abort()
    }
  }, [filters, isAdmin, onDataUpdate])

  const adaptedStats = {
    rpm: stats?.totalCount ?? 0,
    quota: stats?.totalQuota ?? 0,
    tpm: stats?.totalTokens ?? 0,
  }

  const items = statCardsConfig.map((config) => {
    const rawValue = config.getValue(adaptedStats, timeRangeMinutes)
    const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
    const formattedQuota = formatQuotaWithCredit(rawValue, {
      abbreviate: false,
    })
    const formatted =
      config.key === 'quota'
        ? {
            displayValue: formattedQuota,
            fullValue: formattedQuota,
          }
        : formatStatNumber(rawValue, locale)

    return {
      title: config.title,
      value: formatted.displayValue,
      fullValue: formatted.fullValue,
      desc: config.description,
    }
  })

  return (
    <div>
      <div className='grid min-w-0 grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5'>
        {items.map((it, idx) => {
          let valueContent
          if (loading) {
            valueContent = (
              <div className='mt-2 flex flex-col gap-1.5'>
                <Skeleton className='h-6 w-20 sm:h-7 sm:w-24' />
                {it.desc && <Skeleton className='hidden h-3.5 w-28 md:block' />}
              </div>
            )
          } else if (error) {
            valueContent = (
              <>
                <div className='text-muted-foreground mt-2 text-base leading-tight font-bold tracking-tight tabular-nums sm:text-2xl sm:leading-normal'>
                  --
                </div>
                {it.desc && (
                  <div className='text-muted-foreground/40 mt-1.5 hidden text-xs md:block'>
                    {it.desc}
                  </div>
                )}
              </>
            )
          } else {
            valueContent = (
              <>
                <div
                  className='text-foreground mt-2 max-w-full truncate text-base leading-tight font-bold tracking-tight tabular-nums sm:text-2xl sm:leading-normal'
                  title={it.fullValue}
                >
                  {it.value}
                </div>
                {it.desc && (
                  <div className='text-muted-foreground mt-1.5 hidden text-xs md:block'>
                    {it.desc}
                  </div>
                )}
              </>
            )
          }

          return (
            <div
              key={it.title}
              className={cn(
                'bg-card min-w-0 rounded-2xl border px-4 py-4 sm:p-5',
                idx === items.length - 1 &&
                  items.length % 2 !== 0 &&
                  'col-span-2 sm:col-span-1'
              )}
            >
              <div className='text-muted-foreground truncate text-xs font-medium'>
                {it.title}
              </div>

              {valueContent}
            </div>
          )
        })}
      </div>
    </div>
  )
}
