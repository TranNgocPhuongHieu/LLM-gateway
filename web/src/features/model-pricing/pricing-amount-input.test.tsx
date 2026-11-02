// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { useState } from 'react'
import i18n from '@/i18n/config'
import { PricingAmountInput } from './pricing-amount-input'

function ControlledPricingInput() {
  const [value, setValue] = useState('0.7')
  return (
    <PricingAmountInput
      aria-label='Model price'
      value={value}
      onChange={setValue}
      currency={{ label: '$', symbol: '$', exchangeRate: 1 }}
    />
  )
}

let originalLanguage = 'en'

beforeEach(() => {
  originalLanguage = i18n.language
})

afterEach(async () => {
  cleanup()
  await i18n.changeLanguage(originalLanguage)
})

describe('PricingAmountInput locale decimal input', () => {
  it('shows and accepts decimal commas in Vietnamese', async () => {
    await i18n.changeLanguage('vi')
    const user = userEvent.setup()
    render(<ControlledPricingInput />)

    const input = screen.getByRole('textbox', { name: 'Model price' })
    expect((input as HTMLInputElement).value).toBe('0,7')

    await user.clear(input)
    await user.type(input, '0,35')

    expect((input as HTMLInputElement).value).toBe('0,35')
  })
})
