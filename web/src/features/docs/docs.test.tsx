import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { getDocPages, resolveApiOrigin } from './content'

const ORIGIN = 'https://gw.example.test'
const TOOL_SLUGS = ['claude-code', 'codex', 'hermes-agent', 'opencode', 'pi', 'openclaw', 'cursor']

const state = vi.hoisted(() => ({ language: 'en', status: { server_address: 'https://gw.example.test/' } as Record<string, unknown> | null, copy: vi.fn(async (_text: string) => true) }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ i18n: { language: state.language } }) }))
vi.mock('@tanstack/react-router', () => ({ Link: ({ children }: { children: ReactNode }) => <a>{children}</a> }))
vi.mock('@/components/layout', () => ({ PublicLayout: ({ children }: { children: ReactNode }) => <div>{children}</div> }))
vi.mock('@/components/layout/components/footer', () => ({ Footer: () => null }))
vi.mock('@/hooks/use-status', () => ({ useStatus: () => ({ status: state.status, loading: false, error: null }) }))
vi.mock('@/hooks/use-copy-to-clipboard', () => ({ useCopyToClipboard: () => ({ copiedText: null, copyToClipboard: state.copy }) }))

afterEach(() => { cleanup(); state.language = 'en'; state.status = { server_address: 'https://gw.example.test/' }; vi.clearAllMocks() })

