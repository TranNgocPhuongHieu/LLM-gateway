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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { formatNumber, quotaUnitsToDollars } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { computeSpendKpis, type HealthLevel } from './overview-metrics'
import { useCurrentMinute, useSpendKpiQueries } from './use-overview-usage'
import { VND_PER_CREDIT } from './credit-vnd'

const HEALTH_LABEL_KEY: Record<HealthLevel, string> = {
  healthy: 'Healthy',
  caution: 'Caution',
  critical: 'Low balance',
}

const HEALTH_BADGE_CLASS: Record<HealthLevel, string> = {
  healthy: 'bg-emerald-500/15 text-emerald-400',
  caution: 'bg-amber-500/15 text-amber-400',
  critical: 'bg-red-500/15 text-red-400',
}

const CREDIT_FORMAT = {
  abbreviate: false,
  digitsLarge: 2,
  digitsSmall: 4,
} as const

function KpiCard(props: {
  label: string
  children: React.ReactNode
  footer?: React.ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'bg-card flex min-w-0 flex-col gap-1.5 rounded-2xl border p-4 shadow-xs sm:p-5',
        props.className
      )}
    >
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      {props.children}
      {props.footer}
    </div>
  )
}

export function SummaryCards() {
  const { t, i18n } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const now = useCurrentMinute()
  const balanceQuota = Number(user?.quota ?? 0)
  const spend = useSpendKpiQueries(now)
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)

  const kpis = computeSpendKpis({
    balanceQuota,
    todayQuota: spend.todayQuota ?? 0,
    monthQuota: spend.monthQuota ?? 0,
    lastMonthSamePeriodQuota: spend.lastMonthSamePeriodQuota ?? 0,
    last7DaysQuota: spend.last7DaysQuota ?? 0,
    now,
  })
  // Health and run-out use the 7-day average; until it loads they assume none.
  const healthLabel = t(HEALTH_LABEL_KEY[kpis.health])
  const balanceCredits = quotaUnitsToDollars(balanceQuota)
  const balanceVnd = Math.round(balanceCredits * VND_PER_CREDIT)

  // Undefined means the request is pending or failed; never show it as zero.
  const quotaText = (quota: number | undefined) =>
    quota === undefined ? '—' : formatQuotaWithCredit(quota, CREDIT_FORMAT)

  const changeText =
    kpis.monthChangePct === null
      ? null
      : `${new Intl.NumberFormat(locale, {
          signDisplay: 'exceptZero',
          maximumFractionDigits: 1,
        }).format(kpis.monthChangePct)}%`

  let runOutValue: string
  let runOutHint: string
  switch (kpis.runOut.kind) {
    case 'depleted':
      runOutValue = t('Balance depleted')
      runOutHint = t('Top up to keep sending requests')
      break
    case 'no-usage':
      runOutValue = t('No recent usage')
      runOutHint = t('Based on the last {{days}} days', { days: 7 })
      break
    case 'date':
      runOutValue = new Intl.DateTimeFormat(locale, {
        dateStyle: 'medium',
      }).format(kpis.runOut.date)
      runOutHint = t('About {{days}} days at the last 7-day average', {
        days: formatNumber(Math.floor(kpis.runOut.days), locale),
      })
      break
  }

  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
      <KpiCard
        label={t('Credit remaining')}
        className='sm:col-span-2 xl:col-span-1'
        footer={
          <Button
            className='mt-2 h-9 w-fit rounded-xl bg-blue-600 px-4 text-xs font-semibold text-white hover:bg-blue-500'
            render={<Link to='/wallet' />}
          >
            {t('Open wallet')}
          </Button>
        }
      >
        <div className='flex flex-wrap items-center gap-2'>
          <span
            className={cn(
              'rounded-full px-2 py-0.5 text-[11px] font-medium',
              HEALTH_BADGE_CLASS[kpis.health]
            )}
          >
            {healthLabel}
          </span>
        </div>
        <div className='text-2xl font-bold tracking-tight tabular-nums sm:text-3xl'>
          {formatQuotaWithCredit(balanceQuota, {
            compact: true,
            abbreviate: false,
          })}
        </div>
        <div className='text-muted-foreground text-xs tabular-nums'>
          ≈ {formatNumber(balanceVnd, locale)} VND
        </div>
      </KpiCard>

      <KpiCard label={t('Spend today')}>
        <div className='text-xl font-bold tracking-tight tabular-nums sm:text-2xl'>
          {quotaText(spend.todayQuota)}
        </div>
        {spend.isError && (
          <div className='text-muted-foreground text-xs'>
            {t('Could not load usage. Try again later.')}
          </div>
        )}
      </KpiCard>

      <KpiCard label={t('Spend this month')}>
        <div className='flex flex-wrap items-baseline gap-2'>
          <span className='text-xl font-bold tracking-tight tabular-nums sm:text-2xl'>
            {quotaText(spend.monthQuota)}
          </span>
          {changeText && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {changeText} {t('vs same period last month')}
            </span>
          )}
        </div>
      </KpiCard>

      <KpiCard
        label={t('Projected run-out')}
        footer={
          <div className='text-muted-foreground text-xs'>{runOutHint}</div>
        }
      >
        <div className='text-xl font-bold tracking-tight tabular-nums sm:text-2xl'>
          {spend.last7DaysQuota === undefined ? '—' : runOutValue}
        </div>
      </KpiCard>
    </div>
  )
}
