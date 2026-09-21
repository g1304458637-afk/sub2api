import { flushPromises, mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserEducationEmailModal from '@/components/admin/user/UserEducationEmailModal.vue'

const { getEducationEmailStatus, revokeEducationEmail } = vi.hoisted(() => ({
  getEducationEmailStatus: vi.fn(),
  revokeEducationEmail: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { getEducationEmailStatus, revokeEducationEmail } },
}))

vi.mock('@/utils/format', () => ({
  formatDateTime: (value: string) => `formatted:${value}`,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const user = {
  id: 7,
  email: 'user@example.com',
  username: 'student',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: null,
  balance_notify_enabled: false,
  balance_notify_extra_emails: [],
  created_at: '2026-09-20T00:00:00Z',
  updated_at: '2026-09-20T00:00:00Z',
  notes: '',
} as any

// BaseDialog stub 同时渲染 footer slot，让 ConfirmDialog 的确认/取消按钮进入 DOM。
const stubs = {
  BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
  Icon: true,
}

describe('UserEducationEmailModal', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getEducationEmailStatus.mockReset()
    revokeEducationEmail.mockReset()
  })

  it('shows the administrator both feature state and verified campus email', async () => {
    getEducationEmailStatus.mockResolvedValue({
      user_id: 7,
      education_email_verification_enabled: false,
      education_email: {
        bound: true,
        display_name: 'student@muc.edu.cn',
        verified_at: '2026-09-20T01:02:03Z',
      },
    })
    const wrapper = mount(UserEducationEmailModal, {
      props: { show: true, user },
      global: { stubs },
    })

    await flushPromises()

    expect(getEducationEmailStatus).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('student@muc.edu.cn')
    expect(wrapper.text()).toContain('formatted:2026-09-20T01:02:03Z')
    expect(wrapper.text()).toContain('common.disabled')
  })

  it('shows a clear unverified state without exposing an invented email', async () => {
    getEducationEmailStatus.mockResolvedValue({
      user_id: 7,
      education_email_verification_enabled: true,
      education_email: { bound: false },
    })
    const wrapper = mount(UserEducationEmailModal, {
      props: { show: true, user },
      global: { stubs },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.users.educationEmail.notVerified')
    expect(wrapper.text()).toContain('admin.users.educationEmail.noRecord')
    expect(wrapper.text()).not.toContain('@muc.edu.cn')
    // 未认证时不出现"撤销认证"按钮
    expect(wrapper.findAll('button').some((b) => b.text() === 'admin.users.educationEmail.revoke')).toBe(false)
  })

  it('revokes via the confirm dialog, reloads, and emits success', async () => {
    getEducationEmailStatus
      .mockResolvedValueOnce({
        user_id: 7,
        education_email_verification_enabled: false,
        education_email: {
          bound: true,
          display_name: 'student@muc.edu.cn',
          verified_at: '2026-09-20T01:02:03Z',
        },
      })
      .mockResolvedValue({
        user_id: 7,
        education_email_verification_enabled: false,
        education_email: { bound: false },
      })
    revokeEducationEmail.mockResolvedValue({ user_id: 7, revoked_count: 1 })

    const wrapper = mount(UserEducationEmailModal, {
      props: { show: true, user },
      global: { stubs },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('admin.users.educationEmail.revoke')

    // 点击"撤销认证"→ 弹出确认框 → 点击确认
    await wrapper.findAll('button').find((b) => b.text() === 'admin.users.educationEmail.revoke')!.trigger('click')
    await flushPromises()
    expect(revokeEducationEmail).not.toHaveBeenCalled()

    await wrapper.findAll('button').find((b) => b.text() === 'common.confirm')!.trigger('click')
    await flushPromises()

    expect(revokeEducationEmail).toHaveBeenCalledWith(7)
    expect(wrapper.emitted('success')).toHaveLength(1)
    // 撤销后 modal 数据已重载为未认证状态
    expect(wrapper.text()).toContain('admin.users.educationEmail.noRecord')
  })
})
