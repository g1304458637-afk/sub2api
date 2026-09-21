<template>
  <!-- 「我的」卡片的操作区：语言切换、货币切换（站点开启时）、退出登录 -->
  <div class="my-1 border-t border-gray-100 dark:border-dark-800" aria-hidden="true"></div>

  <!-- 语言 -->
  <div
    class="flex items-center gap-3 rounded-xl py-1.5 text-sm text-gray-600 dark:text-dark-300"
    style="padding-left: 1.0625rem; padding-right: 0.875rem"
  >
    <svg
      class="h-5 w-5 shrink-0 text-gray-400 dark:text-dark-400"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="1.5"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418"
      />
    </svg>
    <span class="min-w-0 flex-1 truncate">{{ t('common.language') }}</span>
    <div class="flex shrink-0 overflow-hidden rounded-lg ring-1 ring-gray-200 dark:ring-dark-600">
      <button
        v-for="item in locales"
        :key="item.code"
        type="button"
        class="px-2.5 py-1 text-xs font-medium transition-colors"
        :class="
          locale === item.code
            ? 'bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-400'
            : 'text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-700'
        "
        :disabled="switching"
        @click="switchTo(item.code)"
      >
        {{ item.short }}
      </button>
    </div>
  </div>

  <!-- 人民币/美元显示切换（仅站点开启人民币显示时出现） -->
  <button
    v-if="currencyStore.canDisplayCNY"
    type="button"
    class="sidebar-link mb-0.5 w-full text-sm"
    :title="t('common.currencyToggle')"
    @click="currencyStore.toggleCurrency()"
  >
    <span
      class="flex h-5 w-5 shrink-0 items-center justify-center text-sm font-semibold text-primary-600 dark:text-primary-400"
    >{{ currencyStore.displayCurrency === 'CNY' ? '¥' : '$' }}</span>
    <span class="sidebar-label">{{ t('common.currencyToggle') }}</span>
  </button>

  <!-- 退出登录 -->
  <button
    type="button"
    class="sidebar-link w-full text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
    @click="handleLogout"
  >
    <svg
      class="h-5 w-5 shrink-0"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      stroke-width="1.5"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M12 9l-3 3m0 0l3 3m-3-3h12.75"
      />
    </svg>
    <span class="sidebar-label">{{ t('nav.logout') }}</span>
  </button>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { setLocale } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'

const { t, locale } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const currencyStore = useCurrencyDisplayStore()

const switching = ref(false)

const locales = [
  { code: 'zh', short: '中文' },
  { code: 'en', short: 'EN' },
] as const

async function switchTo(code: string): Promise<void> {
  if (code === locale.value) return
  switching.value = true
  try {
    await setLocale(code)
  } finally {
    switching.value = false
  }
}

async function handleLogout(): Promise<void> {
  try {
    await authStore.logout()
  } catch (error) {
    console.error('Logout error:', error)
  }
  await router.push('/login')
}
</script>
