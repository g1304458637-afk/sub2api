import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import PaymentView from '../PaymentView.vue'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
import { formatPaymentAmount as _formatPaymentAmount } from '@/components/payment/currency'
import AmountInput from '@/components/payment/AmountInput.vue'
import type { CheckoutInfoResponse, MethodLimit, SubscriptionPlan } from '@/types/payment'

const routeState = vi.hoisted(() => ({
  path: '/purchase',
  query: {} as Record<string, unknown>,
}))

const routerReplace = vi.hoisted(() => vi.fn())
const routerPush = vi.hoisted(() => vi.fn())
const routerResolve = vi.hoisted(() => vi.fn(() => ({ href: '/payment/stripe?mock=1' })))
const createOrder = vi.hoisted(() => vi.fn())
const refreshUser = vi.hoisted(() => vi.fn())
const fetchActiveSubscriptions = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))
const showError = vi.hoisted(() => vi.fn())
const showInfo = vi.hoisted(() => vi.fn())
const showWarning = vi.hoisted(() => vi.fn())
const getCheckoutInfo = vi.hoisted(() => vi.fn())
const bridgeInvoke = vi.hoisted(() => vi.fn())
const translate = vi.hoisted(() => vi.fn((key: string) => key))
// Public settings live in a reactive holder so tests can flip feature flags after mount
// and exercise the watchers that react to them.
const appStoreState = vi.hoisted(() => ({
  setPublicSettings: (_value: Record<string, unknown> | undefined) => {},
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({
      replace: routerReplace,
      push: routerPush,
      resolve: routerResolve,
    }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate,
    }),
  }
})

vi.mock('@/stores/currencyDisplay', () => ({
  useCurrencyDisplayStore: () => ({
    displayCurrency: 'USD',
    formatUSD: (amount: number | null | undefined, decimals = 2) => `$${Number(amount ?? 0).toFixed(decimals)}`,
    formatCNY: (amount: number | null | undefined, decimals = 2) => `¥${Number(amount ?? 0).toFixed(decimals)}`,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      username: 'demo-user',
      balance: 0,
    },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores', async () => {
  const { reactive } = await import('vue')
  const state = reactive({ cachedPublicSettings: undefined as Record<string, unknown> | undefined })
  appStoreState.setPublicSettings = (value) => {
    state.cachedPublicSettings = value
  }
  return {
    useAppStore: () => ({
      showError,
      showInfo,
      showWarning,
      get cachedPublicSettings() {
        return state.cachedPublicSettings
      },
    }),
  }
})

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
  },
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => true,
}))

function checkoutInfoFixture(overrides: Partial<CheckoutInfoResponse> = {}) {
  const wxpayMethod: MethodLimit = {
    daily_limit: 0,
    daily_used: 0,
    daily_remaining: 0,
    single_min: 0,
    single_max: 0,
    fee_rate: 0,
    available: true,
  }
  const data: CheckoutInfoResponse = {
    methods: {
      wxpay: wxpayMethod,
    },
    global_min: 0,
    global_max: 0,
    plans: [],
    balance_disabled: false,
    balance_recharge_multiplier: 1,
    subscription_usd_to_cny_rate: 0,
    recharge_fee_rate: 0,
    help_text: '',
    help_image_url: '',
    stripe_publishable_key: '',
  }

  return {
    data: { ...data, ...overrides },
  }
}

function checkoutInfoWithPlansFixture(options: {
  checkout?: Partial<CheckoutInfoResponse>
  method?: Partial<MethodLimit>
  plan?: Partial<SubscriptionPlan>
} = {}) {
  const base = checkoutInfoFixture(options.checkout).data
  const plan: SubscriptionPlan = {
    id: 7,
    group_id: 3,
    name: 'Starter',
    description: '',
    price: 128,
    original_price: 0,
    validity_days: 30,
    validity_unit: 'day',
    rate_multiplier: 1,
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    features: [],
    group_platform: 'openai',
    sort_order: 1,
    for_sale: true,
    group_name: 'OpenAI',
    ...options.plan,
  }

  return {
    data: {
      ...base,
      methods: {
        ...base.methods,
        wxpay: {
          ...base.methods.wxpay,
          ...options.method,
        },
      },
      plans: [plan],
    },
  }
}

function jsapiOrderFixture(resumeToken: string) {
  return {
    order_id: 123,
    amount: 88,
    pay_amount: 88,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    out_trade_no: 'sub2_jsapi_123',
    result_type: 'jsapi_ready' as const,
    resume_token: resumeToken,
    jsapi: {
      appId: 'wx123',
      timeStamp: '1712345678',
      nonceStr: 'nonce',
      package: 'prepay_id=wx123',
      signType: 'RSA',
      paySign: 'signed',
    },
  }
}

