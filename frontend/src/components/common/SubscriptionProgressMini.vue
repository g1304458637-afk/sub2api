<template>
  <div v-if="subscriptionFeatureEnabled && hasActiveSubscriptions" class="relative" ref="containerRef">
    <!-- Mini Progress Display -->
    <button
      @click="toggleTooltip"
      class="flex cursor-pointer items-center gap-2 rounded-xl bg-purple-50 px-3 py-1.5 transition-colors hover:bg-purple-100 dark:bg-purple-900/20 dark:hover:bg-purple-900/30"
      :title="t('subscriptionProgress.viewDetails')"
    >
      <Icon name="creditCard" size="sm" class="text-purple-600 dark:text-purple-400" />
      <div class="flex items-center gap-1.5">
        <!-- Combined progress indicator -->
        <div class="flex items-center gap-0.5">
          <div
            v-for="(sub, index) in displaySubscriptions.slice(0, 3)"
            :key="index"
            class="h-2 w-2 rounded-full"
            :class="getProgressDotClass(sub)"
          ></div>
        </div>
        <span class="text-xs font-medium text-purple-700 dark:text-purple-300">
          {{ subscriptions.length }}
        </span>
      </div>
    </button>

    <!-- Hover/Click Tooltip -->
    <transition name="dropdown">
      <div
        v-if="tooltipOpen"
        class="absolute right-0 z-50 mt-2 w-[340px] overflow-hidden rounded-xl border border-gray-200 bg-white shadow-xl dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="border-b border-gray-100 p-3 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('subscriptionProgress.title') }}
          </h3>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
            {{ t('subscriptionProgress.activeCount', { count: subscriptions.length }) }}
          </p>
        </div>

        <div class="max-h-64 overflow-y-auto">
          <div
            v-for="subscription in displaySubscriptions"
            :key="subscription.id"
            class="border-b border-gray-50 p-3 last:border-b-0 dark:border-dark-700/50"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {{ subscription.display_name }}
              </span>
              <span
                v-if="subscription.expires_at"
                class="text-xs"
                :class="getDaysRemainingClass(subscription.expires_at)"
              >
                {{ formatDaysRemaining(subscription.expires_at) }}
              </span>
            </div>

            <!-- Weekly usage: server-computed integer percent; unmetered shows a badge -->
            <div class="space-y-1.5">
              <div
                v-if="subscription.usage_status === 'unmetered'"
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-primary-50 px-2.5 py-1.5 dark:from-emerald-900/20 dark:to-primary-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('subscriptionProgress.usageStatus.unmetered') }}
                </span>
              </div>

              <template v-else>
                <div class="flex items-center gap-2">
                  <span class="w-8 flex-shrink-0 text-[10px] text-gray-500">{{
                    t('subscriptionProgress.weekly')
                  }}</span>
                  <div class="h-1.5 min-w-0 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressBarClass(subscription)"
                      :style="{ width: getProgressWidth(subscription) }"
                    ></div>
                  </div>
                  <span class="w-24 flex-shrink-0 text-right text-[10px] text-gray-500">
                    {{ getUsageStatusLabel(subscription) }}
                  </span>
                </div>
                <div class="flex items-center justify-between text-[10px] text-gray-400">
                  <span>{{ t('subscriptionProgress.weeklyUsage') }}</span>
                  <span>{{ subscription.weekly_usage_percent }}%</span>
                </div>
              </template>
            </div>
          </div>
        </div>

        <div class="border-t border-gray-100 p-2 dark:border-dark-700">
          <router-link
            to="/subscriptions"
            @click="closeTooltip"
            class="block w-full py-1 text-center text-xs text-primary-600 hover:underline dark:text-primary-400"
          >
            {{ t('subscriptionProgress.viewAll') }}
          </router-link>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { getAccountStatus, type AccountSubscriptionStatus } from '@/api/subscriptions'

const { t } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const tooltipOpen = ref(false)
const subscriptions = ref<AccountSubscriptionStatus[]>([])

const hasActiveSubscriptions = computed(() => subscriptions.value.length > 0)
// 订阅功能关闭后，即使用户仍持有后台分配的订阅，顶栏也不再露出订阅进度与「查看全部订阅」入口。
const subscriptionFeatureEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.subscription))

const displaySubscriptions = computed(() => {
  // 按使用百分比降序（unmetered 视为 0，排最后）
  return [...subscriptions.value].sort((a, b) => {
    const aPct = a.weekly_usage_percent ?? -1
    const bPct = b.weekly_usage_percent ?? -1
    return bPct - aPct
  })
})

async function loadStatus() {
  try {
    const status = await getAccountStatus()
    subscriptions.value = status.subscriptions
  } catch (error) {
    console.error('Failed to load account status in SubscriptionProgressMini:', error)
  }
}

function getUsageStatusLabel(sub: AccountSubscriptionStatus): string {
  const key = String(sub.usage_status)
  const path = `subscriptionProgress.usageStatus.${key}`
  const translated = t(path)
  // 未配置的键会原样返回路径；此时回退为整数百分比
  return translated === path ? `${sub.weekly_usage_percent}%` : translated
}

function getProgressDotClass(sub: AccountSubscriptionStatus): string {
  // unmetered subscriptions get a special color
  if (sub.usage_status === 'unmetered') {
    return 'bg-emerald-500'
  }
  const pct = sub.weekly_usage_percent ?? 0
  if (pct >= 90 || sub.usage_status === 'exhausted') return 'bg-red-500'
  if (pct >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function getProgressBarClass(sub: AccountSubscriptionStatus): string {
  if (sub.usage_status === 'unmetered') return 'bg-gray-400'
  const pct = sub.weekly_usage_percent ?? 0
  if (pct >= 90) return 'bg-red-500'
  if (pct >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function getProgressWidth(sub: AccountSubscriptionStatus): string {
  const pct = sub.weekly_usage_percent ?? 0
  return `${Math.min(pct, 100)}%`
}

function formatDaysRemaining(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return t('subscriptionProgress.expired')
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days === 0) return t('subscriptionProgress.expiresToday')
  if (days === 1) return t('subscriptionProgress.expiresTomorrow')
  return t('subscriptionProgress.daysRemaining', { days })
}

function getDaysRemainingClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-500 dark:text-dark-400'
}

function toggleTooltip() {
  tooltipOpen.value = !tooltipOpen.value
}

function closeTooltip() {
  tooltipOpen.value = false
}

function handleClickOutside(event: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    closeTooltip()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  // Trigger initial fetch if not already loaded
  // The actual data loading is handled by App.vue globally
  if (!subscriptionFeatureEnabled.value) return
  void loadStatus()
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
