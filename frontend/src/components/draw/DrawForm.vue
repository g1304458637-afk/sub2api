<template>
  <div class="card space-y-5">
    <!-- 模型 -->
    <div>
      <label class="input-label">{{ t('draw.form.model') }}</label>
      <Select
        :model-value="model"
        :options="modelOptions"
        :placeholder="t('draw.form.modelPlaceholder')"
        :empty-text="t('draw.form.modelEmpty')"
        @update:model-value="$emit('update:model', String($event ?? ''))"
      />
      <p v-if="selectedModel" class="mt-1.5 text-xs text-gray-400 dark:text-gray-500">
        <span v-if="selectedModel.vendor" class="mr-1.5">{{ selectedModel.vendor }}</span>
        <span v-if="selectedModel.description">{{ selectedModel.description }}</span>
      </p>
    </div>

    <!-- 尺寸 -->
    <div>
      <label class="input-label">{{ t('draw.form.size') }}</label>
      <div
        class="inline-flex w-full max-w-sm rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900/40"
        role="radiogroup"
        :aria-label="t('draw.form.size')"
      >
        <button
          v-for="option in DRAW_SIZE_OPTIONS"
          :key="option.value"
          type="button"
          role="radio"
          :aria-checked="size === option.value"
          class="inline-flex flex-1 flex-col items-center justify-center gap-0.5 rounded-md px-2 py-1.5 text-xs font-medium transition"
          :class="
            size === option.value
              ? 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
              : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
          "
          @click="$emit('update:size', option.value)"
        >
          <span>{{ t(option.labelKey) }}</span>
          <span class="font-mono text-[10px] font-normal opacity-70">{{ option.value }}</span>
        </button>
      </div>
    </div>

    <!-- 张数 -->
    <div>
      <label class="input-label">{{ t('draw.form.count') }}</label>
      <div class="inline-flex items-center gap-2" role="radiogroup" :aria-label="t('draw.form.count')">
        <button
          v-for="n in DRAW_COUNT_MAX"
          :key="n"
          type="button"
          role="radio"
          :aria-checked="count === n"
          class="inline-flex h-9 w-9 items-center justify-center rounded-lg border text-sm font-medium transition"
          :class="
            count === n
              ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-500 dark:bg-primary-900/30 dark:text-primary-300'
              : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-dark-300 dark:hover:border-dark-500'
          "
          @click="$emit('update:count', n)"
        >
          {{ n }}
        </button>
      </div>
    </div>

    <!-- 提示词 -->
    <TextArea
      :model-value="prompt"
      :label="t('draw.form.prompt')"
      :required="true"
      :rows="4"
      :placeholder="t('draw.form.promptPlaceholder')"
      :error="promptErrorText"
      @update:model-value="$emit('update:prompt', String($event ?? ''))"
    />

    <!-- 生成按钮 -->
    <button
      type="button"
      class="btn btn-primary w-full"
      :disabled="generating || modelOptions.length === 0"
      @click="$emit('submit')"
    >
      <Icon
        :name="generating ? 'refresh' : 'sparkles'"
        size="sm"
        class="mr-2"
        :class="generating ? 'animate-spin' : ''"
      />
      {{ generating ? t('draw.form.generating') : t('draw.form.generate') }}
    </button>
  </div>
</template>

<script setup lang="ts">
/**
 * 绘图表单：模型 / 尺寸 / 张数 / 提示词 / 生成按钮。
 * 状态由 DrawView 持有，本组件只做展示与回传（update:* + submit）。
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WebChatModelInfo } from '@/api/webChat'
import Select from '@/components/common/Select.vue'
import TextArea from '@/components/common/TextArea.vue'
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

defineEmits<{
  (e: 'update:model', value: string): void
  (e: 'update:size', value: string): void
  (e: 'update:count', value: number): void
  (e: 'update:prompt', value: string): void
  (e: 'submit'): void
}>()

const { t } = useI18n()

const modelOptions = computed(() =>
  props.models.map((m) => ({ value: m.model, label: m.display_name || m.model })),
)

const selectedModel = computed(
  () => props.models.find((m) => m.model === props.model) ?? null,
)

const promptErrorText = computed(() =>
  props.showPromptError && !props.prompt.trim() ? t('draw.form.promptRequired') : '',
)
</script>
