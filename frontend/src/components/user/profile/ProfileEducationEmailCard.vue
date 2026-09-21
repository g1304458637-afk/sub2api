<template>
  <section
    data-testid="profile-education-email-card"
    class="card border border-gray-100 bg-white/90 p-6 dark:border-dark-700 dark:bg-dark-900/50"
  >
    <div class="flex items-start justify-between gap-4">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('profile.educationEmail.title') }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.educationEmail.description') }}
        </p>
      </div>
      <span
        data-testid="profile-education-email-status"
        :class="['badge', isVerified ? 'badge-success' : 'badge-gray']"
      >
        {{
          isVerified
            ? t('profile.educationEmail.statusVerified')
            : t('profile.educationEmail.statusUnverified')
        }}
      </span>
    </div>

    <div
      v-if="isVerified"
      data-testid="profile-education-email-verified"
      class="mt-5 grid gap-3 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 text-sm dark:border-dark-700 dark:bg-dark-900/30"
    >
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-gray-500 dark:text-gray-400">
          {{ t('profile.educationEmail.verifiedEmailLabel') }}
        </span>
        <span class="font-medium text-gray-900 dark:text-white">
          {{ verifiedEmailDisplay }}
        </span>
      </div>
      <div
        v-if="verifiedAtDisplay"
        class="flex flex-wrap items-center gap-2"
      >
        <span class="text-gray-500 dark:text-gray-400">
          {{ t('profile.educationEmail.verifiedAtLabel') }}
        </span>
        <span class="text-gray-700 dark:text-gray-200">{{ verifiedAtDisplay }}</span>
      </div>
    </div>

    <div
      v-else
      data-testid="profile-education-email-form"
      class="mt-5 grid gap-2 sm:grid-cols-[minmax(0,1.4fr)_auto]"
    >
      <input
        v-model.trim="educationEmailForm.email"
        data-testid="profile-education-email-input"
        type="email"
        class="input"
        :placeholder="t('profile.educationEmail.emailPlaceholder')"
        :disabled="isSendingCode || isVerifying"
      />
      <button
        data-testid="profile-education-email-send-code"
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="isSendingCode || isVerifying"
        @click="sendCode"
      >
        {{ isSendingCode ? t('common.loading') : t('profile.educationEmail.sendCodeAction') }}
      </button>
      <input
        v-model.trim="educationEmailForm.verifyCode"
        data-testid="profile-education-email-code-input"
        type="text"
        inputmode="numeric"
        maxlength="6"
        class="input"
        :placeholder="t('profile.educationEmail.codePlaceholder')"
        :disabled="isVerifying"
      />
      <button
        data-testid="profile-education-email-verify"
        type="button"
        class="btn btn-primary btn-sm"
        :disabled="isVerifying"
        @click="verify"
      >
        {{ isVerifying ? t('common.loading') : t('profile.educationEmail.verifyAction') }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { currentBrand } from '@/brand'
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { sendEducationEmailCode, verifyEducationEmail } from '@/api/user'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { User } from '@/types'

// 校园邮箱认证只接受精确的 @muc.edu.cn 域名（不含子域名或相似后缀）。
const EDUCATION_EMAIL_DOMAIN = '@' + currentBrand.educationDomain
const EDUCATION_EMAIL_PATTERN = /^[A-Za-z0-9._%+-]+@[^@\s]+$/

const props = defineProps<{
  user: User
}>()

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const localUser = ref<User | null>(null)
const isSendingCode = ref(false)
const isVerifying = ref(false)
const educationEmailForm = reactive({
  email: '',
  verifyCode: '',
})

const currentUser = computed(() => localUser.value ?? props.user)
const isVerified = computed(() =>
  currentUser.value.education_email_bound ??
  currentUser.value.education_email?.bound ??
  false
)
const verifiedEmailDisplay = computed(() => {
  const summary = currentUser.value.education_email
  return summary?.display_name?.trim() || summary?.subject_hint?.trim() || ''
})
const verifiedAtDisplay = computed(() => {
  const raw = currentUser.value.education_email?.verified_at
  if (!raw) {
    return ''
  }
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date)
})

function normalizeEducationEmail(value: string): string {
  return value.trim().toLowerCase()
}

function isValidEducationEmail(value: string): boolean {
  const email = normalizeEducationEmail(value)
  return EDUCATION_EMAIL_PATTERN.test(email) && email.endsWith(EDUCATION_EMAIL_DOMAIN)
}

function applyUpdatedUser(user: User): void {
  localUser.value = user
  authStore.user = user
}

async function sendCode(): Promise<void> {
  const email = normalizeEducationEmail(educationEmailForm.email)
  if (!isValidEducationEmail(email)) {
    appStore.showError(t('profile.educationEmail.invalidDomain'))
    return
  }

  isSendingCode.value = true
  try {
    await sendEducationEmailCode(email)
    appStore.showSuccess(t('profile.educationEmail.codeSentTo', { email }))
  } catch (error) {
    appStore.showError((error as { message?: string }).message || t('common.tryAgain'))
  } finally {
    isSendingCode.value = false
  }
}

async function verify(): Promise<void> {
  const email = normalizeEducationEmail(educationEmailForm.email)
  if (!isValidEducationEmail(email)) {
    appStore.showError(t('profile.educationEmail.invalidDomain'))
    return
  }
  if (!educationEmailForm.verifyCode) {
    appStore.showError(t('auth.codeRequired'))
    return
  }

  isVerifying.value = true
  try {
    const user = await verifyEducationEmail(email, educationEmailForm.verifyCode)
    applyUpdatedUser(user)
    educationEmailForm.verifyCode = ''
    appStore.showSuccess(t('profile.educationEmail.verifySuccess'))
  } catch (error) {
    appStore.showError((error as { message?: string }).message || t('common.tryAgain'))
  } finally {
    isVerifying.value = false
  }
}
</script>
