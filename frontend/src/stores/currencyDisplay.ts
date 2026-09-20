import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { i18n } from '@/i18n'
import { useAppStore } from '@/stores/app'
import {
  effectiveDisplayCurrency,
  formatUSDForDisplay,
  hasUsableUSDToCNYRate,
  normalizeDisplayCurrency,
  type DisplayCurrency,
} from '@/utils/currencyDisplay'

const STORAGE_KEY = 'muc_display_currency'

function readPreferredCurrency(): DisplayCurrency {
  if (typeof window === 'undefined') return 'CNY'
  return normalizeDisplayCurrency(window.localStorage.getItem(STORAGE_KEY))
}

export const useCurrencyDisplayStore = defineStore('currencyDisplay', () => {
  const appStore = useAppStore()
  const preferredCurrency = ref<DisplayCurrency>(readPreferredCurrency())
  const usdToCnyRate = computed(() => {
    const value = appStore.cachedPublicSettings?.usd_to_cny_display_rate
    return hasUsableUSDToCNYRate(value) ? value : 0
  })
  const displayCurrency = computed(() => effectiveDisplayCurrency(preferredCurrency.value, usdToCnyRate.value))
  const isCNYUnavailable = computed(() => preferredCurrency.value === 'CNY' && displayCurrency.value !== 'CNY')
  const locale = computed(() => (i18n.global.locale.value === 'zh' ? 'zh-CN' : 'en-US'))

  function setCurrency(currency: DisplayCurrency): boolean {
    if (currency === 'CNY' && !hasUsableUSDToCNYRate(usdToCnyRate.value)) return false
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

  return {
    preferredCurrency,
    displayCurrency,
    usdToCnyRate,
    isCNYUnavailable,
    canDisplayCNY: computed(() => hasUsableUSDToCNYRate(usdToCnyRate.value)),
    setCurrency,
    toggleCurrency,
    formatUSD,
  }
})
