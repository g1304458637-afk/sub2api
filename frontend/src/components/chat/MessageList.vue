<template>
  <div
    ref="containerRef"
    class="min-h-0 flex-1 overflow-y-auto"
    @scroll.passive="handleScroll"
  >
    <!-- Welcome state (empty conversation) -->
    <div
      v-if="messages.length === 0"
      class="flex h-full flex-col items-center justify-center px-4 py-10 text-center"
    >
      <!-- 校名标识行 -->
      <p
        class="mb-3 flex items-center gap-2.5 text-xs font-medium tracking-[0.2em] text-primary-600/80 dark:text-primary-400/80"
      >
        <span class="inline-block h-px w-6 bg-primary-300/70 dark:bg-primary-700/60"></span>
        {{ t('chat.welcome.motto') }}
        <span class="inline-block h-px w-6 bg-primary-300/70 dark:bg-primary-700/60"></span>
      </p>
      <h2
        class="font-serif text-[28px] font-semibold leading-snug text-gray-900 dark:text-white"
      >
        {{ t('chat.welcome.title') }}
      </h2>
      <p class="mt-2.5 max-w-md text-sm leading-relaxed text-gray-400 dark:text-dark-500">
        {{ t('chat.welcome.hint') }}
      </p>

      <!-- 建议问题 -->
      <div class="mt-7 flex flex-wrap items-center justify-center gap-2">
        <button
          v-for="suggestion in suggestions"
          :key="suggestion"
          type="button"
          class="rounded-full border border-gray-200 bg-white px-3.5 py-1.5 text-[13px] text-gray-500 transition-all hover:-translate-y-px hover:border-primary-300 hover:text-primary-700 focus:outline-none dark:border-dark-600 dark:bg-dark-900 dark:text-dark-400 dark:hover:border-primary-700 dark:hover:text-primary-300"
          @click="emit('suggest', suggestion)"
        >
          {{ suggestion }}
        </button>
      </div>
    </div>

    <!-- Messages -->
    <div v-else class="mx-auto w-full max-w-3xl px-4 py-6">
      <div class="space-y-5">
        <MessageItem
          v-for="(message, index) in messages"
          :key="index"
          :message="message"
          :streaming="streaming && index === messages.length - 1"
          :show-regenerate="index === lastAssistantIndex && !streaming && !message.error"
          :error-text="errorTexts[index]"
          @regenerate="emit('regenerate')"
          @retry="emit('retry', index)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MessageItem from './MessageItem.vue'
import type { ChatMessage } from './types'

const props = withDefaults(
  defineProps<{
    messages: ChatMessage[]
    streaming?: boolean
    errorTexts?: Record<number, string>
  }>(),
  {
    streaming: false,
    errorTexts: () => ({}),
  }
)

const emit = defineEmits<{
  (e: 'regenerate'): void
  (e: 'retry', index: number): void
  (e: 'suggest', text: string): void
}>()

const { t } = useI18n()

const suggestions = computed(() => [
  t('chat.welcome.suggestion1'),
  t('chat.welcome.suggestion2'),
  t('chat.welcome.suggestion3'),
])

const containerRef = ref<HTMLElement | null>(null)
const stickToBottom = ref(true)

const lastAssistantIndex = computed(() => {
  for (let i = props.messages.length - 1; i >= 0; i--) {
    const message = props.messages[i]
    if (message && message.role === 'assistant') {
      return i
    }
  }
  return -1
})

function scrollToBottom(): void {
  const el = containerRef.value
  if (!el) {
    return
  }
  el.scrollTop = el.scrollHeight
}

function handleScroll(): void {
  const el = containerRef.value
  if (!el) {
    return
  }
  stickToBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 80
}

watch(
  () => [props.messages.length, props.messages[props.messages.length - 1]?.content.length],
  () => {
    if (!stickToBottom.value) {
      return
    }
    void nextTick(scrollToBottom)
  }
)

onMounted(() => {
  stickToBottom.value = true
  nextTick(scrollToBottom)
})
</script>
