// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { Route } from './audit'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

// TanStack's redirect() throws a Response carrying the target in `options`.
function captureRedirect(): { to?: string } | null {
  try {
    Route.options.beforeLoad?.({} as never)
  } catch (error) {
    return (error as { options?: { to?: string } }).options ?? null
  }
  return null
}

function setRole(role: number | null) {
  useAuthStore
    .getState()
    .auth.setUser(role === null ? null : ({ id: 1, role } as unknown as AuthUser))
}

afterEach(() => {
  setRole(null)
})

describe('usage-logs audit route', () => {
  it('forwards super admins to the single request-logs page', () => {
    setRole(ROLE.SUPER_ADMIN)
    expect(captureRedirect()).toMatchObject({ to: '/request-logs' })
  })

  it('sends non super admins to 403 before any redirect to request-logs', () => {
    setRole(ROLE.USER)
    expect(captureRedirect()).toMatchObject({ to: '/403' })
  })

  it('sends signed-out visitors to 403', () => {
    setRole(null)
    expect(captureRedirect()).toMatchObject({ to: '/403' })
  })
})
