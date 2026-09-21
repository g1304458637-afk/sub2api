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
      :disabled="generating"
      @input="onPromptInput"
    ></textarea>

    <!-- 提示词必填错误 -->
    <p v-if="promptErrorText" class="mt-1 px-2 text-xs text-red-500 dark:text-red-400">
      {{ promptErrorText }}
    </p>

    <!-- 编辑上下文 chip：基于选中图片继续编辑（× 可清除回到全新生成） -->
    <div
      v-if="contextImage"
      class="mx-1 mb-1 mt-1 inline-flex max-w-full items-center gap-2 rounded-2xl bg-gray-100 p-1 pr-2 dark:bg-dark-800"
    >
      <img
        :src="contextImage.src"
        :alt="contextImage.prompt"
        class="h-16 w-16 shrink-0 rounded-xl object-cover"
      />
      <span class="min-w-0 flex-1 truncate text-xs text-gray-500 dark:text-dark-400">
        {{ t('draw.form.editWithContext') }}
      </span>
      <button
        type="button"
        class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-gray-400 transition-colors hover:bg-gray-200/70 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-200"
        :aria-label="t('draw.form.clearContext')"
        :title="t('draw.form.clearContext')"
        :disabled="generating"
        @click="$emit('clear-context')"
      >
        <Icon name="x" size="xs" />
      </button>
    </div>

    <!-- 控件行：模型 / 尺寸 / 张数 / 生成 -->
    <div class="mt-1 flex flex-wrap items-center gap-2 px-1">
      <!-- 模型 chip（点击打开选择弹窗） -->
      <button
        type="button"
        class="flex max-w-[12rem] items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-60 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
        :aria-label="t('draw.form.model')"
        :disabled="generating || modelOptions.length === 0"
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
          class="rounded-full px-3 py-1 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-60"
          :class="
            size === option.value
              ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
              : 'text-gray-500 hover:text-gray-900 dark:text-dark-400 dark:hover:text-white'
          "
          :disabled="generating"
          @click="$emit('update:size', option.value)"
        >
          {{ t(option.labelKey) }}
        </button>
      </div>

      <!-- 张数（编辑模式下隐藏：按上游惯例固定 n=1） -->
      <div
        v-if="!contextImage"
        class="inline-flex items-center gap-1"
        role="radiogroup"
        :aria-label="t('draw.form.count')"
      >
        <button
          v-for="n in DRAW_COUNT_MAX"
          :key="n"
          type="button"
          role="radio"
          :aria-checked="count === n"
          :title="`${t('draw.form.count')} × ${n}`"
          class="inline-flex h-7 w-7 items-center justify-center rounded-full text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-60"
          :class="
            count === n
              ? 'bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900'
              : 'text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800'
          "
          :disabled="generating"
          @click="$emit('update:count', n)"
        >
          {{ n }}
        </button>
      </div>

      <!-- 生成 / 继续编辑按钮 -->
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
        {{ submitLabel }}
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
 * 传入 contextImage 时进入编辑模式：胶囊顶部展示上下文 chip（× 清除回到全新生成），
 * 生成按钮文案切换为“继续编辑”，张数控件隐藏（固定 n=1）。
 * 状态由 DrawView 持有，本组件只做展示与回传（update:* + clear-context + submit）。
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WebChatModelInfo } from '@/api/webChat'
import ModelPickerModal from '@/components/chat/ModelPickerModal.vue'
import Icon from '@/components/icons/Icon.vue'
import { DRAW_SIZE_OPTIONS, DRAW_COUNT_MAX, type DrawImageItem } from './types'

interface Props {
  models: WebChatModelInfo[]
  model: string
  size: string
  count: number
  prompt: string
  generating: boolean
  /** 提交过但提示词为空时显示必填错误 */
  showPromptError: boolean
  /** 当前选为编辑上下文的图片；存在时按钮变为“继续编辑”且隐藏张数控件 */
  contextImage?: DrawImageItem | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:model', value: string): void
  (e: 'update:size', value: string): void
  (e: 'update:count', value: number): void
  (e: 'update:prompt', value: string): void
  (e: 'clear-context'): void
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

const submitLabel = computed(() => {
  if (props.generating) return t('draw.form.generating')
  return props.contextImage ? t('draw.form.continueEdit') : t('draw.form.generate')
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
