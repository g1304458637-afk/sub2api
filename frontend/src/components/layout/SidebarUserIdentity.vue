<template>
  <!-- 用户身份：头像 + 用户名 + 可用余额（顶栏移除后，余额与账户信息统一收敛在侧边栏） -->
  <span
    class="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 text-xs font-medium text-white shadow-sm"
  >
    <img
      v-if="avatarUrl"
      :src="avatarUrl"
      :alt="displayName"
      class="h-full w-full object-cover"
    >
    <span v-else>{{ userInitials }}</span>
  </span>
  <!-- 布局全部用自带 Tailwind 类：scoped 的 .sidebar-label-flex 对多根子组件内部不生效 -->
  <span
    class="flex min-w-0 flex-1 items-center justify-between gap-2 overflow-hidden transition-all duration-200"
    :class="collapsed ? 'max-w-0 -translate-x-1 opacity-0' : 'max-w-[12rem]'"
  >
    <span class="min-w-0 leading-tight">
      <span class="block truncate text-sm font-medium">{{ displayName }}</span>
      <span
        class="block truncate text-xs font-normal text-primary-600 dark:text-primary-400"
        :title="frozenBalance > 0 ? `${t('common.frozenBalance')} ${formatMoney(frozenBalance)}` : undefined"
      >
        {{ formatMoney(availableBalance) }}<span
          v-if="frozenBalance > 0"
          class="text-amber-600 dark:text-amber-400"
        >+{{ formatMoney(frozenBalance) }}</span>
      </span>
    </span>
    <slot name="chevron" />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'

withDefaults(defineProps<{ collapsed?: boolean }>(), { collapsed: false })

const { t } = useI18n()
const authStore = useAuthStore()
const currencyStore = useCurrencyDisplayStore()

const user = computed(() => authStore.user)
const avatarUrl = computed(() => user.value?.avatar_url?.trim() || '')
const availableBalance = computed(() => Number(user.value?.balance || 0))
const frozenBalance = computed(() => Number(user.value?.frozen_balance || 0))

const displayName = computed(() => {
  if (!user.value) return ''
  return user.value.username || user.value.email?.split('@')[0] || ''
})

const userInitials = computed(() => {
  if (!user.value) return ''
  if (user.value.username) {
    return user.value.username.substring(0, 2).toUpperCase()
  }
  if (user.value.email) {
    const localPart = user.value.email.split('@')[0]
    return localPart.substring(0, 2).toUpperCase()
  }
  return ''
})

function formatMoney(value: number): string {
  return currencyStore.formatCNY(value)
}
</script>
