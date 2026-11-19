// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { PaymentSettingsSection } from './payment-settings-section'

const mutateAsync = vi.hoisted(() => vi.fn())

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (k: string) => k }) }))
vi.mock('../api', () => ({ confirmPaymentCompliance: vi.fn() }))
vi.mock('../hooks/use-update-option', () => ({
  useUpdateOption: () => ({ mutateAsync, isPending: false }),
}))
vi.mock('../components/settings-page-context', () => ({
  SettingsPageFormActions: (props: { onSave: () => void }) => (
    <button type='button' onClick={props.onSave}>
      save
    </button>
  ),
}))
vi.mock('../components/settings-section', () => ({
  SettingsSection: (props: { children: React.ReactNode }) => <div>{props.children}</div>,
}))
vi.mock('@/components/json-code-editor', () => ({ JsonCodeEditor: () => null }))
vi.mock('./payment-methods-visual-editor', () => ({
  PaymentMethodsVisualEditor: () => <div>payment-methods-editor</div>,
}))
vi.mock('./amount-options-visual-editor', () => ({
  AmountOptionsVisualEditor: () => <div>amount-options-editor</div>,
}))
vi.mock('./amount-discount-visual-editor', () => ({
  AmountDiscountVisualEditor: () => <div>amount-discount-editor</div>,
}))

const defaults = {
  Price: 1,
  MinTopUp: 2,
  PayMethods: '[]',
  AmountOptions: '[10,20]',
  AmountDiscount: '{}',
}

const confirmed = { confirmed: true, termsVersion: 'v1', confirmedAt: 0, confirmedBy: 0 }

function renderSection(complianceDefaults = confirmed) {
  const queryClient = new QueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <PaymentSettingsSection
        defaultValues={defaults}
        complianceDefaults={complianceDefaults}
      />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mutateAsync.mockResolvedValue(undefined)
})

afterEach(() => {
  cleanup()
  mutateAsync.mockReset()
})

describe('PaymentSettingsSection', () => {
  it('renders the shared General fields without a tab strip', () => {
    renderSection()
    expect(screen.getByText('General Settings')).toBeTruthy()
    expect(screen.getByLabelText('Price (local currency / USD)')).toHaveProperty('value', '1')
    expect(screen.getByLabelText('Minimum top-up (USD)')).toHaveProperty('value', '2')
    expect(screen.getByText('Payment methods')).toBeTruthy()
    expect(screen.getByText('Top-up amount options')).toBeTruthy()
    expect(screen.getByText('Amount discount')).toBeTruthy()
    expect(screen.queryByRole('tab')).toBeNull()
  })

  it('does not render Epay or Stripe tabs or fields', () => {
    renderSection()
    for (const label of [
      'Epay',
      'Epay Gateway',
      'Epay safety reminder',
      'Epay endpoint',
      'Callback address',
      'Epay merchant ID',
      'Epay secret key',
      'Stripe',
      'Stripe Gateway',
      'API secret',
      'Webhook secret',
      'Price ID',
      'Unit price (local currency / USD)',
      'Promotion codes',
    ]) {
      expect(screen.queryByText(label), label).toBeNull()
    }
  })

  it('keeps the compliance confirmation block', () => {
    renderSection()
    expect(screen.getByText('Compliance confirmed')).toBeTruthy()
  })

  it('shows the compliance warning when not confirmed', () => {
    renderSection({ ...confirmed, confirmed: false })
    expect(screen.getByText('Compliance confirmation required')).toBeTruthy()
  })

  it('saves only changed General fields and never sends Epay or Stripe keys', async () => {
    const user = userEvent.setup()
    renderSection()
    fireEvent.change(screen.getByLabelText('Price (local currency / USD)'), {
      target: { value: '7' },
    })
    await user.click(screen.getByRole('button', { name: 'save' }))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1))
    expect(mutateAsync).toHaveBeenCalledWith({ key: 'Price', value: 7 })
    for (const [arg] of mutateAsync.mock.calls) {
      expect(arg.key).not.toMatch(/Epay|Stripe|PayAddress|CustomCallbackAddress/)
    }
  })
})
