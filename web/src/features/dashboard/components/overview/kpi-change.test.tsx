// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { formatPercent } from '@/lib/format'

import { KpiChange } from './kpi-change'

const i18n = vi.hoisted(() => ({
  t: (key: string) => key,
  i18n: { language: 'vi', resolvedLanguage: 'vi' },
}))
vi.mock('react-i18next', () => ({ useTranslation: () => i18n }))

afterEach(() => cleanup())

describe('KpiChange', () => {
  it('uses the UI language decimal separator, not the browser one', () => {
    render(<KpiChange change={-72.14} />)
    expect(screen.getByText('▼ 72,14%')).toBeTruthy()
  })

  it('labels a rise from an almost empty period as new and hides null', () => {
    const { container } = render(<KpiChange change={null} />)
    expect(container.textContent).toBe('')
    render(<KpiChange change={5000} />)
    expect(screen.getByText('New')).toBeTruthy()
  })
})

describe('formatPercent', () => {
  it('formats with the given locale', () => {
    expect(formatPercent(0.05, 'vi')).toBe('0,05%')
    expect(formatPercent(0.05, 'en')).toBe('0.05%')
  })
})
