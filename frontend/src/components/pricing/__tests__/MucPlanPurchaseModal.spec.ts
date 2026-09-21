import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MucPlanPurchaseModal from '../MucPlanPurchaseModal.vue'

// vitest 的 vue-i18n 是 runtime 构建（无消息编译器），这里直接 mock t。
const messages = vi.hoisted(() => ({
  'common.close': 'Close',
  'common.cancel': 'Cancel',
  'payment.amountLabel': 'Amount',
  'payment.fee': 'Fee',
  'payment.actualPay': 'Total',
  'pricing.purchaseModal.title': 'Confirm purchase',
  'pricing.purchaseModal.plan': 'Plan',
  'pricing.purchaseModal.price': 'Plan price',
  'pricing.purchaseModal.note': 'Note',
  'pricing.purchaseModal.confirm': 'Pay',
  'pricing.upgradeModal.payMethod': 'Payment method',
  'pricing.upgradeModal.noMethod': 'No payment method available',
  'pricing.upgradeModal.confirming': 'Creating order…',
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => (messages as Record<string, string>)[key] ?? key }),
}))

const methods = [
  { type: 'alipay', display_name: 'Alipay', fee_rate: 2.5, available: true },
  { type: 'wxpay', display_name: 'WeChat Pay', fee_rate: 0, available: true },
]

function mountModal(overrides: Partial<InstanceType<typeof MucPlanPurchaseModal>['$props']> = {}) {
  return mount(MucPlanPurchaseModal, {
    props: {
      open: true,
      plan: { id: 7, name: 'Pro', description: 'MUCODE Pro' },
      methods,
      selectedMethod: 'alipay',
      displayPrice: '¥71.43',
      feeRatePercent: 2.5,
      validityText: '/ 30 days',
      ...overrides,
    },
    global: {
      stubs: { Teleport: true, Transition: false },
    },
  })
}

describe('MucPlanPurchaseModal', () => {
  it('renders the selected plan with price and validity', () => {
    const wrapper = mountModal()
    expect(wrapper.text()).toContain('Pro')
    expect(wrapper.text()).toContain('¥71.43')
    expect(wrapper.text()).toContain('/ 30 days')
    expect(wrapper.text()).toContain('MUCODE Pro')
  })

  it('adds the gateway fee after the displayed price to match backend pay_amount', () => {
    const wrapper = mountModal()
    const text = wrapper.text()
    // 2.5% of ¥71.43 → fee ¥1.79, total ¥73.22（ceil/round 语义与后端一致）
    expect(text).toContain('2.5%')
    expect(text).toContain('¥1.79')
    expect(text).toContain('¥73.22')
  })

  it('hides the fee breakdown when the method has no fee', () => {
    const wrapper = mountModal({ selectedMethod: 'wxpay', feeRatePercent: 0 })
    expect(wrapper.text()).not.toContain('¥1.79')
  })

  it('emits confirm with the selected method and closes via cancel', async () => {
    const wrapper = mountModal()
    await wrapper.get('button.muc-modal__confirm').trigger('click')
    expect(wrapper.emitted('confirm')).toEqual([['alipay']])

    await wrapper.get('button.muc-modal__dismiss').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('disables confirm while no method is selected', () => {
    const wrapper = mountModal({ selectedMethod: '' })
    const pay = wrapper.get('button.muc-modal__confirm')
    expect(pay.attributes('disabled')).toBeDefined()
  })
})
