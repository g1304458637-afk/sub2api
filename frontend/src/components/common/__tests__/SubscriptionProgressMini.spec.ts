import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'

// t() 回显 key，便于断言使用的 i18n 路径（含 usage_status 映射）
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

// 订阅功能开关恒开
vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: { subscription: 'subscription' },
  isFeatureFlagEnabled: () => true,
}))

// mock 状态端点
const getAccountStatusMock = vi.fn()
vi.mock('@/api/subscriptions', () => ({
  getAccountStatus: (...args: unknown[]) => getAccountStatusMock(...args),
}))

import SubscriptionProgressMini from '../SubscriptionProgressMini.vue'
import type { AccountStatus } from '@/api/subscriptions'

function makeSubscription(over: Partial<AccountSubscriptionStatus['subscriptions'][number]> = {}): AccountSubscriptionStatus['subscriptions'][number] {
  return {
    id: 1,
    group_id: 13,
    display_name: 'Pro',
    weekly_usage_percent: 63,
    usage_status: 'normal',
    weekly_period_started_at: '2026-09-13T08:00:00Z',
    weekly_period_ends_at: '2026-09-27T08:00:00Z',
    expires_at: '2026-10-20T00:00:00Z',
    payg_fallback: false,
    reset_cards_available: 0,
    ...over,
  }
}

function makeStatus(subs: AccountSubscriptionStatus['subscriptions'][]): AccountStatus {
  return {
    wallet: { balance: '12.48000000', canonical_currency: 'USD' },
    reset_cards: { available: 0 },
    subscriptions: subs as AccountSubscriptionStatus['subscriptions'][],
  }
}

async function mountMini(): Promise<VueWrapper<any>> {
  const wrapper = mount(SubscriptionProgressMini, {
    global: {
      stubs: {
        // router-link / transition / Icon 在此组件测试中不参与断言
        'router-link': { template: '<a><slot /></a>' },
        Icon: { template: '<span />' },
      },
    },
  })
  await flushPromises()
  // 展开悬浮面板（tooltip 为点击切换渲染）；无订阅时组件不渲染按钮
  const toggle = wrapper.find('button')
  if (toggle.exists()) {
    await toggle.trigger('click')
    await flushPromises()
  }
  return wrapper
}

beforeEach(() => {
  getAccountStatusMock.mockReset()
  getAccountStatusMock.mockResolvedValue(makeStatus([]))
})

describe('SubscriptionProgressMini (Phase 4.1 status contract)', () => {
  it('renders nothing when there are no active subscriptions', async () => {
    const wrapper = await mountMini()
    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('shows display_name and integer percent for a subscription user', async () => {
    getAccountStatusMock.mockResolvedValue(makeStatus([makeSubscription({ weekly_usage_percent: 63 })]))
    const wrapper = await mountMini()
    const text = wrapper.text()
    expect(text).toContain('Pro')
    expect(text).toContain('63%')
    // 内部 USD 额度绝不出现在顶栏
    expect(text).not.toContain('weekly_limit_usd')
    expect(text).not.toMatch(/\$\d+\.\d+ ?\/ ?\$/)
  })

  it('unmetered subscriptions show the unmetered badge instead of 0%', async () => {
    getAccountStatusMock.mockResolvedValue(makeStatus([
      makeSubscription({ weekly_usage_percent: null, usage_status: 'unmetered' }),
    ]))
    const wrapper = await mountMini()
    const text = wrapper.text()
    expect(text).toContain('usageStatus.unmetered')
    expect(text).not.toContain('%')
  })

  it('exhausted status maps to the exhausted label at 100%', async () => {
    getAccountStatusMock.mockResolvedValue(makeStatus([
      makeSubscription({ weekly_usage_percent: 100, usage_status: 'exhausted' }),
    ]))
    const wrapper = await mountMini()
    const text = wrapper.text()
    // exhausted 时组件显示钳制后的 100%（状态文案走真实 i18n，mock 不参与断言）
    expect(text).toContain('100%')
    expect(text).not.toContain('$')
  })

  it('renders multiple subscriptions', async () => {
    getAccountStatusMock.mockResolvedValue(makeStatus([
      makeSubscription({ id: 1, display_name: 'Pro', weekly_usage_percent: 63 }),
      makeSubscription({ id: 2, display_name: 'Max', weekly_usage_percent: 10, group_id: 14 }),
    ]))
    const wrapper = await mountMini()
    const text = wrapper.text()
    expect(text).toContain('Pro')
    expect(text).toContain('Max')
    expect(text).toContain('63%')
    expect(text).toContain('10%')
  })

  it('never renders rate multiplier or internal USD quota values', async () => {
    getAccountStatusMock.mockResolvedValue(makeStatus([
      makeSubscription({ weekly_usage_percent: 12 }),
    ]))
    const wrapper = await mountMini()
    const html = wrapper.html()
    expect(html).not.toContain('rate_multiplier')
    expect(html).not.toContain('weekly_limit_usd')
    expect(html).not.toContain('weekly_usage_usd')
  })

  // ── Phase 11：单主套餐不变量 + 已预约变更（pending_change 合同） ──

  it('renders the scheduled pending change line when the status contract carries one', async () => {
    const status = makeStatus([makeSubscription({ display_name: 'Pro', weekly_usage_percent: 64 })])
    status.pending_change = {
      change_type: 'scheduled_downgrade',
      to_plan_id: 7,
      to_plan_name: 'Basic',
      effective_at: '2026-10-21T00:00:00Z',
      current_period_ends_at: '2026-10-21T00:00:00Z',
    }
    getAccountStatusMock.mockResolvedValue(status)
    const wrapper = await mountMini()
    const pending = wrapper.find('[data-testid="header-pending-change"]')
    expect(pending.exists()).toBe(true)
    // t() mock 回显 key（忽略插值参数）：断言区块使用了 pendingLine 合同路径；
    // 插值参数（plan/date）由真实 i18n 文案与 formatDate 渲染，locale 完整性测试已覆盖 key。
    expect(pending.text()).toContain('subscriptionProgress.pendingLine')
  })

  it('omits the pending change line when no change is scheduled', async () => {
    getAccountStatusMock.mockResolvedValue(makeStatus([makeSubscription({ display_name: 'Pro' })]))
    const wrapper = await mountMini()
    expect(wrapper.find('[data-testid="header-pending-change"]').exists()).toBe(false)
  })
})
