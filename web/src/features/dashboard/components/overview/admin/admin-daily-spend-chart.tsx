import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { quotaUnitsToDollars } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VChart, VCHART_OPTION, type ISpec } from '@/lib/vchart'

import { getDashboardChartColors } from '../../../lib/charts'

import { buildDailyCostSeries } from '../overview-metrics'
import {
  isSingleLocalDay,
  toUnixRange,
  type TimeRange,
} from '../overview-time-range'
import { useAdminHourlyUsage } from './use-admin-overview'

const TOP_MODEL_COUNT = 5
const OTHER_COLOR = '#94a3b8'

const PALETTE_SIZE = 10

// Colors for the drawn series, in order. A model's color comes from its name,
// not its rank, so it usually stays the same across ranges; when that color is
// already taken by an earlier series, the next free one is used so two models
// never share a color. "Other" is always neutral gray.
export function modelColors(models: string[], otherLabel: string): string[] {
  const palette = getDashboardChartColors(PALETTE_SIZE)
  const used = new Set<number>()
  return models.map((model) => {
    if (model === otherLabel) return OTHER_COLOR
    let hash = 0
    for (const char of model) hash = (hash * 31 + char.charCodeAt(0)) >>> 0
    let index = hash % palette.length
    for (let step = 0; step < palette.length && used.has(index); step++) {
      index = (index + 1) % palette.length
    }
    used.add(index)
    return palette[index] ?? OTHER_COLOR
  })
}

// Credits customers consumed, stacked by the top models plus "Other". A
// single-day range is drawn per hour, longer ranges per day; only buckets
// inside the selected range are drawn.
export function AdminDailySpendChart(props: {
  range: TimeRange | null
  excludeInternal?: boolean
}) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const range = props.range
  const unixRange = useMemo(
    () => (range ? toUnixRange(range) : null),
    [range]
  )
  const query = useAdminHourlyUsage(unixRange, props.excludeInternal)
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
          id: 'admin-daily-spend',
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
        <h2 className='text-sm font-semibold sm:text-base'>
          {t('Credits consumed')}
        </h2>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('Total {{amount}}', {
            amount: formatQuotaWithCredit(
              query.data ? sumQuota(query.data) : 0,
              { abbreviate: false, digitsLarge: 2, digitsSmall: 4 }
            ),
          })}
          {/* quota_data is flushed from memory every few minutes
              (DataExportInterval), so the latest requests show up late and the
              total can trail the log-based KPI cards. */}
          {' · '}
          {t('Updates every few minutes')}
        </span>
      </div>
      <div className='mt-3 min-h-[280px]'>{body}</div>
    </section>
  )
}

function sumQuota(rows: { quota?: number }[]): number {
  return rows.reduce((total, row) => total + (Number(row.quota) || 0), 0)
}

function Message(props: { children: React.ReactNode }) {
  return (
    <p className='text-muted-foreground py-10 text-center text-sm'>
      {props.children}
    </p>
  )
}
