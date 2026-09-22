<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="user" class="fixed inset-0 z-[80] flex justify-end bg-black/40" role="dialog" aria-modal="true" :aria-label="t('user360.title')" @click.self="emit('close')" @keydown.esc="emit('close')">
        <div class="drawer-panel h-full w-full max-w-[440px] overflow-y-auto bg-white p-5 text-gray-900 shadow-2xl dark:bg-dark-900 dark:text-gray-100" tabindex="-1">
          <!-- 头部：用户身份 -->
          <header class="mb-4 flex items-start justify-between gap-3">
            <div class="min-w-0">
              <h3 class="truncate text-base font-semibold" data-testid="u360-name">{{ user.email }}</h3>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
                ID {{ user.id }} · {{ t('user360.registeredAt') }} {{ fmtDate(user.created_at) }}
                <span v-if="user.status" class="ml-1">· {{ user.status }}</span>
              </p>
            </div>
            <button class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 dark:hover:bg-dark-700" :aria-label="t('common.close')" @click="emit('close')">
              <Icon name="x" size="md" />
            </button>
          </header>

          <p v-if="loading" role="status">{{ t('common.loading') }}</p>
          <p v-if="loadError" role="alert">{{ t('user360.actionFailed') }}</p>
          <!-- 四摘要 -->
          <div class="grid grid-cols-2 gap-3">
            <div class="sum-card">
              <span class="sum-label">{{ t('user360.wallet') }}</span>
              <span class="sum-value">${{ userBalance }}</span>
            </div>
            <div class="sum-card">
              <span class="sum-label">{{ t('user360.resetCards') }}</span>
              <span class="sum-value">{{ resetCardsAvailable }}</span>
            </div>
            <div class="sum-card">
              <span class="sum-label">{{ t('user360.subscription') }}</span>
              <span class="sum-value sum-value--sm">{{ subscriptionLine }}</span>
            </div>
            <div class="sum-card">
              <span class="sum-label">{{ t('user360.apiKeys') }}</span>
              <span class="sum-value">{{ apiKeys.length }}</span>
            </div>
          </div>

          <!-- 快捷操作 -->
          <div class="mt-4 flex flex-wrap gap-2">
            <button class="act-btn" :disabled="loading || resetting || !primarySubId" @click="askDirectReset">
              {{ resetting ? t('common.processing') : t('user360.directReset') }}
            </button>
            <button class="act-btn" :disabled="loading || granting" @click="askGrantCard">
              {{ granting ? t('common.processing') : t('user360.grantCard') }}
            </button>
            <button class="act-btn" @click="showPlanChanges = !showPlanChanges">
              {{ t('user360.viewPlanChanges') }}
            </button>
          </div>
          <p v-if="actionMessage" class="mt-2 text-xs" :class="actionOk ? 'text-emerald-600' : 'text-red-500'">
            {{ actionMessage }}
          </p>

          <!-- 订阅详情（内部 USD 默认折叠） -->
          <section v-if="subscriptions.length" class="mt-5">
            <h4 class="sec-title">{{ t('user360.subDetails') }}</h4>
            <div v-for="sub in subscriptions" :key="sub.id" class="rounded-xl border border-gray-100 p-3 text-sm dark:border-dark-700">
              <div class="flex items-center justify-between">
                <span class="font-medium">{{ sub.group?.name || `#${sub.group_id}` }}</span>
                <span class="text-xs text-gray-500">{{ sub.status }}</span>
              </div>
              <div class="mt-1 flex items-center justify-between text-xs text-gray-500 dark:text-dark-400">
                <span>{{ t('user360.expires') }} {{ fmtDate(sub.expires_at) }}</span>
                <button class="underline decoration-dotted" @click="sub._expanded = !sub._expanded">
                  {{ sub._expanded ? t('user360.collapse') : t('user360.internalDetail') }}
                </button>
              </div>
              <div v-if="sub._expanded" class="mt-2 space-y-0.5 rounded-lg bg-gray-50 p-2 font-mono text-[11px] text-gray-600 dark:bg-dark-800 dark:text-dark-300">
                <p>group_id: {{ sub.group_id }}</p>
                <p>weekly_usage_usd: {{ sub.weekly_usage_usd ?? '—' }} / {{ sub.group?.weekly_limit_usd ?? '—' }}</p>
                <p>starts: {{ fmtDate(sub.starts_at) }} → expires: {{ fmtDate(sub.expires_at) }}</p>
              </div>
            </div>
          </section>

          <!-- 套餐变更（来自订单 plan_change + 预约降级不在此：admin 无 changes 端点，BLOCKED #3） -->
          <section v-if="showPlanChanges" class="mt-5">
            <h4 class="sec-title">{{ t('user360.planChanges') }}</h4>
            <p v-if="planChangeOrders.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('user360.noChanges') }}</p>
            <ul v-else class="space-y-1.5 text-sm">
              <li v-for="o in planChangeOrders" :key="o.id" class="flex items-center justify-between rounded-xl border border-gray-100 px-3 py-2 dark:border-dark-700">
                <span>{{ o.label }} · {{ o.tier }} · {{ o.amount }}</span>
                <span class="text-xs text-gray-500">{{ o.status }} · {{ fmtDate(o.created_at) }}</span>
              </li>
            </ul>
          </section>

          <!-- 钱包流水（充值码历史） -->
          <section class="mt-5">
            <h4 class="sec-title">{{ t('user360.walletHistory') }}</h4>
            <p v-if="balanceHistory.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('user360.noHistory') }}</p>
            <ul v-else class="space-y-1.5 text-sm">
              <li v-for="(row, i) in balanceHistory.slice(0, 10)" :key="i" class="flex items-center justify-between rounded-xl border border-gray-100 px-3 py-2 dark:border-dark-700">
                <span class="truncate">{{ t('wallet.ledgerType.' + row.type) }} · ${{ row.amount.toFixed(2) }}</span>
                <span class="text-xs text-gray-500">{{ fmtDate(row.created_at) }}</span>
              </li>
            </ul>
          </section>

          <!-- Reset 卡记录 -->
          <section class="mt-5">
            <h4 class="sec-title">{{ t('user360.cardHistory') }}</h4>
            <p v-if="resetCardRows.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('user360.noCards') }}</p>
            <ul v-else class="space-y-1.5 text-sm">
              <li v-for="card in resetCardRows" :key="card.id" class="flex items-center justify-between rounded-xl border border-gray-100 px-3 py-2 dark:border-dark-700">
                <span>#{{ card.id }}</span>
                <span
                  class="rounded-full border px-2 py-0.5 text-[11px]"
                  :class="card.status === 'available' ? 'border-emerald-200 text-emerald-600' : 'border-gray-200 text-gray-400 dark:border-dark-600 dark:text-dark-400'"
                >
                  {{ card.status }}
                </span>
              </li>
            </ul>
          </section>

          <!-- API Keys（masked） -->
          <section class="mt-5">
            <h4 class="sec-title">{{ t('user360.keysTitle') }}</h4>
            <p v-if="apiKeys.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('user360.noKeys') }}</p>
            <ul v-else class="space-y-1.5 text-sm">
              <li v-for="key in apiKeys.slice(0, 8)" :key="key.id" class="flex items-center justify-between rounded-xl border border-gray-100 px-3 py-2 dark:border-dark-700">
                <span class="truncate font-mono text-xs">{{ maskApiKey(key.key || String(key.id)) }}</span>
                <span class="text-xs text-gray-500">{{ key.group?.name || '—' }}</span>
              </li>
            </ul>
            <p class="mt-1 text-[11px] text-gray-400">{{ t('user360.keysMaskedNote') }}</p>
          </section>

          <!-- 直接重置确认 -->
          <div v-if="resetConfirming" class="fixed inset-0 z-[90] flex items-center justify-center bg-black/50 p-4" role="dialog" aria-modal="true">
            <div class="w-full max-w-sm rounded-2xl bg-white p-5 dark:bg-dark-800">
              <h4 class="text-sm font-semibold">{{ t('user360.resetConfirmTitle') }}</h4>
              <p class="mt-2 text-xs leading-relaxed text-gray-600 dark:text-gray-300">{{ t('user360.resetConfirmBody', { id: primarySubId }) }}</p>
              <div class="mt-4 flex justify-end gap-2">
                <button class="rounded-lg px-3 py-1.5 text-sm text-gray-500" @click="resetConfirming = false">{{ t('common.cancel') }}</button>
                <button class="rounded-lg bg-gray-900 px-3 py-1.5 text-sm font-medium text-white dark:bg-white/10 dark:text-white" @click="confirmDirectReset">
                  {{ t('user360.resetConfirmOk') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { maskApiKey } from '@/utils/maskApiKey'
import type { UserSubscription, ApiKey } from '@/types'
import { getAdminWalletLedger, type ResetCardRow } from '@/api/admin/subscriptionReset'
import type { WalletLedgerEntry } from '@/api/subscriptions'
import { createMutationAttempt } from '@/utils/mutationAttempt'
import { formatPaymentAmount } from '@/components/payment/currency'

/**
 * Admin 用户详情 Customer 360 抽屉（挂在现有 UsersView，不新开页面）。
 * 四摘要：Wallet / Subscription / Reset Cards / API Keys（仅 masked）。
 * 快捷操作：直接重置（Direct Reset 事件，作用于该用户主订阅）/ 发放重置卡 /
 * 查看套餐变更（admin 侧以 plan_change 订单呈现；订阅 changes 审计无 admin 端点 — BLOCKED #3）。
 * 内部 USD 字段默认折叠（管理员可见，但不默认摊开）。
 */
export interface User360Target {
  id: number
  email: string
  status?: string
  balance?: number
  created_at?: string
}

const props = defineProps<{ user: User360Target | null }>()
const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()

interface SubRow extends UserSubscription { _expanded?: boolean }
const subscriptions = ref<SubRow[]>([])
const apiKeys = ref<ApiKey[]>([])
const resetCardRows = ref<ResetCardRow[]>([])
const balanceHistory = ref<WalletLedgerEntry[]>([])
const planChangeOrders = ref<Array<{ id: number; label: string; tier: string; amount: string; status: string; created_at: string }>>([])
const resetCardsAvailable = ref(0)
const resetting = ref(false)
const granting = ref(false)
const resetConfirming = ref(false)
const actionMessage = ref('')
const actionOk = ref(false)
const showPlanChanges = ref(false)
const loading = ref(false)
const loadError = ref(false)
let detailRequest = 0
const resetAttempt = createMutationAttempt('u360-reset')
const grantAttempt = createMutationAttempt('u360-grant')

const userBalance = computed(() => Number(props.user?.balance ?? 0).toFixed(2))

const primarySubId = computed(() => subscriptions.value[0]?.id ?? null)

const subscriptionLine = computed(() => {
  const sub = subscriptions.value[0]
  if (!sub) return t('user360.none')
  const pct = sub.group?.weekly_limit_usd
    ? Math.round(((sub.weekly_usage_usd || 0) / sub.group.weekly_limit_usd) * 100)
    : null
  return `${sub.group?.name ?? ''}${pct !== null && Number.isFinite(pct) ? ` · ${pct}%` : ''}`
})

watch(
  () => props.user?.id,
  async (id) => {
    const request = ++detailRequest
    subscriptions.value = []; apiKeys.value = []; resetCardRows.value = []
    balanceHistory.value = []; planChangeOrders.value = []; resetCardsAvailable.value = 0
    loading.value = !!id; loadError.value = false
    resetConfirming.value = false; resetting.value = false; granting.value = false
    resetAttempt.clear(); grantAttempt.clear()
    if (!id) return
    actionMessage.value = ''
    showPlanChanges.value = false
    // Promise.all + catch(null)：保留元组类型，单项失败不影响其他摘要
    const [subsRes, keysRes, historyRes, cardsRes, ordersRes, countRes] = await Promise.all([
      adminAPI.subscriptions.list(1, 50, { user_id: id, status: 'active' }).catch(() => null),
      adminAPI.users.getUserApiKeys(id).catch(() => null),
      getAdminWalletLedger(id, 1, 10).catch(() => null),
      adminAPI.resetCards.list({ user_id: id, page: 1, page_size: 20 }).catch(() => null),
      adminAPI.planChanges.list({ user_id: id, page: 1, page_size: 20 }).catch(() => null),
      adminAPI.resetCards.count(id).catch(() => null)
    ])
    if (request !== detailRequest) return
    loading.value = false
    loadError.value = [subsRes, keysRes, historyRes, cardsRes, ordersRes, countRes].some(result => result === null)
    subscriptions.value = subsRes?.items ?? []
    apiKeys.value = keysRes?.items ?? []
    balanceHistory.value = historyRes?.data.entries ?? []
    resetCardRows.value = cardsRes?.data.items ?? []
    const ordersRaw = (ordersRes as unknown as {
      data?: { items?: Array<{ ID: number; ChangeType: string; FromTier: number; ToTier: number; AmountDue: number; Currency: string; Status: string; CreatedAt: string }> }
    } | null)?.data
    planChangeOrders.value = (ordersRaw?.items ?? []).map((r) => ({
      id: r.ID,
      label: r.ChangeType === 'upgrade'
        ? t('payment.orders.planChangeUpgrade')
        : t('payment.orders.planChangeDowngrade'),
      tier: `${r.FromTier}→${r.ToTier}`,
      amount: r.ChangeType === 'upgrade' ? formatPaymentAmount(r.AmountDue, r.Currency) : '—',
      status: r.Status,
      created_at: r.CreatedAt
    })).slice(0, 10)
    const countRaw = (countRes as unknown as { data?: { available?: number } } | null)?.data
    resetCardsAvailable.value = typeof countRaw === 'number' ? countRaw : (countRaw?.available ?? 0)
  },
  { immediate: true }
)

function askDirectReset() {
  actionMessage.value = ''
  resetConfirming.value = true
}

async function confirmDirectReset() {
  if (!primarySubId.value || resetting.value) return
  const request = detailRequest
  const subId = primarySubId.value
  resetting.value = true
  try {
    await adminAPI.resetEvents.create({
      target_mode: 'subscription_ids',
      subscription_ids: [subId],
      reason: 'admin user-360 direct reset',
      idempotency_key: resetAttempt.keyFor({ subscription_id: subId })
    })
    if (request !== detailRequest) return
    resetAttempt.clear()
    actionOk.value = true
    actionMessage.value = t('user360.resetQueued')
    resetConfirming.value = false
  } catch {
    if (request !== detailRequest) return
    actionOk.value = false
    actionMessage.value = t('user360.actionFailed')
  } finally {
    if (request === detailRequest) resetting.value = false
  }
}

async function askGrantCard() {
  if (!props.user || granting.value) return
  const request = detailRequest
  const userId = props.user.id
  granting.value = true
  actionMessage.value = ''
  try {
    await adminAPI.resetCards.grant({
      target_mode: 'users',
      user_ids: [userId],
      quantity_per_user: 1,
      reason: 'admin user-360 grant',
      idempotency_key: grantAttempt.keyFor({ user_id: userId })
    })
    if (request !== detailRequest) return
    grantAttempt.clear()
    actionOk.value = true
    actionMessage.value = t('user360.cardGranted')
    resetCardsAvailable.value += 1
  } catch {
    if (request !== detailRequest) return
    actionOk.value = false
    actionMessage.value = t('user360.actionFailed')
  } finally {
    if (request === detailRequest) granting.value = false
  }
}

function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleDateString()
  } catch {
    return iso
  }
}
</script>

<style scoped>
.sum-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-radius: 0.75rem;
  border: 1px solid rgb(229 231 235);
  padding: 0.7rem 0.85rem;
}

.dark .sum-card {
  border-color: rgb(55 65 81);
}

.sum-label {
  font-size: 11px;
  color: rgb(107 114 128);
}

.sum-value {
  font-size: 20px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.sum-value--sm {
  font-size: 14px;
}

.sec-title {
  margin-bottom: 0.5rem;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: rgb(107 114 128);
}

.act-btn {
  border-radius: 0.5rem;
  border: 1px solid rgb(209 213 219);
  padding: 0.3rem 0.7rem;
  font-size: 12px;
  font-weight: 500;
}

.dark .act-btn {
  border-color: rgb(75 85 99);
}

.act-btn:hover:not(:disabled) {
  border-color: rgba(var(--muc-red-bright-rgb, 238, 56, 72), 0.68);
}

.act-btn:disabled {
  opacity: 0.5;
}

.drawer-panel {
  animation: drawer-in 0.28s cubic-bezier(0.22, 1, 0.36, 1);
}

@keyframes drawer-in {
  from {
    transform: translateX(40px);
    opacity: 0;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .drawer-panel {
    animation: none;
  }
}
</style>
