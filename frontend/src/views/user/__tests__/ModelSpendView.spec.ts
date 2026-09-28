import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelSpendView from '../ModelSpendView.vue'

/**
 * 模型消费页契约测试：
 * - 金额只来自真实 Billing 口径（total_actual_cost / actual_cost），不经展示价重算；
 * - 排名按 cost DESC、占比正确；
 * - 钱包余额与套餐剩余分开展示（不合并、不参与排名）；
 * - 不渲染 token / endpoint / group / channel 等技术内部信息。
 */

const { getStatsByDateRange, getDashboardModels, getDashboardTrend, getAccountStatus, formatUSD } = vi.hoisted(() => ({
  getStatsByDateRange: vi.fn(),
  getDashboardModels: vi.fn(),
  getDashboardTrend: vi.fn(),
  getAccountStatus: vi.fn(),
  formatUSD: vi.fn((amount: number) => `¥${amount.toFixed(2)}`),
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getStatsByDateRange,
    getDashboardModels,
    getDashboardTrend,
  },
}))

vi.mock('@/api/subscriptions', () => ({
  getAccountStatus,
}))

vi.mock('@/stores/currencyDisplay', () => ({
  useCurrencyDisplayStore: () => ({
    formatUSD,
    formatCNY: (amount: number) => `¥${amount.toFixed(2)}`,
  }),
}))

vi.mock('@/utils/format', () => ({
  getBrowserTimeZone: () => 'Asia/Shanghai',
}))

const messages: Record<string, string> = {
  'modelSpend.title': '模型消费',
  'modelSpend.subtitle': '查看你的模型使用消费。',
  'modelSpend.rangeLabel': '时间范围',
  'modelSpend.rangePeriod': '当前周期',
  'modelSpend.rangeMonth': '本月',
  'modelSpend.range30d': '近30天',
  'modelSpend.rangeAll': '全部',
  'modelSpend.totalLabel': '本期累计模型消费',
  'modelSpend.totalNote': '按真实结算金额统计（订阅与按量消费合并计入）',
  'modelSpend.walletLabel': '钱包余额',
  'modelSpend.walletHint': '独立 API 按量调用的资金账户',
  'modelSpend.subLabel': '套餐剩余额度',
  'modelSpend.subRemaining': '剩余 {percent}%',
  'modelSpend.subUnmetered': '不限量',
  'modelSpend.byModelTitle': '按模型',
  'modelSpend.byModelDesc': '各模型的实际消费金额与占比。',
  'modelSpend.emptyModels': '当前时间范围内暂无模型消费',
  'modelSpend.dailyTitle': '每日消费',
  'modelSpend.dailyDesc': '按天统计的实际消费金额。',
  'modelSpend.emptyDaily': '当前时间范围内暂无消费记录',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        let text = messages[key] ?? key
        if (params) {
          for (const [name, value] of Object.entries(params)) {
            text = text.replaceAll(`{${name}}`, String(value))
          }
        }
        return text
      },
    }),
  }
})

const simpleStub = { template: '<div><slot /></div>' }

