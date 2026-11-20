import { Link } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { formatNumber, formatPercent } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { AdminSummaryData, AdminTopupMethodStat } from '../../../types'
import { VND_PER_CREDIT } from '../credit-vnd'
import { percentChange } from '../overview-metrics'
import {
  previousComparableRange,
  toUnixRange,
  type TimeRange,
} from '../overview-time-range'
import { useAdminSummary } from './use-admin-overview'

const CREDIT_FORMAT = {
  abbreviate: false,
  digitsLarge: 2,
  digitsSmall: 4,
} as const

// Error rate above this share of requests is highlighted.
const ERROR_RATE_ALERT_PERCENT = 5

const METHOD_LABELS: Record<string, string> = {
  sepay: 'SePay',
  usdt: 'USDT',
  binance: 'USDT',
}

function topupMethodLabel(method: string): string {
  return METHOD_LABELS[method.toLowerCase()] ?? method
}

// "SePay 1,200,000 VND", "USDT 50": the currency is omitted when it is already
// the method label.
function formatMethodAmount(
  item: AdminTopupMethodStat,
  locale: string
): string {
  const label = topupMethodLabel(item.method)
  const currency = item.currency.toUpperCase()
  const amount = formatNumber(item.amount, locale)
  return currency === label.toUpperCase()
    ? `${label} ${amount}`
    : `${label} ${amount} ${currency}`
}

// VND amounts from the orders, shown as credits at the fixed site rate
// (1 credit = 1,000 VND). This must not go through the USD-based currency
// formatter: with a custom display currency it would apply that rate again.
function formatVndAsCredits(vnd: number, locale: string): string {
  return `${formatNumber(vnd / VND_PER_CREDIT, locale)} credit`
}

// Changes beyond this are shown as ">999%" instead of a long number.
const MAX_SHOWN_CHANGE_PERCENT = 999

function errorRatePercent(requests: AdminSummaryData['requests']): number | null {
  return requests.total > 0 ? (requests.errors / requests.total) * 100 : null
}

