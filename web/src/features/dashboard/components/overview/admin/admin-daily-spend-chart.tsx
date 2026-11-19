import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { quotaUnitsToDollars } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VChart, VCHART_OPTION, type ISpec } from '@/lib/vchart'

import { buildDailyCostSeries } from '../overview-metrics'
import { toUnixRange, type TimeRange } from '../overview-time-range'
import { useAdminHourlyUsage } from './use-admin-overview'

const TOP_MODEL_COUNT = 5

// System-wide daily spend stacked by model. Same chart as the customer
// "Daily cost" card, fed by all users' hourly rows.
export function AdminDailySpendChart(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const range = props.range
  const unixRange = useMemo(
    () => (range ? toUnixRange(range) : null),
    [range]
  )
  const query = useAdminHourlyUsage(unixRange)

  const series = useMemo(
    () =>
      range && query.data
        ? buildDailyCostSeries(query.data, range, {
            topN: TOP_MODEL_COUNT,
            otherLabel: t('Other'),
            unknownModelLabel: '-',
            toCredits: quotaUnitsToDollars,
          })
        : null,
    [query.data, range, t]
  )

  const spec = useMemo(() => {
    if (!series) return null
    return {
      type: 'bar',
      data: [
        {
          id: 'admin-daily-spend',
          values: series.points.map((point) => ({
            day: point.day.slice(5),
            series: point.series,
            credits: point.credits,
          })),
        },
      ],
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
  }, [locale, series])

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
          {t('System daily cost')}
        </h2>
        <span className='text-muted-foreground text-xs tabular-nums'>
          {t('Total {{amount}}', {
            amount: formatQuotaWithCredit(
              query.data ? sumQuota(query.data) : 0,
              { abbreviate: false, digitsLarge: 2, digitsSmall: 4 }
            ),
          })}
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
