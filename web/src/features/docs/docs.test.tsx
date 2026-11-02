import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { getDocPages } from './content'

const state = vi.hoisted(() => ({ language: 'en', copy: vi.fn(async (_text: string) => true) }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ i18n: { language: state.language } }) }))
vi.mock('@tanstack/react-router', () => ({ Link: ({ children }: { children: ReactNode }) => <a>{children}</a> }))
vi.mock('@/components/layout', () => ({ PublicLayout: ({ children }: { children: ReactNode }) => <div>{children}</div> }))
vi.mock('@/components/layout/components/footer', () => ({ Footer: () => null }))
vi.mock('@/hooks/use-copy-to-clipboard', () => ({ useCopyToClipboard: () => ({ copiedText: null, copyToClipboard: state.copy }) }))

afterEach(() => { cleanup(); state.language = 'en'; vi.clearAllMocks() })

describe('customer documentation', () => {
  it('covers the same 15 sections in both languages without retired clients or internal data', () => {
    const en = getDocPages(false)
    const vn = getDocPages(true)
    expect(en).toHaveLength(15)
    expect(en.map((page) => page.slug)).toEqual(vn.map((page) => page.slug))
    expect(new Set(en.map((page) => page.slug)).size).toBe(15)
    for (const page of en) {
      expect(page.blocks.length).toBeGreaterThan(0)
      expect(vn.find((item) => item.slug === page.slug)?.summary).not.toBe(page.summary)
    }
    expect(JSON.stringify(en)).not.toMatch(/Cherry Studio|CC Switch|localhost|sk-[A-Za-z0-9]{12}|XPiki/)
    expect(en.flatMap((page) => page.blocks).filter((block) => block.code).every((block) => !block.code?.includes('\n+'))).toBe(true)
  })

  it('uses the correct provider URL conventions and disables automatic SDK retries', () => {
    const pages = getDocPages(false)
    const content = (slug: string) => pages.find((page) => page.slug === slug)!.blocks.map((block) => block.code || '').join('\n')
    expect(content('codex')).toContain('wire_api = "responses"')
    expect(content('codex')).toContain('https://llm.trnhieu.dev/v1')
    expect(content('claude-code')).toContain("ANTHROPIC_BASE_URL='https://llm.trnhieu.dev'")
    expect(content('openai-sdk')).toContain('max_retries=0')
    expect(content('anthropic-sdk')).toContain('maxRetries: 0')
  })

  it('searches full page content and handles no results', async () => {
    const { Docs } = await import('./index')
    render(<Docs />)
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'wire_api' } })
    const navigation = screen.getByRole('navigation', { name: 'Documentation navigation' })
    expect(navigation.textContent).toContain('Application integrationsCodex')
    expect(navigation.querySelectorAll('h2')).toHaveLength(1)
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'nonexistent-section' } })
    expect(screen.getByRole('status').textContent).toBe('No results.')
  })

  it('organizes every article once under four localized navigation groups', async () => {
    const { Docs } = await import('./index')
    const view = render(<Docs />)
    const navigation = () => screen.getByRole('navigation', { name: state.language === 'vi' ? 'Mục lục tài liệu' : 'Documentation navigation' })
    expect(Array.from(navigation().querySelectorAll('h2'), (item) => item.textContent)).toEqual([
      'Getting started', 'Application integrations', 'SDK & API', 'Operations & support',
    ])
    expect(Array.from(navigation().querySelectorAll('section'), (section) => section.querySelectorAll('a').length)).toEqual([4, 4, 4, 3])
    expect(Array.from(navigation().querySelectorAll('a'), (item) => item.textContent)).toEqual(getDocPages(false).map((page) => page.title))
    state.language = 'vi'
    view.rerender(<Docs />)
    expect(Array.from(navigation().querySelectorAll('h2'), (item) => item.textContent)).toEqual([
      'Bắt đầu', 'Tích hợp ứng dụng', 'SDK & API', 'Vận hành & hỗ trợ',
    ])
  })

  it('copies only the public example and reports clipboard failure', async () => {
    const { Docs } = await import('./index')
    state.copy.mockResolvedValueOnce(false)
    render(<Docs slug='codex' />)
    fireEvent.click(screen.getAllByRole('button', { name: 'Copy code' })[0])
    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('Copy failed'))
    expect(state.copy).toHaveBeenCalledWith(expect.stringContaining('env_key = "AI_CHEAP_API_KEY"'))
  })

  it('renders Vietnamese and a recoverable unknown page', async () => {
    const { Docs } = await import('./index')
    state.language = 'vi'
    const view = render(<Docs slug='quickstart' />)
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Bắt đầu nhanh')
    view.rerender(<Docs slug='unknown' />)
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Không tìm thấy tài liệu')
  })
})
