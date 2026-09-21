import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import UserEducationEmailModal from '@/components/admin/user/UserEducationEmailModal.vue'

const { getEducationEmailStatus } = vi.hoisted(() => ({
  getEducationEmailStatus: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { getEducationEmailStatus } },
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

describe('UserEducationEmailModal', () => {
  beforeEach(() => getEducationEmailStatus.mockReset())

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
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } },
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
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.users.educationEmail.notVerified')
    expect(wrapper.text()).toContain('admin.users.educationEmail.noRecord')
    expect(wrapper.text()).not.toContain('@muc.edu.cn')
  })
})
