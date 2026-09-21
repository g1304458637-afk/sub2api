<template>
  <BaseDialog
    :show="show"
    :title="t('admin.users.educationEmail.title')"
    width="normal"
    @close="emit('close')"
  >
    <div v-if="user" class="space-y-4">
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <p class="font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
        <p v-if="user.username" class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ user.username }}</p>
      </div>

      <div v-if="loading" class="flex justify-center py-8">
        <Icon name="refresh" class="animate-spin text-primary-500" />
      </div>

      <div
        v-else-if="loadError"
        class="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
      >
        {{ loadError }}
      </div>

      <template v-else-if="status">
        <div class="flex items-center justify-between rounded-xl border border-gray-200 px-4 py-3 dark:border-dark-600">
          <span class="text-sm text-gray-600 dark:text-dark-300">{{ t('admin.users.educationEmail.featureStatus') }}</span>
          <span :class="['badge', status.education_email_verification_enabled ? 'badge-success' : 'badge-gray']">
            {{ status.education_email_verification_enabled ? t('common.enabled') : t('common.disabled') }}
          </span>
        </div>

        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
          <div class="flex items-center justify-between gap-3">
            <span class="font-medium text-gray-900 dark:text-white">{{ t('admin.users.educationEmail.verificationStatus') }}</span>
            <span :class="['badge', status.education_email.bound ? 'badge-success' : 'badge-gray']">
              {{ status.education_email.bound ? t('admin.users.educationEmail.verified') : t('admin.users.educationEmail.notVerified') }}
            </span>
          </div>
          <template v-if="status.education_email.bound">
            <dl class="mt-4 space-y-3 text-sm">
              <div class="grid gap-1 sm:grid-cols-[7rem_minmax(0,1fr)]">
                <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.users.educationEmail.email') }}</dt>
                <dd class="break-all font-medium text-gray-900 dark:text-white">{{ status.education_email.display_name || '-' }}</dd>
              </div>
              <div class="grid gap-1 sm:grid-cols-[7rem_minmax(0,1fr)]">
                <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.users.educationEmail.verifiedAt') }}</dt>
                <dd class="text-gray-900 dark:text-white">{{ verifiedAt }}</dd>
              </div>
            </dl>
            <button
              type="button"
              class="btn btn-danger mt-4 w-full"
              :disabled="revoking"
              @click="showRevokeConfirm = true"
            >
              <Icon name="trash" size="sm" :class="revoking ? 'animate-spin' : ''" />
              {{ t('admin.users.educationEmail.revoke') }}
            </button>
          </template>
          <p v-else class="mt-3 text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.users.educationEmail.noRecord') }}
          </p>
        </div>

        <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">
          {{ t('admin.users.educationEmail.scope') }}
        </p>
      </template>
    </div>

    <ConfirmDialog
      :show="showRevokeConfirm"
      :title="t('admin.users.educationEmail.revokeConfirmTitle')"
      :message="t('admin.users.educationEmail.revokeConfirmMessage', { email: status?.education_email.display_name })"
      :danger="true"
      @confirm="handleRevoke"
      @cancel="showRevokeConfirm = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { EducationEmailStatus } from '@/api/admin/users'
import type { AdminUser } from '@/types'
import { formatDateTime } from '@/utils/format'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
  user: AdminUser | null
}>()

const emit = defineEmits<{ close: []; success: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const loadError = ref('')
const status = ref<EducationEmailStatus | null>(null)
const revoking = ref(false)
const showRevokeConfirm = ref(false)

const verifiedAt = computed(() => {
  const value = status.value?.education_email.verified_at
  return value ? formatDateTime(value) : '-'
})

async function load(): Promise<void> {
  if (!props.show || !props.user) return
  loading.value = true
  loadError.value = ''
  status.value = null
  try {
    status.value = await adminAPI.users.getEducationEmailStatus(props.user.id)
  } catch (error) {
    loadError.value = (error as { message?: string }).message || t('admin.users.educationEmail.loadFailed')
  } finally {
    loading.value = false
  }
}

// 确认撤销：调撤销接口后重载 modal 数据并通知列表刷新。
// 先刷新自身数据再 emit('success')，保证父级列表更新时 modal 仍持有有效状态。
async function handleRevoke(): Promise<void> {
  if (!props.user || revoking.value) return
  revoking.value = true
  try {
    const result = await adminAPI.users.revokeEducationEmail(props.user.id)
    appStore.showSuccess(t('admin.users.educationEmail.revokeSuccess', { count: result.revoked_count }))
    showRevokeConfirm.value = false
    await load()
    emit('success')
  } catch (error) {
    appStore.showError((error as { message?: string }).message || t('admin.users.educationEmail.revokeFailed'))
  } finally {
    revoking.value = false
  }
}

watch(() => [props.show, props.user?.id] as const, () => {
  showRevokeConfirm.value = false
  void load()
}, { immediate: true })
</script>
