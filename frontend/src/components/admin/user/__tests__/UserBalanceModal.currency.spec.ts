import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import UserBalanceModal from '../UserBalanceModal.vue'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'

const mocks = vi.hoisted(() => ({
  updateBalance: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({ adminAPI: { users: { updateBalance: mocks.updateBalance } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

beforeEach(() => {
  setActivePinia(createPinia())
  useCurrencyDisplayStore().setCurrency('CNY')
  vi.clearAllMocks()
  mocks.updateBalance.mockResolvedValue(undefined)
})

afterEach(() => localStorage.removeItem('sub2api_display_currency'))

describe('UserBalanceModal currency contract', () => {
  it('converts a USD display amount to canonical CNY before updating the wallet', async () => {
    const currencyStore = useCurrencyDisplayStore()
    currencyStore.setCurrency('USD')
    const wrapper = mount(UserBalanceModal, {
      props: {
        show: true,
        user: { id: 8, email: 'student@example.edu', balance: 100 } as never,
        operation: 'add',
      },
      global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } },
    })

    await wrapper.find('input[type="number"]').setValue('10')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(mocks.updateBalance).toHaveBeenCalledWith(8, 67, 'add', '')
  })
})
