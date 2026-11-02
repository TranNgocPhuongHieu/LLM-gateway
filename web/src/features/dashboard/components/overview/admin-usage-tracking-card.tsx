import { useQuery } from '@tanstack/react-query'
import { ChevronDown, KeyRound, SlidersHorizontal } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { getFlowQuotaDates } from '@/features/dashboard/api'
import { getRollingDateRange } from '@/lib/time'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const RANGE_OPTIONS = [1, 7, 30] as const
const LIMIT_OPTIONS = [5, 10, 20] as const

export function AdminUsageTrackingCard() {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const canView = role >= ROLE.SUPER_ADMIN
  const [rangeDays, setRangeDays] = useState<number>(7)
  const [limit, setLimit] = useState<number>(10)
  const [adjustOpen, setAdjustOpen] = useState(false)

  const timeRange = useMemo(() => {
    const { start, end } = getRollingDateRange(rangeDays)
    return {
      start_timestamp: Math.floor(start.getTime() / 1000),
      end_timestamp: Math.floor(end.getTime() / 1000),
    }
  }, [rangeDays])

  const query = useQuery({
    queryKey: ['dashboard', 'admin-usage-tracking', timeRange, limit],
    enabled: canView,
    queryFn: async () => {
      const result = await getFlowQuotaDates(timeRange, true)
      if (!result.success) throw new Error(result.message || 'Failed to load usage')
      return (result.data ?? []).slice(0, limit)
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
  const totalQuota = chartRows.reduce((sum, row) => sum + row.quota, 0)
  const totalRequests = chartRows.reduce((sum, row) => sum + row.requests, 0)

  if (!canView) return null

  return (
    <section className='bg-card overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3 sm:px-5'>
        <div className='flex items-center gap-2'>
          <div className='bg-primary/10 text-primary flex size-8 items-center justify-center rounded-lg'>
            <KeyRound className='size-4' aria-hidden='true' />
          </div>
          <div>
            <h2 className='text-sm font-semibold'>{t('Admin usage tracking')}</h2>
            <p className='text-muted-foreground text-xs'>{t('User · API key · model')}</p>
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
            <span className='text-muted-foreground'>{t('Time range')}</span>
            <select className='bg-background h-9 rounded-lg border px-2' value={rangeDays} onChange={(e) => setRangeDays(Number(e.target.value))}>
              {RANGE_OPTIONS.map((days) => <option key={days} value={days}>{days} {t('days')}</option>)}
            </select>
          </label>
          <label className='grid gap-1 text-xs'>
            <span className='text-muted-foreground'>{t('Rows')}</span>
            <select className='bg-background h-9 rounded-lg border px-2' value={limit} onChange={(e) => setLimit(Number(e.target.value))}>
              {LIMIT_OPTIONS.map((value) => <option key={value} value={value}>{t('Top {{count}}', { count: value })}</option>)}
            </select>
          </label>
        </div>
      )}

      <div className='grid gap-4 p-4 sm:grid-cols-[minmax(0,1fr)_12rem] sm:p-5'>
        <div className='min-w-0 space-y-3'>
          <div className='text-muted-foreground flex items-center justify-between text-xs'>
            <span>{t('Credit usage by user')}</span>
            <span>{rangeDays} {t('days')}</span>
          </div>
          {query.isLoading ? (
            <div className='bg-muted h-40 animate-pulse rounded-xl' />
          ) : chartRows.length === 0 ? (
            <div className='text-muted-foreground flex h-40 items-center justify-center rounded-xl border text-sm'>
              {t('No API key usage in this range')}
            </div>
          ) : (
            <div className='flex h-40 items-end gap-2 overflow-x-auto border-b px-2 pb-0'>
              {chartRows.map((row) => (
                <div key={row.name} className='flex h-full min-w-14 flex-1 flex-col items-center justify-end gap-1'>
                  <span className='text-muted-foreground text-[10px] tabular-nums'>{row.quota.toLocaleString()}</span>
                  <div className='bg-primary/80 hover:bg-primary w-full min-w-8 rounded-t-md transition-colors' style={{ height: `${Math.max((row.quota / chartMax) * 100, 4)}%` }} title={`${row.name}: ${row.quota.toLocaleString()}`} />
                  <span className='text-muted-foreground max-w-20 truncate text-[10px]' title={row.name}>{row.name}</span>
                </div>
              ))}
            </div>
          )}
        </div>
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-1'>
          <div className='bg-muted/20 rounded-xl border p-3'>
            <div className='text-muted-foreground text-xs'>{t('Credits used')}</div>
            <div className='mt-1 text-xl font-semibold tabular-nums'>{totalQuota.toLocaleString()}</div>
          </div>
          <div className='bg-muted/20 rounded-xl border p-3'>
            <div className='text-muted-foreground text-xs'>{t('Requests')}</div>
            <div className='mt-1 text-xl font-semibold tabular-nums'>{totalRequests.toLocaleString()}</div>
          </div>
        </div>
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
            {query.isLoading ? <tr><td colSpan={5} className='px-4 py-8 text-center'>{t('Loading')}</td></tr> : query.data?.length ? query.data.map((row, index) => <tr key={`${row.user_id}-${row.token_id}-${row.model_name}-${index}`} className='hover:bg-muted/20'><td className='px-4 py-2.5 sm:px-5'>{row.username || `#${row.user_id ?? '—'}`}</td><td className='text-muted-foreground px-3 py-2.5'>{row.token_name || (row.token_id ? `#${row.token_id}` : t('Unknown'))}</td><td className='px-3 py-2.5 font-mono text-xs'>{row.model_name || '—'}</td><td className='px-3 py-2.5 text-right tabular-nums'>{Number(row.count ?? 0).toLocaleString()}</td><td className='px-4 py-2.5 text-right tabular-nums sm:px-5'>{Number(row.quota ?? 0).toLocaleString()}</td></tr>) : <tr><td colSpan={5} className='text-muted-foreground px-4 py-8 text-center'>{t('No API key usage in this range')}</td></tr>}
          </tbody>
        </table>
      </div>
      </details>
    </section>
  )
}
