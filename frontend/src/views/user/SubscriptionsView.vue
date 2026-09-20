<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Account-level reset cards（只读展示；消费入口待 Reset Card Runtime） -->
      <div
        v-if="!loading && subscriptions.length > 0 && resetCardsAvailable > 0"
        class="card flex items-center justify-between border border-amber-200 bg-amber-50 p-4 dark:border-amber-900/40 dark:bg-amber-900/10"
      >
        <div class="flex items-center gap-2 text-sm text-amber-800 dark:text-amber-200">
          <Icon name="clock" size="sm" />
          <span>{{ t('userSubscriptions.resetCards', { count: resetCardsAvailable }) }}</span>
        </div>
      </div>

      <!-- Subscriptions Grid -->
      <div v-if="!loading && subscriptions.length > 0" class="grid gap-6 lg:grid-cols-2">
        <div
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="overflow-hidden rounded-2xl border bg-white dark:bg-dark-800"
          :class="platformBorderClass(subscription.group?.platform || '')"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div :class="['h-1.5 w-1.5 shrink-0 rounded-full', platformAccentDotClass(subscription.group?.platform || '')]" />
              <div>
                <div class="flex items-center gap-2">
                  <h3 class="font-semibold text-gray-900 dark:text-white">
                    {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                  </h3>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                    {{ platformLabel(subscription.group?.platform || '') }}
                  </span>
                </div>
                <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group.description }}
                </p>

              </div>
            </div>
            <div class="flex items-center gap-2">
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  subscription.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                    : subscription.status === 'expired'
                      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                      : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                ]"
              >
                {{ t(`userSubscriptions.status.${subscription.status}`) }}
              </span>
              <button
                v-if="subscription.status === 'active'"
                :class="['rounded-lg px-3 py-1.5 text-xs font-semibold text-white transition-colors', platformButtonClass(subscription.group?.platform || '')]"
                @click="router.push({ path: '/purchase', query: { tab: 'subscription', group: String(subscription.group_id) } })"
              >
                {{ t('payment.renewNow') }}
              </button>
            </div>
          </div>

          <!-- Usage Progress -->
          <div class="space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <!-- Weekly Usage（Phase 4.1 合同：仅整数百分比，无内部 USD 值） -->
            <template v-if="statusById[subscription.id]">
              <div
                v-if="statusById[subscription.id].usage_status !== 'unmetered'"
                class="space-y-2"
              >
                <div class="flex items-center justify-between">
                  <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('userSubscriptions.weeklyUsage') }}
                  </span>
                  <span class="text-sm text-gray-500 dark:text-dark-400">
                    {{ t(`userSubscriptions.usageStatus.${statusById[subscription.id].usage_status}`) }}
                  </span>
                </div>
                <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                    :class="getProgressBarClass(statusById[subscription.id])"
                    :style="{ width: getProgressWidth(statusById[subscription.id]) }"
                  ></div>
                </div>
                <p
                  v-if="statusById[subscription.id].weekly_period_ends_at"
                  class="text-xs text-gray-500 dark:text-dark-400"
                >
                  {{
                    t('userSubscriptions.resetIn', {
                      time: formatResetCountdown(statusById[subscription.id].weekly_period_ends_at)
                    })
                  }}
                </p>
              </div>
              <div
                v-else
                class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-primary-50 py-6 dark:from-emerald-900/20 dark:to-primary-900/20"
              >
                <div class="flex items-center gap-3">
                  <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                  <div>
                    <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                      {{ t('userSubscriptions.unmetered') }}
                    </p>
                    <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                      {{ t('userSubscriptions.unlimitedDesc') }}
                    </p>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI, { type AccountSubscriptionStatus } from '@/api/subscriptions'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTimeToMinute } from '@/utils/format'
import { platformBorderClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import {
  getExpirationDateRelation,
  getRemainingDurationParts,
  type RemainingDurationParts
} from '@/utils/subscriptionQuota'

function platformAccentDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
// Phase 4.1 状态合同：百分比/状态/周期由服务端计算，此处仅按订阅 id 消费
const statusById = ref<Record<number, AccountSubscriptionStatus>>({})
const resetCardsAvailable = ref(0)
const loading = ref(true)

async function loadSubscriptions() {
  try {
    loading.value = true
    // 并行拉取：订阅元数据（active 列表）+ 净化状态（百分比/周期/重置卡）
    const [subs, , status] = await Promise.all([
      subscriptionsAPI.getMySubscriptions(),
      subscriptionsAPI.getActiveSubscriptions(),
      subscriptionsAPI.getAccountStatus().catch(() => null)
    ])
    subscriptions.value = subs
    if (status) {
      const map: Record<number, AccountSubscriptionStatus> = {}
      for (const st of status.subscriptions) map[st.id] = st
      statusById.value = map
      resetCardsAvailable.value = status.reset_cards.available
    }
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function getProgressWidth(st: AccountSubscriptionStatus): string {
  const pct = st.weekly_usage_percent ?? 0
  return `${Math.min(pct, 100)}%`
}

function getProgressBarClass(st: AccountSubscriptionStatus): string {
  const pct = st.weekly_usage_percent ?? 0
  if (pct >= 90) return 'bg-red-500'
  if (pct >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatResetCountdown(endsAt: string | null): string {
  if (!endsAt) return t('userSubscriptions.windowNotActive')
  const parts = getRemainingDurationParts(endsAt)
  return parts ? formatDurationParts(parts) : t('userSubscriptions.windowNotActive')
}

function formatExpirationDate(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))
  const relation = getExpirationDateRelation(expires, now)

  if (relation === null) return ''

  if (relation === 'expired') {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateTimeToMinute(expires)

  if (relation === 'today') {
    return `${dateStr} (${t('common.today')})`
  }
  if (relation === 'tomorrow') {
    return `${dateStr} (${t('common.tomorrow')})`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (diff <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatDurationParts(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${parts.minutes}m`
}

onMounted(() => {
  loadSubscriptions()
})
</script>
