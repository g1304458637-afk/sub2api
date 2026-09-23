<template>
  <AppLayout>
    <div class="muc-scope muc-subs">
      <div class="muc-subs__bg" aria-hidden="true"></div>

      <div class="muc-subs__content">
        <header class="muc-subs__header">
          <h1 class="muc-subs__title">{{ t('mySub.title') }}</h1>
          <p class="muc-subs__subtitle">{{ t('mySub.subtitle') }}</p>
        </header>

        <!-- 钱包摘要：完整流水在 Wallet 页 -->
        <MucGlassCard class="muc-subs__wallet" data-testid="subs-wallet">
          <div class="muc-subs__wallet-row">
            <div class="muc-subs__wallet-info">
              <span class="muc-subs__wallet-label">{{ t('mySub.wallet') }}</span>
              <span class="muc-subs__wallet-value">{{ walletDisplay }}</span>
            </div>
            <MucButton pill @click="goRecharge">{{ t('mySub.recharge') }}</MucButton>
          </div>
        </MucGlassCard>

        <!-- 加载骨架 -->
        <template v-if="loading">
          <MucGlassCard v-for="i in 2" :key="i" class="muc-subs__card">
            <MucSkeleton height="26px" />
            <MucSkeleton height="14px" />
            <MucSkeleton height="10px" />
            <MucSkeleton height="48px" />
          </MucGlassCard>
        </template>

        <MucState v-else-if="loadError" error :message="loadError">
          <template #actions>
            <MucButton variant="secondary" @click="load">{{ t('mySub.retry') }}</MucButton>
          </template>
        </MucState>

        <!-- 无订阅空态：钱包按量仍可用 -->
        <MucGlassCard v-else-if="!activeSubscriptions.length" class="muc-subs__card">
          <MucState :message="t('mySub.emptyBody')">
            <template #actions>
              <MucButton @click="goPricing">{{ t('mySub.viewPricing') }}</MucButton>
            </template>
          </MucState>
        </MucGlassCard>

        <!-- 订阅主卡（单主套餐不变量下至多一张；v-for 仅为兼容过渡） -->
        <MucGlassCard
          v-for="sub in activeSubscriptions"
          :key="sub.id"
          variant="strong"
          class="muc-subs__card"
          data-testid="subs-card"
        >
          <div class="muc-subs__card-head">
            <div class="muc-subs__card-title">
              <h2 class="muc-subs__tier">{{ sub.display_name }}</h2>
              <span v-if="priceLine(sub)" class="muc-subs__price">{{ priceLine(sub) }}</span>
            </div>
            <MucBadge :tone="badgeTone(sub.usage_status)">
              {{ statusLabel(sub.usage_status) }}
            </MucBadge>
          </div>

          <QuotaRemaining :subscription="sub" />

          <dl class="muc-subs__meta">
            <div v-if="sub.quota_policy !== 'dual_window_v1'" class="muc-subs__meta-item">
              <dt>{{ t('mySub.nextReset') }}</dt>
              <dd>{{ formatDate(sub.weekly_period_ends_at) }}</dd>
            </div>
            <div class="muc-subs__meta-item">
              <dt>{{ t('mySub.expires') }}</dt>
              <dd>{{ formatDate(sub.expires_at) }}</dd>
            </div>
          </dl>

          <!-- 已计划的到期切换 -->
          <div v-if="scheduledFor(sub)" class="muc-subs__scheduled">
            <Icon name="clock" size="sm" class="muc-subs__scheduled-icon" />
            <div class="muc-subs__scheduled-text">
              <p>{{ t('mySub.scheduledTo', { plan: scheduledTargetName(sub) }) }}</p>
              <p class="muc-subs__scheduled-at">
                {{ t('mySub.scheduledEffective', { date: scheduledEffective(sub) }) }}
              </p>
            </div>
            <button
              type="button"
              class="muc-subs__scheduled-cancel"
              :disabled="cancelBusy"
              @click="askCancelScheduled(sub)"
            >
              {{ t('mySub.cancelPlan') }}
            </button>
          </div>

          <!-- PAYG fallback：用户文案「额度用完后继续使用」 -->
          <div class="muc-subs__payg">
            <div class="muc-subs__payg-text">
              <span>{{ t('mySub.paygTitle') }}</span>
              <span class="muc-subs__payg-hint">{{ t('mySub.paygHint') }}</span>
            </div>
            <button
              type="button"
              class="muc-subs__switch"
              role="switch"
              :aria-checked="sub.payg_fallback"
              :disabled="paygBusyId === sub.id"
              @click="onPaygToggle(sub)"
            >
              <span class="muc-subs__switch-state">{{ sub.payg_fallback ? 'ON' : 'OFF' }}</span>
              <span
                class="muc-subs__switch-knob"
                :class="{ 'muc-subs__switch-knob--on': sub.payg_fallback }"
              ></span>
            </button>
          </div>

          <!-- 重置卡（账户级数量，作用于本订阅当前周期） -->
          <div class="muc-subs__reset">
            <div class="muc-subs__reset-text">
              <span class="muc-subs__reset-count">
                {{ t('mySub.resetCards', { count: resetCards }) }}
              </span>
              <span class="muc-subs__reset-hint">{{ t('mySub.resetHint') }}</span>
            </div>
            <MucButton
              variant="secondary"
              :disabled="resetCards <= 0"
              :loading="resetBusyId === sub.id"
              @click="askUseResetCard(sub)"
            >
              {{ t('mySub.useResetCard') }}
            </MucButton>
          </div>
        </MucGlassCard>

        <!-- 单主套餐不变量防御：正常业务永远至多一张主卡 -->
        <MucGlassCard
          v-if="!loading && activeSubscriptions.length > 1"
          class="muc-subs__card"
          data-testid="subs-multi-active-warning"
        >
          <MucState :message="t('mySub.multiActiveWarning', { count: activeSubscriptions.length })" />
        </MucGlassCard>
      </div>

      <!-- PAYG 开启首次确认 -->
      <MucConfirmDialog
        :open="!!paygPending"
        :title="t('mySub.paygConfirmTitle')"
        :body="paygConfirmBody"
        :confirm-text="t('mySub.paygConfirmOk')"
        :cancel-text="t('common.cancel')"
        :loading="paygBusyId !== null"
        @confirm="confirmPaygEnable"
        @cancel="paygPending = null"
      />

      <!-- 重置卡使用确认 -->
      <MucConfirmDialog
        :open="!!resetPending"
        :title="t('mySub.resetConfirmTitle')"
        :body="resetConfirmBody"
        :confirm-text="t('mySub.resetConfirmOk')"
        :cancel-text="t('common.cancel')"
        :loading="resetBusyId !== null"
        @confirm="confirmUseResetCard"
        @cancel="resetPending = null"
      />

      <!-- 取消到期切换确认 -->
      <MucConfirmDialog
        :open="cancelPending !== null"
        :title="t('pricing.downgrade.cancelTitle')"
        :body="cancelBody"
        :confirm-text="t('pricing.downgrade.cancelConfirm')"
        :cancel-text="t('common.cancel')"
        :loading="cancelBusy"
        danger
        @confirm="confirmCancelScheduled"
        @cancel="cancelPending = null"
      />

      <!-- Reset 成功动画（API 成功后才展示；AVAILABLE QUOTA 语义） -->
      <MucResetSuccessOverlay
        :open="resetOverlayOpen"
        :used-percent-before="resetUsedBefore"
        :next-end-date="resetNextEnd"
        :remaining-cards="resetRemaining"
        @done="resetOverlayOpen = false"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import QuotaRemaining from '@/components/subscription/QuotaRemaining.vue'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import '@/components/pricing/muc-tokens.css'
