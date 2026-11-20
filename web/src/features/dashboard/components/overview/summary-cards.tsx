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
import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { formatNumber, formatPercent, quotaUnitsToDollars } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { VND_PER_CREDIT } from './credit-vnd'
import { KpiChange } from './kpi-change'
import { useLowBalance } from './low-balance-banner'
import {
  averageDailyQuota,
  percentChange,
  projectRunOut,
} from './overview-metrics'
import {
  previousComparableRange,
  toUnixRange,
  type TimeRange,
} from './overview-time-range'
import {
  useCurrentMinute,
  useSelfBreakdown,
  useSelfErrorCount,
  useSpendKpiQueries,
} from './use-overview-usage'

const CREDIT_FORMAT = {
  abbreviate: false,
  digitsLarge: 2,
  digitsSmall: 4,
} as const

// Error rate above this share of requests is highlighted.
const ERROR_RATE_ALERT_PERCENT = 5

function KpiCard(props: {
  label: string
  value: React.ReactNode
  change?: number | null
  children?: React.ReactNode
}) {
  return (
    <div className='bg-card flex min-w-0 flex-col gap-1.5 rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='text-muted-foreground text-xs font-medium'>{props.label}</div>
      <div className='flex flex-wrap items-baseline gap-x-2 gap-y-1'>
        <span className='text-2xl font-bold tracking-tight tabular-nums break-words'>
          {props.value}
        </span>
        <KpiChange change={props.change} neutral />
      </div>
      {props.children}
    </div>
  )
}

// Balance plus two cards that follow the page's range: spend and requests,
// each compared with the same window one period earlier. Low-balance warnings
// live only in LowBalanceBanner.
export function SummaryCards(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const user = useAuthStore((state) => state.auth.user)
  const now = useCurrentMinute()
  const balanceQuota = Number(user?.quota ?? 0)

  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const previousUnixRange = useMemo(
    () =>
      props.range ? toUnixRange(previousComparableRange(props.range)) : null,
    [props.range]
  )
  const current = useSelfBreakdown(unixRange, 'model')
  const previous = useSelfBreakdown(previousUnixRange, 'model')
  const errors = useSelfErrorCount(unixRange)
  // While the low-balance banner shows, it carries the runway and Top up.
  const lowBalance = useLowBalance()
  const recent = useSpendKpiQueries(now)

  const balanceCredits = quotaUnitsToDollars(balanceQuota)
  const balanceVnd = Math.round(balanceCredits * VND_PER_CREDIT)
  const runOut = projectRunOut(
    balanceQuota,
    averageDailyQuota(recent.last7DaysQuota ?? 0),
    now
  )
  let runwayText: string | null = null
  if (!lowBalance && runOut.kind === 'date') {
    runwayText =
      runOut.days >= 1
        ? t('Enough for about {{days}} days at your recent spending', {
            days: formatNumber(Math.floor(runOut.days), locale),
          })
        : t('Less than 1 day left at your recent spending')
  }

  // Undefined data means the request is pending or failed; never show zero.
  const spendQuota = current.data?.total.quota
  const successCount = current.data?.total.count
  const errorCount = errors.data
  const requestCount =
    successCount === undefined ? undefined : successCount + (errorCount ?? 0)
  const previousRequests = previous.data?.total.count
  const errorRate =
    requestCount && errorCount !== undefined
      ? (errorCount / requestCount) * 100
      : null
  const errorRateHigh =
    errorRate !== null && errorRate > ERROR_RATE_ALERT_PERCENT

  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
      <KpiCard
        label={t('Credit remaining')}
        value={formatQuotaWithCredit(balanceQuota, {
          compact: true,
          abbreviate: false,
        })}
      >
        <div className='text-muted-foreground text-xs tabular-nums'>
          ≈ {formatNumber(balanceVnd, locale)} VND
        </div>
        {runwayText && (
          <div className='text-muted-foreground text-xs'>{runwayText}</div>
        )}
        {!lowBalance && (
          <Button
            size='sm'
            className='mt-1 w-fit'
            render={<Link to='/wallet' />}
          >
            {t('Top up')}
          </Button>
        )}
      </KpiCard>

      <KpiCard
        label={t('Spend in this period')}
        value={
          spendQuota === undefined
            ? '—'
            : formatQuotaWithCredit(spendQuota, CREDIT_FORMAT)
        }
        change={
          spendQuota !== undefined && previous.data
            ? percentChange(spendQuota, previous.data.total.quota)
            : null
        }
      >
        {current.isError && (
          <div className='text-muted-foreground text-xs'>
            {t('Could not load usage. Try again later.')}
          </div>
        )}
      </KpiCard>

      <KpiCard
        label={t('Requests')}
        value={
          requestCount === undefined ? '—' : formatNumber(requestCount, locale)
        }
        change={
          successCount !== undefined && previousRequests !== undefined
            ? percentChange(successCount, previousRequests)
            : null
        }
      >
        <div
          className={cn(
            'text-xs tabular-nums',
            errorRateHigh ? 'text-destructive font-semibold' : 'text-muted-foreground'
          )}
        >
          {t('Error rate {{rate}}', {
            rate: errorRate === null ? '-' : formatPercent(errorRate, locale),
          })}
          {errorCount !== undefined &&
            ` · ${t('{{count}} errors', { count: formatNumber(errorCount, locale) })}`}
        </div>
      </KpiCard>
    </div>
  )
}
