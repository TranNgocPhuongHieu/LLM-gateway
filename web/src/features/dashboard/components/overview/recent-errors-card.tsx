import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'

import { formatTimestampToDate, formatNumber } from '@/lib/format'
import { resolveAppLocale } from '@/lib/currency'
import { cn } from '@/lib/utils'

import type { SelfErrorCategoryItem } from '../../types'
import { toUnixRange, type TimeRange } from './overview-time-range'
import { useSelfErrorSummary } from './use-overview-usage'

type ErrorLink = '/wallet' | '/keys' | '/pricing'

interface ErrorCategoryCopy {
  title: string
  hint: string
  to?: ErrorLink
}

// Friendly copy for each server-side category. Unknown categories fall back to
// the generic server error copy so the card never shows a raw identifier as a fix.
export function getErrorCategoryCopy(
  category: string,
  t: TFunction
): ErrorCategoryCopy {
  switch (category) {
    case 'insufficient_balance':
      return {
        title: t('Insufficient balance'),
        hint: t('Top up your wallet'),
        to: '/wallet',
      }
    case 'key_quota_exhausted':
      return {
        title: t('API key quota used up'),
        hint: t("Raise the key's quota"),
        to: '/keys',
      }
    case 'rate_limited':
      return {
        title: t('Rate limited'),
        hint: t('Slow down or retry later'),
      }
    case 'model_unavailable':
      return {
        title: t('Model unavailable'),
        hint: t('Check the model name on the Pricing page'),
        to: '/pricing',
      }
    case 'content_blocked':
      return {
        title: t('Content blocked'),
        hint: t('Review the content of the request'),
      }
    case 'auth_failed':
      return {
        title: t('Authentication failed'),
        hint: t('Check your API key'),
        to: '/keys',
      }
    case 'invalid_request':
      return {
        title: t('Invalid request'),
        hint: t('Check request parameters'),
      }
    case 'server_error':
    default:
      return {
        title: t('Server error'),
        hint: t(
          'Temporary server issue, retry; contact support with the request id if it persists'
        ),
      }
  }
}

export function RecentErrorsCard(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const query = useSelfErrorSummary(unixRange)
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
              {copy.to ? (
                <Link
                  to={copy.to}
                  className='text-primary inline-flex w-fit items-center gap-1 text-xs font-medium hover:underline'
                >
                  {copy.hint}
                  <ArrowRight className='size-3' aria-hidden='true' />
                </Link>
              ) : (
                <span className='text-muted-foreground text-xs'>{copy.hint}</span>
              )}
            </li>
          )
        })}
      </ul>
    )
  }

  return (
    <section className='bg-card min-w-0 rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex flex-col gap-0.5'>
        <h2 className='text-sm font-semibold sm:text-base'>{t('Recent errors')}</h2>
        <p className='text-muted-foreground text-xs'>
          {t('Grouped by cause for the selected period')}
        </p>
      </div>
      <div className='mt-3'>{body}</div>
      {query.data?.truncated && (
        <p className={cn('text-muted-foreground mt-3 text-xs')}>
          {t('Showing the most recent errors only; older errors are not counted.')}
        </p>
      )}
    </section>
  )
}

function Message(props: { children: React.ReactNode }) {
  return <p className='text-muted-foreground py-6 text-center text-sm'>{props.children}</p>
}
