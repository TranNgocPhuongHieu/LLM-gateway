import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

import {
  CUSTOM_RANGE_MAX_DAYS,
  type RangeError,
  type RangePreset,
  type RangeState,
} from './overview-time-range'

const PRESETS: RangePreset[] = ['today', '7d', '30d', 'month', 'custom']

export function OverviewRangePicker(props: {
  value: RangeState
  onChange: (next: RangeState) => void
  todayKey: string
  error: RangeError | null
}) {
  const { t } = useTranslation()
  const { value, onChange } = props

  const presetLabel = (preset: RangePreset): string => {
    switch (preset) {
      case 'today':
        return t('Today')
      case '7d':
        return t('7 days')
      case '30d':
        return t('30 days')
      case 'month':
        return t('This month')
      case 'custom':
        return t('Custom')
    }
  }

  const errorMessage = (error: RangeError): string => {
    switch (error) {
      case 'order':
        return t('End date must be on or after the start date')
      case 'too_long':
        return t('Custom range can be at most {{days}} days', {
          days: CUSTOM_RANGE_MAX_DAYS,
        })
      case 'future':
        return t('Start date cannot be in the future')
      case 'invalid':
        return t('Choose a valid start and end date')
    }
  }

  return (
    <div className='flex flex-col gap-2'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Usage period')}
        </span>
        {/* Five equal columns on phones so "Custom" never wraps alone. */}
        <div
          role='group'
          aria-label={t('Usage period')}
          className='bg-background grid w-full grid-cols-5 gap-1 rounded-xl border p-1 sm:inline-flex sm:w-auto'
        >
          {PRESETS.map((preset) => {
            const active = value.preset === preset
            return (
              <button
                key={preset}
                type='button'
                aria-pressed={active}
                onClick={() => onChange({ ...value, preset })}
                className={cn(
                  'inline-flex h-7 min-w-0 items-center justify-center rounded-lg px-1.5 text-xs font-medium whitespace-nowrap transition-colors sm:px-3',
                  active
                    ? 'bg-muted text-foreground shadow-xs'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                {presetLabel(preset)}
              </button>
            )
          })}
        </div>
      </div>

      {value.preset === 'custom' && (
        <div className='flex flex-wrap items-center gap-2'>
          <label className='text-muted-foreground inline-flex items-center gap-1.5 text-xs'>
            <span>{t('From')}</span>
            <Input
              type='date'
              aria-label={t('Start date')}
              className='h-8 w-40 text-xs'
              max={props.todayKey}
              value={value.customStart}
              onChange={(event) =>
                onChange({ ...value, customStart: event.target.value })
              }
            />
          </label>
          <label className='text-muted-foreground inline-flex items-center gap-1.5 text-xs'>
            <span>{t('To')}</span>
            <Input
              type='date'
              aria-label={t('End date')}
              className='h-8 w-40 text-xs'
              max={props.todayKey}
              value={value.customEnd}
              onChange={(event) =>
                onChange({ ...value, customEnd: event.target.value })
              }
            />
          </label>
          {props.error && (
            <p role='alert' className='text-destructive text-xs'>
              {errorMessage(props.error)}
            </p>
          )}
        </div>
      )}
    </div>
  )
}
