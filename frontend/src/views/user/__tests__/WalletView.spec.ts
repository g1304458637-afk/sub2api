import { mount, flushPromises } from '@vue/test-utils'
import { describe, it, expect, vi } from 'vitest'
import WalletView from '../WalletView.vue'

const mocks = vi.hoisted(() => ({ ledger: vi.fn(), status: vi.fn(), error: vi.fn() }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: mocks.error }) }))
vi.mock('@/api/subscriptions', () => ({ getAccountStatus: mocks.status, getWalletLedger: mocks.ledger }))
vi.mock('@/api/payment', () => ({ paymentAPI: { getConfig: () => Promise.resolve({ data: {} }) } }))
const options = { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, MucGlassCard: { template: '<div><slot /></div>' }, MucButton: true, MucSkeleton: true, MucSectionHeader: true, Pagination: true, MucState: { props: ['message'], template: '<p>{{ message }}</p>' } } } }

describe('Wallet real data', () => {
  it('renders an empty wallet without a sample reward', async () => {
    mocks.status.mockResolvedValue({ wallet: { balance: '0.00000000', canonical_currency: 'USD' } })
    mocks.ledger.mockResolvedValue({ entries: [], total: 0 })
    const wrapper = mount(WalletView, options)
    await flushPromises()
    expect(wrapper.text()).toContain('wallet.historyEmpty')
    expect(wrapper.text()).not.toContain('5.00')
    expect(wrapper.text()).not.toContain('rewardSample')
    wrapper.unmount()
  })
  it('ends loading and exposes an error instead of a false empty history', async () => {
    mocks.status.mockRejectedValue(new Error('offline'))
    mocks.ledger.mockRejectedValue(new Error('offline'))
    const wrapper = mount(WalletView, options)
    await flushPromises()
    expect(wrapper.text()).toContain('wallet.loadError')
    expect(wrapper.text()).not.toContain('wallet.historyEmpty')
    expect(wrapper.find('muc-skeleton-stub').exists()).toBe(false)
    wrapper.unmount()
  })
})
