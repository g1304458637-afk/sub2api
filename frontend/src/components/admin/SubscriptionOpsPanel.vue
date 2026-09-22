<template>
  <section class="space-y-3" data-testid="sub-ops-panel">
    <div class="flex items-center justify-between">
      <h3 class="flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
        <span class="h-3.5 w-[3px] rounded bg-[#c82433]"></span>
        {{ t('adminOps.title') }}
      </h3>
      <button
        v-if="!loading"
        class="text-xs text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200"
        @click="load"
      >
        <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
      </button>
    </div>

    <div v-if="loading" class="grid grid-cols-2 gap-3 md:grid-cols-5">
      <div v-for="i in 5" :key="i" class="h-20 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800"></div>
    </div>

    <template v-else>
      <!-- 核心数字：点击跳对应过滤结果 -->
      <div class="grid grid-cols-2 gap-3 md:grid-cols-5">
        <button class="stat-card" @click="goSubscriptions()">
          <span class="stat-label">{{ t('adminOps.activeSubs') }}</span>
          <span class="stat-value">{{ stats.activeSubs }}</span>
        </button>
        <button class="stat-card" @click="goSubscriptions()">
          <span class="stat-label">{{ t('adminOps.highNearLimit') }}</span>
          <span class="stat-value stat-value--warn">{{ stats.highNear }}</span>
        </button>
        <button class="stat-card" @click="goSubscriptions()">
          <span class="stat-label">{{ t('adminOps.exhausted') }}</span>
          <span class="stat-value stat-value--danger">{{ stats.exhausted }}</span>
        </button>
        <button class="stat-card" @click="goOrders('COMPLETED')">
          <span class="stat-label">{{ t('adminOps.paidToday') }}</span>
          <span class="stat-value">{{ stats.paidToday }}</span>
        </button>
        <button class="stat-card" @click="goOrders('PAID')">
          <span class="stat-label">{{ t('adminOps.fulfillmentIssues') }}</span>
          <span :class="['stat-value', stats.fulfillmentIssues > 0 ? 'stat-value--danger' : '']">
            {{ stats.fulfillmentIssues }}
          </span>
        </button>
      </div>

      <!-- 需要处理 -->
      <div class="card p-4">
        <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">
          {{ t('adminOps.needsAttention') }}
        </h4>
        <p v-if="attentionItems.length === 0" class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('adminOps.allClear') }}
        </p>
        <ul v-else class="space-y-2">
          <li
            v-for="(item, i) in attentionItems"
            :key="i"
            class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-gray-100 px-3 py-2 text-sm dark:border-dark-700"
          >
            <span class="text-gray-800 dark:text-gray-200">{{ item.text }}</span>
            <button
              v-if="item.action"
              class="rounded-lg border border-gray-200 px-2 py-0.5 text-xs text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-700"
              @click="item.action()"
            >
              {{ item.actionLabel }}
            </button>
          </li>
        </ul>
      </div>

      <!-- 订阅健康：档位 × 状态分布 -->
      <div class="card p-4">
        <h4 class="mb-2 text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">
          {{ t('adminOps.healthTitle') }}
        </h4>
        <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
          <div v-for="tier in tierHealth" :key="tier.tier" class="rounded-xl border border-gray-100 p-3 dark:border-dark-700">
            <div class="mb-1 flex items-center justify-between">
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ tier.tier }}</span>
              <span class="text-xs text-gray-500 dark:text-dark-400">{{ tier.total }}</span>
            </div>
            <div class="flex h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
              <div
                v-for="seg in tier.segments"
                :key="seg.key"
                :style="{ width: `${seg.pct}%`, background: seg.color }"
                :title="`${seg.label}: ${seg.count}`"
              ></div>
            </div>
            <div class="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-gray-500 dark:text-dark-400">
              <span v-for="seg in tier.segments" :key="seg.key">{{ seg.label }} {{ seg.count }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 最近 Reset 操作 -->
      <div class="card p-4">
        <div class="mb-2 flex items-center justify-between">
          <h4 class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">
            {{ t('adminOps.recentResets') }}
          </h4>
          <button class="text-xs text-gray-500 hover:underline dark:text-dark-400" @click="goResetCenter">
            {{ t('adminOps.goResetCenter') }}
          </button>
        </div>
        <p v-if="recentEvents.length === 0" class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('adminOps.noResets') }}
        </p>
        <ul v-else class="space-y-1.5">
          <li
            v-for="ev in recentEvents"
            :key="ev.id"
            class="flex flex-wrap items-center justify-between gap-2 text-sm"
          >
            <span class="text-gray-800 dark:text-gray-200">
              #{{ ev.id }} · {{ t('adminOps.targetMode.' + ev.target_mode, ev.target_mode) }}
              <span v-if="ev.reason" class="text-xs text-gray-400">{{ ev.reason }}</span>
            </span>
            <span class="text-xs text-gray-500 dark:text-dark-400">
              {{ ev.applied_count }}/{{ ev.total_targeted }}
              <span v-if="ev.failed_count > 0" class="text-red-500">· {{ t('adminOps.failedCount', { count: ev.failed_count }) }}</span>
            </span>
          </li>
        </ul>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { ResetEventSummary } from '@/api/admin/subscriptionReset'
import type { PaymentOrder } from '@/types/payment'
import type { UserSubscription } from '@/types'

/**
 * 订阅运营面板（Admin Dashboard 增强）。
 * 数据全部来自现有后端列表端点客户端聚合：
 * - 订阅列表 /admin/subscriptions（百分比基于后端返回的 usage/limit 字段；
 *   管理员可见内部数据，用户端才走 usage_status 合同）
 * - 订单 /admin/payment/orders?status=PAID → 支付成功未完成履约
 * - Reset 事件 /admin/subscription-resets
 */