import MucGlassCard from '@/components/muc/MucGlassCard.vue'
import MucButton from '@/components/muc/MucButton.vue'
import MucBadge from '@/components/muc/MucBadge.vue'
import type { MucBadgeTone } from '@/components/muc/MucBadge.vue'
import MucSkeleton from '@/components/muc/MucSkeleton.vue'
import MucState from '@/components/muc/MucState.vue'
import MucConfirmDialog from '@/components/muc/MucConfirmDialog.vue'
import MucResetSuccessOverlay from '@/components/subscription/MucResetSuccessOverlay.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  cancelScheduledDowngrade,
  getAccountStatus,
  getSubscriptionChanges,
  resetWithCard,
  updatePaygFallback,
  type AccountStatus,
  type AccountSubscriptionStatus,
  type PlanChangeRecordDto,
  type UsageStatus
} from '@/api/subscriptions'
import { paymentAPI } from '@/api/payment'
import type { SubscriptionPlan } from '@/types/payment'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'
import { planValiditySuffix } from '@/components/payment/validity'
import { useAppStore } from '@/stores'

/**
 * 我的订阅：用户 Subscription 控制中心。
 * 所有数值（百分比/状态/周期/钱包）来自 /subscriptions/status 服务端合同；
 * 价格展示来自 /payment/plans（仅展示，不参与任何计算）。
 */
