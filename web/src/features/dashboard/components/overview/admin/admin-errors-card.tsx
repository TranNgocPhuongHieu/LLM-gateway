import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { resolveAppLocale } from '@/lib/currency'
import { formatNumber, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { SelfErrorCategoryItem } from '../../../types'
import { toUnixRange, type TimeRange } from '../overview-time-range'
import { getErrorCategoryCopy } from '../recent-errors-card'
import { useAdminErrorSummary } from './use-admin-overview'

// System-wide error causes. Customer fix hints (top up, raise key quota) do not
// apply to operators, so only the cause title and the counts are shown.
export function AdminErrorsCard(props: {
  range: TimeRange | null
  excludeInternal?: boolean
}) {
  const { t, i18n } = useTranslation()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const query = useAdminErrorSummary(unixRange, props.excludeInternal)
  const categories: SelfErrorCategoryItem[] = query.data?.categories ?? []

  let body: React.ReactNode
  if (!props.range) {
    body = <Message>{t('Choose a valid date range to see usage.')}</Message>
  } else if (query.isError) {
    body = (
      <p role='alert' className='text-muted-foreground py-6 text-center text-sm'>
        {t('Could not load recent errors.')}
      </p>
    )
  } else if (query.isPending) {
    body = <Message>{t('Loading')}</Message>
  } else if (categories.length === 0) {
    body = <Message>{t('No errors in this period')}</Message>
  } else {
    body = (
      <ul className='flex flex-col divide-y'>
        {categories.map((item) => {
          const copy = getErrorCategoryCopy(item.category, t)
          return (
            <li
              key={item.category}
              className='flex flex-col gap-1 py-3 first:pt-0 last:pb-0'
            >
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <span className='text-sm font-medium'>{copy.title}</span>
                <span className='bg-muted rounded-full px-2 py-0.5 text-xs font-medium tabular-nums'>
                  {formatNumber(item.count, locale)}
                </span>
              </div>
              <div className='text-muted-foreground text-xs tabular-nums'>
                {t('Last seen {{time}}', {
                  time: formatTimestampToDate(item.last_at),
                })}
                {item.model_name ? ` · ${item.model_name}` : ''}
              </div>
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <section className='bg-card min-w-0 rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex flex-col gap-0.5'>
        <h2 className='text-sm font-semibold sm:text-base'>
          {t('System errors')}
        </h2>
        <p className='text-muted-foreground text-xs'>
          {t('All customers, grouped by cause for the selected period')}
        </p>
      </div>
      <div className='mt-3'>{body}</div>
      {query.data?.truncated && (
        <p
          role='note'
          className={cn(
            'border-warning/40 bg-warning/10 text-foreground mt-3 rounded-lg border px-3 py-2 text-xs'
          )}
        >
          {t(
            'Counts cover only the {{count}} most recent errors in this period; older errors are not grouped here.',
            { count: formatNumber(query.data.total, locale) }
          )}
        </p>
      )}
    </section>
  )
}

function Message(props: { children: React.ReactNode }) {
  return (
    <p className='text-muted-foreground py-6 text-center text-sm'>
      {props.children}
    </p>
  )
}
