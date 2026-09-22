import { useI18n } from 'vue-i18n'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { paymentMethodI18nKey } from '@/views/user/paymentUx'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'

const METHOD_ICONS: Record<string, string> = {
  alipay: alipayIcon,
  wxpay: wxpayIcon,
  wechat: wxpayIcon,
  stripe: stripeIcon,
  airwallex: airwallexIcon
}

/** 套餐弹窗共用的支付方式视觉（图标 + 展示名）。 */
export function usePlanMethodVisuals() {
  const { t } = useI18n()

  function methodIcon(type: string): string {
    return METHOD_ICONS[type] ?? ''
  }

  function methodDisplayName(method: PaymentMethodOption): string {
    return method.display_name || t(paymentMethodI18nKey(method.type))
  }

  return { methodIcon, methodDisplayName }
}
