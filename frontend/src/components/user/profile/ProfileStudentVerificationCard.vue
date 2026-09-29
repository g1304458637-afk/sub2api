<template>
  <section
    data-testid="profile-student-verification-card"
    class="card border border-gray-100 bg-white/90 p-6 dark:border-dark-700 dark:bg-dark-900/50"
  >
    <div class="flex items-start justify-between gap-4">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('profile.studentVerification.title', { brand: currentBrand.name }) }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.studentVerification.description', { n: benefitDays }) }}
        </p>
      </div>
      <span
        data-testid="profile-student-verification-status"
        :class="['badge', isVerifiedView ? 'badge-success' : 'badge-gray']"
      >
        {{
          isVerifiedView
            ? t('profile.studentVerification.statusVerified')
            : t('profile.studentVerification.statusUnverified')
        }}
      </span>
    </div>

    <!-- 已认证：刚完成认证时展示发放结果；刷新页面后用 GET status 回显 -->
    <div
      v-if="isVerifiedView"
      data-testid="profile-student-verification-verified"
      class="mt-5 grid gap-3 rounded-2xl border border-gray-100 bg-gray-50/80 p-4 text-sm dark:border-dark-700 dark:bg-dark-900/30"
    >
      <div class="flex flex-wrap items-center gap-2">
        <span class="font-medium text-gray-900 dark:text-white">
          {{ t('profile.studentVerification.verifySuccess') }}
        </span>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <span class="text-gray-500 dark:text-gray-400">
          {{ t('profile.studentVerification.verifiedEmailLabel') }}
        </span>
        <span class="font-medium text-gray-900 dark:text-white">{{ verifiedEmailDisplay }}</span>
      </div>
      <div
        v-if="isPendingOutcome"
        class="flex flex-wrap items-center gap-2"
      >
        <span class="text-gray-700 dark:text-gray-200">
          {{ t('profile.studentVerification.grantPending') }}
        </span>
      </div>
      <div
        v-else
        class="flex flex-wrap items-center gap-2"
      >
        <span class="text-gray-700 dark:text-gray-200">
          {{ t('profile.studentVerification.grantActive') }}
        </span>
      </div>
      <div
        v-if="validityRangeDisplay"
        class="flex flex-wrap items-center gap-2"
      >
        <span class="text-gray-500 dark:text-gray-400">
          {{ t('profile.studentVerification.validityLabel') }}
        </span>
        <span class="text-gray-700 dark:text-gray-200">{{ validityRangeDisplay }}</span>
      </div>
      <div
        v-if="!verifyResult && verifiedAtDisplay"
        class="flex flex-wrap items-center gap-2"
      >
        <span class="text-gray-500 dark:text-gray-400">
          {{ t('profile.studentVerification.verifiedAtLabel') }}
        </span>
        <span class="text-gray-700 dark:text-gray-200">{{ verifiedAtDisplay }}</span>
      </div>
      <div
        v-if="!verifyResult && grantExpiresDisplay"
        class="flex flex-wrap items-center gap-2"
      >
        <span class="text-gray-500 dark:text-gray-400">
          {{ t('profile.studentVerification.grantExpiresLabel') }}
        </span>
        <span class="text-gray-700 dark:text-gray-200">{{ grantExpiresDisplay }}</span>
      </div>
    </div>

    <!-- 未认证：邮箱 + 验证码表单 -->
    <form
      v-else
      data-testid="profile-student-verification-form"
      class="mt-5 grid gap-2 sm:grid-cols-[minmax(0,1.4fr)_auto]"
      @submit.prevent="verify"
    >
      <label class="sr-only" for="student-verification-email">
        {{ t('profile.studentVerification.emailLabel') }}
      </label>
      <input
        id="student-verification-email"
        v-model.trim="form.email"
        data-testid="profile-student-verification-email-input"
        type="email"
        class="input"
        :placeholder="emailPlaceholder"
        :disabled="isSendingCode || isVerifying"
      />
      <button
        data-testid="profile-student-verification-send-code"
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="isSendingCode || isVerifying || countdown > 0"
        @click="sendCode"
      >
        {{
          isSendingCode
            ? t('common.loading')
            : countdown > 0
              ? t('profile.studentVerification.resendCountdown', { n: countdown })
              : t('profile.studentVerification.sendCodeAction')
        }}
      </button>
      <label class="sr-only" for="student-verification-code">
        {{ t('profile.studentVerification.codeLabel') }}
      </label>
      <input
        id="student-verification-code"
        v-model.trim="form.code"
        data-testid="profile-student-verification-code-input"
        type="text"
        inputmode="numeric"
        maxlength="6"
        class="input"
        :placeholder="t('profile.studentVerification.codePlaceholder')"
        :disabled="isVerifying"
      />
      <button
        data-testid="profile-student-verification-verify"
        type="submit"
        class="btn btn-primary btn-sm"
        :disabled="isVerifying"
      >
        {{ isVerifying ? t('common.loading') : t('profile.studentVerification.verifyAction') }}
      </button>
    </form>
  </section>
</template>

<script setup lang="ts">
import { currentBrand } from '@/brand'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getStudentVerificationStatus,
  sendStudentVerificationCode,
  verifyStudentVerification,
  type StudentVerificationResult,
  type StudentVerificationStatus,
} from '@/api/studentVerification'
import { useAppStore } from '@/stores/app'

