import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { SelfBreakdownGroupBy } from '../../types'
import { getBreakdownRows, type BreakdownRow } from './overview-metrics'
import { toUnixRange, type TimeRange } from './overview-time-range'
import { useSelfBreakdown } from './use-overview-usage'

const CREDIT_FORMAT = { abbreviate: false, digitsLarge: 2, digitsSmall: 4 } as const

export function SpendBreakdownCard(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const [groupBy, setGroupBy] = useState<SelfBreakdownGroupBy>('model')
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const query = useSelfBreakdown(unixRange, groupBy)
  const locale = i18n.resolvedLanguage || i18n.language

  const rows: BreakdownRow[] = useMemo(
    () =>
      getBreakdownRows(query.data, groupBy, (keyId) =>
        t('Key #{{id}} (deleted)', { id: keyId })
      ),
    [groupBy, query.data, t]
  )

  const tabs: { value: SelfBreakdownGroupBy; label: string; heading: string }[] = [
    { value: 'model', label: t('By model'), heading: t('Spend by model') },
    { value: 'token', label: t('By API key'), heading: t('Spend by API key') },
  ]
  const active = tabs.find((tab) => tab.value === groupBy) ?? tabs[0]
  const nameHeader = groupBy === 'model' ? t('Model') : t('API key')

  let body: React.ReactNode
  if (!props.range) {
    body = <Message>{t('Choose a valid date range to see usage.')}</Message>
  } else if (query.isError) {
    body = <Message>{t('Could not load usage. Try again later.')}</Message>
  } else if (query.isPending) {
    body = <Message>{t('Loading')}</Message>
  } else if (rows.length === 0) {
    body = <Message>{t('No data')}</Message>
  } else {
    body = (
      <div className='overflow-x-auto'>
        <table className='w-full text-left text-xs sm:text-sm'>
          <thead className='text-muted-foreground text-xs'>
            <tr>
              <th className='px-2.5 py-2 font-medium'>{nameHeader}</th>
              <th className='px-2.5 py-2 text-right font-medium'>{t('Requests')}</th>
              <th className='px-2.5 py-2 text-right font-medium'>{t('Token count')}</th>
              <th className='px-2.5 py-2 text-right font-medium'>{t('Spend')}</th>
              <th className='px-2.5 py-2 text-right font-medium'>{t('Share')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.key} className='border-border/60 border-t'>
                <td className='max-w-[12rem] truncate px-2.5 py-2.5 font-medium'>
                  {row.label}
                </td>
                <td className='px-2.5 py-2.5 text-right tabular-nums'>
                  {formatNumber(row.requests, locale)}
                </td>
                <td
                  className='px-2.5 py-2.5 text-right tabular-nums'
                  title={formatNumber(row.tokens, locale)}
                >
                  {formatTokenCount(row.tokens, locale)}
                </td>
                <td className='px-2.5 py-2.5 text-right tabular-nums'>
                  {formatQuotaWithCredit(row.quota, CREDIT_FORMAT)}
                </td>
                <td className='px-2.5 py-2.5 text-right tabular-nums'>
                  {formatShare(row.share, locale)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <section className='bg-card min-w-0 rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
        <h2 className='text-sm font-semibold sm:text-base'>{active.heading}</h2>
        <div
          role='tablist'
          aria-label={t('Breakdown')}
          className='bg-background inline-flex h-8 w-fit rounded-xl border p-1'
        >
          {tabs.map((tab) => (
            <button
              key={tab.value}
              type='button'
              role='tab'
              aria-selected={groupBy === tab.value}
              onClick={() => setGroupBy(tab.value)}
              className={cn(
                'inline-flex shrink-0 items-center rounded-lg px-3 text-xs font-medium transition-colors',
                groupBy === tab.value
                  ? 'bg-muted text-foreground shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>
      <div className='mt-3'>{body}</div>
    </section>
  )
}

// Token totals get long (6,189,772), so they are shortened to K/M/B with up to
// two decimals in the UI locale: 6.19M, "6,19M" in Vietnamese. The exact
// number stays in the cell's title.
export function formatTokenCount(tokens: number, locale: string): string {
  const units: [number, string][] = [
    [1e9, 'B'],
    [1e6, 'M'],
    [1e3, 'K'],
  ]
  for (const [size, suffix] of units) {
    if (Math.abs(tokens) >= size) {
      return `${Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(tokens / size)}${suffix}`
    }
  }
  return formatNumber(tokens, locale)
}

// Share of total spend with one decimal, using the UI locale's separator.
export function formatShare(share: number, locale: string): string {
  return `${Intl.NumberFormat(locale, { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(share)}%`
}

function Message(props: { children: React.ReactNode }) {
  return <p className='text-muted-foreground py-6 text-center text-sm'>{props.children}</p>
}
