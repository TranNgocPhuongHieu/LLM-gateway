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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useThemeCustomization } from '@/context/theme-customization-provider'
import {
  CONSUMPTION_DISTRIBUTION_CHART_OPTIONS,
  DEFAULT_TIME_GRANULARITY,
} from '@/features/dashboard/constants'
import { processChartData } from '@/features/dashboard/lib'
import type {
  ConsumptionDistributionChartType,
  QuotaDataItem,
} from '@/features/dashboard/types'
import { useThemeRadiusPx } from '@/lib/theme-radius'
import type { TimeGranularity } from '@/lib/time'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VChart, VCHART_OPTION, type ISpec } from '@/lib/vchart'

interface ConsumptionDistributionChartProps {
  data: QuotaDataItem[]
  loading?: boolean
  timeGranularity?: TimeGranularity
  defaultChartType?: ConsumptionDistributionChartType
}

export function ConsumptionDistributionChart(
  props: ConsumptionDistributionChartProps
) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const { customization } = useThemeCustomization()
  const chartRadius = useThemeRadiusPx(
    '--radius-md',
    `${customization.preset}:${customization.radius}`
  )
  const [chartType, setChartType] = useState<ConsumptionDistributionChartType>(
    props.defaultChartType ?? 'bar'
  )
  const timeGranularity = props.timeGranularity ?? DEFAULT_TIME_GRANULARITY

  useEffect(() => {
    if (props.defaultChartType) setChartType(props.defaultChartType)
  }, [props.defaultChartType])

  const chartData = useMemo(
    () =>
      processChartData(
        props.loading ? [] : props.data,
        timeGranularity,
        t,
        chartRadius
      ),
    [props.data, props.loading, timeGranularity, t, chartRadius]
  )
  const spec = chartType === 'bar' ? chartData.spec_line : chartData.spec_area
  const specType = typeof spec?.type === 'string' ? spec.type : chartType
  const chartKey = [
    chartType,
    specType,
    props.loading ? 'loading' : 'ready',
    props.data.length,
    resolvedTheme,
    customization.preset,
  ].join('-')

  return (
    <div className='bg-card overflow-hidden rounded-2xl border p-4 shadow-xs sm:p-5'>
      <div className='flex w-full flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
        <div className='flex flex-col gap-0.5'>
          <div className='text-sm font-semibold sm:text-base'>
            {t('Quota Distribution')}
          </div>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {t('Chart total prefix', 'Tổng:')} {chartData.totalQuotaDisplay}
          </span>
        </div>

        <div className='bg-background inline-flex h-8 w-fit overflow-x-auto rounded-xl border p-1'>
          {CONSUMPTION_DISTRIBUTION_CHART_OPTIONS.map((item) => (
            <button
              key={item.value}
              type='button'
              onClick={() => setChartType(item.value)}
              className={`inline-flex shrink-0 items-center rounded-lg px-3 text-xs font-medium transition-colors ${
                chartType === item.value
                  ? 'bg-muted text-foreground shadow-xs'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {t(item.labelKey)}
            </button>
          ))}
        </div>
      </div>

      <div className='mt-2 h-[280px] sm:h-80'>
        {themeReady && spec && (
          <VChart
            key={chartKey}
            spec={
              {
                ...spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                background: 'transparent',
              } as ISpec
            }
            options={VCHART_OPTION}
          />
        )}
      </div>
    </div>
  )
}
