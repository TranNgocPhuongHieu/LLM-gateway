import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n/config'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'
import { formatCurrency, formatFiatPayment } from './format'

const originalConfig = useSystemConfigStore.getState().config

describe('wallet payment amount formatting', () => {
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
        },
      },
    })
    await i18n.changeLanguage('vi')
  })

  afterEach(async () => {
    useSystemConfigStore.setState({ config: originalConfig })
    await i18n.changeLanguage('vi')
  })

  it('formats wallet credit amount in $ (/ 1000)', () => {
    // 700 VND -> 0,7
    expect(formatCurrency(700)).toBe('0,7')
    // 1234560 VND -> 1.234,56
    expect(formatCurrency(1234560)).toBe('1.234,56')
  })

  it('formats fiat VND payments with dong symbol to distinguish from credit', () => {
    expect(formatFiatPayment(260000)).toBe('260.000 ₫')
  })
})
