<template>
  <div class="group w-full">
    <!-- User message: right-aligned bubble -->
    <div v-if="message.role === 'user'" class="flex justify-end">
      <div
        class="max-w-[85%] whitespace-pre-wrap break-words rounded-2xl bg-gradient-to-r from-primary-500 to-primary-600 px-4 py-2.5 text-sm leading-relaxed text-white shadow-sm"
      >
        {{ message.content }}
      </div>
    </div>

    <!-- Assistant message: open text, no bubble box -->
    <div v-else class="flex justify-start">
      <div class="relative min-w-0 max-w-[92%] py-0.5">
        <!-- Error bar -->
        <div
          v-if="message.error"
          class="mb-2 flex items-start justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300"
        >
          <p class="min-w-0 break-words">
            {{ errorText || t('chat.error.responseFailed') }}
          </p>
          <button
            type="button"
            class="flex shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium transition-colors hover:bg-red-100 focus:outline-none dark:hover:bg-red-900/40"
            @click="emit('retry')"
          >
            <Icon name="refresh" size="xs" />
            {{ t('chat.message.retry') }}
          </button>
        </div>

        <!-- Markdown content (sanitized with DOMPurify) -->
        <div
          v-if="renderedHtml"
          class="markdown-body text-sm"
          v-html="renderedHtml"
        ></div>
        <div v-else-if="streaming" class="flex items-center gap-1 py-1" aria-hidden="true">
          <span class="h-2 w-2 rounded-full bg-gray-300 dark:bg-dark-600"></span>
          <span class="h-2 w-2 rounded-full bg-gray-300 dark:bg-dark-600"></span>
          <span class="h-2 w-2 rounded-full bg-gray-300 dark:bg-dark-600"></span>
        </div>

        <!-- Streaming cursor -->
        <span
          v-if="streaming"
          class="chat-stream-cursor ml-0.5 inline-block h-4 w-2 rounded-[2px] bg-gray-400 align-text-bottom dark:bg-dark-400"
          aria-hidden="true"
        ></span>

        <!-- Copy button (hover) -->
        <button
          v-if="message.content"
          type="button"
          class="absolute -top-1 right-0 hidden rounded-lg p-1.5 text-gray-300 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none group-hover:block dark:text-dark-600 dark:hover:bg-dark-800 dark:hover:text-dark-300"
          :title="t('chat.message.copy')"
          :aria-label="t('chat.message.copy')"
          @click="copyContent"
        >
          <Icon name="copy" size="sm" />
        </button>

        <!-- Regenerate (last assistant message only) -->
        <div v-if="showRegenerate && !streaming" class="mt-1 flex">
          <button
            type="button"
            class="flex items-center gap-1.5 rounded-lg px-1.5 py-1 text-xs text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none dark:text-dark-500 dark:hover:bg-dark-800 dark:hover:text-dark-200"
            @click="emit('regenerate')"
          >
            <Icon name="refresh" size="xs" />
            {{ t('chat.message.regenerate') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import type { ChatMessage } from './types'
import '@/styles/announcement-markdown.css'

const props = withDefaults(
  defineProps<{
    message: ChatMessage
    streaming?: boolean
    showRegenerate?: boolean
    errorText?: string
  }>(),
  {
    streaming: false,
    showRegenerate: false,
    errorText: '',
  }
)

const emit = defineEmits<{
  (e: 'regenerate'): void
  (e: 'retry'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

marked.setOptions({
  breaks: true,
  gfm: true,
})

const renderedHtml = computed(() => {
  if (props.message.role !== 'assistant' || !props.message.content) {
    return ''
  }
  const html = marked.parse(props.message.content) as string
  return DOMPurify.sanitize(html)
})

async function copyContent(): Promise<void> {
  try {
    await navigator.clipboard.writeText(props.message.content)
    appStore.showSuccess(t('chat.message.copied'))
  } catch {
    appStore.showError(t('chat.message.copyFailed'))
  }
}
</script>

<style scoped>
.chat-stream-cursor {
  animation: chat-cursor-blink 1s step-end infinite;
}

@keyframes chat-cursor-blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0;
  }
}
</style>
