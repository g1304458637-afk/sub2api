import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import QuotaRemaining from '../QuotaRemaining.vue'
import type { AccountSubscriptionStatus } from '@/api/subscriptions'

const subscription: AccountSubscriptionStatus = {
  id: 1, group_id: 1, display_name: 'Pro', quota_policy: 'dual_window_v1',
  short_window: { remaining_percent: 0.4, starts_at: '2026-09-22T00:00:00Z', resets_at: '2026-09-22T05:00:00Z', exhausted: false },
  weekly_window: { remaining_percent: 60, starts_at: '2026-09-22T00:00:00Z', resets_at: null, exhausted: false },
  weekly_usage_percent: 40, usage_status: 'normal', weekly_period_started_at: null, weekly_period_ends_at: null,
  expires_at: '2026-09-25T00:00:00Z', payg_fallback: false,
}
describe('remaining quota windows', () => {
  it('shows both remaining percentages, precise low quota and expiry without money', () => {
    const view = mount(QuotaRemaining, { props: { subscription } })
    expect(view.text()).toContain('5 小时剩余')
    expect(view.text()).toContain('<1%')
    expect(view.text()).toContain('60%')
    expect(view.text()).toContain('到期前不再恢复')
    expect(view.text()).not.toContain('$')
    expect(view.text()).not.toContain('40%')
  })
  it('does not present missing window data as unlimited or full quota', () => {
    const view = mount(QuotaRemaining, { props: { subscription: { ...subscription, short_window: undefined } } })
    expect(view.text()).toContain('状态暂不可用')
    expect(view.text()).not.toContain('100%')
    expect(view.text()).not.toContain('不限')
  })
})