function mountSpendView() {
  return mount(ModelSpendView, {
    global: {
      stubs: {
        AppLayout: simpleStub,
        RouterLink: simpleStub,
        MucGlassCard: simpleStub,
        MucSectionHeader: simpleStub,
        MucState: simpleStub,
        MucSkeleton: simpleStub,
      },
    },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  // 无订阅：默认近30天；钱包有余额
  getAccountStatus.mockResolvedValue({
    wallet: { balance: '670.00', canonical_currency: 'CNY' },
    subscriptions: [],
  })
  getStatsByDateRange.mockResolvedValue({ total_actual_cost: 17, total_cost: 50 })
  getDashboardModels.mockResolvedValue({
    models: [
      { model: 'claude-sonnet', requests: 3, input_tokens: 1, output_tokens: 1, total_tokens: 2, cost: 9, actual_cost: 5 },
      { model: 'gpt-6-astra', requests: 4, input_tokens: 1, output_tokens: 1, total_tokens: 2, cost: 40, actual_cost: 12 },
    ],
  })
  getDashboardTrend.mockResolvedValue({
    trend: [
      { date: '2026-09-28', requests: 1, input_tokens: 0, output_tokens: 0, total_tokens: 0, cost: 1, actual_cost: 2 },
      { date: '2026-09-27', requests: 1, input_tokens: 0, output_tokens: 0, total_tokens: 0, cost: 1, actual_cost: 5 },
    ],
  })
})

describe('ModelSpendView aggregation', () => {
  it('defaults to the last-30-days range and totals spend from actual billing (total_actual_cost)', async () => {
    const wrapper = mountSpendView()
    await flushPromises()

    expect(getStatsByDateRange).toHaveBeenCalledTimes(1)
    const [startDate] = getStatsByDateRange.mock.calls[0]
    expect(startDate).toMatch(/^\d{4}-\d{2}-\d{2}$/)

    const total = wrapper.find('[data-testid="spend-total"]')
    expect(total.text()).toContain('¥17.00')
    // 标准价（total_cost=50）不得出现在消费页
    expect(total.text()).not.toContain('¥50.00')
  })

  it('ranks models by actual cost descending with correct shares', async () => {
    const wrapper = mountSpendView()
    await flushPromises()

    const rows = wrapper.get('[data-testid="spend-models"]').findAll('.muc-spend__model-row')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('gpt-6-astra')
    expect(rows[0].text()).toContain('¥12.00')
    expect(rows[0].text()).toContain('71%')
    expect(rows[1].text()).toContain('claude-sonnet')
    expect(rows[1].text()).toContain('¥5.00')
    expect(rows[1].text()).toContain('29%')
  })

  it('shows wallet balance and subscription remaining separately, never merged', async () => {
    getAccountStatus.mockResolvedValue({
      wallet: { balance: '670.00', canonical_currency: 'CNY' },
      subscriptions: [
        {
          id: 1,
          group_id: 2,
          display_name: '校园套餐 A',
          usage_status: 'normal',
          expires_at: '2026-10-29T00:00:00Z',
          weekly_period_started_at: '2026-09-23T00:00:00Z',
          weekly_period_ends_at: '2026-09-30T00:00:00Z',
          weekly_window: { remaining_percent: 62, starts_at: null, resets_at: null, exhausted: false },
        },
      ],
    })
    const wrapper = mountSpendView()
    await flushPromises()

    const wallet = wrapper.get('[data-testid="spend-wallet"]')
    expect(wallet.text()).toContain('钱包余额')
    expect(wallet.text()).toContain('¥670.00')

    const subscription = wrapper.get('[data-testid="spend-subscription"]')
    expect(subscription.text()).toContain('套餐剩余额度')
    expect(subscription.text()).toContain('剩余 62%')

    // 两个权益来源不得相加展示
    expect(wrapper.text()).not.toContain('¥732')
  })

  it('prefers the current subscription cycle as the default range and queries its bounds', async () => {
    getAccountStatus.mockResolvedValue({
      wallet: { balance: '1.00', canonical_currency: 'CNY' },
      subscriptions: [
        {
          id: 1,
          group_id: 2,
          display_name: '校园套餐 A',
          usage_status: 'normal',
          expires_at: '2026-10-29T00:00:00Z',
          weekly_period_started_at: '2026-09-23T00:00:00Z',
          weekly_period_ends_at: '2026-09-30T00:00:00Z',
          weekly_window: { remaining_percent: 40, starts_at: null, resets_at: null, exhausted: false },
        },
      ],
    })
    const wrapper = mountSpendView()
    await flushPromises()

    expect(wrapper.find('[data-testid="spend-range-period"]').exists()).toBe(true)
    const calls = getStatsByDateRange.mock.calls
    expect(calls).toHaveLength(1)
    // 周期起始日来自订阅 weekly_period_started_at（本地日期），结束为今天
    expect(String(calls[0][0])).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('hides the daily trend section for the all-time range', async () => {
    const wrapper = mountSpendView()
    await flushPromises()
    vi.mocked(getDashboardTrend).mockClear()

    await wrapper.get('[data-testid="spend-range-all"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="spend-daily"]').exists()).toBe(false)
    expect(getDashboardTrend).not.toHaveBeenCalled()
  })

  it('never renders technical internals (tokens, endpoints, groups, channels)', async () => {
    const wrapper = mountSpendView()
    await flushPromises()

    const text = wrapper.text()
    expect(text).not.toContain('input_tokens')
    expect(text).not.toContain('endpoint')
    expect(text).not.toContain('group_id')
    expect(text).not.toContain('channel')
    expect(text).not.toContain('cache')
  })
})