function oauthOrderFixture() {
  return {
    order_id: 456,
    amount: 128,
    pay_amount: 128,
    fee_rate: 0,
    expires_at: '2099-01-01T00:10:00.000Z',
    payment_type: 'wxpay',
    result_type: 'oauth_required' as const,
    oauth: {
      authorize_url: '/api/v1/auth/oauth/wechat/payment/start?payment_type=wxpay&redirect=%2Fpurchase%3Ffrom%3Dwechat',
      appid: 'wx123',
      scope: 'snsapi_base',
      redirect_url: '/auth/wechat/payment/callback',
    },
  }
}

async function _mountSubscriptionConfirm(options: Parameters<typeof checkoutInfoWithPlansFixture>[0] = {}) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = {
    tab: 'subscription',
    group: '3',
  }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoWithPlansFixture(options))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

async function _mountSubscriptionPlanList(planCount: number) {
  vi.useRealTimers()
  routeState.path = '/purchase'
  routeState.query = { tab: 'subscription' }
  routerReplace.mockReset().mockResolvedValue(undefined)
  routerPush.mockReset().mockResolvedValue(undefined)
  routerResolve.mockClear()
  createOrder.mockReset()
  refreshUser.mockReset()
  fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
  showError.mockReset()
  showInfo.mockReset()
  showWarning.mockReset()
  const basePlan = checkoutInfoWithPlansFixture().data.plans[0]
  const plans = Array.from({ length: planCount }, (_, index) => ({
    ...basePlan,
    id: index + 1,
    name: `Plan ${index + 1}`,
  }))
  getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ plans }))
  bridgeInvoke.mockReset()
  window.localStorage.clear()
  ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

  const wrapper = shallowMount(PaymentView, {
    global: {
      stubs: {
        AppLayout: {
          template: '<div><slot /></div>',
        },
        Teleport: true,
        Transition: false,
      },
    },
  })
  await flushPromises()
  await flushPromises()
  return wrapper
}

describe('PaymentView help text', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    createOrder.mockReset()
    window.localStorage.clear()
  })

  async function mountHelp(help_text: string, help_image_url = '') {
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({ help_text, help_image_url }))
    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    return wrapper
  }

  it('renders headings, emphasis, links, and lists in payment help without starting checkout', async () => {
    const wrapper = await mountHelp('## Recharge help\n\n**Read first**\n\n- [Contact support](https://example.com/help)')
    const help = wrapper.get('.markdown-body')
    expect(help.get('h2').text()).toBe('Recharge help')
    expect(help.get('strong').text()).toBe('Read first')
    expect(help.get('li a').attributes('href')).toBe('https://example.com/help')
    expect(createOrder).not.toHaveBeenCalled()
  })

  it('removes scripts, event handlers, and unsafe URLs from rendered help', async () => {
    const wrapper = await mountHelp([
      '<script>alert(1)</script>',
      '<img src="https://example.com/help.png" onerror="alert(1)">',
      '[Unsafe](javascript:alert%281%29)',
      '[Support](https://example.com/help)',
    ].join('\n\n'))
    const help = wrapper.get('.markdown-body')
    expect(help.find('script').exists()).toBe(false)
    expect(help.get('img').attributes('onerror')).toBeUndefined()
    expect(help.findAll('a').map(link => link.attributes('href'))).toEqual([undefined, 'https://example.com/help'])
  })

  it('keeps plain-text soft line breaks and the separate help image preview', async () => {
    const wrapper = await mountHelp('First line\nSecond line', 'https://example.com/help.png')
    const help = wrapper.get('.markdown-body')
    expect(help.get('p').text()).toBe('First line\nSecond line')
    expect(help.find('br').exists()).toBe(false)
    await wrapper.get('img').trigger('click')
    expect(wrapper.findAll('img')).toHaveLength(2)
    expect(wrapper.findAll('img')[1].attributes('src')).toBe('https://example.com/help.png')
  })

  it('keeps image-only help without an empty Markdown container', async () => {
    const wrapper = await mountHelp('', 'https://example.com/help.png')
    expect(wrapper.find('.markdown-body').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/help.png')
  })
})

