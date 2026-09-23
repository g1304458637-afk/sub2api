export type DisplayCurrency = 'CNY' | 'USD'

export function normalizeDisplayCurrency(value: unknown): DisplayCurrency {
  return value === 'USD' ? 'USD' : 'CNY'
}

export function hasUsableUSDToCNYRate(rate: number | null | undefined): rate is number {
  return typeof rate === 'number' && Number.isFinite(rate) && rate > 0
}

export function effectiveDisplayCurrency(
  preferred: DisplayCurrency,
  rate: number | null | undefined,
): DisplayCurrency {
  return preferred === 'CNY' && !hasUsableUSDToCNYRate(rate) ? 'USD' : preferred
}

export function convertUSDForDisplay(
  amountUSD: number,
  currency: DisplayCurrency,
  rate: number | null | undefined,
): number {
  if (!Number.isFinite(amountUSD)) return 0
  return currency === 'CNY' && hasUsableUSDToCNYRate(rate) ? amountUSD * rate : amountUSD
}

export function convertDisplayAmountToUSD(
  amount: number,
  currency: DisplayCurrency,
  rate: number | null | undefined,
): number {
  if (!Number.isFinite(amount)) return 0
  return currency === 'CNY' && hasUsableUSDToCNYRate(rate) ? amount / rate : amount
}

export function convertCNYForDisplay(
  amountCNY: number,
  currency: DisplayCurrency,
  rate: number | null | undefined,
): number {
  if (!Number.isFinite(amountCNY)) return 0
  return currency === 'USD' && hasUsableUSDToCNYRate(rate) ? amountCNY / rate : amountCNY
}

export function formatUSDForDisplay(
  amountUSD: number,
  currency: DisplayCurrency,
  rate: number | null | undefined,
  locale: string,
  fractionDigits = 2,
): string {
  const amount = convertUSDForDisplay(amountUSD, currency, rate)
  const digits = Math.max(0, Math.min(8, Math.trunc(fractionDigits)))
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: currency === 'CNY' && hasUsableUSDToCNYRate(rate) ? 'CNY' : 'USD',
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(amount)
  } catch {
    const symbol = currency === 'CNY' && hasUsableUSDToCNYRate(rate) ? '¥' : '$'
    return `${symbol}${amount.toFixed(digits)}`
  }
}

export function formatCNYForDisplay(
  amountCNY: number,
  currency: DisplayCurrency,
  rate: number | null | undefined,
  locale: string,
  fractionDigits = 2,
): string {
  const canConvertToUSD = currency === 'USD' && hasUsableUSDToCNYRate(rate)
  const amount = convertCNYForDisplay(amountCNY, currency, rate)
  const digits = Math.max(0, Math.min(8, Math.trunc(fractionDigits)))
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: canConvertToUSD ? 'USD' : 'CNY',
      currencyDisplay: 'narrowSymbol',
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    }).format(amount)
  } catch {
    const symbol = canConvertToUSD ? '$' : '¥'
    return symbol + amount.toFixed(digits)
  }
}
