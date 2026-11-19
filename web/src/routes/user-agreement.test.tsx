// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { Route } from './user-agreement'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('@/lib/api', () => ({ api: { get: vi.fn() } }))

const UserAgreement = Route.options.component as () => React.ReactElement

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <UserAgreement />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.mocked(api.get).mockReset()
})

afterEach(() => {
  cleanup()
})

describe('user agreement page', () => {
  it('requests the public endpoint and renders the configured Markdown', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, message: '', data: '# Service terms\n\nYou agree to fair use.' },
    })
    renderPage()

    expect(await screen.findByRole('heading', { name: 'Service terms' })).not.toBeNull()
    expect(screen.getByText('You agree to fair use.')).not.toBeNull()
    expect(api.get).toHaveBeenCalledWith('/api/user-agreement', expect.anything())
  })

  it('shows the empty state when no agreement is configured', async () => {
    vi.mocked(api.get).mockResolvedValue({ data: { success: true, message: '', data: '' } })
    renderPage()

    expect(
      await screen.findByText('No user agreement has been published yet.')
    ).not.toBeNull()
    expect(screen.queryByText('You agree to fair use.')).toBeNull()
  })

  it('links to an external agreement URL instead of embedding it', async () => {
    vi.mocked(api.get).mockResolvedValue({
      data: { success: true, message: '', data: 'https://example.com/terms' },
    })
    renderPage()

    const link = await screen.findByRole('link', { name: 'Open the user agreement' })
    expect(link.getAttribute('href')).toBe('https://example.com/terms')
  })

  it('shows an error state when the request fails', async () => {
    vi.mocked(api.get).mockRejectedValue(new Error('network down'))
    renderPage()

    expect(await screen.findByText('Could not load the user agreement')).not.toBeNull()
  })
})
