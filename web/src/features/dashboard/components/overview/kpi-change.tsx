import { useTranslation } from 'react-i18next'

import { formatPercent } from '@/lib/format'
import { cn } from '@/lib/utils'

// A rise beyond this means the previous period had almost no activity, so the
// percentage says nothing useful: the badge reads "New" instead.
const MAX_SHOWN_CHANGE_PERCENT = 999

// Percent change against the previous period (percentChange). null or a
// non-finite value renders nothing. Shared by the customer and admin KPIs.
// neutral drops the good/bad colors: for a customer, spending more or less is
// neither good nor bad news.
export function KpiChange(props: {
  change: number | null | undefined
  neutral?: boolean
}) {
  const { t } = useTranslation()
  const change = props.change
  if (change == null || !Number.isFinite(change)) return null
  return (
    <span
      className={cn(
        'text-xs font-medium tabular-nums',
        props.neutral
          ? 'text-muted-foreground'
          : change >= 0
            ? 'text-success'
            : 'text-destructive'
      )}
      title={t('Compared with the previous period')}
    >
      {change > MAX_SHOWN_CHANGE_PERCENT
        ? t('New')
        : `${change >= 0 ? '▲' : '▼'} ${formatPercent(Math.abs(change))}`}
    </span>
  )
}