const { t } = useI18n()
const router = useRouter()

const loading = ref(true)
const subs = ref<UserSubscription[]>([])
const paidOrders = ref<PaymentOrder[]>([])
const recentEvents = ref<ResetEventSummary[]>([])

const stats = computed(() => {
  let highNear = 0
  let exhausted = 0
  for (const s of subs.value) {
    const pct = subPercent(s)
    if (pct === null) continue
    if (pct >= 100) exhausted++
    else if (pct >= 70) highNear++
  }
  return {
    activeSubs: subs.value.length,
    highNear,
    exhausted,
    paidToday: paidOrders.value.filter((o) => isToday(o.paid_at || o.completed_at || o.created_at)).length,
    fulfillmentIssues: paidOrders.value.length
  }
})

interface Segment { key: string; label: string; count: number; pct: number; color: string }
const tierHealth = computed(() => {
  const groups = new Map<string, UserSubscription[]>()
  for (const s of subs.value) {
    const key = s.group?.name || t('adminOps.otherTier')
    const list = groups.get(key) ?? []
    list.push(s)
    groups.set(key, list)
  }
  return [...groups.entries()].slice(0, 6).map(([tier, list]) => {
    const buckets: Record<string, number> = { normal: 0, high: 0, near_limit: 0, exhausted: 0, unmetered: 0 }
    for (const s of list) {
      const pct = subPercent(s)
      const key = pct === null ? 'unmetered' : pct >= 100 ? 'exhausted' : pct >= 90 ? 'near_limit' : pct >= 70 ? 'high' : 'normal'
      buckets[key]++
    }
    const colors: Record<string, string> = {
      normal: 'rgba(255,255,255,0.45)', high: '#ecc94b', near_limit: '#f08c3a',
      exhausted: '#ff4550', unmetered: '#d6b46a'
    }
    const labels: Record<string, string> = {
      normal: t('adminOps.b.normal'), high: t('adminOps.b.high'), near_limit: t('adminOps.b.near_limit'),
      exhausted: t('adminOps.b.exhausted'), unmetered: t('adminOps.b.unmetered')
    }
    const segments: Segment[] = Object.entries(buckets)
      .filter(([, count]) => count > 0)
      .map(([key, count]) => ({
        key, count,
        pct: Math.round((count / Math.max(list.length, 1)) * 100),
        label: labels[key], color: colors[key]
      }))
    return { tier, total: list.length, segments }
  })
})

interface AttentionItem { text: string; actionLabel?: string; action?: () => void }
const attentionItems = computed<AttentionItem[]>(() => {
  const items: AttentionItem[] = []
  if (stats.value.fulfillmentIssues > 0) {
    items.push({
      text: t('adminOps.attentionFulfillment', { count: stats.value.fulfillmentIssues }),
      actionLabel: t('adminOps.view'),
      action: () => goOrders('PAID')
    })
  }
  const failedEvents = recentEvents.value.filter((e) => e.failed_count > 0)
  if (failedEvents.length > 0) {
    items.push({
      text: t('adminOps.attentionResets', { count: failedEvents.length }),
      actionLabel: t('adminOps.view'),
      action: () => goResetCenter()
    })
  }
  return items
})

function subPercent(s: UserSubscription): number | null {
  const limit = s.group?.weekly_limit_usd
  if (!limit) return null
  return Math.min(((s.weekly_usage_usd || 0) / limit) * 100, 100)
}

function isToday(iso?: string): boolean {
  if (!iso) return false
  const d = new Date(iso)
  const now = new Date()
  return d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate()
}

async function load() {
  loading.value = true
  try {
    const [subsRes, paidRes, eventsRes] = await Promise.allSettled([
      adminAPI.subscriptions.list(1, 500, { status: 'active' }),
      adminAPI.payment.getOrders({ page: 1, page_size: 100, status: 'PAID' }),
      adminAPI.resetEvents.list({ page: 1, page_size: 5 })
    ])
    if (subsRes.status === 'fulfilled') {
      subs.value = (subsRes.value as unknown as { items?: UserSubscription[] }).items ?? []
    }
    if (paidRes.status === 'fulfilled') paidOrders.value = paidRes.value.data.items ?? []
    if (eventsRes.status === 'fulfilled') {
      const payload = eventsRes.value.data as { items?: ResetEventSummary[] } | ResetEventSummary[]
      recentEvents.value = Array.isArray(payload) ? payload : (payload.items ?? [])
    }
  } finally {
    loading.value = false
  }
}

function goSubscriptions() { void router.push('/admin/subscriptions') }
function goOrders(status: string) { void router.push({ path: '/admin/payment', query: { status } }) }
function goResetCenter() { void router.push('/admin/reset-center') }

onMounted(load)
</script>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-radius: 0.75rem;
  border: 1px solid rgb(229 231 235);
  padding: 0.85rem 1rem;
  text-align: left;
  transition: border-color 0.2s ease;
}

:root.dark .stat-card,
.dark .stat-card {
  border-color: rgb(55 65 81);
}

.stat-card:hover {
  border-color: rgba(var(--muc-red-bright-rgb, 238, 56, 72), 0.68);
}

.stat-label {
  font-size: 11px;
  letter-spacing: 0.04em;
  color: rgb(107 114 128);
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  line-height: 1.1;
  color: rgb(17 24 39);
  font-variant-numeric: tabular-nums;
}

.dark .stat-value {
  color: #fff;
}

.stat-value--warn { color: #b45309; }
.dark .stat-value--warn { color: #ecc94b; }
.stat-value--danger { color: #e11d48; }
.dark .stat-value--danger { color: #ff4550; }
</style>
