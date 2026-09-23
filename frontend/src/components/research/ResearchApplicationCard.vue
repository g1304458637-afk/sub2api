<template>
  <div
    class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800"
    :class="{
      'border border-emerald-200 dark:border-emerald-800/50': application.status === 'approved',
      'border border-red-200 dark:border-red-800/50': application.status === 'rejected'
    }"
  >
    <div class="space-y-3">
      <!-- Header: status + submitted time -->
      <div class="flex flex-wrap items-center justify-between gap-2">
        <ResearchStatusBadge :status="application.status" />
        <span class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('research.list.submittedAt', { time: formatDateTime(application.created_at) }) }}
        </span>
      </div>

      <!-- Description (collapsible when long) -->
      <div>
        <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
          {{ t('research.list.descriptionLabel') }}
        </p>
        <p
          class="mt-1 whitespace-pre-wrap break-words text-sm text-gray-700 dark:text-gray-300"
          :class="{ 'line-clamp-3': !expanded && isLongDescription }"
        >
          {{ application.description }}
        </p>
        <button
          v-if="isLongDescription"
          type="button"
          class="mt-1 text-xs font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          @click="expanded = !expanded"
        >
          {{ expanded ? t('research.list.collapse') : t('research.list.expand') }}
        </button>
      </div>

      <!-- Review notes -->
      <div v-if="isReviewed" class="rounded-lg bg-white p-3 dark:bg-dark-900/40">
        <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
          {{ t('research.list.reviewNotesLabel') }}
        </p>
        <p class="mt-1 whitespace-pre-wrap break-words text-sm text-gray-700 dark:text-gray-300">
          {{ application.review_notes || t('research.list.noReviewNotes') }}
        </p>
        <p v-if="application.reviewed_at" class="mt-1 text-xs text-gray-400 dark:text-dark-500">
          {{ t('research.list.reviewedAt', { time: formatDateTime(application.reviewed_at) }) }}
        </p>
      </div>

      <!-- Granted reward -->
      <div
        v-if="application.status === 'approved' && application.reward_amount !== null"
        class="flex items-center justify-between rounded-lg bg-emerald-50 px-3 py-2 dark:bg-emerald-900/20"
      >
        <span class="text-sm font-medium text-emerald-800 dark:text-emerald-300">
          {{ t('research.list.rewardGranted') }}
        </span>
        <span class="text-sm font-semibold text-emerald-600 dark:text-emerald-400">
          +{{ currencyStore.formatCNY(application.reward_amount) }}
        </span>
      </div>

      <!-- Attachment chips -->
      <div v-if="application.attachments.length > 0">
        <p class="text-xs font-medium text-gray-500 dark:text-dark-400">
          {{ t('research.list.attachmentsLabel', { count: application.attachments.length }) }}
        </p>
        <div class="mt-2 flex flex-wrap gap-2">
          <button
            v-for="attachment in application.attachments"
            :key="attachment.id"
            type="button"
            class="inline-flex max-w-full items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-600 transition-colors hover:border-primary-300 hover:text-primary-600 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-primary-600 dark:hover:text-primary-400"
            :disabled="downloadingId !== null"
            :title="t('research.list.download')"
            @click="handleDownload(attachment)"
          >
            <Icon name="document" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
            <span class="max-w-[10rem] truncate">{{ attachment.name }}</span>
            <span class="flex-shrink-0 text-gray-400 dark:text-dark-500">
              {{ formatBytes(attachment.size) }}
            </span>
            <svg
              v-if="downloadingId === attachment.id"
              class="h-3.5 w-3.5 flex-shrink-0 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            <Icon v-else name="download" size="sm" class="flex-shrink-0" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'
import Icon from '@/components/icons/Icon.vue'
import ResearchStatusBadge from './ResearchStatusBadge.vue'
import { researchAPI, type ResearchApplication, type ResearchAttachment } from '@/api/research'
import { formatBytes, formatDateTime } from '@/utils/format'

const DESCRIPTION_PREVIEW_LENGTH = 160

const props = defineProps<{
  application: ResearchApplication
}>()

const { t } = useI18n()
const appStore = useAppStore()
const currencyStore = useCurrencyDisplayStore()

const expanded = ref(false)
const downloadingId = ref<string | null>(null)

const isLongDescription = computed(
  () => props.application.description.length > DESCRIPTION_PREVIEW_LENGTH
)

const isReviewed = computed(() => props.application.status !== 'pending')

const handleDownload = async (attachment: ResearchAttachment) => {
  if (downloadingId.value) return
  downloadingId.value = attachment.id
  try {
    await researchAPI.downloadAttachment(props.application.id, attachment)
  } catch (error) {
    console.error('Failed to download research attachment:', error)
    appStore.showError(t('research.list.downloadFailed'))
  } finally {
    downloadingId.value = null
  }
}
</script>
