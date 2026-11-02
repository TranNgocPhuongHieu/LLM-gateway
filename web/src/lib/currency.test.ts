import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n/config'
import {
  formatBillingCurrencyFromUSD,
  formatCurrencyFromUSD,
  formatLocalCurrencyAmount,
  formatQuotaWithCurrency,
  formatVndAmountToCustomDollar,
  formatVndFiat,
  getCurrencyLabel,
} from './currency'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

const originalConfig = useSystemConfigStore.getState().config

describe('Credit display (1.000 VND = 1 credit)', () => {
  beforeEach(async () => {
    useSystemConfigStore.setState({
      config: {
        ...originalConfig,
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CUSTOM',
          customCurrencySymbol: '₫',
          customCurrencyExchangeRate: 26000,
          usdExchangeRate: 26000,
          quotaPerUnit: 500000,
        },
      },
    })
    await i18n.changeLanguage('vi')
  })

  afterEach(async () => {
    useSystemConfigStore.setState({ config: originalConfig })
    await i18n.changeLanguage('vi')
  })

  it('formats VND amounts according to the 1.000 VND = 1 $ convention', () => {
    // Required cases:
    // 700 VND -> 0,7 $ (vi) / 0.7 $ (en)
    expect(formatLocalCurrencyAmount(700, { locale: 'vi-VN' })).toBe('0,7 credit')
    expect(formatLocalCurrencyAmount(700, { locale: 'en-US' })).toBe('0.7 credit')

    // 350 VND -> 0,35 $ (vi) / 0.35 $ (en)
    expect(formatLocalCurrencyAmount(350, { locale: 'vi-VN' })).toBe('0,35 credit')
    expect(formatLocalCurrencyAmount(350, { locale: 'en-US' })).toBe('0.35 credit')

    // 3.500 VND -> 3,5 $ (vi) / 3.5 $ (en)
    expect(formatLocalCurrencyAmount(3500, { locale: 'vi-VN' })).toBe('3,5 credit')
    expect(formatLocalCurrencyAmount(3500, { locale: 'en-US' })).toBe('3.5 credit')

    // 1.234.560 VND -> 1.234,56 $ (vi) / 1,234.56 $ (en)
    expect(formatLocalCurrencyAmount(1234560, { locale: 'vi-VN' })).toBe('1.234,56 credit')
    expect(formatLocalCurrencyAmount(1234560, { locale: 'en-US' })).toBe('1,234.56 credit')

    // Small fees: 0,01 VND -> 0,00001 $, must not round to 0
    expect(formatLocalCurrencyAmount(0.01, { locale: 'vi-VN' })).toBe('0,00001 credit')
    expect(formatLocalCurrencyAmount(0.01, { locale: 'en-US' })).toBe('0.00001 credit')

    // 0 VND -> 0 $
    expect(formatLocalCurrencyAmount(0, { locale: 'vi-VN' })).toBe('0 credit')
    expect(formatLocalCurrencyAmount(0, { locale: 'en-US' })).toBe('0 credit')

    // Negative amounts
    expect(formatLocalCurrencyAmount(-1234560, { locale: 'vi-VN' })).toBe('-1.234,56 credit')
    expect(formatLocalCurrencyAmount(-1234560, { locale: 'en-US' })).toBe('-1,234.56 credit')

    // Without symbol
    expect(formatLocalCurrencyAmount(1234560, { showSymbol: false, locale: 'vi-VN' })).toBe('1.234,56')
    expect(formatLocalCurrencyAmount(1234560, { showSymbol: false, locale: 'en-US' })).toBe('1,234.56')

    // Missing / invalid data
    expect(formatLocalCurrencyAmount(null)).toBe('-')
    expect(formatLocalCurrencyAmount(undefined)).toBe('-')
    expect(formatLocalCurrencyAmount(Number.NaN)).toBe('-')
  })

  it('formats direct VND via formatVndAmountToCustomDollar', () => {
    expect(formatVndAmountToCustomDollar('700', { locale: 'vi-VN' })).toBe('0,7 credit')
    expect(formatVndAmountToCustomDollar('1,234,560', { locale: 'vi-VN' })).toBe('1.234,56 credit')
    expect(formatVndAmountToCustomDollar(0.01, { locale: 'vi-VN' })).toBe('0,00001 credit')
  })

  it('keeps explicit VND formatting in dollars regardless of quota display setting', () => {
    for (const quotaDisplayType of ['USD', 'CNY', 'CUSTOM'] as const) {
      useSystemConfigStore.setState((state) => ({
        config: {
          ...state.config,
          currency: { ...state.config.currency, quotaDisplayType },
        },
      }))
      expect(formatVndAmountToCustomDollar('700', { locale: 'vi-VN' })).toBe('0,7 credit')
    }
  })

  it('formats internal USD amounts through the VND exchange rate then / 1000', () => {
    // 1 USD internal = 26,000 VND = 26 $ custom
    expect(formatCurrencyFromUSD(1, { locale: 'vi-VN', abbreviate: false })).toBe('26 credit')
    expect(formatCurrencyFromUSD(10, { locale: 'vi-VN', abbreviate: false })).toBe('260 credit')

    // 700 VND / 26000 = USD billing amount
    const solInputUSD = 700 / 26000
    expect(formatBillingCurrencyFromUSD(solInputUSD, { locale: 'vi-VN' })).toBe('0,7 credit')
    expect(formatBillingCurrencyFromUSD(solInputUSD, { locale: 'en-US' })).toBe('0.7 credit')

    const solOutputUSD = 3500 / 26000
    expect(formatBillingCurrencyFromUSD(solOutputUSD, { locale: 'vi-VN' })).toBe('3,5 credit')
  })

  it('formats quota tokens correctly without double-conversion', () => {
    // 500,000 quota units = 1 USD internal = 26 $ custom
    expect(formatQuotaWithCurrency(500000, { locale: 'vi-VN', abbreviate: false })).toBe('26 credit')
    // 1,000,000 quota units = 2 USD internal = 52 $ custom
    expect(formatQuotaWithCurrency(1000000, { locale: 'vi-VN', abbreviate: false })).toBe('52 credit')
  })

  it('formats real fiat VND payments distinctly with dong symbol', () => {
    expect(formatVndFiat(260000, { locale: 'vi-VN' })).toBe('260.000 ₫')
    expect(formatVndFiat(260000, { locale: 'en-US' })).toBe('260,000 ₫')
    expect(formatVndFiat(null)).toBe('-')
  })

  it('reflects app language switching without relying on browser locale', async () => {
    await i18n.changeLanguage('vi')
    expect(formatLocalCurrencyAmount(1234560)).toBe('1.234,56 credit')

    await i18n.changeLanguage('en')
    expect(formatLocalCurrencyAmount(1234560)).toBe('1,234.56 credit')
  })

  it('returns credit for custom currency mode', () => {
    expect(getCurrencyLabel()).toBe('credit')
  })

  it('uses credit when quotaDisplayType is USD', () => {
    useSystemConfigStore.setState((state) => ({
      config: {
        ...state.config,
        currency: { ...state.config.currency, quotaDisplayType: 'USD' },
      },
    }))
    expect(formatCurrencyFromUSD(1234.56, { locale: 'en-US', abbreviate: false })).toBe('1,234.56 credit')
  })
})
