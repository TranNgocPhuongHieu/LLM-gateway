import { Link } from '@tanstack/react-router'
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Switch } from '@/components/ui/switch'

import { AdminUsageTrackingCard } from '../admin-usage-tracking-card'
import { toUnixRange, type TimeRange } from '../overview-time-range'
import { RecentUsagePanel } from '../recent-usage-panel'
import { AdminDailySpendChart } from './admin-daily-spend-chart'
import { AdminErrorsCard } from './admin-errors-card'
import { AdminSummaryCards } from './admin-summary-cards'

// Per-viewer preference; storage can be unavailable (private mode), so every
// access is guarded and the default applies.
const EXCLUDE_INTERNAL_STORAGE_KEY = 'admin-overview:exclude-internal'

function readExcludeInternal(): boolean {
  try {
    return window.localStorage.getItem(EXCLUDE_INTERNAL_STORAGE_KEY) !== 'false'
  } catch {
    return true
  }
}

function writeExcludeInternal(value: boolean) {
  try {
    window.localStorage.setItem(EXCLUDE_INTERNAL_STORAGE_KEY, String(value))
  } catch {
    // Preference is not persisted; the toggle still works for this session.
  }
}

// Super admin Overview, top to bottom: pending top-up banner, four KPIs, the
// credits-consumed chart, top customers next to error causes, and the latest
// requests. Every panel is system-wide and follows the same range picker.
// "Hide internal accounts" drops admin and root accounts from usage figures;
// revenue and signups stay platform-wide.
export function AdminOverview(props: { range: TimeRange | null }) {
  const { t } = useTranslation()
  const switchId = useId()
  const [excludeInternal, setExcludeInternal] = useState(readExcludeInternal)
  const unixRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )

  const handleExcludeInternalChange = (value: boolean) => {
    setExcludeInternal(value)
    writeExcludeInternal(value)
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex items-center justify-end gap-2'>
        <Switch
          id={switchId}
          checked={excludeInternal}
          onCheckedChange={handleExcludeInternalChange}
        />
        <label htmlFor={switchId} className='text-muted-foreground text-xs font-medium'>
          {t('Hide internal accounts')}
        </label>
      </div>

      <AdminSummaryCards range={props.range} excludeInternal={excludeInternal} />

      <AdminDailySpendChart range={props.range} excludeInternal={excludeInternal} />

      <div className='grid gap-4 lg:grid-cols-2'>
        <AdminUsageTrackingCard range={props.range} excludeInternal={excludeInternal} />
        <AdminErrorsCard range={props.range} excludeInternal={excludeInternal} />
      </div>

      <RecentUsagePanel range={unixRange} limit={5} />

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
