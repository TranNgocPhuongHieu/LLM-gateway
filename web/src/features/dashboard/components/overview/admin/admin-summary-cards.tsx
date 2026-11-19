import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { formatNumber, formatPercent, quotaUnitsToDollars } from '@/lib/format'

import type { AdminTopupMethodStat } from '../../../types'
import { VND_PER_CREDIT } from '../credit-vnd'
import { toUnixRange, type TimeRange } from '../overview-time-range'
import { useAdminSummary } from './use-admin-overview'

const CREDIT_FORMAT = {
  abbreviate: false,
  digitsLarge: 2,
  digitsSmall: 4,
} as const

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

// Quota stored in the ledger, shown as credits with the VND equivalent.
function creditsToVnd(quota: number): number {
  return Math.round(quotaUnitsToDollars(quota) * VND_PER_CREDIT)
}

export function AdminSummaryCards(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const query = useAdminSummary(unixRange)
  const data = query.data

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

  const { topup, users, requests } = data
  const errorRate =
    requests.total > 0
      ? formatPercent((requests.errors / requests.total) * 100)
      : '-'

  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-5'>
      <KpiCard
        label={t('Revenue')}
        value={formatQuotaWithCredit(topup.credited_quota, CREDIT_FORMAT)}
      >
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('≈ {{amount}} VND', {
            amount: formatNumber(creditsToVnd(topup.credited_quota), locale),
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
      </KpiCard>

      <KpiCard label={t('Top-ups')} value={formatNumber(topup.count, locale)}>
        {topup.pending_count > 0 && (
          <Link
            to='/topup-management'
            className='text-warning hover:underline w-fit text-xs font-medium'
          >
            {t('{{count}} pending', { count: topup.pending_count })}
          </Link>
        )}
      </KpiCard>

      <KpiCard label={t('New signups')} value={formatNumber(users.new_users, locale)}>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('{{count}} via Google', { count: users.new_google_users })}
        </span>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('Granted {{amount}} at signup', {
            amount: formatQuotaWithCredit(users.signup_quota_granted, CREDIT_FORMAT),
          })}
        </span>
      </KpiCard>

      <KpiCard
        label={t('Active customers')}
        value={formatNumber(users.active_users, locale)}
      />

      <KpiCard label={t('Requests')} value={formatNumber(requests.total, locale)}>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('Error rate {{rate}}', { rate: errorRate })}
        </span>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('{{count}} errors', { count: requests.errors })}
        </span>
      </KpiCard>
    </div>
  )
}

function KpiCard(props: {
  label: string
  value: string
  children?: React.ReactNode
}) {
  return (
    <div className='bg-card flex min-w-0 flex-col gap-1.5 rounded-2xl border p-4 shadow-xs'>
      <div className='text-muted-foreground text-xs font-medium'>{props.label}</div>
      <div className='text-xl font-semibold tabular-nums break-words'>
        {props.value}
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
