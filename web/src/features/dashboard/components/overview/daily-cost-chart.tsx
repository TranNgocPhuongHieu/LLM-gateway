import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { formatQuotaWithCredit, resolveAppLocale } from '@/lib/currency'
import { quotaUnitsToDollars } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VChart, VCHART_OPTION, type ISpec } from '@/lib/vchart'
import { cn } from '@/lib/utils'

import { buildDailyCostSeries, getBreakdownRows } from './overview-metrics'
import {
  toUnixRange,
  type TimeRange,
} from './overview-time-range'
import { useSelfBreakdown, useSelfDailyUsage } from './use-overview-usage'

const TOP_MODEL_COUNT = 5
type CostView = 'model' | 'key'

export function DailyCostChart(props: { range: TimeRange | null }) {
  const { t, i18n } = useTranslation()
  const [view, setView] = useState<CostView>('model')
  const { resolvedTheme, themeReady } = useChartTheme()
  const locale = resolveAppLocale(i18n.resolvedLanguage || i18n.language)
  const range = props.range
  const unixRange = useMemo(() => (range ? toUnixRange(range) : null), [range])

  const dailyQuery = useSelfDailyUsage(view === 'model' ? unixRange : null)
  const keyQuery = useSelfBreakdown(view === 'key' ? unixRange : null, 'token')

  const series = useMemo(
    () =>
      range && dailyQuery.data
        ? buildDailyCostSeries(dailyQuery.data, range, {
            topN: TOP_MODEL_COUNT,
            otherLabel: t('Other'),
            unknownModelLabel: '-',
            toCredits: quotaUnitsToDollars,
          })
        : null,
    [dailyQuery.data, range, t]
  )

  const keyRows = useMemo(
    () =>
      getBreakdownRows(keyQuery.data, 'token', (keyId) =>
        t('Key #{{id}} (deleted)', { id: keyId })
      ),
    [keyQuery.data, t]
  )
  const maxKeyQuota = Math.max(1, ...keyRows.map((row) => row.quota))

  const spec = useMemo(() => {
    if (!series) return null
    return {
      type: 'bar',
      data: [
        {
          id: 'daily-cost',
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

  const isLoading =
    view === 'model' ? dailyQuery.isPending : keyQuery.isPending
  const isError = view === 'model' ? dailyQuery.isError : keyQuery.isError
  const totalQuota =
    view === 'model'
      ? sumQuota(dailyQuery.data)
      : Number(keyQuery.data?.total.quota ?? 0)

  return (
    <section className='bg-card min-w-0 rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
        <div className='flex flex-col gap-0.5'>
          <h2 className='text-sm font-semibold sm:text-base'>
            {t('Daily cost')}
          </h2>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {t('Total {{amount}}', {
              amount: formatQuotaWithCredit(totalQuota, {
                abbreviate: false,
                digitsLarge: 2,
                digitsSmall: 4,
              }),
            })}
          </span>
        </div>
        <div
          role='group'
          aria-label={t('Group chart by')}
          className='bg-background inline-flex h-8 w-fit rounded-xl border p-1'
        >
          {(['model', 'key'] as const).map((item) => (
            <button
              key={item}
              type='button'
              aria-pressed={view === item}
              onClick={() => setView(item)}
              className={cn(
                'inline-flex shrink-0 items-center rounded-lg px-3 text-xs font-medium transition-colors',
                view === item
                  ? 'bg-muted text-foreground shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              )}
            >
              {item === 'model' ? t('By model') : t('By API key')}
            </button>
          ))}
        </div>
      </div>

      <div className='mt-3 min-h-[280px]'>
        {!range ? (
          <p className='text-muted-foreground py-10 text-center text-sm'>
            {t('Choose a valid date range to see usage.')}
          </p>
        ) : isError ? (
          <p className='text-muted-foreground py-10 text-center text-sm'>
            {t('Could not load usage. Try again later.')}
          </p>
        ) : isLoading ? (
          <p className='text-muted-foreground py-10 text-center text-sm'>
            {t('Loading')}
          </p>
        ) : view === 'model' ? (
          spec && themeReady && series && series.totalCredits > 0 ? (
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
          ) : (
            <p className='text-muted-foreground py-10 text-center text-sm'>
              {t('No data')}
            </p>
          )
        ) : keyRows.length === 0 ? (
          <p className='text-muted-foreground py-10 text-center text-sm'>
            {t('No data')}
          </p>
        ) : (
          <ul className='flex flex-col gap-3'>
            {keyRows.map((row) => (
              <li key={row.key} className='flex flex-col gap-1'>
                <div className='flex items-center justify-between gap-3 text-xs'>
                  <span className='min-w-0 truncate font-medium'>{row.label}</span>
                  <span className='text-muted-foreground shrink-0 tabular-nums'>
                    {formatQuotaWithCredit(row.quota, {
                      abbreviate: false,
                      digitsLarge: 2,
                      digitsSmall: 4,
                    })}
                  </span>
                </div>
                <div className='bg-muted h-2 overflow-hidden rounded-full'>
                  <div
                    className='bg-chart-1 h-full rounded-full'
                    style={{ width: `${(row.quota / maxKeyQuota) * 100}%` }}
                  />
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  )
}

function sumQuota(rows: { quota?: number }[] | undefined): number {
  return (rows ?? []).reduce((total, row) => total + (Number(row.quota) || 0), 0)
}
