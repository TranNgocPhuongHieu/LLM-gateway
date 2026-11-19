// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useSystemConfigStore } from '@/stores/system-config-store'

import { QuotaSettingsSection } from './quota-settings-section'

const mutateAsync = vi.hoisted(() => vi.fn())

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (k: string) => k }) }))
vi.mock('../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))
vi.mock('../components/settings-page-context', () => ({
  SettingsPageFormActions: (props: { onSave: () => void }) => (
    <button type='button' onClick={props.onSave}>
      save
    </button>
  ),
}))
vi.mock('../components/form-dirty-indicator', () => ({ FormDirtyIndicator: () => null }))
vi.mock('../components/form-navigation-guard', () => ({ FormNavigationGuard: () => null }))
vi.mock('../components/settings-section', () => ({
  SettingsSection: (props: { children: React.ReactNode }) => <div>{props.children}</div>,
}))

const defaults = (quotaForNewUser: number) => ({
  QuotaForNewUser: quotaForNewUser,
  PreConsumedQuota: 500,
  QuotaForInviter: 0,
  QuotaForInvitee: 0,
  TopUpLink: '',
  general_setting: { docs_link: '' },
  quota_setting: { enable_free_model_pre_consume: true },
})

beforeEach(() => {
  // Production display: 1 USD = 26 credit, 500,000 quota per USD.
  useSystemConfigStore.getState().setConfig({
    currency: {
      quotaDisplayType: 'CUSTOM',
      customCurrencyExchangeRate: 26000,
      quotaPerUnit: 500000,
    },
  } as never)
})

afterEach(() => {
  cleanup()
  mutateAsync.mockReset()
})

describe('QuotaSettingsSection new user credit', () => {
  it('shows the stored quota as credits', () => {
    render(<QuotaSettingsSection defaultValues={defaults(96154)} />)
    expect(screen.getByLabelText('New user credit')).toHaveProperty('value', '5')
  })

  it('saves credits typed by the admin as quota units', async () => {
    const user = userEvent.setup()
    render(<QuotaSettingsSection defaultValues={defaults(0)} />)
    const input = screen.getByLabelText('New user credit')
    await user.clear(input)
    await user.type(input, '5')
    await user.click(screen.getByRole('button', { name: 'save' }))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith({ key: 'QuotaForNewUser', value: 96154 })
  })
})
