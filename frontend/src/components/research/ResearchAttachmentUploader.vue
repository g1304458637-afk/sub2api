<template>
  <div class="space-y-3">
    <div>
      <label class="input-label">{{ t('research.form.attachmentsLabel', { max: max }) }}</label>
      <p class="input-hint">{{ t('research.form.attachmentsHint') }}</p>
    </div>

    <!-- Drop zone / file picker trigger -->
    <div
      v-if="canAddMore"
      role="button"
      tabindex="0"
      class="flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors"
      :class="
        dragActive
          ? 'border-primary-400 bg-primary-50 dark:border-primary-500 dark:bg-primary-900/20'
          : 'border-gray-300 hover:border-primary-300 dark:border-dark-600 dark:hover:border-primary-600'
      "
      @click="openFilePicker"
      @keydown.enter.prevent="openFilePicker"
      @dragover.prevent="dragActive = true"
      @dragleave.prevent="dragActive = false"
      @drop.prevent="handleDrop"
    >
      <div
        class="mb-2 flex h-10 w-10 items-center justify-center rounded-xl bg-gray-100 dark:bg-dark-800"
      >
        <Icon name="upload" size="md" class="text-gray-400 dark:text-dark-500" />
      </div>
      <p class="text-sm text-gray-600 dark:text-gray-300">
        {{ t('research.form.uploadAreaText') }}
      </p>
      <input
        ref="fileInputRef"
        type="file"
        multiple
        :accept="acceptAttr"
        class="hidden"
        @change="handleFileChange"
      />
    </div>

    <!-- Upload in progress -->
    <div v-if="uploadingItems.length > 0" class="space-y-2">
      <div
        v-for="item in uploadingItems"
        :key="item.key"
        class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800"
      >
        <div class="flex items-center justify-between gap-3">
          <span class="min-w-0 flex-1 truncate text-sm text-gray-700 dark:text-gray-300">
            {{ item.name }}
          </span>
          <span class="flex-shrink-0 text-xs text-gray-500 dark:text-dark-400">
            {{ t('research.form.uploading', { percent: item.percent }) }}
          </span>
        </div>
        <div class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
          <div
            class="h-full rounded-full bg-primary-500 transition-all"
            :style="{ width: `${item.percent}%` }"
          ></div>
        </div>
      </div>
    </div>

    <!-- Uploaded attachments -->
    <div v-if="attachments.length > 0" class="flex flex-wrap gap-2">
      <span
        v-for="attachment in attachments"
        :key="attachment.id"
        class="inline-flex max-w-full items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
      >
        <Icon name="document" size="sm" class="flex-shrink-0 text-gray-400 dark:text-dark-500" />
        <span class="max-w-[12rem] truncate">{{ attachment.name }}</span>
        <span class="flex-shrink-0 text-gray-400 dark:text-dark-500">
          {{ formatBytes(attachment.size) }}
        </span>
        <button
          type="button"
          class="flex-shrink-0 rounded p-0.5 text-gray-400 transition-colors hover:text-red-500 dark:text-dark-500 dark:hover:text-red-400"
          :disabled="disabled"
          :title="t('research.form.removeAttachment')"
          @click="removeAttachment(attachment.id)"
        >
          <Icon name="x" size="sm" :stroke-width="2" />
        </button>
      </span>
    </div>

    <!-- Validation / upload errors -->
    <ul v-if="errors.length > 0" class="space-y-1">
      <li
        v-for="(error, index) in errors"
        :key="index"
        class="text-xs text-red-600 dark:text-red-400"
      >
        {{ error }}
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatBytes } from '@/utils/format'
import {
  researchAPI,
  RESEARCH_ALLOWED_ATTACHMENT_MIME_TYPES,
  RESEARCH_MAX_ATTACHMENT_SIZE,
  type ResearchAttachment
} from '@/api/research'

interface UploadingItem {
  key: string
  name: string
  percent: number
  file: File
}

