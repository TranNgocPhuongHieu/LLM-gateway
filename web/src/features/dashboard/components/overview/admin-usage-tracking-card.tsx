import { useQuery } from '@tanstack/react-query'
import { ChevronDown, KeyRound, SlidersHorizontal } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getFlowQuotaDates } from '@/features/dashboard/api'
import { formatQuotaWithCredit } from '@/lib/currency'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { toUnixRange, type TimeRange } from './overview-time-range'

const LIMIT_OPTIONS = [5, 10, 20] as const

// Quota is stored in raw units; every displayed quota goes through this formatter.
const CREDIT_FORMAT = { abbreviate: false, digitsLarge: 2, digitsSmall: 4 } as const

function formatCredits(quota: number): string {
  return formatQuotaWithCredit(quota, CREDIT_FORMAT)
}

// Follows the Overview range picker; excludeInternal hides admin and root
// accounts like the rest of the admin Overview.
export function AdminUsageTrackingCard(props: {
  range: TimeRange | null
  excludeInternal?: boolean
}) {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const canView = role >= ROLE.SUPER_ADMIN
  const [limit, setLimit] = useState<number>(10)
  const [adjustOpen, setAdjustOpen] = useState(false)

  const excludeInternal = Boolean(props.excludeInternal)
  const timeRange = useMemo(
    () => (props.range ? toUnixRange(props.range) : null),
    [props.range]
  )

  // All rows are kept so per-user totals are complete; limit applies to the
  // user bars and the detail table only.
  const query = useQuery({
    queryKey: ['dashboard', 'admin-usage-tracking', timeRange, excludeInternal],
    enabled: canView && Boolean(timeRange),
    queryFn: async () => {
      if (!timeRange) return []
      const result = await getFlowQuotaDates(
        { ...timeRange, exclude_internal: excludeInternal },
        true
      )
      if (!result.success) throw new Error(result.message || 'Failed to load usage')
      return result.data ?? []
    },
    staleTime: 60_000,
    retry: 1,
    meta: { errorToast: false },
  })

  const chartRows = useMemo(() => {
    const grouped = new Map<string, { quota: number; requests: number }>()
    for (const row of query.data ?? []) {
      const name = row.username || `#${row.user_id ?? '—'}`
      const current = grouped.get(name) ?? { quota: 0, requests: 0 }
      current.quota += Number(row.quota ?? 0)
      current.requests += Number(row.count ?? 0)
      grouped.set(name, current)
    }
    return [...grouped.entries()]
      .map(([name, values]) => ({ name, ...values }))
      .sort((a, b) => b.quota - a.quota)
      .slice(0, limit)
  }, [limit, query.data])

  const chartMax = Math.max(...chartRows.map((row) => row.quota), 1)
  const allRows = query.data ?? []
  const totalQuota = allRows.reduce((sum, row) => sum + Number(row.quota ?? 0), 0)
  const detailRows = allRows.slice(0, limit)

  if (!canView) return null

  return (
    <section className='bg-card min-w-0 overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5'>
        <div className='flex items-center gap-2'>
          <div className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg'>
            <KeyRound className='size-4' aria-hidden='true' />
          </div>
          <div>
            <h2 className='text-sm font-semibold'>{t('Top customers')}</h2>
            <p className='text-muted-foreground text-xs'>
              {t('By credits used in the selected period')}
            </p>
          </div>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => setAdjustOpen((open) => !open)}
          aria-expanded={adjustOpen}
        >
          <SlidersHorizontal data-icon='inline-start' />
          {t('Adjust tracking')}
          <ChevronDown data-icon='inline-end' className={adjustOpen ? 'rotate-180' : ''} />
        </Button>
      </div>

      {adjustOpen && (
        <div className='bg-muted/20 flex flex-wrap items-end gap-3 border-b px-4 py-3 sm:px-5'>
          <label className='grid gap-1 text-xs'>
            <span className='text-muted-foreground'>{t('Rows')}</span>
            <select className='bg-background h-9 rounded-lg border px-2' value={limit} onChange={(e) => setLimit(Number(e.target.value))}>
              {LIMIT_OPTIONS.map((value) => <option key={value} value={value}>{t('Top {{count}}', { count: value })}</option>)}
            </select>
          </label>
        </div>
      )}

      <div className='p-4 sm:p-5'>
        {query.isLoading ? (
          <div className='bg-muted h-40 animate-pulse rounded-xl' />
        ) : chartRows.length === 0 ? (
          <div className='text-muted-foreground flex h-40 items-center justify-center rounded-xl border text-sm'>
            {t('No API key usage in this range')}
          </div>
        ) : (
          <ol className='flex flex-col gap-2.5'>
            {chartRows.map((row, index) => (
              <li key={row.name} className='flex flex-col gap-1'>
                <div className='flex items-baseline justify-between gap-3 text-sm'>
                  <span className='flex min-w-0 items-baseline gap-2'>
                    <span className='text-muted-foreground w-5 shrink-0 text-xs tabular-nums'>{index + 1}</span>
                    <span className='truncate font-medium' title={row.name}>{row.name}</span>
                  </span>
                  <span className='shrink-0 text-right tabular-nums'>
                    {formatCredits(row.quota)}
                    <span className='text-muted-foreground ml-2 text-xs'>
                      {t('{{count}} req', { count: row.requests.toLocaleString() })}
                      {totalQuota > 0 ? ` · ${Math.round((row.quota / totalQuota) * 100)}%` : ''}
                    </span>
                  </span>
                </div>
                <div className='bg-muted h-1.5 overflow-hidden rounded-full'>
                  <div
                    className='bg-primary/80 h-full rounded-full'
                    style={{ width: `${Math.max((row.quota / chartMax) * 100, 2)}%` }}
                  />
                </div>
              </li>
            ))}
          </ol>
        )}
      </div>

      <details className='border-t'>
        <summary className='text-muted-foreground hover:text-foreground cursor-pointer px-4 py-3 text-xs sm:px-5'>
          {t('View API key and model details')}
        </summary>
      <div className='overflow-x-auto'>
        <table className='w-full min-w-[680px] text-sm'>
          <thead className='bg-muted/20 text-muted-foreground text-left text-xs'>
            <tr><th className='px-4 py-2 sm:px-5'>{t('User')}</th><th className='px-3 py-2'>{t('API key')}</th><th className='px-3 py-2'>{t('Model')}</th><th className='px-3 py-2 text-right'>{t('Requests')}</th><th className='px-4 py-2 text-right sm:px-5'>{t('Credits used')}</th></tr>
          </thead>
          <tbody className='divide-border/60 divide-y'>
            {query.isLoading ? <tr><td colSpan={5} className='px-4 py-8 text-center'>{t('Loading')}</td></tr> : detailRows.length ? detailRows.map((row, index) => <tr key={`${row.user_id}-${row.token_id}-${row.model_name}-${index}`} className='hover:bg-muted/20'><td className='px-4 py-2.5 sm:px-5'>{row.username || `#${row.user_id ?? '—'}`}</td><td className='text-muted-foreground px-3 py-2.5'>{row.token_name || (row.token_id ? `#${row.token_id}` : t('Unknown'))}</td><td className='px-3 py-2.5 font-mono text-xs'>{row.model_name || '—'}</td><td className='px-3 py-2.5 text-right tabular-nums'>{Number(row.count ?? 0).toLocaleString()}</td><td className='px-4 py-2.5 text-right tabular-nums sm:px-5'>{formatCredits(Number(row.quota ?? 0))}</td></tr>) : <tr><td colSpan={5} className='text-muted-foreground px-4 py-8 text-center'>{t('No API key usage in this range')}</td></tr>}
          </tbody>
        </table>
      </div>
      </details>
    </section>
  )
}
