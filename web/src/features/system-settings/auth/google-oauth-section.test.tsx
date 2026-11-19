// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { GoogleOAuthSection, googleCallbackUrl } from './google-oauth-section'

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
vi.mock('../components/settings-section', () => ({
  SettingsSection: (props: { children: React.ReactNode }) => <div>{props.children}</div>,
}))

afterEach(() => {
  cleanup()
  mutateAsync.mockReset()
})

describe('GoogleOAuthSection', () => {
  it('saves client ID and secret before enabling sign-in', async () => {
    const user = userEvent.setup()
    render(
      <GoogleOAuthSection
        serverAddress='https://gw.test/'
        defaultValues={{ GoogleOAuthEnabled: false, GoogleClientId: '', GoogleClientSecret: '' }}
      />
    )
    expect(screen.getByDisplayValue('https://gw.test/api/oauth/google/callback')).toBeTruthy()
    await user.type(screen.getByPlaceholderText('xxxx.apps.googleusercontent.com'), 'id-1.apps.googleusercontent.com')
    await user.type(screen.getByPlaceholderText('Leave empty to keep the saved secret'), 'GOCSPX-test')
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: 'save' }))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(3))
    expect(mutateAsync.mock.calls.map(([arg]) => arg.key)).toEqual([
      'GoogleClientId',
      'GoogleClientSecret',
      'GoogleOAuthEnabled',
    ])
    expect(mutateAsync.mock.calls[0][0].value).toBe('id-1.apps.googleusercontent.com')
  })

  it('keeps typed values when the page re-renders with equal settings', async () => {
    const user = userEvent.setup()
    const props = () => ({
      serverAddress: 'https://gw.test',
      defaultValues: { GoogleOAuthEnabled: false, GoogleClientId: '', GoogleClientSecret: '' },
    })
    const { rerender } = render(<GoogleOAuthSection {...props()} />)
    const input = screen.getByPlaceholderText('xxxx.apps.googleusercontent.com') as HTMLInputElement
    await user.type(input, 'id-1')
    // A settings refetch (e.g. on window focus) rebuilds an equal props object.
    rerender(<GoogleOAuthSection {...props()} />)
    expect(input.value).toBe('id-1')
  })

  it('derives the redirect URI from the server address', () => {
    expect(googleCallbackUrl('https://gw.test//')).toBe('https://gw.test/api/oauth/google/callback')
    expect(googleCallbackUrl('')).toBe('')
  })
})