// 发送验证码后的冷却时间（秒）
const RESEND_COOLDOWN_SECONDS = 60
// 前端仅做基础邮箱格式校验，学生邮箱域名由后端权威校验
const EMAIL_PATTERN = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/

const { t } = useI18n()
const appStore = useAppStore()

const statusInfo = ref<StudentVerificationStatus | null>(null)
const verifyResult = ref<StudentVerificationResult | null>(null)
const isSendingCode = ref(false)
const isVerifying = ref(false)
const countdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const form = reactive({
  email: '',
  code: '',
})

const benefitDays = computed(() => statusInfo.value?.benefit_days ?? 30)
const emailPlaceholder = computed(() => {
  const domain = statusInfo.value?.email_domain || currentBrand.educationDomain
  return domain ? `student@${domain}` : 'student@stu.example.edu'
})
const isVerifiedView = computed(
  () => verifyResult.value?.verified === true || statusInfo.value?.email_verified === true
)
// pending：当前有其他订阅，权益到期后自动衔接
const isPendingOutcome = computed(() => verifyResult.value?.outcome?.action === 'pending')
const verifiedEmailDisplay = computed(
  () => verifyResult.value?.email || statusInfo.value?.email || ''
)

function formatDate(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

const verifiedAtDisplay = computed(() => {
  const raw = verifyResult.value?.verified_at || statusInfo.value?.verified_at
  return formatDate(raw)
})

// 有效期： activated_at → expires_at（extended 时回退 previous_expires → expires_at）
const validityRangeDisplay = computed(() => {
  if (verifyResult.value) {
    const start =
      verifyResult.value.activated_at || verifyResult.value.outcome?.previous_expires
    const end = verifyResult.value.expires_at || verifyResult.value.outcome?.expires_at
    if (end) {
      return t('profile.studentVerification.validityRange', {
        start: formatDate(start),
        end: formatDate(end),
      })
    }
    return ''
  }
  return ''
})

const grantExpiresDisplay = computed(() => formatDate(statusInfo.value?.grant_expires_at))

// 后端错误 reason 可能落在 code（HTTP 错误）或 reason（200 包装错误）字段
function resolveErrorReason(error: unknown): string {
  const err = error as { code?: unknown; reason?: unknown } | undefined
  if (typeof err?.code === 'string' && err.code) return err.code
  if (typeof err?.reason === 'string' && err.reason) return err.reason
  return ''
}

// 已知错误 reason 到 i18n 文案的映射；未识别的 reason 回退到后端 message
const ERROR_REASON_MESSAGE_KEYS: Record<string, string> = {
  STUDENT_VERIFICATION_DISABLED: 'profile.studentVerification.errorDisabled',
  STUDENT_EMAIL_INVALID: 'profile.studentVerification.errorInvalidEmailDomain',
  VERIFY_CODE_TOO_FREQUENT: 'profile.studentVerification.errorTooFrequent',
  NOTIFY_CODE_USER_RATE_LIMIT: 'profile.studentVerification.errorTooFrequent',
  INVALID_VERIFY_CODE: 'profile.studentVerification.errorInvalidCode',
}

function resolveErrorMessage(error: unknown): string {
  const key = ERROR_REASON_MESSAGE_KEYS[resolveErrorReason(error)]
  if (key) {
    return t(key)
  }
  return (error as { message?: string }).message || t('common.tryAgain')
}

function startCountdown(): void {
  stopCountdown()
  countdown.value = RESEND_COOLDOWN_SECONDS
  countdownTimer = setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0) {
      stopCountdown()
    }
  }, 1000)
}

function stopCountdown(): void {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
  countdown.value = 0
}

function normalizeEmail(value: string): string {
  return value.trim().toLowerCase()
}

function isValidEmail(value: string): boolean {
  return EMAIL_PATTERN.test(normalizeEmail(value))
}

async function sendCode(): Promise<void> {
  const email = normalizeEmail(form.email)
  if (!isValidEmail(email)) {
    appStore.showError(t('profile.studentVerification.invalidEmail'))
    return
  }

  isSendingCode.value = true
  try {
    await sendStudentVerificationCode(currentBrand, email)
    startCountdown()
    appStore.showSuccess(t('profile.studentVerification.codeSent', { email }))
  } catch (error) {
    appStore.showError(resolveErrorMessage(error))
  } finally {
    isSendingCode.value = false
  }
}

async function verify(): Promise<void> {
  const email = normalizeEmail(form.email)
  if (!isValidEmail(email)) {
    appStore.showError(t('profile.studentVerification.invalidEmail'))
    return
  }
  if (!form.code) {
    appStore.showError(t('profile.studentVerification.codeRequired'))
    return
  }

  isVerifying.value = true
  try {
    const result = await verifyStudentVerification(currentBrand, email, form.code)
    verifyResult.value = result
    form.code = ''
    stopCountdown()
    appStore.showSuccess(t('profile.studentVerification.verifySuccess'))
  } catch (error) {
    appStore.showError(resolveErrorMessage(error))
  } finally {
    isVerifying.value = false
  }
}

onMounted(async () => {
  try {
    statusInfo.value = await getStudentVerificationStatus(currentBrand)
  } catch (error) {
    console.error('Failed to load student verification status:', error)
  }
})

onBeforeUnmount(() => {
  stopCountdown()
})
</script>
