import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SystemBrand } from './system-brand'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'vi' } }),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: { system_name: 'AI giá rẻ' } }),
}))

vi.mock('@/hooks/use-system-config', () => ({
  useSystemConfig: () => ({ logo: '/logo.png?v=test' }),
}))

afterEach(cleanup)

describe('SystemBrand', () => {
  it('keeps the configured logo visible in the authenticated app header', () => {
    render(<SystemBrand variant='inline' />)

    const logo = screen.getByRole('img', { name: 'Logo' })
    expect(logo.getAttribute('src')).toBe('/logo.png?v=test')
    expect(logo.className).not.toContain('opacity-0')
  })
})
