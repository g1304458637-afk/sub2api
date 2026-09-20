import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileEducationEmailCard from '@/components/user/profile/ProfileEducationEmailCard.vue'
import { useAppStore, useAuthStore } from '@/stores'
import type { User } from '@/types'

let pinia: ReturnType<typeof createPinia>

const userApiMocks = vi.hoisted(() => ({
  sendEducationEmailCode: vi.fn(),
  verifyEducationEmail: vi.fn(),
}))

vi.mock('@/api/user', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/user')>()
  return {
    ...actual,
    sendEducationEmailCode: (...args: any[]) => userApiMocks.sendEducationEmailCode(...args),
    verifyEducationEmail: (...args: any[]) => userApiMocks.verifyEducationEmail(...args),
  }
})

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'profile.educationEmail.title') return 'Campus email verification'
        if (key === 'profile.educationEmail.description') return 'Verify with an @muc.edu.cn email'
        if (key === 'profile.educationEmail.statusVerified') return 'Verified'
        if (key === 'profile.educationEmail.statusUnverified') return 'Not verified'
        if (key === 'profile.educationEmail.verifiedEmailLabel') return 'Verified email'
        if (key === 'profile.educationEmail.verifiedAtLabel') return 'Verified at'
        if (key === 'profile.educationEmail.emailPlaceholder') return 'name@muc.edu.cn'
        if (key === 'profile.educationEmail.codePlaceholder') return 'Enter 6-digit code'
        if (key === 'profile.educationEmail.sendCodeAction') return 'Send code'
        if (key === 'profile.educationEmail.verifyAction') return 'Verify'
        if (key === 'profile.educationEmail.codeSentTo') return `Code sent to ${params?.email || ''}`.trim()
        if (key === 'profile.educationEmail.verifySuccess') return 'Campus email verified'
        if (key === 'profile.educationEmail.invalidDomain') return 'Only exact @muc.edu.cn emails are accepted'
        return key
      },
    }),
  }
})

function createUser(overrides: Partial<User> = {}): User {
  return {
    id: 9,
    username: 'alice',
    email: 'alice@example.com',
    role: 'user',
    balance: 10,
    concurrency: 2,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-04-20T00:00:00Z',
    updated_at: '2026-04-20T00:00:00Z',
    ...overrides,
  }
}

describe('ProfileEducationEmailCard', () => {
  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    userApiMocks.sendEducationEmailCode.mockReset()
    userApiMocks.verifyEducationEmail.mockReset()
  })

  it('renders the verification form for an unverified user', () => {
    const wrapper = mount(ProfileEducationEmailCard, {
      global: { plugins: [pinia] },
      props: { user: createUser() },
    })

    expect(wrapper.get('[data-testid="profile-education-email-status"]').text()).toBe('Not verified')
    expect(wrapper.get('[data-testid="profile-education-email-form"]').exists()).toBe(true)
  })

  it('rejects non campus domains before calling the API', async () => {
    const appStore = useAppStore()
    const showErrorSpy = vi.spyOn(appStore, 'showError')

    const wrapper = mount(ProfileEducationEmailCard, {
      global: { plugins: [pinia] },
      props: { user: createUser() },
    })

    await wrapper.get('[data-testid="profile-education-email-input"]').setValue('alice@gmail.com')
    await wrapper.get('[data-testid="profile-education-email-send-code"]').trigger('click')

    expect(showErrorSpy).toHaveBeenCalledWith('Only exact @muc.edu.cn emails are accepted')
    expect(userApiMocks.sendEducationEmailCode).not.toHaveBeenCalled()
  })

  it('rejects subdomains and look-alike suffixes of muc.edu.cn', async () => {
    const appStore = useAppStore()
    const showErrorSpy = vi.spyOn(appStore, 'showError')

    const wrapper = mount(ProfileEducationEmailCard, {
      global: { plugins: [pinia] },
      props: { user: createUser() },
    })

    for (const email of ['alice@mail.muc.edu.cn', 'alice@muc.edu.cn.evil.com', 'alice@muc.edu.cnx']) {
      await wrapper.get('[data-testid="profile-education-email-input"]').setValue(email)
      await wrapper.get('[data-testid="profile-education-email-send-code"]').trigger('click')
      expect(userApiMocks.sendEducationEmailCode).not.toHaveBeenCalled()
    }
    expect(showErrorSpy).toHaveBeenCalledTimes(3)
  })

  it('sends the verification code to the normalized campus email', async () => {
    userApiMocks.sendEducationEmailCode.mockResolvedValue(undefined)
    const appStore = useAppStore()
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const wrapper = mount(ProfileEducationEmailCard, {
      global: { plugins: [pinia] },
      props: { user: createUser() },
    })

    await wrapper.get('[data-testid="profile-education-email-input"]').setValue('Alice@MUC.edu.cn')
    await wrapper.get('[data-testid="profile-education-email-send-code"]').trigger('click')

    expect(userApiMocks.sendEducationEmailCode).toHaveBeenCalledWith('alice@muc.edu.cn')
    expect(showSuccessSpy).toHaveBeenCalledWith('Code sent to alice@muc.edu.cn')
  })

  it('verifies the code and updates the signed-in user with the refreshed profile', async () => {
    userApiMocks.verifyEducationEmail.mockResolvedValue(
      createUser({
        education_email_bound: true,
        education_email: {
          bound: true,
          bound_count: 1,
          provider: 'education_email',
          display_name: 'alice@muc.edu.cn',
          subject_hint: 'a***e@muc.edu.cn',
          verified_at: '2026-09-20T00:00:00Z',
          can_bind: false,
          can_unbind: false,
        },
      })
    )
    const appStore = useAppStore()
    const authStore = useAuthStore()
    authStore.user = createUser()
    const showSuccessSpy = vi.spyOn(appStore, 'showSuccess')

    const wrapper = mount(ProfileEducationEmailCard, {
      global: { plugins: [pinia] },
      props: { user: authStore.user },
    })

    await wrapper.get('[data-testid="profile-education-email-input"]').setValue('alice@muc.edu.cn')
    await wrapper.get('[data-testid="profile-education-email-code-input"]').setValue('123456')
    await wrapper.get('[data-testid="profile-education-email-verify"]').trigger('click')

    expect(userApiMocks.verifyEducationEmail).toHaveBeenCalledWith('alice@muc.edu.cn', '123456')
    expect(showSuccessSpy).toHaveBeenCalledWith('Campus email verified')
    expect(authStore.user?.education_email_bound).toBe(true)
    expect(wrapper.get('[data-testid="profile-education-email-status"]').text()).toBe('Verified')
    expect(wrapper.get('[data-testid="profile-education-email-verified"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-education-email-form"]').exists()).toBe(false)
  })

  it('shows the verified campus email without the form for a verified user', () => {
    const wrapper = mount(ProfileEducationEmailCard, {
      global: { plugins: [pinia] },
      props: {
        user: createUser({
          education_email_bound: true,
          education_email: {
            bound: true,
            bound_count: 1,
            provider: 'education_email',
            display_name: 'alice@muc.edu.cn',
            subject_hint: 'a***e@muc.edu.cn',
            verified_at: '2026-09-20T08:30:00Z',
            can_bind: false,
            can_unbind: false,
          },
        }),
      },
    })

    expect(wrapper.get('[data-testid="profile-education-email-status"]').text()).toBe('Verified')
    expect(wrapper.get('[data-testid="profile-education-email-verified"]').text()).toContain('alice@muc.edu.cn')
    expect(wrapper.find('[data-testid="profile-education-email-form"]').exists()).toBe(false)
  })
})
