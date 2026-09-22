import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import User360Drawer from '../User360Drawer.vue'

const mocks = vi.hoisted(() => ({ subs: vi.fn(), keys: vi.fn(), cards: vi.fn(), changes: vi.fn(), count: vi.fn(), ledger: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  subscriptions: { list: mocks.subs }, users: { getUserApiKeys: mocks.keys },
  resetCards: { list: mocks.cards, count: mocks.count }, planChanges: { list: mocks.changes }
} }))
vi.mock('@/api/admin/subscriptionReset', () => ({ getAdminWalletLedger: mocks.ledger }))
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r }); return { promise, resolve } }

describe('User360 data ownership', () => {
  it('keeps cards and wallet responses correctly assigned and discards a previous user response', async () => {
    const old = deferred<{ items: unknown[] }>()
    mocks.subs.mockImplementation((_page, _size, params) => params.user_id === 1 ? old.promise : Promise.resolve({ items: [{ id: 22, group_id: 2, group: { name: 'New user plan' } }] }))
    mocks.keys.mockResolvedValue({ items: [] })
    mocks.cards.mockResolvedValue({ data: { items: [{ id: 909, status: 'available' }] } })
    mocks.changes.mockResolvedValue({ data: { items: [] } })
    mocks.count.mockResolvedValue({ data: { available: 1 } })
    mocks.ledger.mockResolvedValue({ data: { entries: [{ id: 'reward:1', type: 'reward', amount: 7, created_at: '2026-09-01' }] } })
    const wrapper = mount(User360Drawer, { props: { user: { id: 1, email: 'first@example.test' } }, global: { stubs: { Teleport: true, Transition: false, Icon: true } } })
    await wrapper.setProps({ user: { id: 2, email: 'second@example.test' } })
    await flushPromises()
    expect(wrapper.text()).toContain('New user plan')
    expect(wrapper.text()).toContain('#909')
    expect(wrapper.text()).toContain('$7.00')
    old.resolve({ items: [{ id: 11, group: { name: 'Old user plan' } }] })
    await flushPromises()
    expect(wrapper.text()).not.toContain('Old user plan')
    expect(wrapper.text()).toContain('New user plan')
    await wrapper.setProps({ user: null })
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
