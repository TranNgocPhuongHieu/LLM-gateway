import { readFileSync } from 'node:fs'
import { fileURLToPath, URL as NodeURL } from 'node:url'
import { describe, expect, it } from 'vitest'
import { ROLE } from '@/lib/roles'
import { DASHBOARD_SECTION_IDS } from '@/features/dashboard/section-registry'
import { resolveLogsViewAccess } from './components/usage-logs-provider'

function readSource(relativePath: string): string {
  return readFileSync(
    fileURLToPath(new NodeURL(relativePath, import.meta.url)),
    'utf8'
  )
}

describe('dashboard usage logs', () => {
  it('renders the usage log table inside the dashboard, not only on a standalone page', () => {
    // The dashboard must expose a Usage Logs tab...
    expect(DASHBOARD_SECTION_IDS).toContain('logs')
    expect(readSource('../dashboard/section-registry.tsx')).toContain(
      "id: 'logs'"
    )

    // ...and that tab must mount the shared usage-log table.
    const dashboard = readSource('../dashboard/index.tsx')
    expect(dashboard).toContain('DashboardUsageLogsPanel')
    expect(dashboard).toContain('UsageLogsProvider')
    expect(dashboard).toContain("'logs'")

    const panel = readSource(
      '../dashboard/components/logs/usage-logs-panel.tsx'
    )
    expect(panel).toContain(
      "from '@/features/usage-logs/components/usage-logs-table'"
    )
    expect(panel).toContain('showActualModel={false}')

    // The dashboard usage-log tab keeps its statistics above the table.
    expect(panel).toContain('LogStatCards')
    expect(panel).toContain('PerformanceOverview')
  })

  it('keeps the legacy usage-logs route pointing at the dashboard tab', () => {
    const route = readSource(
      '../../routes/_authenticated/usage-logs/$section.tsx'
    )
    expect(route).toContain("to: '/dashboard/$section'")
    expect(route).toContain("section: 'logs'")
  })

  it('does not leak the administrator request-log table into the dashboard', () => {
    const dashboard = readSource('../dashboard/index.tsx')
    const panel = readSource(
      '../dashboard/components/logs/usage-logs-panel.tsx'
    )
    for (const source of [dashboard, panel]) {
      expect(source).not.toContain('xpiki-quota')
      expect(source).not.toContain('XPikiLogsCard')
      expect(source).not.toContain('XPikiRequestLogsPage')
    }
  })

  it('keeps the usage-log table free of request-log only filters', () => {
    const table = readSource('./components/usage-logs-table.tsx')
    // Request-log multi-value filters belong to /request-logs only.
    expect(table).not.toContain('MultiSelect')
    expect(table).not.toContain('user_options')
  })

  it('renders logs in Activity and binds table hooks to the mounted dashboard route', () => {
    const dashboard = readSource('../dashboard/index.tsx')
    expect(dashboard).toContain(
      '<DashboardLogsSection filters={modelFilters} tableOnly />'
    )
    for (const file of [
      './components/usage-logs-table.tsx',
      './components/common-logs-filter-bar.tsx',
    ]) {
      const source = readSource(file)
      expect(source).toContain(
        "getRouteApi('/_authenticated/dashboard/$section')"
      )
      expect(source).not.toContain(
        "getRouteApi('/_authenticated/usage-logs/$section')"
      )
    }
  })

  it('keeps dashboard log refresh controls bounded to 30 seconds, 5 minutes, and 15 minutes', () => {
    const table = readSource('./components/usage-logs-table.tsx')
    const toolbar = readSource('./components/common-logs-filter-bar.tsx')
    expect(toolbar).toContain('value={30_000}')
    expect(toolbar).toContain('value={300_000}')
    expect(toolbar).toContain('value={900_000}')
    expect(toolbar).toContain('Load new logs')
    expect(table).toContain(
      'refetchInterval: dashboardControls ? refreshInterval : false'
    )
  })

  it('restricts all users and channel details to root administrators', () => {
    expect(resolveLogsViewAccess(ROLE.ADMIN, 'all')).toBe('self')
    expect(resolveLogsViewAccess(ROLE.SUPER_ADMIN, 'all')).toBe('root')
    expect(resolveLogsViewAccess(ROLE.SUPER_ADMIN, 'self')).toBe('self')
  })
})
