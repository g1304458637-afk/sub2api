import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { i18n } from '@/i18n'
import {
  effectiveDisplayCurrency,
  formatCNYForDisplay,
  formatUSDForDisplay,
  hasUsableUSDToCNYRate,
  normalizeDisplayCurrency,
  type DisplayCurrency,
} from '@/utils/currencyDisplay'

const STORAGE_KEY = 'sub2api_display_currency'
const WALLET_USD_TO_CNY_RATE = 6.7

function readPreferredCurrency(): DisplayCurrency {
  if (typeof window === 'undefined') return 'CNY'
  return normalizeDisplayCurrency(window.localStorage.getItem(STORAGE_KEY))
}

export const useCurrencyDisplayStore = defineStore('currencyDisplay', () => {
  const preferredCurrency = ref<DisplayCurrency>(readPreferredCurrency())
  const usdToCnyRate = computed(() => WALLET_USD_TO_CNY_RATE)
  const displayCurrency = computed(() => effectiveDisplayCurrency(preferredCurrency.value, usdToCnyRate.value))
  const isCNYUnavailable = computed(() => preferredCurrency.value === 'CNY' && displayCurrency.value !== 'CNY')
  const locale = computed(() => (i18n.global.locale.value === 'zh' ? 'zh-CN' : 'en-US'))

  function setCurrency(currency: DisplayCurrency): boolean {
    preferredCurrency.value = currency
    if (typeof window !== 'undefined') window.localStorage.setItem(STORAGE_KEY, currency)
    return true
  }

  function toggleCurrency(): void {
    if (displayCurrency.value === 'CNY') setCurrency('USD')
    else setCurrency('CNY')
  }

  function formatUSD(amount: number | null | undefined, fractionDigits = 2): string {
    return formatUSDForDisplay(Number(amount), displayCurrency.value, usdToCnyRate.value, locale.value, fractionDigits)
  }

  function formatCNY(amount: number | null | undefined, fractionDigits = 2): string {
    return formatCNYForDisplay(Number(amount), displayCurrency.value, usdToCnyRate.value, locale.value, fractionDigits)
  }

  function cnyToUSD(amount: number): number {
    const rate = hasUsableUSDToCNYRate(usdToCnyRate.value) ? usdToCnyRate.value : 6.7
    return Number.isFinite(amount) ? amount / rate : 0
  }

  return {
    preferredCurrency,
    displayCurrency,
    usdToCnyRate,
    isCNYUnavailable,
    canDisplayCNY: computed(() => true),
    setCurrency,
    toggleCurrency,
    formatUSD,
    formatCNY,
    cnyToUSD,
  }
})