describe('PaymentView CNY wallet credit', () => {
  it('converts USD payment amounts to CNY without applying the retired recharge multiplier', async () => {
    translate.mockClear()
    routeState.path = '/purchase'
    routeState.query = {}
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture({
      balance_recharge_multiplier: 0.5,
      methods: {
        stripe: {
          ...checkoutInfoFixture().data.methods.wxpay,
          currency: 'USD',
        },
      },
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    wrapper.getComponent(AmountInput).vm.$emit('update:modelValue', 10)
    await flushPromises()

    expect(wrapper.text()).toContain('¥67.00')
    expect(translate).not.toHaveBeenCalledWith('payment.rechargeRatePreview', expect.anything())
  })
})

describe('PaymentView legacy subscription entry redirect', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined
    appStoreState.setPublicSettings(undefined)
  })

  afterEach(() => {
    appStoreState.setPublicSettings(undefined)
  })

  async function mountPurchase(overrides: Partial<CheckoutInfoResponse> = {}) {
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture(overrides))
    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()
    return wrapper
  }

  it('redirects ?tab=subscription to /pricing — the only plan catalog', async () => {
    routeState.query = { tab: 'subscription' }
    await mountPurchase()
    expect(routerReplace).toHaveBeenCalledWith('/pricing')
  })

  it('redirects subscription-only sites to /pricing on plain visits', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: true, payment_balance_disabled: true })
    await mountPurchase()
    expect(routerReplace).toHaveBeenCalledWith('/pricing')
  })

  it('does not redirect subscription-only sites when a wechat resume is in flight', async () => {
    appStoreState.setPublicSettings({ subscription_enabled: true, payment_balance_disabled: true })
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-sub-only',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
    }
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue(oauthOrderFixture())

    const originalLocation = window.location
    const locationState = { href: 'http://localhost/purchase', origin: 'http://localhost' }
    Object.defineProperty(window, 'location', { configurable: true, value: locationState })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    // 恢复链先剥离查询参数（replace 到干净路径），随后不得把带恢复态的会话甩到 /pricing
    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(routerReplace).not.toHaveBeenCalledWith('/pricing')
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      order_type: 'subscription',
      plan_id: 7,
      wechat_resume_token: 'resume-sub-only',
    }))

    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation })
  })

  it('shows the billing-unavailable notice when balance recharge is disabled', async () => {
    const wrapper = await mountPurchase({ balance_disabled: true })
    expect(wrapper.text()).toContain('payment.billingUnavailable')
    expect(wrapper.text()).not.toContain('payment.rechargeAccount')
    expect(createOrder).not.toHaveBeenCalled()
  })
})

describe('PaymentView payment recovery', () => {
  beforeEach(() => {
    vi.useRealTimers()
    routeState.path = '/purchase'
    routeState.query = {}
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined
  })

  it('restores a custom EasyPay method as the selected payment method', async () => {
    getCheckoutInfo.mockResolvedValue(checkoutInfoFixture({
      methods: {
        wxpay: checkoutInfoFixture().data.methods.wxpay,
        ldc: {
          daily_limit: 0,
          daily_used: 0,
          daily_remaining: 0,
          single_min: 0,
          single_max: 0,
          fee_rate: 0,
          available: true,
          display_name: 'LDC Pay',
        },
      },
    }))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 888,
      amount: 66,
      qrCode: 'ldc-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'ldc',
      payUrl: 'https://pay.example.com/ldc',
      outTradeNo: 'sub2_ldc_888',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.now(),
    }))

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          AppLayout: {
            template: '<div><slot /></div>',
          },
          PaymentStatusPanel: {
            template: '<button data-test="payment-done" @click="$emit(\'done\')" />',
          },
          PaymentMethodSelector: {
            props: ['selected'],
            template: '<div data-test="method-selector">{{ selected }}</div>',
          },
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()
    await wrapper.find('[data-test="payment-done"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="method-selector"]').text()).toBe('ldc')
  })
})

