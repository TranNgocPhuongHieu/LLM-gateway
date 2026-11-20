// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SystemBehaviorSection } from './system-behavior-section'

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

const defaults = { DefaultCollapseSidebar: false }

afterEach(() => {
  cleanup()
  mutateAsync.mockReset()
})

describe('SystemBehaviorSection', () => {
  it('renders only the Default Collapse Sidebar toggle', () => {
    render(<SystemBehaviorSection defaultValues={defaults} />)
    expect(screen.getByText('Default Collapse Sidebar')).toBeTruthy()
    expect(screen.getAllByRole('switch')).toHaveLength(1)
  })

  it('does not render Demo Site Mode or Self-Use Mode toggles', () => {
    render(<SystemBehaviorSection defaultValues={defaults} />)
    expect(screen.queryByText('Demo Site Mode')).toBeNull()
    expect(screen.queryByText('Self-Use Mode')).toBeNull()
  })

  it('saves only DefaultCollapseSidebar', async () => {
    const user = userEvent.setup()
    render(<SystemBehaviorSection defaultValues={defaults} />)
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: 'save' }))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith({
      key: 'DefaultCollapseSidebar',
      value: true,
    })
  })
})
