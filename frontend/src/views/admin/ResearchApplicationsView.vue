<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <Select
            v-model="statusFilter"
            :options="filterStatusOptions"
            class="w-36"
            @change="handleFilterChange"
          />
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadApplications" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="applications" :loading="loading">
          <template #cell-applicant="{ row }">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                {{ applicantLabelOf(row) }}
              </p>
              <p v-if="row.user?.username" class="truncate text-xs text-gray-500 dark:text-dark-400">
                {{ row.user.username }}
              </p>
            </div>
          </template>

          <template #cell-description="{ value }">
            <span class="block max-w-xs truncate text-sm text-gray-500 dark:text-dark-400" :title="value">
              {{ value }}
            </span>
          </template>

          <template #cell-attachments="{ row }">
            <span class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.research.attachmentCount', { count: row.attachments?.length ?? 0 }) }}
            </span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-status="{ value }">
            <span :class="['badge', statusBadgeClass(value)]">
              {{ t(`admin.research.status.${value}`) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <button
              type="button"
              class="rounded-lg px-2 py-1 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 dark:text-primary-400 dark:hover:bg-primary-900/20 dark:hover:text-primary-300"
              @click="openDetail(row)"
            >
              {{ t('admin.research.viewDetail') }}
            </button>
          </template>

          <template #empty>
            <div class="empty-state py-8">
              <div
                class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800"
              >
                <Icon name="beaker" size="xl" class="text-gray-400 dark:text-dark-500" />
              </div>
              <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.research.list.empty') }}
              </p>
              <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
                {{ t('admin.research.list.emptyHint') }}
              </p>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Detail Dialog -->
    <BaseDialog
      :show="showDetailDialog"
      :title="t('admin.research.detail.title')"
      width="wide"
      @close="closeDetailDialog"
    >
      <div v-if="detailApplication" class="space-y-4">
        <!-- Meta info -->
        <dl class="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
          <div>
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.research.detail.applicant') }}
            </dt>
            <dd class="mt-1 break-all text-sm font-medium text-gray-900 dark:text-white">
              {{ applicantLabelOf(detailApplication) }}
            </dd>
            <dd v-if="detailApplication.user?.username" class="text-xs text-gray-500 dark:text-dark-400">
              {{ detailApplication.user.username }}
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.research.columns.status') }}
            </dt>
            <dd class="mt-1">
              <span :class="['badge', statusBadgeClass(detailApplication.status)]">
                {{ t(`admin.research.status.${detailApplication.status}`) }}
              </span>
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.research.detail.submittedAt') }}
            </dt>
            <dd class="mt-1 text-sm text-gray-900 dark:text-white">
              {{ formatDateTime(detailApplication.created_at) }}
            </dd>
          </div>
          <div>
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.research.detail.reviewedAt') }}
            </dt>
            <dd class="mt-1 text-sm text-gray-900 dark:text-white">
              {{
                detailApplication.reviewed_at
                  ? formatDateTime(detailApplication.reviewed_at)
                  : t('admin.research.detail.notReviewed')
              }}
            </dd>
          </div>
          <div
            v-if="detailApplication.status === 'approved' && detailApplication.reward_amount !== null"
          >
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.research.detail.rewardAmount') }}
            </dt>
            <dd class="mt-1 text-sm font-semibold text-emerald-600 dark:text-emerald-400">
              +{{ currencyStore.formatCNY(detailApplication.reward_amount) }}
            </dd>
          </div>
          <div v-if="detailApplication.review_notes" class="sm:col-span-2">
            <dt class="text-xs font-medium text-gray-500 dark:text-dark-400">
              {{ t('admin.research.detail.reviewNotes') }}
            </dt>
            <dd class="mt-1 whitespace-pre-wrap break-words text-sm text-gray-700 dark:text-gray-300">
              {{ detailApplication.review_notes }}
            </dd>
          </div>
        </dl>

        <!-- Full description -->
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
            {{ t('admin.research.detail.descriptionLabel') }}
          </p>
          <p
            class="mt-1 max-h-60 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-900/40 dark:text-gray-300"
          >
            {{ detailApplication.description }}
          </p>
        </div>

        <!-- Attachments -->
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
            {{ t('admin.research.detail.attachmentsLabel') }}
            <span class="ml-1 text-gray-400 dark:text-dark-500">
              ({{ detailApplication.attachments.length }})
            </span>
          </p>
          <p
            v-if="detailApplication.attachments.length === 0"
            class="mt-1 text-sm text-gray-400 dark:text-dark-500"
          >
            {{ t('admin.research.detail.noAttachments') }}
          </p>
          <ul v-else class="mt-2 space-y-2">
            <li
              v-for="attachment in detailApplication.attachments"
              :key="attachment.id"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="flex min-w-0 items-center gap-2">
                  <Icon
                    name="document"
                    size="sm"
                    class="flex-shrink-0 text-gray-400 dark:text-dark-500"
                  />
                  <span
                    class="min-w-0 truncate text-sm text-gray-700 dark:text-gray-300"
                    :title="attachment.name"
                  >
                    {{ attachment.name }}
                  </span>
                  <span class="flex-shrink-0 text-xs text-gray-400 dark:text-dark-500">
                    {{ formatBytes(attachment.size) }}
                  </span>
                </div>
                <div class="flex flex-shrink-0 items-center gap-3">
                  <button
                    v-if="isImageMime(attachment.mime)"
                    type="button"
                    class="text-xs font-medium text-primary-600 transition-colors hover:text-primary-700 disabled:cursor-not-allowed disabled:opacity-60 dark:text-primary-400 dark:hover:text-primary-300"
                    :disabled="previewingId !== null"
                    @click="togglePreview(attachment)"
                  >
                    {{
                      previewUrlOf(attachment)
                        ? t('common.close')
                        : t('admin.research.detail.preview')
                    }}
                  </button>
                  <button
                    type="button"
                    class="text-xs font-medium text-primary-600 transition-colors hover:text-primary-700 disabled:cursor-not-allowed disabled:opacity-60 dark:text-primary-400 dark:hover:text-primary-300"
                    :disabled="downloadingId !== null"
                    @click="handleDownload(attachment)"
                  >
                    {{ t('admin.research.detail.download') }}
                  </button>
                </div>
              </div>
              <img
                v-if="previewUrlOf(attachment)"
                :src="previewUrlOf(attachment)"
                :alt="attachment.name"
                class="mt-3 max-h-72 w-full rounded-lg bg-gray-50 object-contain dark:bg-dark-900/40"
              />
            </li>
          </ul>
        </div>
      </div>

      <template #footer>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <button
            v-if="detailApplication?.status === 'pending'"
            type="button"
            class="btn btn-danger"
            @click="openRejectDialog"
          >
            {{ t('admin.research.detail.reject') }}
          </button>
          <button
            v-if="detailApplication?.status === 'pending'"
            type="button"
            class="btn btn-primary"
            @click="openApproveDialog"
          >
            {{ t('admin.research.detail.approve') }}
          </button>
          <button type="button" class="btn btn-secondary" @click="closeDetailDialog">
            {{ t('admin.research.detail.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Approve Dialog -->
    <BaseDialog
      :show="showApproveDialog"
      :title="t('admin.research.approve.title')"
      @close="showApproveDialog = false"
    >
      <form class="space-y-4" @submit.prevent="handleApproveSubmit">
        <p
          v-if="detailApplication"
          class="text-sm text-gray-500 dark:text-gray-400"
        >
          {{
            t('admin.research.approve.applicantHint', {
              name: applicantLabelOf(detailApplication)
            })
          }}
        </p>
        <div>
          <label for="research-approve-amount" class="input-label">
            {{ t('admin.research.approve.amountLabel') }}
          </label>
          <input
            id="research-approve-amount"
            v-model.number="approveForm.amount"
            type="number"
            min="0.01"
            step="0.01"
            required
            class="input mt-1"
          />
        </div>
        <div>
          <label for="research-approve-notes" class="input-label">
            {{ t('admin.research.approve.notesLabel') }}
          </label>
          <textarea
            id="research-approve-notes"
            v-model="approveForm.notes"
            rows="3"
            :placeholder="t('admin.research.approve.notesPlaceholder')"
            class="input mt-1"
          ></textarea>
        </div>
        <div class="flex justify-end gap-3 pt-2">
          <button type="button" class="btn btn-secondary" @click="showApproveDialog = false">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="approving">
            {{ approving ? t('admin.research.approve.submitting') : t('admin.research.approve.confirm') }}
          </button>
        </div>
      </form>
    </BaseDialog>

    <!-- Reject Dialog -->
    <BaseDialog
      :show="showRejectDialog"
      :title="t('admin.research.reject.title')"
      @close="showRejectDialog = false"
    >
      <form class="space-y-4" @submit.prevent="handleRejectSubmit">
        <div>
          <label for="research-reject-notes" class="input-label">
            {{ t('admin.research.reject.notesLabel') }}
          </label>
          <textarea
            id="research-reject-notes"
            v-model="rejectForm.notes"
            rows="3"
            required
            :placeholder="t('admin.research.reject.notesPlaceholder')"
            class="input mt-1"
          ></textarea>
        </div>
        <div class="flex justify-end gap-3 pt-2">
          <button type="button" class="btn btn-secondary" @click="showRejectDialog = false">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-danger" :disabled="rejecting">
            {{ rejecting ? t('admin.research.reject.submitting') : t('admin.research.reject.confirm') }}
          </button>
        </div>
      </form>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'
import { adminAPI } from '@/api/admin'
import type { AdminResearchApplication } from '@/api/admin/research'
import {
  fetchAttachmentBlob,
  type ResearchApplicationStatus,
  type ResearchAttachment
} from '@/api/research'
import { formatBytes, formatDateTime } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const currencyStore = useCurrencyDisplayStore()

// ==================== List state ====================

const applications = ref<AdminResearchApplication[]>([])
const loading = ref(false)
const statusFilter = ref<ResearchApplicationStatus | ''>('')
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

let abortController: AbortController | null = null

const columns = computed<Column[]>(() => [
  { key: 'applicant', label: t('admin.research.columns.applicant') },
  { key: 'description', label: t('admin.research.columns.description') },
  { key: 'attachments', label: t('admin.research.columns.attachments') },
  { key: 'created_at', label: t('admin.research.columns.createdAt') },
  { key: 'status', label: t('admin.research.columns.status') },
  { key: 'actions', label: t('admin.research.columns.actions') }
])

const filterStatusOptions = computed(() => [
  { value: '', label: t('admin.research.filter.all') },
  { value: 'pending', label: t('admin.research.filter.pending') },
  { value: 'approved', label: t('admin.research.filter.approved') },
  { value: 'rejected', label: t('admin.research.filter.rejected') }
])

const loadApplications = async () => {
  if (abortController) {
    abortController.abort()
  }
  const currentController = new AbortController()
  abortController = currentController
  loading.value = true
  try {
    const response = await adminAPI.research.list(
      {
        status: statusFilter.value,
        page: pagination.page,
        page_size: pagination.page_size
      },
      { signal: currentController.signal }
    )
    if (currentController.signal.aborted) {
      return
    }
    applications.value = response.items
    pagination.total = response.total
  } catch (error: any) {
    if (
      currentController.signal.aborted ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(t('admin.research.list.loadFailed'))
    console.error('Error loading research applications:', error)
  } finally {
    if (abortController === currentController && !currentController.signal.aborted) {
      loading.value = false
      abortController = null
    }
  }
}

const handleFilterChange = () => {
  pagination.page = 1
  loadApplications()
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadApplications()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadApplications()
}

// ==================== Detail dialog ====================

const showDetailDialog = ref(false)
const detailApplication = ref<AdminResearchApplication | null>(null)
const previewingId = ref<string | null>(null)
const downloadingId = ref<string | null>(null)
const previewUrls = ref<Record<string, string>>({})

const isImageMime = (mime: string): boolean => mime.startsWith('image/')

const statusBadgeClass = (status: ResearchApplicationStatus): string => {
  switch (status) {
    case 'approved':
      return 'badge-success'
    case 'rejected':
      return 'badge-danger'
    default:
      return 'badge-warning'
  }
}

const applicantLabelOf = (application: AdminResearchApplication): string => {
  if (application.user?.email) return application.user.email
  return t('admin.research.applicantFallback', { id: application.user?.id ?? application.id })
}

const attachmentPath = (applicationId: string, attachmentId: string): string =>
  `/admin/research-applications/${encodeURIComponent(
    applicationId
  )}/attachments/${encodeURIComponent(attachmentId)}`

const previewUrlOf = (attachment: ResearchAttachment): string | undefined =>
  previewUrls.value[attachment.id]

const revokeAllPreviews = () => {
  for (const url of Object.values(previewUrls.value)) {
    URL.revokeObjectURL(url)
  }
  previewUrls.value = {}
}

const openDetail = (application: AdminResearchApplication) => {
  revokeAllPreviews()
  detailApplication.value = application
  showDetailDialog.value = true
}

const closeDetailDialog = () => {
  revokeAllPreviews()
  showDetailDialog.value = false
  detailApplication.value = null
}

const togglePreview = async (attachment: ResearchAttachment) => {
  const application = detailApplication.value
  if (!application) return

  if (previewUrls.value[attachment.id]) {
    const next = { ...previewUrls.value }
    URL.revokeObjectURL(next[attachment.id])
    delete next[attachment.id]
    previewUrls.value = next
    return
  }

  if (previewingId.value) return
  previewingId.value = attachment.id
  try {
    const { blob } = await fetchAttachmentBlob(
      attachmentPath(application.id, attachment.id)
    )
    previewUrls.value = { ...previewUrls.value, [attachment.id]: URL.createObjectURL(blob) }
  } catch (error) {
    console.error('Failed to preview research attachment:', error)
    appStore.showError(t('admin.research.detail.previewFailed'))
  } finally {
    previewingId.value = null
  }
}

const handleDownload = async (attachment: ResearchAttachment) => {
  const application = detailApplication.value
  if (!application || downloadingId.value) return
  downloadingId.value = attachment.id
  try {
    await adminAPI.research.downloadAttachment(application.id, attachment)
  } catch (error) {
    console.error('Failed to download research attachment:', error)
    appStore.showError(t('admin.research.detail.downloadFailed'))
  } finally {
    downloadingId.value = null
  }
}

// ==================== Review actions ====================

const showApproveDialog = ref(false)
const showRejectDialog = ref(false)
const approving = ref(false)
const rejecting = ref(false)
const approveForm = reactive({
  amount: 5,
  notes: ''
})
const rejectForm = reactive({
  notes: ''
})

const openApproveDialog = () => {
  approveForm.amount = 5
  approveForm.notes = ''
  showApproveDialog.value = true
}

const openRejectDialog = () => {
  rejectForm.notes = ''
  showRejectDialog.value = true
}

const handleApproveSubmit = async () => {
  if (!detailApplication.value) return
  const amount = Number(approveForm.amount)
  if (!Number.isFinite(amount) || amount <= 0) {
    appStore.showError(t('admin.research.approve.amountInvalid'))
    return
  }

  approving.value = true
  try {
    await adminAPI.research.approve(detailApplication.value.id, {
      amount,
      notes: approveForm.notes.trim() || undefined
    })
    appStore.showSuccess(t('admin.research.approve.success'))
    showApproveDialog.value = false
    closeDetailDialog()
    loadApplications()
  } catch (error) {
    console.error('Failed to approve research application:', error)
    appStore.showError(t('admin.research.approve.failed'))
  } finally {
    approving.value = false
  }
}

const handleRejectSubmit = async () => {
  if (!detailApplication.value) return
  const notes = rejectForm.notes.trim()
  if (!notes) {
    appStore.showError(t('admin.research.reject.notesRequired'))
    return
  }

  rejecting.value = true
  try {
    await adminAPI.research.reject(detailApplication.value.id, { notes })
    appStore.showSuccess(t('admin.research.reject.success'))
    showRejectDialog.value = false
    closeDetailDialog()
    loadApplications()
  } catch (error) {
    console.error('Failed to reject research application:', error)
    appStore.showError(t('admin.research.reject.failed'))
  } finally {
    rejecting.value = false
  }
}

onMounted(() => {
  loadApplications()
})

onUnmounted(() => {
  abortController?.abort()
  revokeAllPreviews()
})
</script>