// Pending top-up banner plus four KPI cards, each compared with the same
// window one period earlier (previousComparableRange).
export function AdminSummaryCards(props: {
  range: TimeRange | null
  excludeInternal?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const previousUnixRange = useMemo(
    () =>
      props.range ? toUnixRange(previousComparableRange(props.range)) : null,
    [props.range]
  )
  const query = useAdminSummary(unixRange, props.excludeInternal)
  const previousQuery = useAdminSummary(previousUnixRange, props.excludeInternal)
  const data = query.data
  // The comparison is optional: without it the cards simply show no delta.
  const previous = previousQuery.data

  if (!props.range) {
    return <Message>{t('Choose a valid date range to see usage.')}</Message>
  }
  if (query.isError) {
    return (
      <p role='alert' className='text-muted-foreground py-6 text-center text-sm'>
        {t('Could not load system summary.')}
      </p>
    )
  }
  if (!data) {
    return <Message>{t('Loading')}</Message>
  }

  const { topup, users, requests, usage } = data
  const errorRate = errorRatePercent(requests)
  const errorRateHigh = errorRate !== null && errorRate > ERROR_RATE_ALERT_PERCENT

  return (
    <div className='flex flex-col gap-3'>
      {topup.pending_count > 0 && (
        <Link
          to='/topup-management'
          role='alert'
          className='border-warning/50 bg-warning/10 hover:bg-warning/15 flex items-center gap-3 rounded-2xl border px-4 py-3 text-sm font-medium transition-colors'
        >
          <AlertTriangle className='text-warning size-5 shrink-0' aria-hidden='true' />
          <span className='flex-1'>
            {t('{{count}} top-up orders need review', {
              count: topup.pending_count,
            })}
          </span>
          <span className='text-warning text-xs font-semibold'>{t('Review')}</span>
        </Link>
      )}

      <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
        {/* Money customers paid (SePay VND, USDT at the order rate), not the
            credited quota: bonus credits are not revenue. */}
        <KpiCard
          label={t('Top-up revenue')}
          value={formatVndAsCredits(topup.paid_vnd, locale)}
          change={previous ? percentChange(topup.paid_vnd, previous.topup.paid_vnd) : null}
        >
          <span className='text-muted-foreground text-xs tabular-nums'>
            {t('≈ {{amount}} VND · {{count}} orders', {
              amount: formatNumber(topup.paid_vnd, locale),
              count: formatNumber(topup.count, locale),
            })}
          </span>
          {topup.by_method.length > 0 && (
            <ul className='text-muted-foreground flex flex-col gap-0.5 text-xs tabular-nums'>
              {topup.by_method.map((item) => (
                <li key={`${item.method}-${item.currency}`}>
                  {formatMethodAmount(item, locale)}
                </li>
              ))}
            </ul>
          )}
          {topup.bonus_vnd > 0 && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('+ {{amount}} bonus granted', {
                amount: formatVndAsCredits(topup.bonus_vnd, locale),
              })}
            </span>
          )}
        </KpiCard>

        <KpiCard
          label={t('Credits customers used')}
          value={formatQuotaWithCredit(usage.consumed_quota, CREDIT_FORMAT)}
          change={
            previous
              ? percentChange(usage.consumed_quota, previous.usage.consumed_quota)
              : null
          }
        />

        <KpiCard
          label={t('Active customers')}
          value={formatNumber(users.active_users, locale)}
          change={
            previous
              ? percentChange(users.active_users, previous.users.active_users)
              : null
          }
        >
          <span className='text-muted-foreground text-xs tabular-nums'>
            {t('{{count}} new signups ({{google}} via Google)', {
              count: formatNumber(users.new_users, locale),
              google: formatNumber(users.new_google_users, locale),
            })}
          </span>
          {users.signup_quota_granted > 0 && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('Granted {{amount}} at signup', {
                amount: formatQuotaWithCredit(users.signup_quota_granted, CREDIT_FORMAT),
              })}
            </span>
          )}
        </KpiCard>

        <KpiCard
          label={t('Requests')}
          value={formatNumber(requests.total, locale)}
          change={previous ? percentChange(requests.total, previous.requests.total) : null}
        >
          <span
            className={cn(
              'text-xs tabular-nums',
              errorRateHigh ? 'text-destructive font-semibold' : 'text-muted-foreground'
            )}
          >
            {t('Error rate {{rate}}', {
              rate: errorRate === null ? '-' : formatPercent(errorRate),
            })}
            {' · '}
            {t('{{count}} errors', { count: formatNumber(requests.errors, locale) })}
          </span>
        </KpiCard>
      </div>
    </div>
  )
}

function KpiCard(props: {
  label: string
  value: string
  // Percent change against the previous period; null hides the badge.
  change?: number | null
  children?: React.ReactNode
}) {
  const { t } = useTranslation()
  const change = props.change
  return (
    <div className='bg-card flex min-w-0 flex-col gap-1.5 rounded-2xl border p-4 shadow-xs'>
      <div className='text-muted-foreground text-xs font-medium'>{props.label}</div>
      <div className='flex flex-wrap items-baseline gap-x-2 gap-y-1'>
        <span className='text-xl font-semibold tabular-nums break-words'>
          {props.value}
        </span>
        {change != null && Number.isFinite(change) && (
          <span
            className={cn(
              'text-xs font-medium tabular-nums',
              change >= 0 ? 'text-success' : 'text-destructive'
            )}
            title={t('Compared with the previous period')}
          >
            {change >= 0 ? '▲' : '▼'}{' '}
            {Math.abs(change) > MAX_SHOWN_CHANGE_PERCENT
              ? `>${MAX_SHOWN_CHANGE_PERCENT}%`
              : formatPercent(Math.abs(change))}
          </span>
        )}
      </div>
      {props.children}
    </div>
  )
}

function Message(props: { children: React.ReactNode }) {
  return (
    <p className='text-muted-foreground py-6 text-center text-sm'>
      {props.children}
    </p>
  )
}
