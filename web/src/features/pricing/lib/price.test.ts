import { describe, expect, it } from 'vitest'

import { stripTrailingZeros } from './price'

describe('stripTrailingZeros', () => {
  it('keeps thousands separators in both locales', () => {
    expect(stripTrailingZeros('1.000 $', 'vi-VN')).toBe('1.000 $')
    expect(stripTrailingZeros('1,000 $', 'en-US')).toBe('1,000 $')
  })

  it('removes only insignificant decimal zeroes', () => {
    expect(stripTrailingZeros('1.234,50 $', 'vi-VN')).toBe('1.234,5 $')
    expect(stripTrailingZeros('1,234.50 $', 'en-US')).toBe('1,234.5 $')
    expect(stripTrailingZeros('0,3500 $', 'vi-VN')).toBe('0,35 $')
  })
})
