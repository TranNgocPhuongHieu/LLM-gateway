// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { ModelCard } from './model-card'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}))
vi.mock('@/lib/api', () => ({ getPromotionAnnouncement: vi.fn().mockResolvedValue({ data: [] }) }))

afterEach(cleanup)

it('shows the group without displaying supported endpoints on the card', () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <ModelCard
        onClick={() => {}}
        model={{
          id: 1, model_name: 'fixture-model', quota_type: 0,
          model_ratio: 1, completion_ratio: 1, enable_groups: ['default'],
          supported_endpoint_types: ['anthropic', 'openai'],
        }}
      />
    </QueryClientProvider>
  )
  expect(screen.getByText('Groups')).toBeTruthy()
  expect(screen.getByText('default')).toBeTruthy()
  expect(screen.queryByText('Endpoints')).toBeNull()
  expect(screen.queryByText('anthropic, openai')).toBeNull()
  client.clear()
})
