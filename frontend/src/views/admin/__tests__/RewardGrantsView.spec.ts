import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import RewardGrantsView from '../RewardGrantsView.vue'

const { listRewardGrants } = vi.hoisted(() => ({
  listRewardGrants: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { rewardGrants: { list: listRewardGrants } }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/composables/usePersistedPageSize', () => ({
  getPersistedPageSize: () => 20
}))

const mountView = () => shallowMount(RewardGrantsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: {
        props: ['data'],
        template: '<div data-test="rows">{{ data.map(row => row.id).join(",") }}</div>'
      },
      Pagination: {
        props: ['page'],
        emits: ['update:page'],
        template: '<button data-test="page" @click="$emit(\'update:page\', 2)">{{ page }}</button>'
      },
      Select: {
        props: ['modelValue', 'options'],
        emits: ['update:modelValue', 'change'],
        template: `<select data-test="source-type" :value="modelValue"
          @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value)">
          <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>`
      }
    }
  }
})

let wrapper: ReturnType<typeof mountView> | undefined

beforeEach(() => {
  vi.clearAllMocks()
  // total > 0 让 Pagination 挂载（视图里 v-if="total > 0"）
  listRewardGrants.mockResolvedValue({ items: [], total: 2, page: 1, page_size: 20, pages: 1 })
})

afterEach(() => wrapper?.unmount())

describe('reward grants view', () => {
  it('loads the first page on mount', async () => {
    wrapper = mountView()
    await flushPromises()

    expect(listRewardGrants).toHaveBeenCalledWith(
      expect.objectContaining({ page: 1, page_size: 20 })
    )
  })

  it('sends exact-match filters and restarts at page one when searching', async () => {
    wrapper = mountView()
    await flushPromises()

    const userId = wrapper.find('input[type="text"]')
    await userId.setValue('42')
    await userId.trigger('keyup.enter')
    await flushPromises()

    expect(listRewardGrants).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, user_id: 42 })
    )

    await wrapper.get('select[data-test="source-type"]').setValue('student_verification')
    await flushPromises()

    expect(listRewardGrants).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1, user_id: 42, source_type: 'student_verification' })
    )
  })

  it('ignores a non-numeric user id instead of sending it', async () => {
    wrapper = mountView()
    await flushPromises()

    const userId = wrapper.find('input[type="text"]')
    await userId.setValue('abc')
    await userId.trigger('keyup.enter')
    await flushPromises()

    const query = listRewardGrants.mock.lastCall?.[0]
    expect(query.user_id).toBeUndefined()
    expect(query.campaign).toBeUndefined()
  })

  it('keeps going to page two via pagination with the active filters', async () => {
    wrapper = mountView()
    await flushPromises()

    await wrapper.get('select[data-test="source-type"]').setValue('student_verification')
    await flushPromises()
    await wrapper.get('[data-test="page"]').trigger('click')
    await flushPromises()

    expect(listRewardGrants).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 2, source_type: 'student_verification' })
    )
  })

  it('shows rows from the response', async () => {
    listRewardGrants.mockResolvedValue({
      items: [{ id: 7 }, { id: 9 }],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="rows"]').text()).toBe('7,9')
  })
})
