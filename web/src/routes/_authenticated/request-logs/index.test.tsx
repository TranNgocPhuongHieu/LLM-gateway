// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

import { Route } from './index'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

// TanStack's redirect() throws a Response carrying the target in `options`.
function captureRedirect(): { to?: string; params?: unknown; replace?: boolean } | null {
  try {
    Route.options.beforeLoad?.({} as never)
  } catch (error) {
    return (error as { options?: { to?: string; params?: unknown; replace?: boolean } }).options ?? null
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

describe('request-logs route', () => {
  it('redirects super admins to the admin usage logs', () => {
    setRole(ROLE.SUPER_ADMIN)
    expect(captureRedirect()).toMatchObject({
      to: '/dashboard/$section',
      params: { section: 'logs' },
      replace: true,
    })
  })

  it('sends non super admins to 403 before any redirect to the logs', () => {
    setRole(ROLE.USER)
    expect(captureRedirect()).toMatchObject({ to: '/403' })
  })

  it('sends signed-out visitors to 403', () => {
    setRole(null)
    expect(captureRedirect()).toMatchObject({ to: '/403' })
  })
})
