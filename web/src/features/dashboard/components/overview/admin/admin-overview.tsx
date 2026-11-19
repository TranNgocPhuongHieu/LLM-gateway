import { Link } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ConsumptionDistributionChart } from '../../models/consumption-distribution-chart'
import { AdminUsageTrackingCard } from '../admin-usage-tracking-card'
import { toUnixRange, type TimeRange } from '../overview-time-range'
import { RecentUsagePanel } from '../recent-usage-panel'
import { AdminDailySpendChart } from './admin-daily-spend-chart'
import { AdminErrorsCard } from './admin-errors-card'
import { AdminSummaryCards } from './admin-summary-cards'
import { useAdminHourlyUsage } from './use-admin-overview'

// Super admin Overview: every panel is system-wide and follows the same range
// picker. Customer-only content (setup guide, own balance and spend) is not
// rendered here.
export function AdminOverview(props: { range: TimeRange | null }) {
  const { t } = useTranslation()
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )
  const hourlyQuery = useAdminHourlyUsage(unixRange)

  return (
    <div className='flex flex-col gap-4'>
      <AdminSummaryCards range={props.range} />

      <AdminDailySpendChart range={props.range} />

      <div className='grid gap-4 lg:grid-cols-2'>
        <ConsumptionDistributionChart
          data={hourlyQuery.data ?? []}
          loading={Boolean(unixRange) && hourlyQuery.isPending}
          timeGranularity='hour'
        />
        <AdminErrorsCard range={props.range} />
      </div>

      <AdminUsageTrackingCard />

      <RecentUsagePanel />

      <div className='flex justify-end'>
        <Link
          to='/dashboard/$section'
          params={{ section: 'models' }}
          className='text-muted-foreground hover:text-foreground text-xs font-medium underline underline-offset-4'
        >
          {t('View model call analytics on the Models tab')}
        </Link>
      </div>
    </div>
  )
}