const router = useRouter()
const { t } = useI18n()
const appStore = useAppStore()
const currencyStore = useCurrencyDisplayStore()

const loading = ref(true)
const loadError = ref('')
const status = ref<AccountStatus | null>(null)
const plans = ref<SubscriptionPlan[]>([])

const scheduledBySubId = ref<Record<number, PlanChangeRecordDto>>({})
const paygPending = ref<AccountSubscriptionStatus | null>(null)
const paygBusyId = ref<number | null>(null)
const resetPending = ref<AccountSubscriptionStatus | null>(null)
const resetBusyId = ref<number | null>(null)
const cancelPending = ref<AccountSubscriptionStatus | null>(null)
const cancelBusy = ref(false)

const resetOverlayOpen = ref(false)
const resetUsedBefore = ref<number | null>(null)
const resetNextEnd = ref('')
const resetRemaining = ref<number | null>(null)

const activeSubscriptions = computed(() => status.value?.subscriptions ?? [])
const resetCards = computed(() => status.value?.reset_cards.available ?? 0)

const walletDisplay = computed(() => {
  const w = status.value?.wallet
  if (!w) return '—'
  const value = parseFloat(w.balance)
  return w.canonical_currency?.toUpperCase() === 'USD'
    ? currencyStore.formatUSD(Number.isFinite(value) ? value : 0)
    : currencyStore.formatCNY(Number.isFinite(value) ? value : 0)
})

const paygConfirmBody = computed(() => t('mySub.paygConfirmBody', { wallet: walletDisplay.value }))

const resetConfirmBody = computed(() => resetPending.value?.quota_policy === 'dual_window_v1'
  ? '消耗一张重置卡，同时恢复 5 小时和周额度并重新计时。未用额度不叠加，不延长订阅；尚未结算的请求将消耗恢复后的额度。'
  : t('mySub.resetConfirmBody', { count: resetCards.value }))

const cancelBody = computed(() => {
  const sub = cancelPending.value
  if (!sub) return ''
  const rec = scheduledFor(sub)
  return t('pricing.downgrade.cancelBody', {
    plan: sub.display_name,
    date: rec?.EffectiveAt ? formatDate(rec.EffectiveAt) : '—'
  })
})

function planForSub(sub: AccountSubscriptionStatus): SubscriptionPlan | undefined {
  return plans.value.find((p) => p.group_id === sub.group_id)
}

function priceLine(sub: AccountSubscriptionStatus): string {
  const plan = planForSub(sub)
  if (!plan) return ''
  const price = plan.currency?.toUpperCase() === 'USD'
    ? currencyStore.formatUSD(plan.price)
    : currencyStore.formatCNY(plan.price)
  return `${price} / ${planValiditySuffix(plan, t)}`
}

function scheduledFor(sub: AccountSubscriptionStatus): PlanChangeRecordDto | null {
  return scheduledBySubId.value[sub.id] ?? null
}

function scheduledTargetName(sub: AccountSubscriptionStatus): string {
  const rec = scheduledFor(sub)
  if (!rec) return ''
  const target = plans.value.find((p) => p.id === rec.ToPlanID)
  return target?.name ?? `#${rec.ToPlanID}`
}

function scheduledEffective(sub: AccountSubscriptionStatus): string {
  const rec = scheduledFor(sub)
  return rec?.EffectiveAt ? formatDate(rec.EffectiveAt) : '—'
}

function statusLabel(statusValue: UsageStatus): string {
  return t(`mySub.usageStatus.${statusValue}`)
}

function badgeTone(statusValue: UsageStatus): MucBadgeTone {
  switch (statusValue) {
    case 'exhausted':
      return 'exhausted'
    case 'near_limit':
      return 'near_limit'
    case 'high':
      return 'high'
    case 'unmetered':
      return 'unmetered'
    default:
      return 'normal'
  }
}



function formatDate(iso: string | null): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  } catch {
    return iso
  }
}

function cryptoRandomKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `mucsub-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function extractErrorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const message = String((err as { message?: unknown }).message ?? '').trim()
    if (message && message !== 'Unknown error') return message
  }
  return fallback
}

// ── 数据加载 ──
async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [statusRes, plansRes] = await Promise.all([
      getAccountStatus(),
      paymentAPI.getPlans().catch(() => null)
    ])
    status.value = statusRes
    plans.value = plansRes?.data ?? []
    await refreshScheduled()
  } catch (err) {
    loadError.value = extractErrorMessage(err, t('mySub.loadError'))
  } finally {
    loading.value = false
  }
}

async function refreshScheduled() {
  const entries = await Promise.allSettled(
    activeSubscriptions.value.map(async (sub) => {
      const records = await getSubscriptionChanges(sub.id)
      const pending =
        records.find((r) => r.ChangeType === 'scheduled_downgrade' && r.Status === 'scheduled') ??
        null
      return [sub.id, pending] as const
    })
  )
  const next: Record<number, PlanChangeRecordDto> = {}
  for (const entry of entries) {
    if (entry.status === 'fulfilled' && entry.value[1]) next[entry.value[0]] = entry.value[1]
  }
  scheduledBySubId.value = next
}

onMounted(load)

// ── PAYG fallback ──
function onPaygToggle(sub: AccountSubscriptionStatus) {
  if (sub.payg_fallback) {
    void disablePayg(sub)
  } else {
    paygPending.value = sub
  }
}

async function disablePayg(sub: AccountSubscriptionStatus) {
  paygBusyId.value = sub.id
  try {
    await updatePaygFallback(sub.id, false)
    sub.payg_fallback = false
  } catch (err) {
    appStore.showError(extractErrorMessage(err, t('mySub.paygError')))
  } finally {
    paygBusyId.value = null
  }
}

async function confirmPaygEnable() {
  const sub = paygPending.value
  if (!sub) return
  paygBusyId.value = sub.id
  try {
    await updatePaygFallback(sub.id, true)
    sub.payg_fallback = true
    paygPending.value = null
  } catch (err) {
    appStore.showError(extractErrorMessage(err, t('mySub.paygError')))
  } finally {
    paygBusyId.value = null
  }
}

// ── 重置卡 ──
function askUseResetCard(sub: AccountSubscriptionStatus) {
  resetPending.value = sub
}

async function confirmUseResetCard() {
  const sub = resetPending.value
  if (!sub) return
  resetBusyId.value = sub.id
  try {
    // 先请求后端，成功后才播放成功动画（禁止先播动画再等后端）
    const result = await resetWithCard(sub.id, cryptoRandomKey())
    resetUsedBefore.value = sub.weekly_usage_percent
    resetNextEnd.value = result.weekly_period_ends_at
    resetPending.value = null
    // 重新拉取账户状态（0% + 剩余卡数），随后进入动画收尾
    await load()
    resetRemaining.value = resetCards.value
    if (sub.quota_policy === 'dual_window_v1') { appStore.showSuccess('额度已重置，当前剩余以刷新结果为准') } else { resetOverlayOpen.value = true }
  } catch (err) {
    appStore.showError(extractErrorMessage(err, t('mySub.resetError')))
  } finally {
    resetBusyId.value = null
  }
}

// ── 到期切换取消 ──
function askCancelScheduled(sub: AccountSubscriptionStatus) {
  cancelPending.value = sub
}

async function confirmCancelScheduled() {
  const sub = cancelPending.value
  if (!sub) return
  cancelBusy.value = true
  try {
    await cancelScheduledDowngrade(sub.id)
    cancelPending.value = null
    const next = { ...scheduledBySubId.value }
    delete next[sub.id]
    scheduledBySubId.value = next
    appStore.showSuccess(t('pricing.downgrade.cancelledToast'))
  } catch (err) {
    appStore.showError(extractErrorMessage(err, t('pricing.errors.cancelFailed')))
  } finally {
    cancelBusy.value = false
  }
}

function goRecharge() {
  void router.push('/purchase')
}

function goPricing() {
  void router.push('/pricing')
}
</script>

<style scoped>
.muc-subs {
  position: relative;
  overflow: hidden;
  min-height: calc(100vh - 96px);
  border-radius: 24px;
  background: var(--muc-bg-primary);
  color: var(--muc-text-primary);
}

/* 深色渐变背景：比 Pricing 克制，仅顶部极弱红晕 */
.muc-subs__bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(70% 40% at 50% 0%, rgba(var(--muc-red-rgb, 200, 36, 51), 0.12), transparent 70%),
    linear-gradient(180deg, var(--muc-bg-primary), var(--muc-bg-secondary));
  pointer-events: none;
}

.muc-subs__content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-width: 880px;
  margin: 0 auto;
  padding: clamp(28px, 5vh, 56px) clamp(16px, 4vw, 40px) 44px;
}

.muc-subs__header {
  text-align: center;
}

.muc-subs__title {
  font-size: clamp(22px, 3vw, 30px);
  font-weight: 700;
}

.muc-subs__subtitle {
  margin-top: 8px;
  font-size: 13.5px;
  color: var(--muc-text-secondary);
}

.muc-subs__wallet {
  padding: 4px;
}

.muc-subs__wallet-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 18px;
}

.muc-subs__wallet-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.muc-subs__wallet-label {
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--muc-text-muted);
}

.muc-subs__wallet-value {
  font-size: 24px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.muc-subs__card {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 24px;
}

.muc-subs__card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.muc-subs__card-title {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}

.muc-subs__tier {
  font-size: 24px;
  font-weight: 800;
  letter-spacing: 0.03em;
  text-transform: uppercase;
}

.muc-subs__price {
  font-size: 13px;
  color: var(--muc-text-secondary);
  white-space: nowrap;
}

.muc-subs__usage {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.muc-subs__usage-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}

.muc-subs__usage-label {
  font-size: 12.5px;
  color: var(--muc-text-secondary);
}

.muc-subs__usage-percent {
  font-size: 15px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.muc-subs__meta {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin: 0;
}

.muc-subs__meta-item {
  padding: 10px 14px;
  border-radius: 12px;
  border: 1px solid var(--muc-glass-border);
  background: rgba(255, 255, 255, 0.02);
}

.muc-subs__meta-item dt {
  font-size: 11px;
  letter-spacing: 0.06em;
  color: var(--muc-text-muted);
}

.muc-subs__meta-item dd {
  margin: 4px 0 0;
  font-size: 14px;
  font-weight: 600;
}

.muc-subs__scheduled {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid var(--muc-red-border);
  background: var(--muc-red-soft);
  font-size: 12.5px;
}

.muc-subs__scheduled-icon {
  flex-shrink: 0;
  margin-top: 1px;
  color: var(--muc-red-bright);
}

.muc-subs__scheduled-text {
  flex: 1;
  min-width: 0;
}

.muc-subs__scheduled-text p {
  margin: 0;
}

.muc-subs__scheduled-at {
  margin-top: 2px;
  font-size: 11px;
  color: var(--muc-text-muted);
}

.muc-subs__scheduled-cancel {
  flex-shrink: 0;
  border: none;
  background: none;
  padding: 2px 4px;
  font-size: 12px;
  color: var(--muc-red-bright);
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.muc-subs__scheduled-cancel:disabled {
  opacity: 0.5;
  cursor: wait;
}

.muc-subs__payg {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 14px;
  border: 1px solid var(--muc-glass-border);
  background: rgba(255, 255, 255, 0.02);
}

.muc-subs__payg-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
  font-size: 13.5px;
  font-weight: 600;
}

.muc-subs__payg-hint {
  font-size: 11.5px;
  font-weight: 400;
  line-height: 1.5;
  color: var(--muc-text-muted);
}

.muc-subs__switch {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
  border: 1px solid var(--muc-glass-border-strong);
  background: rgba(255, 255, 255, 0.04);
  border-radius: 999px;
  padding: 6px 12px;
  cursor: pointer;
  transition: border-color 0.2s ease;
}

.muc-subs__switch:hover:not(:disabled) {
  border-color: var(--muc-red-border-hover);
}

.muc-subs__switch:disabled {
  opacity: 0.6;
  cursor: wait;
}

.muc-subs__switch-state {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
  color: var(--muc-text-secondary);
}

.muc-subs__switch-knob {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--muc-text-muted);
  transition: background 0.2s ease, box-shadow 0.2s ease;
}

.muc-subs__switch-knob--on {
  background: var(--muc-red-bright);
  box-shadow: 0 0 12px var(--muc-red-glow);
}

.muc-subs__reset {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 14px;
  border: 1px solid rgba(214, 180, 106, 0.35);
  background: rgba(214, 180, 106, 0.05);
}

.muc-subs__reset-text {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.muc-subs__reset-count {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--muc-gold);
}

.muc-subs__reset-hint {
  font-size: 11.5px;
  line-height: 1.5;
  color: var(--muc-text-muted);
}

@media (max-width: 640px) {
  .muc-subs__meta {
    grid-template-columns: 1fr;
  }

  .muc-subs__payg,
  .muc-subs__reset {
    flex-direction: column;
    align-items: stretch;
  }

  .muc-subs__payg .muc-subs__switch,
  .muc-subs__reset .muc-btn {
    align-self: flex-start;
  }
}
</style>
