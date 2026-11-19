import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  formatThresholdCredits,
  NotificationTab,
  parseThresholdCredits,
  thresholdCreditsToVnd,
} from './notification-tab'

const mocks = vi.hoisted(() => ({ update: vi.fn() }))
const i18n = vi.hoisted(() => ({
  t: (key: string, options?: Record<string, unknown>) =>
    options
      ? Object.entries(options).reduce(
          (text, [name, value]) => text.replace(`{{${name}}}`, String(value)),
          key,
        )
      : key,
}))

vi.mock('react-i18next', () => ({ useTranslation: () => i18n }))
vi.mock('sonner', () => ({ toast: { success: vi.fn() } }))
vi.mock('../../api', () => ({ updateUserSettings: mocks.update }))

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

// Default display config (quotaPerUnit 500000, 26 VND per USD unit / 1000)
// is used here, so 500000 quota units equal 26 credit (26,000 VND).
describe('quota warning threshold unit conversion', () => {
  it('round-trips stored quota units through credits without drift', () => {
    for (const quota of [1, 500000, 123457, 2000000]) {
      const credits = Number(formatThresholdCredits(quota))
      expect(parseThresholdCredits(credits)).toBe(quota)
    }
  })

  it('converts the default threshold to 26 credit and 26,000 VND', () => {
    expect(formatThresholdCredits(500000)).toBe('26')
    expect(thresholdCreditsToVnd(500000)).toBe(26000)
  })

  it('stores the converted quota when the user types credits', async () => {
    mocks.update.mockResolvedValue({ success: true })
    const profile = {
      id: 1,
      role: 1,
      setting: JSON.stringify({ quota_warning_threshold: 500000 }),
    } as never

    render(<NotificationTab profile={profile} onUpdate={() => {}} />)
    const input = screen.getByLabelText('Quota Warning Threshold') as HTMLInputElement
    expect(input.value).toBe('26')
    expect(screen.getByText(/≈ 26,000 VND/)).toBeTruthy()

    fireEvent.change(input, { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Settings' }))

    await waitFor(() => expect(mocks.update).toHaveBeenCalledTimes(1))
    const payload = mocks.update.mock.calls[0][0]
    expect(payload.quota_warning_threshold).toBe(parseThresholdCredits(2))
    expect(payload.quota_warning_threshold).toBe(38462)
    // The persisted key is unchanged.
    expect(Object.keys(payload)).toContain('quota_warning_threshold')
  })
})
