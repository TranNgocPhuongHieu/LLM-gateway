import { Link } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { DEFAULT_QUOTA_WARNING_THRESHOLD } from '@/features/profile/constants'
import { formatQuotaWithCredit } from '@/lib/currency'
import { useAuthStore } from '@/stores/auth-store'

import {
  averageDailyQuota,
  getHealthLevel,
  isBelowWarningThreshold,
  projectRunOut,
} from './overview-metrics'
import {
  useCurrentMinute,
  useSpendKpiQueries,
  useWarningThreshold,
} from './use-overview-usage'

const CREDIT_FORMAT = { abbreviate: false, digitsLarge: 2, digitsSmall: 2 } as const

// Shown when the balance is under the customer's warning threshold or the
// projected runway is critical. Renders nothing otherwise.
export function LowBalanceBanner() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const now = useCurrentMinute()
  const balanceQuota = Number(user?.quota ?? 0)
  const threshold = useWarningThreshold()
  const spend = useSpendKpiQueries(now)

  const thresholdQuota = threshold.data ?? DEFAULT_QUOTA_WARNING_THRESHOLD
  const avgDaily = averageDailyQuota(spend.last7DaysQuota ?? 0)
  const health = getHealthLevel(balanceQuota, avgDaily)
  const belowThreshold = isBelowWarningThreshold(balanceQuota, thresholdQuota)
  if (!belowThreshold && health !== 'critical') return null

  const balance = formatQuotaWithCredit(balanceQuota, CREDIT_FORMAT)
  // Explain why the banner is shown: under the user's threshold, or the
  // balance will run out soon at the recent spending rate.
  let detail: string
  if (belowThreshold) {
    detail = t('Balance {{balance}} · warning threshold {{threshold}}', {
      balance,
      threshold: formatQuotaWithCredit(thresholdQuota, CREDIT_FORMAT),
    })
  } else {
    const runOut = projectRunOut(balanceQuota, avgDaily, now)
    detail =
      runOut.kind === 'date' && runOut.days >= 1
        ? t('Balance {{balance}} · about {{days}} days left at your recent spending', {
            balance,
            days: Math.floor(runOut.days),
          })
        : t('Balance {{balance}} · less than 1 day left at your recent spending', {
            balance,
          })
  }

  return (
    <div
      role='status'
      className='border-warning/40 bg-warning/10 flex flex-wrap items-center justify-between gap-3 rounded-2xl border px-4 py-3'
    >
      <div className='flex min-w-0 items-start gap-2.5'>
        <AlertTriangle
          className='text-warning mt-0.5 size-4 shrink-0'
          aria-hidden='true'
        />
        <div className='min-w-0 text-sm'>
          <div className='font-medium'>{t('Your balance is running low')}</div>
          <div className='text-muted-foreground text-xs tabular-nums'>
            {detail}
          </div>
        </div>
      </div>
      <Button size='sm' className='shrink-0' render={<Link to='/wallet' />}>
        {t('Top up')}
      </Button>
    </div>
  )
}
