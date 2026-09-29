import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import SubscriptionsView from '../SubscriptionsView.vue'

const { list, bulkAction, grantBulk, grantPreview, listUsers, showError } = vi.hoisted(() => ({
  list: vi.fn(), bulkAction: vi.fn(), grantBulk: vi.fn(), grantPreview: vi.fn(), listUsers: vi.fn(), showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { list, bulkAction },
    subscriptionGrants: { bulk: grantBulk, preview: grantPreview, create: vi.fn(), list: vi.fn(), revoke: vi.fn() },
    groups: { getAll: vi.fn().mockResolvedValue([]) },
    users: { list: listUsers }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const rows = [
  { id: 1, user_id: 11, group_id: 1, status: 'active', user: { email: 'active@example.com' } },
  { id: 2, user_id: 22, group_id: 1, status: 'expired', user: { email: 'expired@example.com' } },
  { id: 3, user_id: 33, group_id: 1, status: 'revoked', user: { email: 'revoked@example.com' } }
]

function mountView() {
  return mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: { name: 'DataTable', props: ['data', 'selectedKeys'], emits: ['update:selectedKeys', 'sort'], template: '<div />' },
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        Pagination: true, Select: true, ConfirmDialog: true, Icon: true,
        GroupBadge: true, GroupOptionItem: true, EmptyState: true, Teleport: true, RouterLink: true
      }
    }
  })
}

let wrapper: ReturnType<typeof mountView>

beforeEach(async () => {
  vi.clearAllMocks()
  setActivePinia(createPinia())
  localStorage.clear()
  sessionStorage.clear()
  localStorage.setItem('auth_user', JSON.stringify({ id: 777 }))
  list.mockResolvedValue({ items: rows, total: 60, pages: 3 })
  wrapper = mountView()
  await flushPromises()
})
afterEach(() => {
  wrapper.unmount()
  vi.useRealTimers()
})

async function select(ids: number[]) {
  wrapper.getComponent({ name: 'DataTable' }).vm.$emit('update:selectedKeys', ids)
  await flushPromises()
}

describe('subscription bulk operations', () => {
  it('uses eligible selected rows and retains failures and untouched selections after a partial result', async () => {
    bulkAction.mockResolvedValue({ success_count: 1, failed_count: 1, results: [
      { subscription_id: 1, success: true },
      { subscription_id: 2, success: false, error: 'Cannot adjust this subscription' }
    ] })
    await select([1, 2, 3])
    await wrapper.get('[data-test="bulk-extend"]').trigger('click')
    const form = wrapper.get('#bulk-subscription-action-form')
    expect(form.text()).toContain('active@example.com')
    expect(form.text()).toContain('expired@example.com')
    expect(form.text()).not.toContain('revoked@example.com')
    await form.trigger('submit')
    await flushPromises()

    expect(bulkAction).toHaveBeenCalledWith({ subscription_ids: [1, 2], action: 'extend', days: 30 }, expect.any(String))
    expect(wrapper.getComponent({ name: 'DataTable' }).props('selectedKeys')).toEqual([2, 3])
    expect(form.text()).toContain('Cannot adjust this subscription')
    expect(list).toHaveBeenCalledTimes(2)
  })

  it('disables operations that do not apply to the selected status', async () => {
    await select([3])
    for (const action of ['extend', 'reset_quota', 'revoke']) {
      expect(wrapper.get(`[data-test="bulk-${action}"]`).attributes('disabled')).toBeDefined()
    }
    expect(wrapper.get('[data-test="bulk-restore"]').attributes('disabled')).toBeUndefined()
  })

  it('clears selection on pagination, sorting, and filters and rejects IDs outside the visible page', async () => {
    await select([1, 999])
    expect(wrapper.getComponent({ name: 'DataTable' }).props('selectedKeys')).toEqual([1])
    wrapper.getComponent({ name: 'Pagination' }).vm.$emit('update:page', 2)
    await flushPromises()
    expect(wrapper.getComponent({ name: 'DataTable' }).props('selectedKeys')).toEqual([])
    await select([2])
    wrapper.getComponent({ name: 'DataTable' }).vm.$emit('sort', 'status', 'asc')
    await flushPromises()
    expect(wrapper.getComponent({ name: 'DataTable' }).props('selectedKeys')).toEqual([])
    await select([3])
    wrapper.findAllComponents({ name: 'Select' })[0]!.vm.$emit('change')
    await flushPromises()
    expect(wrapper.getComponent({ name: 'DataTable' }).props('selectedKeys')).toEqual([])
  })

  it('grants multiple users once and retries only failed users', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    await wrapper.findAll('button').find(button => button.text() === 'admin.subscriptions.grant.action')!.trigger('click')
    const form = wrapper.get('#assign-subscription-form')
    await form.get('input[type="checkbox"]').setValue(true)
    form.getComponent({ name: 'Select' }).vm.$emit('update:modelValue', 7)
    const search = form.get('[data-assign-user-search] input')
    for (const id of [11, 22]) {
      listUsers.mockResolvedValue({ items: [{ id, email: `user${id}@example.com` }] })
      await search.trigger('focus')
      await search.setValue(`user${id}`)
      await vi.advanceTimersByTimeAsync(300)
      await flushPromises()
      await form.get('[data-assign-user-search] button').trigger('click')
    }
    expect(form.get('[data-test="assign-users"]').text()).toContain('user11@example.com')
    expect(form.get('[data-test="assign-users"]').text()).toContain('user22@example.com')
    let resolveGrant!: (result: unknown) => void
    grantBulk.mockReturnValueOnce(new Promise(resolve => { resolveGrant = resolve }))
    await form.trigger('submit')
    await form.trigger('submit')
    expect(grantBulk).toHaveBeenCalledTimes(1)
    expect(grantBulk).toHaveBeenCalledWith(
      { user_ids: [11, 22], group_id: 7, duration_days: 30, effective_policy: 'immediate', source: 'admin_grant', reason: undefined, notes: undefined },
      expect.any(String)
    )
    resolveGrant({
      items: [
        { user_id: 11, success: true, action: 'activated_new' },
        { user_id: 22, success: false, error: 'User 22: conflict' }
      ],
      total: 2,
      success_count: 1,
      failed_count: 1
    })
    await flushPromises()
    expect(form.get('[data-test="assign-users"]').text()).not.toContain('user11@example.com')
    expect(form.get('[data-test="assign-users"]').text()).toContain('user22@example.com')
    expect(form.get('[data-test="batch-assign-result"]').text()).toContain('User 22: conflict')
    const firstKey = grantBulk.mock.calls[0]![1] as string
    grantBulk.mockResolvedValueOnce({
      items: [{ user_id: 22, success: true, action: 'activated_new' }],
      total: 1,
      success_count: 1,
      failed_count: 0
    })
    await form.trigger('submit')
    await flushPromises()
    expect(grantBulk).toHaveBeenLastCalledWith(
      { user_ids: [22], group_id: 7, duration_days: 30, effective_policy: 'immediate', source: 'admin_grant', reason: undefined, notes: undefined },
      expect.any(String)
    )
    // 同一弹窗会话内重试复用同一 Idempotency-Key，保证幂等
    expect(grantBulk.mock.calls[1]![1]).toBe(firstKey)
    expect(form.find('[data-test="assign-users"]').exists()).toBe(false)
  })
})
