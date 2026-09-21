<template>
  <Teleport to="body">
    <Transition name="muc-modal">
      <div
        v-if="open"
        class="muc-scope muc-modal-overlay"
        role="dialog"
        aria-modal="true"
        :aria-label="t('pricing.purchaseModal.title')"
        @click.self="emit('close')"
        @keydown.esc="emit('close')"
      >
        <div class="muc-modal" tabindex="-1">
          <header class="muc-modal__head">
            <h3 class="muc-modal__title">{{ t('pricing.purchaseModal.title') }}</h3>
            <button type="button" class="muc-modal__close" :aria-label="t('common.close')" @click="emit('close')">
              <Icon name="x" size="md" />
            </button>
          </header>

          <div v-if="plan" class="muc-modal__body">
            <div class="muc-modal__route">
              <div class="muc-modal__route-plan muc-modal__route-plan--to">
                <span class="muc-modal__route-label">{{ t('pricing.purchaseModal.plan') }}</span>
                <span class="muc-modal__route-name">{{ plan.name }}</span>
              </div>
            </div>

            <div class="muc-modal__amount">
              <span class="muc-modal__amount-label">{{ t('pricing.purchaseModal.price') }}</span>
              <span class="muc-modal__amount-value">{{ displayPrice }}</span>
              <span v-if="validityText" class="muc-modal__validity">{{ validityText }}</span>
            </div>

            <p v-if="plan.description" class="muc-modal__plan-desc">{{ plan.description }}</p>

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

            <dl v-if="feeAmountValue > 0" class="muc-modal__breakdown">
              <div class="muc-modal__breakdown-row">
                <dt>{{ t('payment.amountLabel') }}</dt>
                <dd>{{ displayPrice }}</dd>
              </div>
              <div class="muc-modal__breakdown-row">
                <dt>{{ t('payment.fee') }} ({{ feeRatePercent }}%)</dt>
                <dd>{{ formatAmount(feeAmountValue) }}</dd>
              </div>
              <div class="muc-modal__breakdown-row">
                <dt>{{ t('payment.actualPay') }}</dt>
                <dd>{{ formatAmount(totalAmountValue) }}</dd>
              </div>
            </dl>

            <p class="muc-modal__note">{{ t('pricing.purchaseModal.note') }}</p>
          </div>

          <footer v-if="plan" class="muc-modal__foot">
            <button type="button" class="muc-modal__dismiss" @click="emit('close')">
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              class="muc-modal__confirm"
              :disabled="!selectedMethod || confirming"
              @click="emit('confirm', selectedMethod)"
            >
              {{ confirming ? t('pricing.upgradeModal.confirming') : t('pricing.purchaseModal.confirm') }}
              <span v-if="!confirming" class="muc-modal__confirm-amount">
                {{ formatAmount(totalAmountValue) }}
              </span>
            </button>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
// 新购套餐确认弹窗：/pricing 是全站唯一套餐目录面，新购在此完成选择与确认。
// 仅做展示与支付方式选择；订单创建与支付启动由父级（PricingView）执行。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/components/pricing/muc-tokens.css'
import Icon from '@/components/icons/Icon.vue'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { usePlanMethodVisuals } from '@/components/pricing/planModalShared'

interface PurchasePlanDisplay {
  id: number
  name: string
  description?: string
}

const props = withDefaults(defineProps<{
  open: boolean
  plan: PurchasePlanDisplay | null
  methods: PaymentMethodOption[]
  selectedMethod: string
  confirming?: boolean
  /** 套餐展示价（父级已按 USD/CNY 语义换算），如 "¥39.00" */
  displayPrice: string
  /** 支付网关手续费率（%），0 = 无手续费 */
  feeRatePercent?: number
  validityText?: string
}>(), {
  confirming: false,
  feeRatePercent: 0,
  validityText: '',
})

const emit = defineEmits<{
  close: []
  'update:selectedMethod': [method: string]
  confirm: [method: string]
}>()

const { t } = useI18n()
const { methodIcon, methodDisplayName } = usePlanMethodVisuals()

function parseAmount(display: string): number {
  const numeric = Number.parseFloat(display.replace(/[^0-9.]/g, ''))
  return Number.isFinite(numeric) ? numeric : 0
}

const priceValue = computed(() => parseAmount(props.displayPrice))
const feeAmountValue = computed(() =>
  props.feeRatePercent > 0 && priceValue.value > 0
    ? Math.ceil(((priceValue.value * props.feeRatePercent) / 100) * 100) / 100
    : 0
)
const totalAmountValue = computed(() =>
  props.feeRatePercent > 0 && priceValue.value > 0
    ? Math.round((priceValue.value + feeAmountValue.value) * 100) / 100
    : priceValue.value
)

function formatAmount(value: number): string {
  // 与父级 displayPrice 同币种同格式：借用其货币前/后缀，仅重算数值。
  const match = props.displayPrice.match(/^([^\d.,-]*)([\d.,]+)(.*)$/)
  if (!match) return props.displayPrice
  const [, prefix, , suffix] = match
  return `${prefix}${value.toFixed(2)}${suffix}`
}
</script>

<style scoped src="./muc-modal.css"></style>
