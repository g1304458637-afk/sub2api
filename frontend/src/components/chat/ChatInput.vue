<template>
  <div class="flex items-end gap-2 rounded-3xl border border-gray-200/90 bg-white p-2 shadow-lg shadow-gray-200/50 transition-colors focus-within:border-primary-500/60 dark:border-dark-600 dark:bg-dark-900 dark:shadow-black/20 dark:focus-within:border-primary-500/60">
    <textarea
      ref="textareaRef"
      v-model="text"
      rows="1"
      class="max-h-[200px] min-h-[40px] w-full flex-1 resize-none bg-transparent px-2 py-2 text-sm leading-relaxed text-gray-900 placeholder:text-gray-400 focus:outline-none disabled:cursor-not-allowed dark:text-gray-100 dark:placeholder:text-dark-500"
      :placeholder="placeholder"
      :disabled="disabled"
      @keydown="handleKeydown"
      @input="resize"
    ></textarea>
    <button
      v-if="streaming"
      type="button"
      class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gray-800 text-white transition-colors hover:bg-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:bg-gray-200 dark:text-gray-900 dark:hover:bg-white"
      :title="t('chat.input.stop')"
      :aria-label="t('chat.input.stop')"
      @click="emit('stop')"
    >
      <span class="h-3 w-3 rounded-[2px] bg-current" aria-hidden="true"></span>
    </button>
    <button
      v-else
      type="button"
      class="btn btn-primary h-9 w-9 shrink-0 !px-0"
      :title="t('chat.input.send')"
      :aria-label="t('chat.input.send')"
      :disabled="disabled || !canSend"
      @click="submit"
    >
      <Icon name="arrowUp" size="sm" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(
  defineProps<{
    disabled?: boolean
    streaming?: boolean
    placeholder?: string
  }>(),
  {
    disabled: false,
    streaming: false,
    placeholder: '',
  }
)

const emit = defineEmits<{
  (e: 'send', text: string): void
  (e: 'stop'): void
}>()

const { t } = useI18n()

const MAX_TEXTAREA_HEIGHT = 200

const text = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const canSend = computed(() => text.value.trim().length > 0)

function resize(): void {
  const el = textareaRef.value
  if (!el) {
    return
  }
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, MAX_TEXTAREA_HEIGHT)}px`
}

watch(text, () => {
  void nextTick(resize)
})

function submit(): void {
  const value = text.value.trim()
  if (!value || props.disabled || props.streaming) {
    return
  }
  emit('send', value)
  text.value = ''
  void nextTick(resize)
}

function handleKeydown(event: KeyboardEvent): void {
  // Enter sends; Shift+Enter inserts a newline; IME composing Enter is ignored
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault()
    submit()
  }
}

function focus(): void {
  textareaRef.value?.focus()
}

function setText(value: string): void {
  text.value = value
}

defineExpose({ focus, setText })
</script>
