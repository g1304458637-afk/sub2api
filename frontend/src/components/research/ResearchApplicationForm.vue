<template>
  <form class="space-y-5" @submit.prevent="handleSubmit">
    <div>
      <label for="research-description" class="input-label">
        {{ t('research.form.descriptionLabel') }}
      </label>
      <textarea
        id="research-description"
        v-model="description"
        rows="5"
        required
        :placeholder="t('research.form.descriptionPlaceholder')"
        :disabled="submitting"
        class="input mt-1 resize-y"
      ></textarea>
      <div class="mt-1 flex items-center justify-between gap-3">
        <p v-if="descriptionTooLong" class="input-hint text-red-600 dark:text-red-400">
          {{ t('research.form.descriptionTooLong', { max: maxDescriptionLength }) }}
        </p>
        <span v-else class="flex-1"></span>
        <span
          :class="[
            'flex-shrink-0 font-mono text-xs',
            descriptionTooLong ? 'text-red-600 dark:text-red-400' : 'text-gray-400 dark:text-dark-500'
          ]"
        >
          {{ t('research.form.charCount', { count: description.length, max: maxDescriptionLength }) }}
        </span>
      </div>
    </div>

    <ResearchAttachmentUploader
      v-model="attachments"
      :disabled="submitting"
      @uploading-change="attachmentsUploading = $event"
    />

    <button type="submit" :disabled="!canSubmit" class="btn btn-primary w-full py-3">
      <svg
        v-if="submitting"
        class="-ml-1 mr-2 h-5 w-5 animate-spin"
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
      <Icon v-else name="checkCircle" size="md" class="mr-2" />
      {{ submitting ? t('research.form.submitting') : t('research.form.submit') }}
    </button>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'
import ResearchAttachmentUploader from './ResearchAttachmentUploader.vue'
import {
  researchAPI,
  RESEARCH_MAX_DESCRIPTION_LENGTH,
  type ResearchApplication,
  type ResearchAttachment
} from '@/api/research'

const emit = defineEmits<{
  (e: 'submitted', application: ResearchApplication): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const maxDescriptionLength = RESEARCH_MAX_DESCRIPTION_LENGTH

const description = ref('')
const attachments = ref<ResearchAttachment[]>([])
const attachmentsUploading = ref(false)
const submitting = ref(false)

const descriptionTooLong = computed(() => description.value.length > maxDescriptionLength)

const canSubmit = computed(
  () =>
    !!description.value.trim() &&
    !descriptionTooLong.value &&
    !attachmentsUploading.value &&
    !submitting.value
)

const handleSubmit = async () => {
  if (!description.value.trim()) {
    appStore.showError(t('research.form.descriptionRequired'))
    return
  }
  if (descriptionTooLong.value) {
    appStore.showError(t('research.form.descriptionTooLong', { max: maxDescriptionLength }))
    return
  }
  if (attachmentsUploading.value) {
    appStore.showError(t('research.form.attachmentsUploading'))
    return
  }

  submitting.value = true
  try {
    const application = await researchAPI.submitApplication(
      description.value.trim(),
      attachments.value.map((attachment) => attachment.id)
    )

    // Reset the form for a potential next submission
    description.value = ''
    attachments.value = []

    emit('submitted', application)
    appStore.showSuccess(t('research.form.submitSuccess'))
  } catch (error) {
    console.error('Failed to submit research application:', error)
    appStore.showError(t('research.form.submitFailed'))
  } finally {
    submitting.value = false
  }
}
</script>
