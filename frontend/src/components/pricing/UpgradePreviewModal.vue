<template>
  <Teleport to="body">
    <Transition name="muc-modal">
      <div
        v-if="open"
        class="muc-scope muc-modal-overlay"
        role="dialog"
        aria-modal="true"
        :aria-label="t('pricing.upgradeModal.title')"
        @click.self="emit('close')"
        @keydown.esc="emit('close')"
      >
        <div class="muc-modal" tabindex="-1">
          <header class="muc-modal__head">
            <h3 class="muc-modal__title">{{ t('pricing.upgradeModal.title') }}</h3>
            <button type="button" class="muc-modal__close" :aria-label="$t('common.close')" @click="emit('close')">
              <Icon name="x" size="md" />
            </button>
          </header>

          <!-- 报价加载 / 错误 / 内容 -->
          <div v-if="quoteLoading" class="muc-modal__state">
            <LoadingSpinner />
            <p>{{ t('pricing.upgradeModal.quoteLoading') }}</p>
          </div>

          <div v-else-if="quoteError" class="muc-modal__state">
            <Icon name="exclamationCircle" size="lg" class="muc-modal__error-icon" />
            <p>{{ quoteError }}</p>
            <button type="button" class="muc-modal__retry" @click="emit('retry')">
              <Icon name="refresh" size="sm" />
              {{ t('pricing.upgradeModal.retry') }}
            </button>
          </div>

          <div v-else-if="quote" class="muc-modal__body">
            <div class="muc-modal__route">
              <div class="muc-modal__route-plan">
                <span class="muc-modal__route-label">{{ t('pricing.upgradeModal.from') }}</span>
                <span class="muc-modal__route-name">{{ quote.from_display_name }}</span>
              </div>
              <Icon name="arrowRight" size="md" class="muc-modal__route-arrow" />
              <div class="muc-modal__route-plan muc-modal__route-plan--to">
                <span class="muc-modal__route-label">{{ t('pricing.upgradeModal.to') }}</span>
                <span class="muc-modal__route-name">{{ quote.to_display_name }}</span>
              </div>
            </div>

            <div class="muc-modal__amount">
              <span class="muc-modal__amount-label">{{ t('pricing.upgradeModal.amountDue') }}</span>
              <span class="muc-modal__amount-value">{{ formatAmount(quote.amount_due) }}</span>
            </div>

            <dl class="muc-modal__breakdown">
              <div v-if="creditValue > 0" class="muc-modal__breakdown-row">
                <dt>{{ t('pricing.upgradeModal.unusedCredit') }}</dt>
                <dd>-{{ formatAmount(quote.unused_credit) }}</dd>
              </div>
              <div v-if="chargeValue > 0" class="muc-modal__breakdown-row">
                <dt>{{ t('pricing.upgradeModal.proratedCharge') }}</dt>
                <dd>{{ formatAmount(quote.prorated_charge) }}</dd>
              </div>
            </dl>

            <ul class="muc-modal__facts">
              <li>
                <Icon name="checkCircle" size="sm" class="muc-modal__fact-icon" />
                <span>{{ t('pricing.upgradeModal.effectiveNow', { date: formatDate(quote.current_expiry) }) }}</span>
              </li>
              <li v-if="quote.short_remaining_percent_after !== undefined">
                <span>升级后预计剩余：5 小时 {{ formatRemainingPercent(quote.short_remaining_percent_after) }} · 本周 {{ formatRemainingPercent(quote.weekly_remaining_percent_after) }}。已用量与恢复时间不变，实际以付款履约时用量为准。</span>
              </li>
              <li v-else-if="quote.weekly_usage_percent_before !== null">
                <Icon name="chart" size="sm" class="muc-modal__fact-icon" />
                <span>{{
                  t('pricing.upgradeModal.usageShift', {
                    before: quote.weekly_usage_percent_before,
                    after: quote.weekly_usage_percent_after
                  })
                }}</span>
                <span class="muc-modal__status-chip" :class="statusChipClass(quote.usage_status_after)">
                  {{ statusLabel(quote.usage_status_after) }}
                </span>
              </li>
              <li v-else>
                <Icon name="chart" size="sm" class="muc-modal__fact-icon" />
                <span>{{ t('pricing.upgradeModal.usageAfter', { after: quote.weekly_usage_percent_after }) }}</span>
              </li>
              <li>
                <Icon name="key" size="sm" class="muc-modal__fact-icon" />
                <span>{{ t('pricing.upgradeModal.inheritKeys') }}</span>
              </li>
              <li v-if="quote.keys_to_migrate_count > 0">
                <Icon name="key" size="sm" class="muc-modal__fact-icon" />
                <span>{{ t('pricing.upgradeModal.keysMigrate', { count: quote.keys_to_migrate_count }) }}</span>
              </li>
            </ul>

            <div class="muc-modal__methods">
              <span class="muc-modal__methods-label">{{ t('pricing.upgradeModal.payMethod') }}</span>
              <div v-if="methods.length" class="muc-modal__methods-grid">
                <button
                  v-for="method in methods"
                  :key="method.type"
                  type="button"
                  class="muc-modal__method"
                  :class="{ 'muc-modal__method--selected': selectedMethod === method.type }"
                  :aria-pressed="selectedMethod === method.type"
                  @click="emit('update:selectedMethod', method.type)"
                >
                  <img v-if="methodIcon(method.type)" :src="methodIcon(method.type)" alt="" class="muc-modal__method-icon" />
                  <span class="muc-modal__method-name">{{ methodDisplayName(method) }}</span>
                  <span v-if="method.fee_rate > 0" class="muc-modal__method-fee">{{ method.fee_rate }}%</span>
                </button>
              </div>
              <p v-else class="muc-modal__no-method">{{ t('pricing.upgradeModal.noMethod') }}</p>
            </div>

            <p class="muc-modal__note">{{ t('pricing.upgradeModal.note') }}</p>
          </div>

          <footer v-if="quote && !quoteLoading && !quoteError" class="muc-modal__foot">
            <button type="button" class="muc-modal__dismiss" @click="emit('close')">
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              class="muc-modal__confirm"
              :disabled="!selectedMethod || confirming"
              @click="emit('confirm', selectedMethod)"
            >
              {{ confirming ? t('pricing.upgradeModal.confirming') : t('pricing.upgradeModal.confirm') }}
              <span v-if="quote && !confirming" class="muc-modal__confirm-amount">
                {{ formatAmount(quote.amount_due) }}
              </span>
            </button>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { formatRemainingPercent } from '@/utils/quotaDisplay'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/components/pricing/muc-tokens.css'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { usePlanMethodVisuals } from '@/components/pricing/planModalShared'
