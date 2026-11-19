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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { CopyButton } from '@/components/copy-button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const googleOAuthSchema = z.object({
  GoogleOAuthEnabled: z.boolean(),
  GoogleClientId: z.string().trim().optional(),
  GoogleClientSecret: z.string().trim().optional(),
})

type GoogleOAuthFormValues = z.infer<typeof googleOAuthSchema>

type GoogleOAuthSectionProps = {
  serverAddress: string
  defaultValues: GoogleOAuthFormValues
}

export function googleCallbackUrl(serverAddress: string): string {
  const base = serverAddress.trim().replace(/\/+$/, '')
  return base ? `${base}/api/oauth/google/callback` : ''
}

export function GoogleOAuthSection({
  serverAddress,
  defaultValues,
}: GoogleOAuthSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const callbackUrl = googleCallbackUrl(serverAddress)

  const form = useForm<GoogleOAuthFormValues>({
    resolver: zodResolver(googleOAuthSchema),
    defaultValues,
  })

  // Reset only when the saved values change. The registry rebuilds an equal
  // defaultValues object on every page re-render (e.g. a settings refetch on
  // window focus), which would otherwise wipe what the admin is typing.
  const savedEnabled = defaultValues.GoogleOAuthEnabled
  const savedClientId = defaultValues.GoogleClientId
  useEffect(() => {
    form.reset({
      GoogleOAuthEnabled: savedEnabled,
      GoogleClientId: savedClientId,
      GoogleClientSecret: '',
    })
  }, [savedEnabled, savedClientId, form])

  const onSubmit = async (data: GoogleOAuthFormValues) => {
    // The secret is write-only: the server never returns it, so an empty
    // field means "keep the stored secret" rather than "clear it".
    const updates = Object.entries(data).filter(([key, value]) => {
      if (key === 'GoogleClientSecret') return Boolean(value)
      return value !== defaultValues[key as keyof GoogleOAuthFormValues]
    })
    // Save credentials before the switch so sign-in never turns on half-configured.
    updates.sort(([a], [b]) =>
      a === 'GoogleOAuthEnabled' ? 1 : b === 'GoogleOAuthEnabled' ? -1 : 0
    )
    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
    if (data.GoogleClientSecret) form.setValue('GoogleClientSecret', '')
  }

  return (
    <SettingsSection title={t('Google Sign-in')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='GoogleOAuthEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Google sign-in')}</FormLabel>
                  <FormDescription>
                    {t(
                      'New accounts are created only through a verified Google email. Existing accounts can link Google in Security.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormItem>
            <FormLabel>{t('Authorized redirect URI')}</FormLabel>
            <div className='flex items-center gap-2'>
              <Input
                readOnly
                value={callbackUrl || t('Set the server address first')}
              />
              {callbackUrl && <CopyButton value={callbackUrl} />}
            </div>
            <FormDescription>
              {t(
                'Add this exact URI to the OAuth client in Google Cloud Console.'
              )}
            </FormDescription>
          </FormItem>

          <FormField
            control={form.control}
            name='GoogleClientId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Client ID')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder='xxxx.apps.googleusercontent.com'
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='GoogleClientSecret'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Client secret')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Leave empty to keep the saved secret')}
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('The secret is never shown again after saving.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
