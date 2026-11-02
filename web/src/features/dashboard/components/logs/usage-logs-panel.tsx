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
import { FadeIn } from '@/components/page-transition'
import { CacheStatsDialog } from '@/features/system-settings/general/channel-affinity/cache-stats-dialog'
import { UserInfoDialog } from '@/features/usage-logs/components/dialogs/user-info-dialog'
import { UsageLogsTable } from '@/features/usage-logs/components/usage-logs-table'
import { useUsageLogsContext } from '@/features/usage-logs/components/usage-logs-provider'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type { DashboardFilters } from '../../types'
import { LogStatCards } from '../models/log-stat-cards'
import { PerformanceOverview } from '../models/performance-overview'

interface DashboardUsageLogsPanelProps {
  filters: DashboardFilters
  tableOnly?: boolean
}

/**
 * Usage-log tab inside the dashboard.
 *
 * The statistics cards and the performance health bar stay above the table,
 * exactly like the dashboard reference. The table itself is the existing
 * `UsageLogsTable` — same API, columns, filters, permissions and loading
 * behaviour as the usage-log feature; nothing from the administrator request
 * logs is applied here.
 */
export function DashboardUsageLogsPanel(props: DashboardUsageLogsPanelProps) {
  const userRole = useAuthStore((state) => state.auth.user?.role)
  const isAdmin = Boolean(userRole && userRole >= ROLE.ADMIN)
  const {
    selectedUserId,
    userInfoDialogOpen,
    setUserInfoDialogOpen,
    affinityTarget,
    affinityDialogOpen,
    setAffinityDialogOpen,
  } = useUsageLogsContext()

  return (
    <div className='flex flex-col gap-3 sm:gap-4'>
      {!props.tableOnly && (
        <FadeIn>
          <LogStatCards filters={props.filters} />
        </FadeIn>
      )}
      {!props.tableOnly && isAdmin && (
        <FadeIn delay={0.05}>
          <PerformanceOverview />
        </FadeIn>
      )}
      <FadeIn delay={0.1}>
        <UsageLogsTable
          logCategory='common'
          dashboardControls
          showActualModel={false}
        />
      </FadeIn>

      <UserInfoDialog
        userId={selectedUserId}
        open={userInfoDialogOpen}
        onOpenChange={setUserInfoDialogOpen}
      />
      <CacheStatsDialog
        open={affinityDialogOpen}
        onOpenChange={setAffinityDialogOpen}
        target={
          affinityTarget
            ? {
                rule_name: affinityTarget.rule_name || '',
                using_group:
                  affinityTarget.using_group ||
                  affinityTarget.selected_group ||
                  '',
                key_hint: affinityTarget.key_hint || '',
                key_fp: affinityTarget.key_fp || '',
              }
            : null
        }
      />
    </div>
  )
}
