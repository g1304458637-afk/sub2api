<template>
  <AppLayout>
    <div class="muc-scope muc-pricing">
      <PricingBackground />

    <div class="muc-pricing__content">
      <!-- 品牌水印：大字民大红渐变 + 小字 MUCODE 暖白 -->
      <div class="muc-pricing__watermark" aria-hidden="true">
        <span class="muc-pricing__watermark-caption">MUCODE</span>
        <span class="muc-pricing__watermark-word">PRICING</span>
      </div>

      <header class="muc-pricing__header">
        <h1 class="muc-pricing__title">{{ t('pricing.title') }}</h1>
        <p class="muc-pricing__subtitle">{{ t('pricing.subtitle') }}</p>
      </header>

      <!-- 账户状态条：钱包 + 生效中订阅（服务端合同：整数百分比 + usage_status） -->
      <section class="muc-status" data-testid="pricing-status-strip">
        <div class="muc-status__wallet">
          <span class="muc-status__label">{{ t('pricing.statusStrip.wallet') }}</span>
          <span v-if="wallet" class="muc-status__wallet-value">
            {{ walletDisplay }}
          </span>
          <span v-else class="muc-status__muted">—</span>
        </div>

        <div class="muc-status__divider" aria-hidden="true"></div>

        <div v-if="!account" class="muc-status__muted">{{ t('pricing.statusStrip.loading') }}</div>
        <div v-else-if="!activeSubscriptions.length" class="muc-status__muted">
          {{ t('pricing.statusStrip.noSubs') }}
        </div>
        <div v-else class="muc-status__subs">
          <div v-for="sub in activeSubscriptions" :key="sub.id" class="muc-status__sub">
            <div class="muc-status__sub-head">
              <span class="muc-status__sub-name">{{ sub.display_name }}</span>
              <span class="muc-status__chip" :class="statusChipClass(sub.usage_status)">
                {{ statusLabel(sub.usage_status) }}
              </span>
            </div>
            <div class="muc-status__sub-bar-row">
              <div class="muc-status__sub-bar">
                <div
                  class="muc-status__sub-bar-fill"
                  :class="statusBarClass(sub.usage_status)"
                  :style="{ width: `${Math.min(Math.max(sub.weekly_usage_percent ?? 0, 0), 100)}%` }"
                ></div>
              </div>
              <span class="muc-status__sub-percent">
                {{
                  sub.weekly_usage_percent === null
                    ? t('pricing.usageStatus.unmetered')
                    : `${sub.weekly_usage_percent}%`
                }}
              </span>
            </div>
            <div class="muc-status__sub-meta">
              <span v-if="sub.expires_at">
                {{ t('pricing.statusStrip.expires') }} {{ formatDate(sub.expires_at) }}
              </span>
              <span v-if="resetCards > 0">
                {{ t('pricing.statusStrip.resetCards', { count: resetCards }) }}
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- 套餐卡：移动端横向 scroll-snap，桌面端三列 -->
      <div v-if="plansLoading" class="muc-pricing__state">
        <LoadingSpinner />
      </div>
      <p v-else-if="!plans.length" class="muc-pricing__state muc-pricing__state--text">
        {{ t('pricing.empty') }}
      </p>
      <section v-else class="muc-pricing__cards" data-testid="pricing-cards">
        <MucPlanCard
          v-for="plan in sortedPlans"
          :key="plan.id"
          class="muc-pricing__card"
          :display="cardDisplay(plan)"
          :variant="cardVariant(plan)"
          :cta-kind="ctaKind(plan)"
          :cta-label="ctaLabel(plan)"
          :popular="isPopular(plan)"
          :is-current="ctaKind(plan) === 'current'"
          :scheduled-plan-name="scheduledTargetName"
          :scheduled-effective-text="scheduledEffectiveText"
          :busy="busyKey === 'cancel-scheduled'"
          @cta="onCardCta(plan)"
          @cancel-scheduled="onCancelScheduled"
        />
      </section>

      <p class="muc-pricing__footer-note">{{ t('pricing.footerNote') }}</p>
    </div>

    <!-- 升级：服务端权威报价 + 支付方式 -->
    <UpgradePreviewModal
      v-model:selected-method="selectedMethod"
      :open="upgradeModalOpen"
      :quote="quote"
      :quote-loading="quoteLoading"
      :quote-error="quoteError"
      :methods="methodOptions"
      :confirming="creatingOrder"
      @close="closeUpgradeModal"
      @retry="fetchQuote"
      @confirm="onConfirmUpgrade"
    />

    <!-- 到期切换确认 -->
    <Teleport to="body">
      <Transition name="muc-modal">
        <div
          v-if="downgradeTarget"
          class="muc-scope muc-modal-overlay"
          role="dialog"
          aria-modal="true"
          :aria-label="t('pricing.downgrade.title')"
          @click.self="downgradeTarget = null"
        >
          <div class="muc-dialog">
            <h3 class="muc-dialog__title">{{ t('pricing.downgrade.title') }}</h3>
            <p class="muc-dialog__body">
              {{
                t('pricing.downgrade.body', {
                  date: formatDate(primarySub?.expires_at ?? ''),
                  plan: downgradeTarget.name
                })
              }}
            </p>
            <div class="muc-dialog__actions">
              <button type="button" class="muc-dialog__dismiss" @click="downgradeTarget = null">
                {{ t('common.cancel') }}
              </button>
              <button
                type="button"
                class="muc-dialog__confirm"
                :disabled="schedulingDowngrade"
                @click="onConfirmDowngrade"
              >
                {{ schedulingDowngrade ? t('pricing.downgrade.confirming') : t('pricing.downgrade.confirm') }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 取消到期切换确认 -->
    <Teleport to="body">
      <Transition name="muc-modal">
        <div
          v-if="cancelTarget"
          class="muc-scope muc-modal-overlay"
          role="dialog"
          aria-modal="true"
          :aria-label="t('pricing.downgrade.cancelTitle')"
          @click.self="cancelTarget = false"
        >
          <div class="muc-dialog">
            <h3 class="muc-dialog__title">{{ t('pricing.downgrade.cancelTitle') }}</h3>
            <p class="muc-dialog__body">
              {{ t('pricing.downgrade.cancelBody', { plan: currentPlanName }) }}
            </p>
            <div class="muc-dialog__actions">
              <button type="button" class="muc-dialog__dismiss" @click="cancelTarget = false">
                {{ t('common.cancel') }}
              </button>
              <button
                type="button"
                class="muc-dialog__confirm"
                :disabled="cancellingDowngrade"
                @click="onConfirmCancelScheduled"
              >
                {{ t('pricing.downgrade.cancelConfirm') }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import '@/components/pricing/muc-tokens.css'
import PricingBackground from '@/components/pricing/PricingBackground.vue'
import MucPlanCard, {
  type MucPlanCardDisplay
} from '@/components/pricing/MucPlanCard.vue'
import UpgradePreviewModal from '@/components/pricing/UpgradePreviewModal.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { paymentAPI } from '@/api/payment'
import {
  createUpgrade,
  getAccountStatus,
  getSubscriptionChanges,
  cancelScheduledDowngrade,
  previewUpgrade,
  scheduleDowngrade,
  type AccountStatus,
  type AccountSubscriptionStatus,
  type PlanChangeQuote,
  type PlanChangeRecordDto,
  type UsageStatus
} from '@/api/subscriptions'
import type { CheckoutInfoResponse, SubscriptionPlan } from '@/types/payment'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { getVisibleMethods } from '@/components/payment/paymentFlow'
import {
  DEFAULT_PAYMENT_CURRENCY,
  formatPaymentAmount,
  normalizePaymentCurrency
} from '@/components/payment/currency'
import { planValiditySuffix } from '@/components/payment/validity'
import { useAppStore } from '@/stores'

// 客户端仅做展示排序的 tier 近似（sort_order）；升/降级真值以后端 tier_rank 校验为准。
type MucPlanRow = SubscriptionPlan

type CtaKind = 'buy' | 'upgrade' | 'downgrade' | 'scheduled' | 'current' | 'unavailable'

const router = useRouter()
const { t, tm, locale } = useI18n()
const appStore = useAppStore()

const plans = ref<MucPlanRow[]>([])
const plansLoading = ref(true)
const checkout = ref<CheckoutInfoResponse | null>(null)
const account = ref<AccountStatus | null>(null)

const upgradeModalOpen = ref(false)
const modalPlan = ref<MucPlanRow | null>(null)
const quote = ref<PlanChangeQuote | null>(null)
const quoteLoading = ref(false)
const quoteError = ref('')
const selectedMethod = ref('')
const creatingOrder = ref(false)

const downgradeTarget = ref<MucPlanRow | null>(null)
const schedulingDowngrade = ref(false)
const cancelTarget = ref(false)
const cancellingDowngrade = ref(false)
const scheduledRecord = ref<PlanChangeRecordDto | null>(null)
/** 进行中的卡片动作（禁用对应按钮），形如 `cancel-{planId}` */
const busyKey = ref('')

const sortedPlans = computed(() => plans.value)

const activeSubscriptions = computed(() => account.value?.subscriptions ?? [])
const wallet = computed(() => account.value?.wallet ?? null)
const resetCards = computed(() => account.value?.reset_cards.available ?? 0)

const walletDisplay = computed(() => {
  const w = wallet.value
  if (!w) return '—'
  const value = parseFloat(w.balance)
  return formatPaymentAmount(
    Number.isFinite(value) ? value : 0,
    normalizePaymentCurrency(w.canonical_currency),
    typeof locale.value === 'string' ? locale.value : undefined
  )
})

const planByGroup = computed(() => {
  const map = new Map<number, MucPlanRow>()
  for (const plan of sortedPlans.value) map.set(plan.group_id, plan)
  return map
})

function planForSub(sub: AccountSubscriptionStatus): MucPlanRow | undefined {
  return planByGroup.value.get(sub.group_id)
}

/** Plan Change 作用的主订阅：取可映射到在售套餐中档位（sort_order）最高的一条。 */
const primarySub = computed<AccountSubscriptionStatus | null>(() => {
  const mapped = activeSubscriptions.value.filter((s) => planForSub(s))
  if (!mapped.length) return activeSubscriptions.value[0] ?? null
  return [...mapped].sort(
    (a, b) => (planForSub(b)?.sort_order ?? 0) - (planForSub(a)?.sort_order ?? 0)
  )[0]
})

const currentGroupIds = computed(() => new Set(activeSubscriptions.value.map((s) => s.group_id)))

const currentPlanName = computed(() => {
  const sub = primarySub.value
  return (sub && planForSub(sub)?.name) || sub?.display_name || ''
})

/** 已预约的到期切换（仅主订阅语义；记录为 PascalCase 合同）。 */
const scheduledTargetName = computed(() => {
  const rec = scheduledRecord.value
  if (!rec) return ''
  const target = sortedPlans.value.find((p) => p.id === rec.ToPlanID)
  return target?.name ?? `#${rec.ToPlanID}`
})

const scheduledEffectiveText = computed(() => {
  const rec = scheduledRecord.value
  if (!rec?.EffectiveAt) return null
  return formatDate(rec.EffectiveAt)
})

// ── 支付方式（与购买页同源 checkout-info）──
const methodOptions = computed<PaymentMethodOption[]>(() => {
  const visible = checkout.value ? getVisibleMethods(checkout.value.methods) : {}
  return Object.entries(visible).map(([type, ml]) => ({
    type,
    display_name: ml.display_name,
    fee_rate: ml.fee_rate ?? 0,
    available: ml.available !== false
  }))
})

// ── 数据加载 ──
async function loadAll() {
  const [plansRes, checkoutRes, statusRes] = await Promise.allSettled([
    paymentAPI.getPlans(),
    paymentAPI.getCheckoutInfo(),
    getAccountStatus()
  ])
  if (plansRes.status === 'fulfilled') {
    plans.value = plansRes.value
      .data
      .map((p) => ({ ...p, features: parseFeatures(p.features) }))
      .sort((a, b) => a.sort_order - b.sort_order || a.price - b.price)
  }
  if (checkoutRes.status === 'fulfilled') checkout.value = checkoutRes.value.data
  if (statusRes.status === 'fulfilled') account.value = statusRes.value
  plansLoading.value = false
  await refreshScheduled()
}

/** /payment/plans 的 features 是原始 JSON 字符串，checkout-info 才解析；这里自行容错解析。 */
function parseFeatures(raw: unknown): string[] {
  if (Array.isArray(raw)) return raw.filter((f): f is string => typeof f === 'string')
  if (typeof raw !== 'string' || !raw.trim()) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((f): f is string => typeof f === 'string') : []
  } catch {
    return []
  }
}

async function refreshScheduled() {
  const sub = primarySub.value
  if (!sub) {
    scheduledRecord.value = null
    return
  }
  try {
    const records = await getSubscriptionChanges(sub.id)
    scheduledRecord.value =
      records.find((r) => r.ChangeType === 'scheduled_downgrade' && r.Status === 'scheduled') ??
      null
  } catch {
    scheduledRecord.value = null
  }
}

onMounted(loadAll)

// ── CTA 判定 ──
function ctaKind(plan: MucPlanRow): CtaKind {
  if (currentGroupIds.value.has(plan.group_id)) return 'current'
  const primary = primarySub.value
  const primaryPlan = primary ? planForSub(primary) : undefined
  if (!primary || !primaryPlan) return 'buy'
  if (plan.sort_order > primaryPlan.sort_order) return 'upgrade'
  if (plan.sort_order < primaryPlan.sort_order) return 'downgrade'
  return 'buy'
}

function ctaLabel(plan: MucPlanRow): string {
  const scheduled =
    ctaKind(plan) === 'downgrade' &&
    scheduledRecord.value &&
    scheduledRecord.value.ToPlanID === plan.id
  switch (scheduled ? 'scheduled' : ctaKind(plan)) {
    case 'upgrade':
      return t('pricing.cta.upgradeTo', { plan: plan.name })
    case 'downgrade':
      return t('pricing.cta.downgrade')
    case 'scheduled':
      return t('pricing.cta.scheduled')
    case 'current':
      return t('pricing.cta.current')
    case 'unavailable':
      return t('pricing.cta.unavailable')
    default:
      return t('pricing.cta.buy')
  }
}

/** 档位视觉：0=Basic 黑灰，中位=Pro 红，末位=Max 红+少量金 */
function cardVariant(plan: MucPlanRow): 'basic' | 'pro' | 'max' {
  const list = sortedPlans.value
  const idx = list.findIndex((p) => p.id === plan.id)
  if (list.length >= 3) {
    if (idx === list.length - 1) return 'max'
    if (idx > 0) return 'pro'
    return 'basic'
  }
  return idx === list.length - 1 ? 'pro' : 'basic'
}

function isPopular(plan: MucPlanRow): boolean {
  return cardVariant(plan) === 'pro'
}

function cardDisplay(plan: MucPlanRow): MucPlanCardDisplay {
  return {
    name: plan.name,
    description: plan.description || undefined,
    price: priceDisplay(plan),
    originalPrice: plan.original_price ? priceDisplay({ ...plan, price: plan.original_price }) : undefined,
    validitySuffix: t('pricing.perValidity', { validity: planValiditySuffix(plan, t) }),
    // 后端 features 为空时使用定性文案（不编造具体数值）
    features: plan.features.length ? plan.features : fallbackFeatures(plan)
  }
}

/** 后端无 features 时的定性权益文案（i18n 以 | 分隔，叶子须为字符串以满足键完整性测试）。 */
function fallbackFeatures(plan: MucPlanRow): string[] {
  const variant = cardVariant(plan)
  const messages = tm('pricing.fallbackFeatures') as Record<string, unknown> | undefined
  const raw = messages?.[variant]
  return typeof raw === 'string' && raw.trim()
    ? raw.split('|').map((f) => f.trim()).filter(Boolean)
    : []
}

/** 套餐价 USD 语义：配置了折算汇率则按 CNY 展示（与购买页口径严格镜像）。 */
function priceDisplay(plan: MucPlanRow): string {
  const rate = checkout.value?.subscription_usd_to_cny_rate ?? 0
  const loc = typeof locale.value === 'string' ? locale.value : undefined
  if (rate > 0) return formatPaymentAmount(plan.price * rate, DEFAULT_PAYMENT_CURRENCY, loc)
  return formatPaymentAmount(plan.price, normalizePaymentCurrency(plan.currency), loc)
}

// ── 卡片动作 ──
function onCardCta(plan: MucPlanRow) {
  switch (ctaKind(plan)) {
    case 'current':
      break
    case 'upgrade':
      openUpgradeModal(plan)
      break
    case 'downgrade':
      downgradeTarget.value = plan
      break
    default:
      // 新购/并存购买走既有购买页（支付编排全部复用）
      void router.push('/purchase')
  }
}

// ── 升级（服务端报价）──
async function openUpgradeModal(plan: MucPlanRow) {
  modalPlan.value = plan
  quote.value = null
  quoteError.value = ''
  selectedMethod.value = methodOptions.value.find((m) => m.available)?.type ?? ''
  upgradeModalOpen.value = true
  await fetchQuote()
}

async function fetchQuote() {
  const sub = primarySub.value
  const plan = modalPlan.value
  if (!sub || !plan) return
  quoteLoading.value = true
  quoteError.value = ''
  try {
    quote.value = await previewUpgrade(sub.id, plan.id)
  } catch (err) {
    quote.value = null
    quoteError.value = extractErrorMessage(err, t('pricing.upgradeModal.quoteError'))
  } finally {
    quoteLoading.value = false
  }
}

async function onConfirmUpgrade(paymentType: string) {
  const sub = primarySub.value
  const plan = modalPlan.value
  if (!sub || !plan || !paymentType) return
  creatingOrder.value = true
  try {
    const order = await createUpgrade(
      sub.id,
      plan.id,
      paymentType,
      cryptoRandomKey()
    )
    closeUpgradeModal()
    dispatchOrder(order)
  } catch (err) {
    const reason = extractErrorReason(err)
    if (reason === 'PLAN_QUOTE_EXPIRED') {
      appStore.showInfo(t('pricing.errors.quoteExpired'))
      await fetchQuote()
    } else {
      appStore.showError(
        extractErrorMessage(err, t('pricing.errors.upgradeFailed'))
      )
    }
  } finally {
    creatingOrder.value = false
  }
}

/** 升级下单返回的订单沿用既有支付落地页（QR / 跳转 / 结果轮询）。 */
function dispatchOrder(order: { order_id: number; qr_code?: string; pay_url?: string; payment_type?: string }) {
  if (order.qr_code) {
    void router.push({
      path: '/payment/qrcode',
      query: {
        order_id: String(order.order_id),
        qr: order.qr_code,
        payment_type: order.payment_type ?? ''
      }
    })
    return
  }
  if (order.pay_url) {
    window.location.href = order.pay_url
    return
  }
  void router.push({ path: '/payment/result', query: { order_id: String(order.order_id) } })
}

function closeUpgradeModal() {
  upgradeModalOpen.value = false
  modalPlan.value = null
  quote.value = null
  quoteError.value = ''
}

// ── 到期切换（scheduled downgrade）──
async function onConfirmDowngrade() {
  const sub = primarySub.value
  const plan = downgradeTarget.value
  if (!sub || !plan) return
  schedulingDowngrade.value = true
  try {
    await scheduleDowngrade(sub.id, plan.id, cryptoRandomKey())
    downgradeTarget.value = null
    appStore.showSuccess(t('pricing.downgrade.scheduledToast'))
    await refreshScheduled()
  } catch (err) {
    appStore.showError(extractErrorMessage(err, t('pricing.errors.scheduleFailed')))
  } finally {
    schedulingDowngrade.value = false
  }
}

function onCancelScheduled() {
  if (!scheduledRecord.value) return
  cancelTarget.value = true
}

async function onConfirmCancelScheduled() {
  const sub = primarySub.value
  if (!sub) return
  cancellingDowngrade.value = true
  busyKey.value = 'cancel-scheduled'
  try {
    await cancelScheduledDowngrade(sub.id)
    cancelTarget.value = false
    scheduledRecord.value = null
    appStore.showSuccess(t('pricing.downgrade.cancelledToast'))
  } catch (err) {
    appStore.showError(extractErrorMessage(err, t('pricing.errors.cancelFailed')))
  } finally {
    cancellingDowngrade.value = false
    busyKey.value = ''
  }
}

// ── 工具 ──
function cryptoRandomKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `pricing-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function extractErrorReason(err: unknown): string {
  if (err && typeof err === 'object' && 'reason' in err) {
    return String((err as { reason?: unknown }).reason ?? '')
  }
  return ''
}

function extractErrorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const message = String((err as { message?: unknown }).message ?? '').trim()
    if (message && message !== 'Unknown error') return message
  }
  return fallback
}

function formatDate(iso: string): string {
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

function statusLabel(status: UsageStatus): string {
  return t(`pricing.usageStatus.${status}`)
}

/** 业务状态色与品牌红分层：exhausted 才用高饱和警示红（--muc-danger）。 */
function statusChipClass(status: UsageStatus): string {
  switch (status) {
    case 'exhausted':
      return 'muc-chip--exhausted'
    case 'near_limit':
      return 'muc-chip--near-limit'
    case 'high':
      return 'muc-chip--high'
    case 'unmetered':
      return 'muc-chip--unmetered'
    default:
      return 'muc-chip--normal'
  }
}

function statusBarClass(status: UsageStatus): string {
  switch (status) {
    case 'exhausted':
      return 'muc-bar--exhausted'
    case 'near_limit':
      return 'muc-bar--near-limit'
    case 'high':
      return 'muc-bar--high'
    case 'unmetered':
      return 'muc-bar--unmetered'
    default:
      return 'muc-bar--normal'
  }
}
</script>

<style scoped>
/* token 定义见 components/pricing/muc-tokens.css（.muc-scope）——Teleport 浮层同样挂该 class */
.muc-pricing {
  position: relative;
  overflow: hidden;
  min-height: calc(100vh - 96px);
  border-radius: 24px;
  background: var(--muc-bg-primary);
  color: var(--muc-text-primary);
}

.muc-pricing__content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 28px;
  max-width: 1120px;
  margin: 0 auto;
  padding: clamp(32px, 6vh, 72px) clamp(18px, 4vw, 40px) 40px;
}

/* ── 水印 ── */
.muc-pricing__watermark {
  pointer-events: none;
  position: absolute;
  left: 50%;
  top: clamp(4px, 2vh, 28px);
  transform: translateX(-50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  user-select: none;
}

.muc-pricing__watermark-caption {
  font-size: clamp(11px, 1.2vw, 14px);
  font-weight: 600;
  letter-spacing: 0.55em;
  text-indent: 0.55em;
  color: rgba(255, 226, 228, 0.55);
}

.muc-pricing__watermark-word {
  font-size: clamp(88px, 15vw, 250px);
  font-weight: 800;
  line-height: 0.95;
  letter-spacing: 0.02em;
  white-space: nowrap;
  background: var(--muc-watermark-gradient);
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  opacity: 0.42;
}

/* 窄屏收窄水印，避免字母在视口两侧被裁切 */
@media (max-width: 640px) {
  .muc-pricing__watermark-word {
    font-size: clamp(40px, 18.5vw, 88px);
  }
}

.muc-pricing__header {
  position: relative;
  margin-top: clamp(64px, 14vh, 150px);
  text-align: center;
}

.muc-pricing__title {
  font-size: clamp(26px, 3.4vw, 38px);
  font-weight: 700;
  letter-spacing: 0.01em;
}

.muc-pricing__subtitle {
  margin-top: 10px;
  font-size: clamp(13px, 1.4vw, 15px);
  color: var(--muc-text-secondary);
}

/* ── 账户状态条 ── */
.muc-status {
  display: flex;
  align-items: stretch;
  gap: 20px;
  padding: 16px 20px;
  border-radius: 16px;
  border: 1px solid var(--muc-glass-border);
  background: var(--muc-glass);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
}

.muc-status__wallet {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 4px;
  min-width: 128px;
}

.muc-status__label {
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--muc-text-muted);
}

.muc-status__wallet-value {
  font-size: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.muc-status__divider {
  width: 1px;
  background: var(--muc-glass-border);
}

.muc-status__muted {
  display: flex;
  align-items: center;
  font-size: 13px;
  color: var(--muc-text-muted);
}

.muc-status__subs {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: 14px;
}

.muc-status__sub {
  flex: 1 1 240px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.muc-status__sub-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.muc-status__sub-name {
  font-size: 13.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.muc-status__chip {
  flex-shrink: 0;
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
  border: 1px solid transparent;
}

.muc-chip--normal {
  color: rgba(255, 255, 255, 0.85);
  border-color: rgba(255, 255, 255, 0.25);
  background: rgba(255, 255, 255, 0.06);
}

.muc-chip--high {
  color: #ecc94b;
  border-color: rgba(236, 201, 75, 0.4);
  background: rgba(236, 201, 75, 0.1);
}

.muc-chip--near-limit {
  color: #f08c3a;
  border-color: rgba(240, 140, 58, 0.45);
  background: rgba(240, 140, 58, 0.1);
}

.muc-chip--exhausted {
  color: var(--muc-danger);
  border-color: rgba(255, 69, 80, 0.55);
  background: rgba(255, 69, 80, 0.12);
}

.muc-chip--unmetered {
  color: var(--muc-gold);
  border-color: rgba(214, 180, 106, 0.4);
  background: rgba(214, 180, 106, 0.08);
}

.muc-status__sub-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.muc-status__sub-bar {
  flex: 1;
  height: 5px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.1);
  overflow: hidden;
}

.muc-status__sub-bar-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.4s ease;
}

/* normal=柔和浅色（不用品牌红表达进度） */
.muc-bar--normal {
  background: rgba(255, 255, 255, 0.72);
}

.muc-bar--high {
  background: #ecc94b;
}

.muc-bar--near-limit {
  background: #f08c3a;
}

.muc-bar--exhausted {
  background: var(--muc-danger);
}

.muc-bar--unmetered {
  background: linear-gradient(to right, var(--muc-gold), rgba(214, 180, 106, 0.35));
}

.muc-status__sub-percent {
  flex-shrink: 0;
  min-width: 40px;
  text-align: right;
  font-size: 12px;
  color: var(--muc-text-secondary);
  font-variant-numeric: tabular-nums;
}

.muc-status__sub-meta {
  display: flex;
  gap: 12px;
  font-size: 11.5px;
  color: var(--muc-text-muted);
}

/* ── 套餐卡区：移动端 snap 横滑 ── */
.muc-pricing__cards {
  display: flex;
  gap: 20px;
  overflow-x: auto;
  scroll-snap-type: x mandatory;
  -webkit-overflow-scrolling: touch;
  padding: 4px 2px 12px;
  scrollbar-width: none;
}

.muc-pricing__cards::-webkit-scrollbar {
  display: none;
}

.muc-pricing__card {
  flex: 0 0 min(84%, 340px);
  scroll-snap-align: center;
}

@media (min-width: 900px) {
  .muc-pricing__cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
    gap: 22px;
    overflow: visible;
    align-items: stretch;
  }

  .muc-pricing__card {
    flex: initial;
  }
}

.muc-pricing__state {
  display: flex;
  justify-content: center;
  padding: 48px 0;
  color: var(--muc-text-muted);
}

.muc-pricing__state--text {
  font-size: 14px;
}

.muc-pricing__footer-note {
  text-align: center;
  font-size: 11.5px;
  letter-spacing: 0.04em;
  color: var(--muc-text-muted);
}

/* ── 小确认弹窗（到期切换 / 取消预约）── */
.muc-modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 90;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(4, 4, 5, 0.72);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}

.muc-dialog {
  width: min(420px, 100%);
  border-radius: 18px;
  border: 1px solid var(--muc-glass-border-strong);
  background: linear-gradient(175deg, rgba(24, 24, 27, 0.94), rgba(12, 12, 14, 0.97));
  box-shadow: 0 30px 90px rgba(0, 0, 0, 0.55), 0 0 40px rgba(238, 56, 72, 0.1);
  padding: 22px;
  color: var(--muc-text-primary);
}

.muc-dialog__title {
  font-size: 16px;
  font-weight: 700;
}

.muc-dialog__body {
  margin-top: 10px;
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--muc-text-secondary);
}

.muc-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}

.muc-dialog__dismiss {
  border: none;
  background: none;
  padding: 9px 14px;
  border-radius: 11px;
  font-size: 13.5px;
  color: var(--muc-text-muted);
  cursor: pointer;
}

.muc-dialog__dismiss:hover {
  color: var(--muc-text-primary);
}

.muc-dialog__confirm {
  padding: 9px 16px;
  border-radius: 11px;
  border: 1px solid transparent;
  background: #fff;
  color: #0a0a0b;
  font-size: 13.5px;
  font-weight: 600;
  cursor: pointer;
}

.muc-dialog__confirm:hover:not(:disabled) {
  box-shadow: 0 6px 26px rgba(238, 56, 72, 0.3);
}

.muc-dialog__confirm:disabled {
  opacity: 0.55;
  cursor: wait;
}

.muc-modal-enter-active,
.muc-modal-leave-active {
  transition: opacity 0.22s ease;
}

.muc-modal-enter-active .muc-dialog,
.muc-modal-leave-active .muc-dialog {
  transition: transform 0.22s ease, opacity 0.22s ease;
}

.muc-modal-enter-from,
.muc-modal-leave-to {
  opacity: 0;
}

.muc-modal-enter-from .muc-dialog,
.muc-modal-leave-to .muc-dialog {
  transform: translateY(10px) scale(0.98);
  opacity: 0;
}
</style>
