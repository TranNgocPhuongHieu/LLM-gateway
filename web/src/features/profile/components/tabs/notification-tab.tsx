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
import { Loader2 } from 'lucide-react'
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { quotaUnitsToDollars, parseQuotaFromDollars } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { ROLE } from '@/lib/roles'

import { updateUserSettings } from '../../api'
import { normalizeUserSettings } from '../../lib/user-settings'
import type { UserProfile } from '../../types'

// Site display rate: 1 credit = 1,000 VND (same as wallet-stats-card).
const VND_PER_CREDIT = 1000

// The stored threshold stays in quota units. These helpers only translate the
// value for display and input, using the same helpers as the rest of the app.
export function formatThresholdCredits(quota: number): string {
  const credits = quotaUnitsToDollars(quota)
  return String(Number(credits.toFixed(8)))
}

export function parseThresholdCredits(credits: number): number {
  return parseQuotaFromDollars(credits)
}

export function thresholdCreditsToVnd(quota: number): number {
  return Math.round(quotaUnitsToDollars(quota) * VND_PER_CREDIT)
}

// ============================================================================
// Settings Tab Component
// ============================================================================

interface NotificationTabProps {
  profile: UserProfile | null
  onUpdate: () => void
}

export function NotificationTab({ profile, onUpdate }: NotificationTabProps) {
  const { t } = useTranslation()
  const isAdmin = (profile?.role ?? 0) >= ROLE.ADMIN
  const [loading, setLoading] = useState(false)
  const [settings, setSettings] = useState(() => normalizeUserSettings())
  // Text the user is typing in credits; kept separately so partial input such
  // as "1." is not rewritten while typing.
  const [thresholdCreditText, setThresholdCreditText] = useState(() =>
    formatThresholdCredits(normalizeUserSettings().quota_warning_threshold)
  )

  // Update form field helper
  const updateField = useCallback(
    <K extends keyof typeof settings>(
      field: K,
      value: (typeof settings)[K]
    ) => {
      setSettings((prev) => ({ ...prev, [field]: value }))
    },
    []
  )

  useEffect(() => {
    if (profile?.setting) {
      const next = normalizeUserSettings(profile.setting)
      setSettings(next)
      setThresholdCreditText(formatThresholdCredits(next.quota_warning_threshold))
    }
  }, [profile])

  const handleThresholdChange = (text: string) => {
    setThresholdCreditText(text)
    const credits = Number(text)
    updateField(
      'quota_warning_threshold',
      Number.isFinite(credits) ? parseThresholdCredits(credits) : 0
    )
  }

  const handleSave = async () => {
    try {
      setLoading(true)
      const { record_ip_log: _recordIpLog, ...notificationSettings } = settings
      const response = await updateUserSettings(notificationSettings)

      if (response.success) {
        toast.success(t('Settings updated successfully'))
        onUpdate()
      } else {
        handleServerError(response, t('Failed to update settings'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to update settings'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className='space-y-4 sm:space-y-6'>
      {/* Warning Threshold */}
      <div className='space-y-1.5'>
        <Label htmlFor='threshold'>{t('Quota Warning Threshold')}</Label>
        <div className='flex items-center gap-2'>
          <Input
            id='threshold'
            type='number'
            step='any'
            className='h-9'
            value={thresholdCreditText}
            onChange={(e) => handleThresholdChange(e.target.value)}
            placeholder={t('Enter threshold')}
            aria-describedby='threshold-help'
          />
          <span className='text-muted-foreground shrink-0 text-xs'>
            {t('credit')}
          </span>
        </div>
        <p id='threshold-help' className='text-muted-foreground text-xs'>
          {t('Get notified when balance falls below this value')}
          {' · '}
          {t('≈ {{vnd}} VND', {
            vnd: new Intl.NumberFormat().format(
              thresholdCreditsToVnd(settings.quota_warning_threshold)
            ),
          })}
        </p>
      </div>

      {/* Email Settings */}

      <div className='space-y-1.5'>
        <Label htmlFor='notifyEmail'>{t('Notification Email')}</Label>
        <Input
          id='notifyEmail'
          type='email'
          className='h-9'
          value={settings.notification_email}
          onChange={(e) => updateField('notification_email', e.target.value)}
          placeholder={t('Leave empty to use account email')}
        />
      </div>

      {/* Divider */}
      <div className='border-t' />

      {/* Preferences Section */}
      <div className='space-y-3'>
        <div>
          <h4 className='text-sm font-medium'>{t('Preferences')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Configure your account behavior preferences')}
          </p>
        </div>

        {/* Receive Upstream Model Update Notifications (admin only) */}
        {isAdmin && (
          <div className='flex items-start justify-between gap-3 rounded-lg border p-3 sm:items-center sm:p-4'>
            <div className='space-y-0.5'>
              <Label htmlFor='upstreamModelUpdateNotify'>
                {t('Receive Upstream Model Update Notifications')}
              </Label>
              <p className='text-muted-foreground line-clamp-3 text-xs sm:line-clamp-none sm:text-sm'>
                {t(
                  'Only available for admins. When enabled, you will receive a summary notification via your selected method when the scheduled model check detects upstream model changes or check failures.'
                )}
              </p>
            </div>
            <Switch
              id='upstreamModelUpdateNotify'
              className='shrink-0'
              checked={settings.upstream_model_update_notify_enabled}
              onCheckedChange={(checked) =>
                updateField('upstream_model_update_notify_enabled', checked)
              }
            />
          </div>
        )}

        {/* Accept Unset Model Price */}
        <div className='flex items-start justify-between gap-3 rounded-lg border p-3 sm:items-center sm:p-4'>
          <div className='space-y-0.5'>
            <Label htmlFor='acceptUnsetPrice'>
              {t('Accept Unpriced Models')}
            </Label>
            <p className='text-muted-foreground text-xs sm:text-sm'>
              {t('Allow using models without price configuration')}
            </p>
          </div>
          <Switch
            id='acceptUnsetPrice'
            className='shrink-0'
            checked={settings.accept_unset_model_ratio_model}
            onCheckedChange={(checked) =>
              updateField('accept_unset_model_ratio_model', checked)
            }
          />
        </div>
      </div>

      {/* Save Button */}
      <div className='flex justify-end'>
        <Button onClick={handleSave} disabled={loading}>
          {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
          {loading ? t('Saving...') : t('Save Settings')}
        </Button>
      </div>
    </div>
  )
}
