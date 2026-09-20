import { describe, expect, it } from 'vitest'
import {
  convertDisplayAmountToUSD,
  convertUSDForDisplay,
  effectiveDisplayCurrency,
  formatUSDForDisplay,
  hasUsableUSDToCNYRate,
  normalizeDisplayCurrency,
} from '@/utils/currencyDisplay'

describe('currencyDisplay', () => {
  it('normalizes unknown values to CNY preference and accepts USD', () => {
    expect(normalizeDisplayCurrency(undefined)).toBe('CNY')
    expect(normalizeDisplayCurrency('cny')).toBe('CNY')
    expect(normalizeDisplayCurrency('USD')).toBe('USD')
    expect(normalizeDisplayCurrency('EUR')).toBe('CNY')
  })

  it('treats only positive finite rates as usable', () => {
    expect(hasUsableUSDToCNYRate(7.2)).toBe(true)
    expect(hasUsableUSDToCNYRate(0)).toBe(false)
    expect(hasUsableUSDToCNYRate(-1)).toBe(false)
    expect(hasUsableUSDToCNYRate(Number.NaN)).toBe(false)
    expect(hasUsableUSDToCNYRate(Number.POSITIVE_INFINITY)).toBe(false)
    expect(hasUsableUSDToCNYRate(null)).toBe(false)
  })

  it('falls back to USD display when no usable CNY rate is configured', () => {
    // 没有汇率时不得把美元账本直接标成人民币。
    expect(effectiveDisplayCurrency('CNY', null)).toBe('USD')
    expect(effectiveDisplayCurrency('CNY', 0)).toBe('USD')
    expect(effectiveDisplayCurrency('CNY', 7.2)).toBe('CNY')
    expect(effectiveDisplayCurrency('USD', null)).toBe('USD')
  })

  it('converts ledger amounts both ways only when a usable rate exists', () => {
    expect(convertUSDForDisplay(10, 'CNY', 7)).toBeCloseTo(70, 10)
    expect(convertDisplayAmountToUSD(70, 'CNY', 7)).toBeCloseTo(10, 10)
    // 无汇率:金额原样保留,不换算也不重标。
    expect(convertUSDForDisplay(10, 'CNY', null)).toBe(10)
    expect(convertDisplayAmountToUSD(10, 'CNY', null)).toBe(10)
  })

  it('formats with the ledger currency when CNY is unavailable', () => {
    expect(formatUSDForDisplay(10, 'CNY', 7, 'zh-CN')).toBe('¥70.00')
    expect(formatUSDForDisplay(10, 'CNY', null, 'en-US')).toBe('$10.00')
    expect(formatUSDForDisplay(10, 'USD', 7, 'en-US')).toBe('$10.00')
  })
})