const props = withDefaults(
  defineProps<{
    modelValue: ResearchAttachment[]
    max?: number
    disabled?: boolean
  }>(),
  {
    max: 5,
    disabled: false
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: ResearchAttachment[]): void
  (e: 'uploading-change', busy: boolean): void
}>()

const { t } = useI18n()

const fileInputRef = ref<HTMLInputElement | null>(null)
const dragActive = ref(false)
const uploadingItems = ref<UploadingItem[]>([])
const errors = ref<string[]>([])
// Local authoritative copy so concurrent upload completions never build on a
// stale snapshot of props.modelValue within the same tick.
const attachments = ref<ResearchAttachment[]>([...props.modelValue])

watch(
  () => props.modelValue,
  (value) => {
    attachments.value = [...value]
  }
)

let keySeq = 0

const acceptAttr = computed(() => RESEARCH_ALLOWED_ATTACHMENT_MIME_TYPES.join(','))

const canAddMore = computed(
  () =>
    !props.disabled &&
    attachments.value.length + uploadingItems.value.length < props.max
)

const isAllowedMime = (file: File): boolean =>
  (RESEARCH_ALLOWED_ATTACHMENT_MIME_TYPES as readonly string[]).includes(file.type)

function openFilePicker() {
  if (props.disabled) return
  errors.value = []
  fileInputRef.value?.click()
}

function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  handleFiles(input.files)
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
  }
}

function handleDrop(event: DragEvent) {
  dragActive.value = false
  if (props.disabled) return
  handleFiles(event.dataTransfer?.files ?? null)
}

function handleFiles(fileList: FileList | null) {
  if (!fileList || fileList.length === 0) return
  if (props.disabled) return

  const files = Array.from(fileList)
  const slotCount = attachments.value.length + uploadingItems.value.length
  const nextErrors: string[] = []
  const validFiles: File[] = []
  let exceededCount = false

  for (const file of files) {
    if (file.size > RESEARCH_MAX_ATTACHMENT_SIZE) {
      nextErrors.push(t('research.form.fileTooLarge', { name: file.name }))
    } else if (!isAllowedMime(file)) {
      nextErrors.push(t('research.form.fileTypeUnsupported', { name: file.name }))
    } else if (slotCount + validFiles.length >= props.max) {
      exceededCount = true
    } else {
      validFiles.push(file)
    }
  }

  if (exceededCount) {
    nextErrors.push(t('research.form.tooManyFiles', { max: props.max }))
  }

  errors.value = nextErrors
  if (validFiles.length > 0) {
    void uploadFiles(validFiles)
  }
}

async function uploadFiles(files: File[]) {
  const items: UploadingItem[] = files.map((file) => ({
    key: `research-upload-${++keySeq}`,
    name: file.name,
    percent: 0,
    file
  }))
  uploadingItems.value.push(...items)
  emitUploadingChange()

  await Promise.all(items.map((item) => uploadOne(item)))

  emitUploadingChange()
}

async function uploadOne(item: UploadingItem) {
  try {
    const attachment = await researchAPI.uploadAttachment(item.file, (percent) => {
      const target = uploadingItems.value.find((entry) => entry.key === item.key)
      if (target) {
        target.percent = percent
      }
    })
    attachments.value = [...attachments.value, attachment]
    emit('update:modelValue', attachments.value)
  } catch (error) {
    console.error('Failed to upload research attachment:', error)
    errors.value = [...errors.value, t('research.form.uploadFailed', { name: item.name })]
  } finally {
    uploadingItems.value = uploadingItems.value.filter((entry) => entry.key !== item.key)
  }
}

function emitUploadingChange() {
  emit('uploading-change', uploadingItems.value.length > 0)
}

function removeAttachment(id: string) {
  if (props.disabled) return
  attachments.value = attachments.value.filter((attachment) => attachment.id !== id)
  emit('update:modelValue', attachments.value)
}
</script>
