import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { quotaUnitsToDollars } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VChart, VCHART_OPTION, type ISpec } from '@/lib/vchart'

import { modelColors } from './model-colors'
import { buildDailyCostSeries } from './overview-metrics'
import {
  isSingleLocalDay,
  toUnixRange,
  type TimeRange,
} from './overview-time-range'
import { useSelfDailyUsage } from './use-overview-usage'

const TOP_MODEL_COUNT = 5

// The customer's spend, stacked by the top models plus "Other". A single-day
// range is drawn per hour, longer ranges per day; model colors follow the
// model name, as on the admin chart. The per-key split lives in
// SpendBreakdownCard.
export function DailyCostChart(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const range = props.range
  const unixRange = useMemo(() => (range ? toUnixRange(range) : null), [range])
  const query = useSelfDailyUsage(unixRange)
  const hourly = range ? isSingleLocalDay(range) : false
  const otherLabel = t('Other')

  const series = useMemo(
    () =>
      range && query.data
        ? buildDailyCostSeries(query.data, range, {
            topN: TOP_MODEL_COUNT,
            otherLabel,
            unknownModelLabel: '-',
            toCredits: quotaUnitsToDollars,
            granularity: hourly ? 'hour' : 'day',
          })
        : null,
    [query.data, range, otherLabel, hourly]
  )

  const spec = useMemo(() => {
    if (!series) return null
    return {
      type: 'bar',
      data: [
        {
          id: 'daily-cost',
          values: series.points.map((point) => ({
            // "YYYY-MM-DD HH" -> "HH:00"; "YYYY-MM-DD" -> "MM-DD".
            day: hourly ? `${point.day.slice(11)}:00` : point.day.slice(5),
            series: point.series,
            credits: point.credits,
          })),
        },
      ],
      color: {
        type: 'ordinal',
        domain: series.series,
        range: modelColors(series.series, otherLabel),
      },
      xField: 'day',
      yField: 'credits',
      seriesField: 'series',
      stack: true,
      legends: { visible: true, orient: 'bottom' },
      axes: [
        {
          orient: 'left',
          label: {
            formatMethod: (value: string | number) =>
              new Intl.NumberFormat(locale, {
                maximumFractionDigits: 2,
              }).format(Number(value)),
          },
        },
        { orient: 'bottom' },
      ],
      background: 'transparent',
    }
  }, [locale, series, hourly, otherLabel])

  let body: React.ReactNode
  if (!range) {
    body = <Message>{t('Choose a valid date range to see usage.')}</Message>
  } else if (query.isError) {
    body = <Message>{t('Could not load usage. Try again later.')}</Message>
  } else if (query.isPending) {
    body = <Message>{t('Loading')}</Message>
  } else if (spec && themeReady && series && series.totalCredits > 0) {
    body = (
      <div className='h-[280px] sm:h-80'>
        <VChart
          key={`${resolvedTheme}-${series.days.length}-${series.series.join('|')}`}
          spec={
            {
              ...spec,
              theme: resolvedTheme === 'dark' ? 'dark' : 'light',
            } as ISpec
          }
          options={VCHART_OPTION}
        />
      </div>
    )
  } else {
    body = <Message>{t('No data')}</Message>
  }

  return (
    <section className='bg-card min-w-0 rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex flex-col gap-0.5'>
        <h2 className='text-sm font-semibold sm:text-base'>{t('Spend')}</h2>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('Total {{amount}}', {
            amount: formatQuotaWithCredit(sumQuota(query.data), {
              abbreviate: false,
              digitsLarge: 2,
              digitsSmall: 4,
            }),
          })}
          {/* quota_data is flushed from memory every few minutes
              (DataExportInterval), so the latest requests show up late. */}
          {' · '}
          {t('Updates every few minutes')}
        </span>
      </div>
      <div className='mt-3 min-h-[280px]'>{body}</div>
    </section>
  )
}

function sumQuota(rows: { quota?: number }[] | undefined): number {
  return (rows ?? []).reduce((total, row) => total + (Number(row.quota) || 0), 0)
}

function Message(props: { children: React.ReactNode }) {
  return (
    <p className='text-muted-foreground py-10 text-center text-sm'>
      {props.children}
    </p>
  )
}