describe('PaymentView WeChat JSAPI flow', () => {
  beforeEach(() => {
    routeState.path = '/purchase'
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-123',
    }
    routerReplace.mockReset().mockResolvedValue(undefined)
    routerPush.mockReset().mockResolvedValue(undefined)
    routerResolve.mockClear()
    createOrder.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset().mockResolvedValue(undefined)
    showError.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()
    getCheckoutInfo.mockReset().mockResolvedValue(checkoutInfoFixture())
    bridgeInvoke.mockReset()
    window.localStorage.clear()
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = {
      invoke: bridgeInvoke,
    }
  })

  it('resets payment state and redirects to /payment/result after JSAPI reports success', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-123'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:ok' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(routerPush).toHaveBeenCalledWith({
      path: '/payment/result',
      query: {
        order_id: '123',
        out_trade_no: 'sub2_jsapi_123',
        resume_token: 'resume-token-123',
      },
    })
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('resets payment state when JSAPI reports cancellation', async () => {
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-cancel'))
    bridgeInvoke.mockImplementation((_action, _payload, callback) => {
      callback({ err_msg: 'get_brand_wcpay_request:cancel' })
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(showInfo).toHaveBeenCalledWith('payment.qr.cancelled')
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('clears stale recovery state when JSAPI never becomes available', async () => {
    vi.useFakeTimers()
    createOrder.mockResolvedValue(jsapiOrderFixture('resume-token-missing-bridge'))
    ;(window as Window & { WeixinJSBridge?: { invoke: typeof bridgeInvoke } }).WeixinJSBridge = undefined

    const wrapper = shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })

    await flushPromises()
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      'payment.errors.wechatJsapiUnavailable payment.errors.wechatOpenInWeChatHint',
    )
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
    expect(wrapper.html()).not.toContain('payment-status-panel-stub')
  })

  it('clears a stale recovery snapshot before handling wechat resume callback params', async () => {
    createOrder.mockRejectedValueOnce(new Error('resume failed'))
    window.localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId: 999,
      amount: 66,
      qrCode: 'stale-qr',
      expiresAt: '2099-01-01T00:10:00.000Z',
      paymentType: 'alipay',
      payUrl: 'https://pay.example.com/stale',
      outTradeNo: 'stale-out-trade-no',
      clientSecret: '',
      intentId: '',
      currency: '',
      countryCode: '',
      paymentEnv: '',
      payAmount: 66,
      orderType: 'balance',
      paymentMode: 'popup',
      resumeToken: '',
      createdAt: Date.UTC(2099, 0, 1, 0, 0, 0),
    }))

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      wechat_resume_token: 'resume-token-123',
    }))
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toBeNull()
  })

  it('keeps subscription resume context for token-only WeChat callbacks', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-subscription-7',
      payment_type: 'wxpay_direct',
      order_type: 'subscription',
      plan_id: '7',
    }
    getCheckoutInfo.mockResolvedValue(checkoutInfoWithPlansFixture())
    createOrder.mockResolvedValue(oauthOrderFixture())

    const originalLocation = window.location
    const locationState = {
      href: 'http://localhost/purchase',
      origin: 'http://localhost',
    }
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: locationState,
    })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(routerReplace).toHaveBeenCalledWith({ path: '/purchase', query: {} })
    expect(createOrder).toHaveBeenCalledWith(expect.objectContaining({
      payment_type: 'wxpay',
      order_type: 'subscription',
      plan_id: 7,
      wechat_resume_token: 'resume-subscription-7',
    }))
    expect(locationState.href).toContain('/api/v1/auth/oauth/wechat/payment/start?')
    expect(new URL(locationState.href, 'http://localhost').searchParams.get('redirect')).toBe(
      '/purchase?from=wechat&payment_type=wxpay&order_type=subscription&plan_id=7',
    )

    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
  })

  it('falls back to QR flow when mobile WeChat payment is unavailable', async () => {
    routeState.query = {
      wechat_resume: '1',
      wechat_resume_token: 'resume-token-h5',
      payment_type: 'wxpay_direct',
    }
    createOrder
      .mockRejectedValueOnce({ reason: 'WECHAT_H5_NOT_AUTHORIZED' })
      .mockResolvedValueOnce({
        order_id: 778,
        amount: 88,
        pay_amount: 88,
        fee_rate: 0,
        expires_at: '2099-01-01T00:10:00.000Z',
        payment_type: 'wxpay',
        qr_code: 'weixin://wxpay/bizpayurl?pr=fallback-native',
        out_trade_no: 'sub2_qr_778',
      })

    shallowMount(PaymentView, {
      global: {
        stubs: {
          Teleport: true,
          Transition: false,
        },
      },
    })
    await flushPromises()
    await flushPromises()

    expect(createOrder).toHaveBeenNthCalledWith(1, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: true,
      wechat_resume_token: 'resume-token-h5',
    }))
    expect(createOrder).toHaveBeenNthCalledWith(2, expect.objectContaining({
      payment_type: 'wxpay',
      is_mobile: false,
      payment_source: 'hosted_redirect',
    }))
    expect(showWarning).toHaveBeenCalledWith('payment.errors.mobilePaymentFallbackToQr')
    expect(showError).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY)).toContain('weixin://wxpay/bizpayurl?pr=fallback-native')
  })
})
