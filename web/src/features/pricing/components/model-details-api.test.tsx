// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import type { PricingModel } from '../types'
import { ModelDetailsApi } from './model-details-api'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}))
vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: null, loading: false, error: null }),
}))
vi.mock('@/components/ai-elements/code-block', () => ({
  CodeBlock: () => null,
  CodeBlockCopyButton: () => null,
}))

afterEach(cleanup)

it('renders no fabricated rate-limit or supported-parameter sections', () => {
  const model: PricingModel = {
    id: 1,
    model_name: 'fixture-chat-model',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['default'],
    supported_endpoint_types: ['openai'],
  }
  render(
    <ModelDetailsApi
      model={model}
      endpointMap={{ openai: { path: '/v1/chat/completions', method: 'POST' } }}
    />
  )
  expect(screen.getByText('Code samples')).toBeTruthy()
  expect(screen.getByText('Authentication')).toBeTruthy()
  expect(screen.queryByText('Supported parameters')).toBeNull()
  expect(screen.queryByText('Rate limits')).toBeNull()
})
