import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const { models } = vi.hoisted(() => ({
  models: [
    { model_name: 'other-model' },
    { model_name: 'gpt-6-astra' },
    { model_name: 'claude-opus-5-5' },
    { model_name: 'gpt-6-luna' },
    { model_name: 'gpt-6.1-sol' },
  ],
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/features/pricing/hooks/use-pricing-data', () => ({
  usePricingData: () => ({
    models,
    isLoading: false,
    error: null,
    priceRate: 1,
    usdExchangeRate: 1,
  }),
}))

vi.mock('@/features/pricing/lib/dynamic-price', () => ({
  getDynamicDisplayGroupRatio: () => 1,
  getDynamicPricingSummary: (model: { model_name: string }) => ({
    tiers: [{}],
    hasRequestRules: false,
    isSpecialExpression: false,
    entries: ['inputPrice', 'cacheReadPrice', 'outputPrice'].map((field) => ({
      field,
      formatted: `${model.model_name}:${field}`,
    })),
  }),
}))

afterEach(cleanup)

describe('homepage pricing preview', () => {
  it('shows the four requested models in the configured order, independent of API order', async () => {
    const { PricingPreview } = await import('./pricing-preview')
    render(<PricingPreview />)

    const rows = screen.getAllByRole('row').slice(1)
    expect(rows.map((row) => row.querySelector('td')?.textContent)).toEqual([
      'gpt-6.1-sol',
      'gpt-6-luna',
      'claude-opus-5-5',
      'gpt-6-astra',
    ])
  })
})
