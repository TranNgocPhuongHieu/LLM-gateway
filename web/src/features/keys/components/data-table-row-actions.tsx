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
import type { Row } from '@tanstack/react-table'
import { Trash2, Edit, ArrowRightLeft, Copy, Link, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableRowActionMenu } from '@/components/data-table/core/row-action-menu'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { encodeChannelConnectionInfo } from '@/lib/channel-connection-info'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { handleServerError } from '@/lib/handle-server-error'

import { updateApiKeyStatus } from '../api'
import {
  API_KEY_STATUS,
  API_KEY_STATUSES,
  ERROR_MESSAGES,
  SUCCESS_MESSAGES,
} from '../constants'
import { apiKeySchema } from '../types'
import { useApiKeys } from './api-keys-provider'

function getServerAddress(): string {
  try {
    const raw = localStorage.getItem('status')
    if (raw) {
      const status = JSON.parse(raw)
      if (status.server_address) return status.server_address as string
    }
  } catch {
    /* empty */
  }
  return window.location.origin
}

type DataTableRowActionsProps<TData> = {
  row: Row<TData>
}

export function DataTableRowActions<TData>({
  row,
}: DataTableRowActionsProps<TData>) {
  const { t } = useTranslation()
  const apiKey = apiKeySchema.parse(row.original)
  const {
    setOpen,
    setCurrentRow,
    triggerRefresh,
    setResolvedKey,
    resolveRealKey,
    loadingKeys,
  } = useApiKeys()
  const isEnabled = apiKey.status === API_KEY_STATUS.ENABLED
  const [isTogglingStatus, setIsTogglingStatus] = useState(false)
  const isRealKeyLoading = Boolean(loadingKeys[apiKey.id])

  const toggleLabel = isEnabled ? t('Disable') : t('Enable')
  const visibleStatusLabel = t(
    API_KEY_STATUSES[apiKey.status]?.label ?? 'Unknown'
  )

  const handleToggleStatus = async (enabled: boolean) => {
    const newStatus = enabled ? API_KEY_STATUS.ENABLED : API_KEY_STATUS.DISABLED

    setIsTogglingStatus(true)
    try {
      const result = await updateApiKeyStatus(apiKey.id, newStatus)
      if (result.success) {
        const message = enabled
          ? t(SUCCESS_MESSAGES.API_KEY_ENABLED)
          : t(SUCCESS_MESSAGES.API_KEY_DISABLED)
        toast.success(message)
        triggerRefresh()
      } else {
        handleServerError(result, t(ERROR_MESSAGES.STATUS_UPDATE_FAILED))
      }
    } catch (error) {
      handleServerError(error, t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsTogglingStatus(false)
    }
  }

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <div className='flex min-w-[94px] items-center gap-2'>
        <Switch
          size='sm'
          checked={isEnabled}
          disabled={isTogglingStatus}
          aria-label={toggleLabel}
          onClick={(event) => event.stopPropagation()}
          onCheckedChange={(checked) => void handleToggleStatus(checked)}
        />
        <span
          className={
            isEnabled
              ? 'text-xs font-medium text-emerald-700 dark:text-emerald-400'
              : 'text-muted-foreground min-w-[52px] text-xs font-medium'
          }
        >
          {visibleStatusLabel}
        </span>
        {isTogglingStatus && (
          <Loader2 className='text-muted-foreground size-3.5 animate-spin' />
        )}
      </div>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => {
                setCurrentRow(apiKey)
                setOpen('update')
              }}
              aria-label={t('Edit')}
            />
          }
        >
          <Edit />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <DataTableRowActionMenu
        ariaLabel={t('Open menu')}
        contentClassName='w-[200px]'
        modal={false}
      >
        <DropdownMenuItem
          disabled={isRealKeyLoading}
          onClick={async () => {
            const realKey = await resolveRealKey(apiKey.id)
            if (!realKey) return
            const ok = await copyToClipboard(realKey)
            if (ok) toast.success(t('Copied'))
          }}
        >
          {t('Copy Key')}
          <DropdownMenuShortcut>
            <Copy size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuItem
          disabled={isRealKeyLoading}
          onClick={async () => {
            const realKey = await resolveRealKey(apiKey.id)
            if (!realKey) return
            const connStr = encodeChannelConnectionInfo(
              realKey,
              getServerAddress()
            )
            const ok = await copyToClipboard(connStr)
            if (ok) toast.success(t('Copied'))
          }}
        >
          {t('Copy Connection Info')}
          <DropdownMenuShortcut>
            <Link size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={async () => {
            const realKey = await resolveRealKey(apiKey.id)
            if (!realKey) return
            setResolvedKey(realKey)
            setCurrentRow(apiKey)
            setOpen('cc-switch')
          }}
        >
          {t('CC Switch')}
          <DropdownMenuShortcut>
            <ArrowRightLeft size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => {
            setCurrentRow(apiKey)
            setOpen('delete')
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>
    </div>
  )
}
