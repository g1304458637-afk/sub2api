<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.features.webChat.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.features.webChat.description') }}
      </p>
    </div>

    <div class="space-y-5 p-6">
      <!-- 总开关 -->
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.features.webChat.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.features.webChat.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="enabled" />
      </div>

      <template v-if="enabled">
        <!-- 默认模型 -->
        <div>
          <label class="input-label">
            {{ t('admin.settings.features.webChat.defaultModel') }}
          </label>
          <Select
            v-model="defaultModel"
            class="max-w-md"
            :options="chatModelOptions"
            :placeholder="t('admin.settings.features.webChat.defaultModelPlaceholder')"
            :empty-text="t('admin.settings.features.webChat.defaultModelEmpty')"
            searchable
            creatable
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.features.webChat.defaultModelHint') }}
          </p>
        </div>

        <!-- 模型配置表 -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <div>
              <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.features.webChat.modelsTitle') }}
              </p>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.features.webChat.modelsHint') }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm shrink-0" @click="addModel">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.settings.features.webChat.addModel') }}
            </button>
          </div>

          <div
            v-if="rows.length === 0"
            class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-400 dark:border-dark-600 dark:text-gray-500"
          >
            {{ t('admin.settings.features.webChat.empty') }}
          </div>

          <div v-else class="overflow-x-auto">
            <table class="min-w-[900px] w-full text-sm">
              <thead>
                <tr class="text-left text-xs text-gray-500 dark:text-gray-400">
                  <th class="pb-2 pr-3 font-medium">{{ t('admin.settings.features.webChat.col.model') }}</th>
                  <th class="pb-2 pr-3 font-medium">{{ t('admin.settings.features.webChat.col.displayName') }}</th>
                  <th class="pb-2 pr-3 font-medium">{{ t('admin.settings.features.webChat.col.vendor') }}</th>
                  <th class="pb-2 pr-3 font-medium">{{ t('admin.settings.features.webChat.col.description') }}</th>
                  <th class="pb-2 pr-3 font-medium">{{ t('admin.settings.features.webChat.col.type') }}</th>
                  <th class="pb-2 pr-3 text-center font-medium">{{ t('admin.settings.features.webChat.col.apiOnly') }}</th>
                  <th class="pb-2 text-right font-medium">{{ t('admin.settings.features.webChat.col.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(row, index) in rows"
                  :key="index"
                  class="align-top"
                >
                  <td class="py-1.5 pr-3">
                    <Select
                      :model-value="row.model"
                      class="min-w-[180px]"
                      :options="availableModelOptions"
                      :placeholder="t('admin.settings.features.webChat.modelPlaceholder')"
                      searchable
                      creatable
                      @update:model-value="row.model = String($event ?? '')"
                    />
                  </td>
                  <td class="py-1.5 pr-3">
                    <input
                      v-model="row.display_name"
                      type="text"
                      class="input h-9 w-32 text-sm"
                      :placeholder="t('admin.settings.features.webChat.displayNamePlaceholder')"
                    />
                  </td>
                  <td class="py-1.5 pr-3">
                    <input
                      v-model="row.vendor"
                      type="text"
                      class="input h-9 w-28 text-sm"
                      :placeholder="t('admin.settings.features.webChat.vendorPlaceholder')"
                    />
                  </td>
                  <td class="py-1.5 pr-3">
                    <input
                      v-model="row.description"
                      type="text"
                      class="input h-9 w-44 text-sm"
                      :placeholder="t('admin.settings.features.webChat.descriptionPlaceholder')"
                    />
                  </td>
                  <td class="py-1.5 pr-3">
                    <Select
                      :model-value="row.type"
                      class="w-28"
                      :options="typeOptions"
                      @update:model-value="row.type = coerceWebChatModelType($event)"
                    />
                  </td>
                  <td class="py-1.5 pr-3 text-center">
                    <Toggle v-model="row.api_only" />
                  </td>
                  <td class="py-1.5 text-right">
                    <button
                      type="button"
                      class="rounded p-1.5 text-gray-400 transition hover:text-red-600 dark:hover:text-red-400"
                      :aria-label="t('admin.settings.features.webChat.removeModel')"
                      :title="t('admin.settings.features.webChat.removeModel')"
                      @click="removeModel(index)"
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * MUC: 网页聊天设置（features tab 卡片）。
 * 与 SettingsView 现有区块一致，直接绑定在全局 reactive form 上：
 * - web_chat_enabled: boolean
 * - web_chat_models: JSON 数组字符串（解析失败视为空）
 * - web_chat_default_model: string
 * 保存复用 SettingsView 现有保存链路（payload 展开本模块的 buildWebChatSettingsPayload）。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  coerceWebChatModelType,
  getWebChatAvailableModels,
  parseWebChatModels,
  serializeWebChatModels,
  type WebChatModelSetting,
} from '@/api/admin/webChat'

/** SettingsView 全局 form 中与本组件相关的字段（其余字段不感知；索引签名保证任意 form 对象可传入） */
interface WebChatSettingsFormState {
  [key: string]: unknown
  web_chat_enabled?: boolean
  web_chat_models?: string
  web_chat_default_model?: string
}

const props = defineProps<{ form: WebChatSettingsFormState }>()

const { t } = useI18n()

// ── 总开关 / 默认模型：可写 computed，避免 form 初始缺键时的 undefined 绑定问题 ──
const enabled = computed({
  get: () => props.form.web_chat_enabled === true,
  set: (value: boolean) => {
    props.form.web_chat_enabled = value
  },
})

const defaultModel = computed({
  get: () => props.form.web_chat_default_model ?? '',
  set: (value: string | number | boolean | null) => {
    props.form.web_chat_default_model = value == null ? '' : String(value)
  },
})

// ── 模型行状态：内部数组为编辑源，双向同步 form.web_chat_models 字符串 ──
const rows = ref<WebChatModelSetting[]>(parseWebChatModels(props.form.web_chat_models))

/** 最近一次由本组件写出的字符串；外部（loadSettings）写入相同内容时不再回灌行 */
let lastSerialized: string | null = null

watch(
  rows,
  (value) => {
    const serialized = serializeWebChatModels(value)
    lastSerialized = serialized
    props.form.web_chat_models = serialized
  },
  { deep: true },
)

watch(
  () => props.form.web_chat_models,
  (raw) => {
    if ((raw ?? '') === lastSerialized) return
    lastSerialized = null
    rows.value = parseWebChatModels(raw)
  },
)

// ── 候选模型：来自 available-models 端点；拉取失败不阻塞（仍可手填自定义 ID） ──
const availableModels = ref<string[]>([])
const availableModelOptions = computed(() =>
  availableModels.value.map((model) => ({ value: model, label: model })),
)

const typeOptions = computed(() => [
  { value: 'chat' as const, label: t('admin.settings.features.webChat.typeChat') },
  { value: 'image' as const, label: t('admin.settings.features.webChat.typeImage') },
  { value: 'tts' as const, label: t('admin.settings.features.webChat.typeTts') },
  { value: 'music' as const, label: t('admin.settings.features.webChat.typeMusic') },
])

/** 默认模型选项 = 模型表中 type=chat 且未勾选“仅限 API”的行 */
const chatModelOptions = computed(() =>
  rows.value
    .filter((row) => row.type === 'chat' && !row.api_only && row.model.trim())
    .map((row) => ({
      value: row.model.trim(),
      label: row.display_name.trim() || row.model.trim(),
    })),
)

function addModel(): void {
  rows.value.push({
    model: '',
    display_name: '',
    vendor: '',
    description: '',
    type: 'chat',
    api_only: false,
  })
}

function removeModel(index: number): void {
  rows.value.splice(index, 1)
}

onMounted(async () => {
  try {
    availableModels.value = await getWebChatAvailableModels()
  } catch {
    // 候选列表拉取失败时保持可手填，不打断设置编辑
  }
})
</script>