import type { PlanChangeQuote, UsageStatus } from '@/api/subscriptions'

const props = defineProps<{
  open: boolean
  quote: PlanChangeQuote | null
  quoteLoading: boolean
  quoteError: string
  methods: PaymentMethodOption[]
  selectedMethod: string
  confirming: boolean
}>()

const emit = defineEmits<{
  close: []
  retry: []
  confirm: [paymentType: string]
  'update:selectedMethod': [type: string]
}>()

const { t, locale } = useI18n()

const creditValue = computed(() => parseFloat(props.quote?.unused_credit ?? '0') || 0)
const chargeValue = computed(() => parseFloat(props.quote?.prorated_charge ?? '0') || 0)

function formatAmount(decimalString: string): string {
  const value = parseFloat(decimalString)
  return formatPaymentAmount(
    Number.isFinite(value) ? value : 0,
    normalizePaymentCurrency(props.quote?.currency),
    typeof locale.value === 'string' ? locale.value : undefined
  )
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
  } catch {
    return iso
  }
}

function statusLabel(status: UsageStatus): string {
  return t(`pricing.usageStatus.${status}`)
}

/** 业务状态色与品牌红分层：exhausted 用高饱和警示红，normal 用柔和浅色。 */
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

const { methodIcon, methodDisplayName } = usePlanMethodVisuals()
</script>

<style scoped src="./muc-modal.css"></style>
