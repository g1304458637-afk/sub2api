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
      <div
        class="mb-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary-50 dark:bg-primary-900/20"
      >
        <Icon name="sparkles" size="lg" class="text-primary-500 dark:text-primary-400" />
      </div>
      <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
        {{ t('chat.welcome.title') }}
      </h2>
      <p class="mt-2 max-w-md text-sm leading-relaxed text-gray-500 dark:text-dark-400">
        {{ t('chat.welcome.hint') }}
      </p>
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
import Icon from '@/components/icons/Icon.vue'
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
}>()

const { t } = useI18n()

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
