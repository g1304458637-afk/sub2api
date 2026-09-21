<template>
  <div
    class="rounded-3xl border border-gray-200/90 bg-white p-3 shadow-lg shadow-gray-200/50 transition-colors focus-within:border-primary-500/60 dark:border-dark-600 dark:bg-dark-900 dark:shadow-black/20 dark:focus-within:border-primary-500/60"
  >
    <!-- 提示词 -->
    <textarea
      :value="prompt"
      rows="3"
      class="w-full resize-none bg-transparent px-2 pt-1.5 text-sm leading-relaxed text-gray-900 placeholder:text-gray-400 focus:outline-none disabled:cursor-not-allowed dark:text-gray-100 dark:placeholder:text-dark-500"
      :placeholder="t('draw.form.promptPlaceholder')"
      :aria-label="t('draw.form.prompt')"
      @input="onPromptInput"
    ></textarea>

    <!-- 提示词必填错误 -->
    <p v-if="promptErrorText" class="mt-1 px-2 text-xs text-red-500 dark:text-red-400">
      {{ promptErrorText }}
    </p>

    <!-- 控件行：模型 / 尺寸 / 张数 / 生成 -->
    <div class="mt-1 flex flex-wrap items-center gap-2 px-1">
      <!-- 模型 chip（点击打开选择弹窗） -->
      <button
        type="button"
        class="flex max-w-[12rem] items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-60 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
        :aria-label="t('draw.form.model')"
        :disabled="modelOptions.length === 0"
        @click="pickerOpen = true"
      >
        <Icon name="cube" size="sm" class="shrink-0 text-primary-500/80 dark:text-primary-400/80" />
        <span class="truncate">{{ modelLabel }}</span>
        <Icon name="chevronDown" size="xs" class="shrink-0 text-gray-400 dark:text-dark-500" />
      </button>

      <!-- 尺寸 -->
      <div
        class="inline-flex rounded-full bg-gray-100 p-1 dark:bg-dark-800"
        role="radiogroup"
        :aria-label="t('draw.form.size')"
      >
        <button
          v-for="option in DRAW_SIZE_OPTIONS"
          :key="option.value"
          type="button"
          role="radio"
          :aria-checked="size === option.value"
          :title="option.value"
          class="rounded-full px-3 py-1 text-xs font-medium transition"
          :class="
            size === option.value
              ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
              : 'text-gray-500 hover:text-gray-900 dark:text-dark-400 dark:hover:text-white'
          "
          @click="$emit('update:size', option.value)"
        >
          {{ t(option.labelKey) }}
        </button>
      </div>

      <!-- 张数 -->
      <div class="inline-flex items-center gap-1" role="radiogroup" :aria-label="t('draw.form.count')">
        <button
          v-for="n in DRAW_COUNT_MAX"
          :key="n"
          type="button"
          role="radio"
          :aria-checked="count === n"
          :title="`${t('draw.form.count')} × ${n}`"
          class="inline-flex h-7 w-7 items-center justify-center rounded-full text-xs font-medium transition"
          :class="
            count === n
              ? 'bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900'
              : 'text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800'
          "
          @click="$emit('update:count', n)"
        >
          {{ n }}
        </button>
      </div>

      <!-- 生成按钮 -->
      <button
        type="button"
        class="btn btn-primary ml-auto h-9 rounded-full px-4"
        :disabled="generating || modelOptions.length === 0"
        @click="$emit('submit')"
      >
        <Icon
          :name="generating ? 'refresh' : 'sparkles'"
          size="sm"
          class="mr-1.5"
          :class="generating ? 'animate-spin' : ''"
        />
        {{ generating ? t('draw.form.generating') : t('draw.form.generate') }}
      </button>
    </div>

    <!-- 模型选择弹窗（与聊天页共用） -->
    <ModelPickerModal
      :show="pickerOpen"
      :models="models"
      :selected="model"
      @select="onSelectModel"
      @close="pickerOpen = false"
    />
  </div>
</template>

<script setup lang="ts">
/**
 * 绘图输入区：与聊天页同一套扁平风格——单个输入胶囊内含提示词与内联控件
 * （模型 chip + 尺寸/张数胶囊 + 生成按钮），不再有独立表单卡片。
 * 状态由 DrawView 持有，本组件只做展示与回传（update:* + submit）。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WebChatModelInfo } from '@/api/webChat'
import ModelPickerModal from '@/components/chat/ModelPickerModal.vue'
import Icon from '@/components/icons/Icon.vue'
import { DRAW_SIZE_OPTIONS, DRAW_COUNT_MAX } from './types'

interface Props {
  models: WebChatModelInfo[]
  model: string
  size: string
  count: number
  prompt: string
  generating: boolean
  /** 提交过但提示词为空时显示必填错误 */
  showPromptError: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:model', value: string): void
  (e: 'update:size', value: string): void
  (e: 'update:count', value: number): void
  (e: 'update:prompt', value: string): void
  (e: 'submit'): void
}>()

const { t } = useI18n()

const pickerOpen = ref(false)

const modelOptions = computed(() =>
  props.models.map((m) => ({ value: m.model, label: m.display_name || m.model })),
)

const modelLabel = computed(() => {
  if (!props.model) return t('draw.form.modelPlaceholder')
  return props.models.find((m) => m.model === props.model)?.display_name || props.model
})

const promptErrorText = computed(() =>
  props.showPromptError && !props.prompt.trim() ? t('draw.form.promptRequired') : '',
)

function onPromptInput(event: Event): void {
  emit('update:prompt', String((event.target as HTMLTextAreaElement | null)?.value ?? ''))
}

function onSelectModel(value: string): void {
  emit('update:model', value)
  pickerOpen.value = false
}
</script>