describe('customer documentation', () => {
  it('covers the same 18 sections in both languages without retired clients or internal data', () => {
    const en = getDocPages(false, ORIGIN)
    const vn = getDocPages(true, ORIGIN)
    expect(en).toHaveLength(18)
    expect(en.map((page) => page.slug)).toEqual(vn.map((page) => page.slug))
    expect(new Set(en.map((page) => page.slug)).size).toBe(18)
    expect(en.map((page) => page.slug)).toEqual(expect.arrayContaining(TOOL_SLUGS))
    for (const retired of ['kilo-code', 'roo-code', 'factory-droid', 'copilot']) expect(en.some((page) => page.slug === retired)).toBe(false)
    const guides = JSON.stringify(en.filter((page) => page.slug !== 'changelog'))
    expect(guides).not.toMatch(/Kilo|Roo Code|Droid|Copilot/)
    for (const page of en) {
      expect(page.blocks.length).toBeGreaterThan(0)
      expect(vn.find((item) => item.slug === page.slug)?.summary).not.toBe(page.summary)
    }
    expect(JSON.stringify([en, vn])).not.toMatch(/Cherry Studio|CC Switch|localhost|trnhieu|sk-[A-Za-z0-9]{12}|XPiki/)
    expect(en.flatMap((page) => page.blocks).filter((block) => block.code).every((block) => !block.code?.includes('\n+'))).toBe(true)
  })

  it('uses the correct provider URL conventions and disables automatic SDK retries', () => {
    const pages = getDocPages(false, `${ORIGIN}/`)
    const content = (slug: string) => pages.find((page) => page.slug === slug)!.blocks.map((block) => block.code || '').join('\n')
    expect(content('codex')).toContain('wire_api = "responses"')
    expect(content('codex')).toContain(`base_url = "${ORIGIN}/v1"`)
    expect(content('claude-code')).toContain(`ANTHROPIC_BASE_URL='${ORIGIN}'`)
    expect(content('claude-code')).toContain('ANTHROPIC_AUTH_TOKEN')
    expect(content('claude-code')).not.toContain(`${ORIGIN}/v1`)
    expect(content('hermes-agent')).toContain(`api: ${ORIGIN}/v1`)
    expect(content('hermes-agent')).toContain('transport: chat_completions')
    expect(content('opencode')).toContain(`"baseURL": "${ORIGIN}/v1"`)
    expect(content('opencode')).toContain('@opencode/ai/providers/openai-compatible')
    expect(content('pi')).toContain('"api": "openai-completions"')
    expect(content('pi')).toContain('"authHeader": true')
    expect(content('openclaw')).toContain(`baseUrl: "${ORIGIN}/v1",\n        apiKey: "\${LLM_API_KEY}",\n        api: "openai-completions"`)
    expect(content('openclaw')).toContain(`baseUrl: "${ORIGIN}",\n        apiKey: "\${LLM_API_KEY}",\n        api: "anthropic-messages"`)
    expect(content('cursor')).toContain(`Override OpenAI Base URL: ${ORIGIN}/v1`)
    expect(pages.flatMap((page) => page.blocks).filter((block) => block.code).some((block) => block.code!.includes('/v1/v1'))).toBe(false)
    expect(content('openai-sdk')).toContain('max_retries=0')
    expect(content('anthropic-sdk')).toContain('maxRetries: 0')
  })

  it('searches full page content and handles no results', async () => {
    const { Docs } = await import('./index')
    render(<Docs />)
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'wire_api' } })
    const navigation = screen.getByRole('navigation', { name: 'Documentation navigation' })
    expect(navigation.textContent).toContain('Application integrationsCodex')
    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'Model unavailable' } })
    expect(navigation.textContent).toContain('Application integrationsOpenCode')
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
    expect(Array.from(navigation().querySelectorAll('section'), (section) => section.querySelectorAll('a').length)).toEqual([4, 7, 4, 3])
    expect(Array.from(navigation().querySelectorAll('a'), (item) => item.textContent)).toEqual(getDocPages(false, ORIGIN).map((page) => page.title))
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
    fireEvent.click(screen.getAllByRole('button', { name: 'Copy code' })[1])
    await waitFor(() => expect(screen.getByRole('alert').textContent).toContain('Copy failed'))
    expect(state.copy).toHaveBeenCalledWith(expect.stringContaining('env_key = "LLM_API_KEY"'))
  })

  it('renders Vietnamese and a recoverable unknown page', async () => {
    const { Docs } = await import('./index')
    state.language = 'vi'
    const view = render(<Docs slug='quickstart' />)
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Bắt đầu nhanh')
    view.rerender(<Docs slug='unknown' />)
    expect(screen.getByRole('heading', { level: 1 }).textContent).toBe('Không tìm thấy tài liệu')
  })

  it('builds every tool guide with protocol, base URL, key, model, config and errors', () => {
    for (const vn of [false, true]) {
      const pages = getDocPages(vn, ORIGIN)
      for (const slug of TOOL_SLUGS) {
        const page = pages.find((item) => item.slug === slug)!
        const headings = page.blocks.map((block) => block.heading).filter(Boolean)
        expect(headings).toHaveLength(6)
        expect(headings).toContain('Base URL')
        for (const line of page.blocks.find((block) => block.heading === 'Base URL')!.code!.split('\n')) expect(line).toMatch(new RegExp(`(^|: )${ORIGIN}(/v1)?$`))
        expect(page.blocks.filter((block) => block.code).length).toBeGreaterThan(1)
        expect(page.blocks.at(-1)?.items?.length).toBeGreaterThan(2)
      }
    }
  })

  it('takes the public API origin from status server_address with a site fallback', async () => {
    expect(resolveApiOrigin({ server_address: ' https://api.example.test/ ' }, 'https://site.test')).toBe('https://api.example.test')
    expect(resolveApiOrigin({ data: { server_address: 'https://nested.test' } }, 'https://site.test')).toBe('https://nested.test')
    expect(resolveApiOrigin({ server_address: '' }, 'https://site.test/')).toBe('https://site.test')
    expect(resolveApiOrigin(null, 'https://site.test')).toBe('https://site.test')
    const { Docs } = await import('./index')
    const view = render(<Docs slug='codex' />)
    expect(view.container.textContent).toContain('base_url = "https://gw.example.test/v1"')
    state.status = null
    view.rerender(<Docs slug='codex' />)
    expect(view.container.textContent).toContain(`base_url = "${window.location.origin}/v1"`)
  })
})
