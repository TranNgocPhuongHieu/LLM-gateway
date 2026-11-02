import { beforeAll, describe, expect, it } from 'vitest'
import i18n from '@/i18n/config'
import {
  actualTariffRows,
  formatLogCost,
  formatXPikiVND,
  tokenCostExplanation,
  xpikiTokenCostSchema,
} from './token-cost'
import type { XPikiQuotaModelRow } from './api'

describe('token capital cost', () => {
  beforeAll(async () => {
    await i18n.changeLanguage('vi')
  })
  it('converts VND display to credit without premature rounding or altering accounting', () => {
    for (const [amount, display] of [
      ['0', '0 credit'],
      ['0.000', '0 credit'],
      ['0.01', '0,00001 credit'],
      ['700', '0,7 credit'],
      ['350', '0,35 credit'],
      ['3500', '3,5 credit'],
      ['1234560', '1.234,56 credit'],
    ])
      expect(formatLogCost(amount)).toBe(display)
    expect(formatLogCost()).toBe('—')
    expect(formatLogCost('-1')).toBe('—')
    expect(formatLogCost('NaN')).toBe('—')
  })
  it('normalizes comma-grouped provider VND values and converts to credit', () => {
    expect(formatXPikiVND('1,040 ₫')).toBe('1,04 credit')
    expect(formatXPikiVND('12,345.6', false)).toBe('12,3456')
    expect(formatXPikiVND('1,23')).toBe('—')
    expect(formatXPikiVND('1234,567')).toBe('—')
  })
  const model: XPikiQuotaModelRow = {
    model: 'Opus 5.5',
    model_slug: 'claude-opus-5-5',
    price_vnd_per_1m: '260',
    output_price_vnd_per_1m: '1300',
    tokens_per_quota: '',
    tokens_for_26000: '',
    cache_read_vnd_per_1m: '130',
    cache_write_vnd_per_1m: '',
    verified_samples: 0,
    unverified_samples: 0,
  }
  it('shows separate actual charges for multiple levels of the same model', () => {
    const rows = actualTariffRows([
      {
        ...model,
        observed_tariffs: [
          {
            multiplier: '4',
            input_vnd_per_1m: '1040',
            output_vnd_per_1m: '5200',
            cache_read_vnd_per_1m: '520',
            samples: 50,
            cache_samples: 40,
            latest_at: '2026-10-10T01:00:00+07:00',
          },
          {
            multiplier: '10',
            input_vnd_per_1m: '2600',
            output_vnd_per_1m: '13000',
            samples: 8,
            cache_samples: 0,
            latest_at: '2026-10-10T00:00:00+07:00',
          },
        ],
      },
    ])
    expect(rows.map((row) => row.effective_input_vnd_per_1m)).toEqual([
      '1040',
      '2600',
    ])
    expect(rows.map((row) => row.observed_multiplier)).toEqual(['4', '10'])
    expect(rows[1].effective_cache_read_vnd_per_1m).toBeUndefined()
  })
  it('never replaces an unknown actual tariff with a basic price or a stale single rate', () => {
    const rows = actualTariffRows([
      {
        ...model,
        effective_input_vnd_per_1m: '1040',
        observed_multiplier: '4',
      },
    ])
    expect(rows[0].effective_input_vnd_per_1m).toBe('')
    expect(rows[0].observed_multiplier).toBe('')
    expect(rows[0].effective_cache_read_vnd_per_1m).toBe('')
  })
  it('shows effective component prices without applying the multiplier twice', () => {
    const cost = xpikiTokenCostSchema.parse({
      status: 'reconciled',
      multiplier: '3',
      input_tokens: 1000,
      output_tokens: 20,
      cache_read_tokens: 10000,
      cache_write_tokens: 0,
      input_vnd_per_1m: '624',
      output_vnd_per_1m: '2340',
      cache_read_vnd_per_1m: '312',
      input_vnd: '0.624',
      output_vnd: '0.0468',
      cache_read_vnd: '3.12',
      rounding_vnd: '-0.0028',
      total_vnd: '3.788',
      cost_vnd: '3.788 ₫',
    })
    const text = tokenCostExplanation(cost)
    expect(text).toContain('Input không cache')
    expect(text).toContain('0,624 credit /1M')
    expect(text).toContain('2,34 credit /1M')
    expect(text).toContain('0,312 credit /1M')
    expect(text).toContain('không nhân lại')
    expect(text).not.toContain('quota')
    expect(text).not.toContain('đ')
    expect(text).not.toContain('₫')
  })
  it('keeps explicit zero distinct from unknown', () => {
    const zero = xpikiTokenCostSchema.parse({
      status: 'confirmed_zero',
      input_tokens: 500,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 0,
      total_vnd: '0',
      cost_vnd: '0 $',
    })
    expect(tokenCostExplanation(zero)).toBe('XPiki xác nhận không thu phí')
    expect(tokenCostExplanation()).toContain('Chưa đối soát')
  })
  it('explains unknown cache-write instead of presenting a zero cost', () => {
    const unknown = xpikiTokenCostSchema.parse({
      status: 'unverified',
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_write_tokens: 900,
      warning: 'Cache-write chưa xác minh',
    })
    expect(unknown.cost_vnd).toBeUndefined()
    expect(tokenCostExplanation(unknown)).toBe('Cache-write chưa xác minh')
  })
})
