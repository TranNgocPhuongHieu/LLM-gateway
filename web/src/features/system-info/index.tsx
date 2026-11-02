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
import { useTranslation } from 'react-i18next'
import { useRef } from 'react'
import { useQuery } from '@tanstack/react-query'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { api } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { SystemInstancesPanel } from './components/system-instances-panel'
import { SystemTasksPanel } from './components/system-tasks-panel'

export function SystemInfo() {
  const { t } = useTranslation()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('System Info')}</span>
          <Badge variant='outline' className='shrink-0'>
            Root
          </Badge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <SystemInstancesPanel />
          <GatewayHealth />
          <SystemTasksPanel />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

type Pool = {
  MaxOpenConnections: number
  OpenConnections: number
  InUse: number
  Idle: number
  WaitCount: number
  WaitDuration: number
}
type HealthStats = {
  active_relay_requests?: number
  database_pools: Record<string, Pool | null>
  request_health_unavailable?: boolean
  channel_cache_freshness?: {
    last_success_at?: string
    last_attempt_at?: string
    last_error?: string
    is_stale?: boolean
    recovery_active?: boolean
  }
  request_health?: {
    start_at: number
    end_at: number
    requests: number
    attempts: number
    capped: boolean
    outcomes: Record<string, number>
    failures: Record<string, number>
    alerts: string[]
  }
}

function GatewayHealth() {
  const { t } = useTranslation()
  const previous = useRef<{
    pools: Record<string, Pool | null>
    at: number
    waiting: number
    unavailable: number
  } | null>(null)
  const query = useQuery({
    queryKey: ['gateway-operational-health'],
    queryFn: async ({ signal }) => {
      const [stats, ready] = await Promise.all([
        api.get<{ success: boolean; data: HealthStats }>(
          '/api/performance/stats?request_health=true',
          { signal, timeout: 5000 }
        ),
        api
          .get('/readyz', { signal, timeout: 3000, validateStatus: () => true })
          .catch(() => null),
      ])
      const data = requireServerSuccess(stats.data).data
      const now = Date.now()
      const old = previous.current
      // Long gaps/remounts/process restarts start a fresh observation window.
      const contiguous = old && now - old.at < 45000
      const waits =
        contiguous &&
        Object.entries(data.database_pools).some(([name, pool]) => {
          const before = old.pools[name]
          return (
            pool &&
            before &&
            pool.WaitCount > before.WaitCount &&
            pool.WaitDuration > before.WaitDuration
          )
        })
      const state = {
        pools: data.database_pools,
        at: now,
        waiting: waits ? (old?.waiting ?? 0) + 1 : 0,
        unavailable:
          ready?.status === 200 ? 0 : (contiguous ? old.unavailable : 0) + 1,
      }
      previous.current = state
      return { ...data, ready: ready?.status === 200, ...state }
    },
    retry: false,
    refetchInterval: 30000,
    refetchIntervalInBackground: false,
    refetchOnWindowFocus: false,
  })
  const data = query.data
  const health = data?.request_health
  return (
    <section className='space-y-3 rounded-xl border p-4'>
      <h3 className='font-semibold'>{t('Gateway operational health')}</h3>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Local 15-minute cohort; refreshed every 30 seconds while this page is open. No external notifications.'
        )}
      </p>
      {query.isError ? (
        <Alert variant='destructive'>
          <AlertDescription>{t('Health data unavailable')}</AlertDescription>
        </Alert>
      ) : !data ? (
        <p>{t('Loading...')}</p>
      ) : (
        <>
          <div className='flex flex-wrap items-center gap-2'>
            <Badge variant={data.ready ? 'outline' : 'destructive'}>
              {data.ready ? t('Ready') : t('Not ready')}
            </Badge>
            <Badge variant='outline'>
              {t('In-flight relay requests')}:{' '}
              {data.active_relay_requests ?? t('Unknown')}
            </Badge>
            {data.channel_cache_freshness && (
              <Badge variant={data.channel_cache_freshness.is_stale ? 'destructive' : 'secondary'}>
                {data.channel_cache_freshness.is_stale
                  ? (data.channel_cache_freshness.recovery_active
                      ? t('Cache stale (using last-good snapshot)')
                      : t('Cache error'))
                  : t('Channel cache synced')}
                {data.channel_cache_freshness.last_success_at && (
                  <span className='ms-1.5 opacity-80 font-normal'>
                    ({new Date(data.channel_cache_freshness.last_success_at).toLocaleTimeString()})
                  </span>
                )}
              </Badge>
            )}
          </div>
          {data.channel_cache_freshness?.is_stale && data.channel_cache_freshness.last_error && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('Channel cache refresh failed')}: {data.channel_cache_freshness.last_error}
              </AlertDescription>
            </Alert>
          )}
          {(data.unavailable >= 3 || data.waiting >= 3) && (
            <Alert variant='destructive'>
              <AlertDescription>
                {data.unavailable >= 3
                  ? t('Readiness failed three consecutive checks.')
                  : t(
                      'Database pool waits increased for three consecutive checks. Inspect load before changing limits.'
                    )}
              </AlertDescription>
            </Alert>
          )}
          {Object.entries(data.database_pools).map(([name, pool]) => (
            <p key={name} className='font-mono text-xs'>
              {name}:{' '}
              {pool
                ? `open ${pool.OpenConnections}/${pool.MaxOpenConnections}; in-use ${pool.InUse}; idle ${pool.Idle}; waits ${pool.WaitCount}; wait-ms ${(pool.WaitDuration / 1e6).toFixed(1)}`
                : t('Unknown')}
            </p>
          ))}
          {data.request_health_unavailable || !health ? (
            <p>{t('Request health unavailable')}</p>
          ) : (
            <>
              <p className='text-xs'>
                {new Date(health.start_at * 1000).toLocaleString()} →{' '}
                {new Date(health.end_at * 1000).toLocaleString()}:{' '}
                {health.requests} {t('Requests')}, {health.attempts}{' '}
                {t('Attempts')}
              </p>
              <div className='flex flex-wrap gap-2'>
                {Object.entries(health.outcomes).map(([kind, count]) => (
                  <Badge key={kind} variant='outline'>
                    {t(kind)}: {count}
                  </Badge>
                ))}
              </div>
              <div className='flex flex-wrap gap-2'>
                {Object.entries(health.failures).map(([kind, count]) => (
                  <Badge key={kind} variant='secondary'>
                    {t(kind)}: {count}
                  </Badge>
                ))}
              </div>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'HTTP categories describe returned errors, not proven root causes. Missing terminal evidence is unknown; in-flight requests cannot be inferred from persisted logs.'
                )}
              </p>
              {health.capped && (
                <Alert>
                  <AlertDescription>
                    {t(
                      'Cohort capped at 10000 attempts; rates and alerts suppressed.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              {health.alerts.length > 0 && (
                <Alert variant='destructive'>
                  <AlertDescription>
                    {t(
                      'Final failures reached 10% in a 15-minute cohort of at least 100 known requests. This is a local warning, not a baseline regression alert.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
            </>
          )}
        </>
      )}
    </section>
  )
}
