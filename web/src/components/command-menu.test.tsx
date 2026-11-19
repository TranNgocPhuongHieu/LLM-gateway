// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CommandMenu } from './command-menu'
import { ROLE } from '@/lib/roles'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('@/features/topup-management/use-topup-review-badge', () => ({ useTopupReviewBadge: () => undefined }))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useLocation: (options?: { select?: (location: { pathname: string }) => unknown }) => {
    const location = { pathname: '/dashboard/overview' }
    return options?.select ? options.select(location) : location
  },
}))
vi.mock('@/context/search-provider', () => ({
  useSearch: () => ({ open: true, setOpen: vi.fn() }),
}))
vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ setTheme: vi.fn() }),
}))
// Keep the real role gating in use-sidebar-view; only skip the server-driven overlay.
vi.mock('@/hooks/use-sidebar-config', () => ({
  useSidebarConfig: (groups: unknown) => groups,
}))

const customerOnlyItems = ['Overview', 'Wallet', 'Profile']
const adminOnlyItems = ['Channels', 'Request Logs', 'Users', 'System Settings']

function setRole(role: number) {
  useAuthStore.getState().auth.setUser({ id: 1, role } as unknown as AuthUser)
}

beforeEach(() => {
  Element.prototype.scrollIntoView = () => {}
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('CommandMenu', () => {
  it('lists only the sidebar items a customer can see', () => {
    setRole(ROLE.USER)
    render(<CommandMenu />)

    for (const title of customerOnlyItems) {
      expect(screen.getByText(title)).not.toBeNull()
    }
    for (const title of adminOnlyItems) {
      expect(screen.queryByText(title)).toBeNull()
    }
  })

  it('lists the admin group for a super admin', () => {
    setRole(ROLE.SUPER_ADMIN)
    render(<CommandMenu />)

    for (const title of [...customerOnlyItems, ...adminOnlyItems]) {
      expect(screen.getByText(title)).not.toBeNull()
    }
  })
})
