<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-3xl flex-col">
      <!-- 页头 -->
      <div class="pb-6 pt-2 text-center">
        <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('tts.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('tts.subtitle') }}</p>
      </div>

      <!-- 配置加载中 -->
      <div v-if="!loaded && !loadFailed" class="flex items-center justify-center py-24">
        <LoadingSpinner />
      </div>

      <!-- 功能未开放：无可用的网页语音合成模型 -->
      <EmptyState
        v-else-if="!ready"
        :title="t('tts.empty.title')"
        :description="t('tts.empty.description')"
      >
        <template #icon>
          <Icon name="sparkles" size="xl" class="text-gray-400 dark:text-gray-500" />
        </template>
      </EmptyState>

      <template v-else>
        <!-- 输入胶囊：与绘图页同一套扁平风格 -->
        <div
          class="rounded-3xl border border-gray-200/90 bg-white p-3 shadow-lg shadow-gray-200/50 transition-colors focus-within:border-primary-500/60 dark:border-dark-600 dark:bg-dark-900 dark:shadow-black/20 dark:focus-within:border-primary-500/60"
        >
          <!-- 待合成文本 -->
          <textarea
            :value="prompt"
            rows="3"
            class="w-full resize-none bg-transparent px-2 pt-1.5 text-sm leading-relaxed text-gray-900 placeholder:text-gray-400 focus:outline-none disabled:cursor-not-allowed dark:text-gray-100 dark:placeholder:text-dark-500"
            :placeholder="t('tts.form.promptPlaceholder')"
            :aria-label="t('tts.form.prompt')"
            :disabled="generating"
            @input="onPromptInput"
          ></textarea>

          <!-- 字符计数 -->
          <p class="mt-0.5 px-2 text-right text-xs text-gray-400 dark:text-dark-500">
            {{ t('tts.form.charCount', { count: prompt.length }) }}
          </p>

          <!-- 文本必填错误 -->
          <p
            v-if="showPromptError && !prompt.trim()"
            class="mt-1 px-2 text-xs text-red-500 dark:text-red-400"
          >
            {{ t('tts.form.promptRequired') }}
          </p>

          <!-- 控件行：模型 chip / 生成按钮 -->
          <div class="mt-1 flex flex-wrap items-center gap-2 px-1">
            <!-- 模型 chip（点击打开选择弹窗） -->
            <button
              type="button"
              class="flex max-w-[12rem] items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-60 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
              :aria-label="t('tts.form.model')"
              :disabled="ttsModels.length === 0"
              @click="pickerOpen = true"
            >
              <Icon name="cube" size="sm" class="shrink-0 text-primary-500/80 dark:text-primary-400/80" />
              <span class="truncate">{{ modelLabel }}</span>
              <Icon name="chevronDown" size="xs" class="shrink-0 text-gray-400 dark:text-dark-500" />
            </button>

            <!-- 生成按钮 -->
            <button
              type="button"
              class="btn btn-primary ml-auto h-9 rounded-full px-4"
              :disabled="generating || ttsModels.length === 0"
              @click="generate"
            >
              <Icon
                :name="generating ? 'refresh' : 'sparkles'"
                size="sm"
                class="mr-1.5"
                :class="generating ? 'animate-spin' : ''"
              />
              {{ generating ? t('tts.form.generating') : t('tts.form.generate') }}
            </button>
          </div>

          <!-- 模型选择弹窗（与聊天页共用） -->
          <ModelPickerModal
            :show="pickerOpen"
            :models="ttsModels"
            :selected="selectedModel"
            @select="onSelectModel"
            @close="pickerOpen = false"
          />
        </div>

        <!-- 结果区：最新在前，仅保存在内存中 -->
        <div v-if="results.length > 0" class="mt-8">
          <h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('tts.results.title') }}
          </h2>
          <div class="space-y-4">
            <div
              v-for="item in results"
              :key="item.id"
              class="rounded-2xl border border-gray-200/90 bg-white p-4 dark:border-dark-600 dark:bg-dark-900"
            >
              <div class="flex items-center justify-between gap-3">
                <p class="min-w-0 flex-1 truncate text-xs text-gray-500 dark:text-dark-400" :title="item.text">
                  {{ item.text }}
                </p>
                <a
                  :href="item.url"
                  :download="item.filename"
                  class="shrink-0 rounded-lg p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
                  :aria-label="t('tts.results.download')"
                  :title="t('tts.results.download')"
                >
                  <Icon name="download" size="sm" />
                </a>
              </div>
              <audio controls class="mt-2 w-full" :src="item.url"></audio>
            </div>
          </div>
        </div>

        <p class="mt-6 text-center text-xs text-gray-400 dark:text-gray-500">{{ t('tts.saveHint') }}</p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * 语音合成页（/tts，路由由外部注册）。
 * - 语音能力来自网页聊天配置（useWebChatStore）中 type=tts 且非 api_only 的模型；
 * - 合成走 speechWebChatAudio（返回音频 Blob，前端转 object URL 供播放/下载）；
 * - 不做历史持久化：结果仅保留在内存中，刷新即清；卸载时回收 object URL。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelPickerModal from '@/components/chat/ModelPickerModal.vue'
import { useWebChatStore } from '@/stores/webChat'
import { speechWebChatAudio } from '@/api/webChat'
import type { WebChatModelInfo } from '@/api/webChat'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const webChatStore = useWebChatStore()

// ── 可用语音模型 ──
const loaded = computed(() => webChatStore.loaded.value)
const loadFailed = ref(false)

const ttsModels = computed<WebChatModelInfo[]>(() =>
  (webChatStore.config.value?.models ?? []).filter(
    (m) => m.type === 'tts' && m.api_only === false,
  ),
)

/** config 缺失 / 未启用 / 无可用的非 API 语音模型时视为功能未开放 */
const ready = computed(
  () => webChatStore.config.value?.enabled === true && ttsModels.value.length > 0,
)

// ── 表单状态 ──
const selectedModel = ref('')
const prompt = ref('')
const showPromptError = ref(false)
const pickerOpen = ref(false)

watch(prompt, (value) => {
  if (value.trim()) showPromptError.value = false
})

// 默认模型：优先配置中的 default_model（需在可用列表内），否则取第一个
watch(
  ttsModels,
  (models) => {
    if (models.length === 0) return
    const current = models.find((m) => m.model === selectedModel.value)
    if (current) return
    const defaultModel = webChatStore.config.value?.default_model ?? ''
    selectedModel.value = models.some((m) => m.model === defaultModel)
      ? defaultModel
      : (models[0]?.model ?? '')
  },
  { immediate: true },
)

const modelLabel = computed(() => {
  if (!selectedModel.value) return t('tts.form.modelPlaceholder')
  return ttsModels.value.find((m) => m.model === selectedModel.value)?.display_name || selectedModel.value
})

function onPromptInput(event: Event): void {
  prompt.value = String((event.target as HTMLTextAreaElement | null)?.value ?? '')
}

function onSelectModel(value: string): void {
  selectedModel.value = value
  pickerOpen.value = false
}

// ── 生成 ──
interface TtsResultItem {
  id: number
  text: string
  url: string
  filename: string
}

const generating = ref(false)
const results = ref<TtsResultItem[]>([])
let nextResultId = 1

/** 从 Blob MIME 推断下载扩展名（audio/mpeg → mp3，未知回落 mp3） */
function audioExtension(blob: Blob): string {
  const subtype = blob.type.split('/')[1] ?? ''
  switch (subtype) {
    case 'mpeg':
    case 'mp3':
      return 'mp3'
    case 'wav':
    case 'x-wav':
    case 'wave':
      return 'wav'
    case 'ogg':
      return 'ogg'
    case 'mp4':
    case 'm4a':
    case 'x-m4a':
      return 'm4a'
    case 'flac':
    case 'x-flac':
      return 'flac'
    default:
      return 'mp3'
  }
}

async function generate(): Promise<void> {
  if (generating.value) return
  showPromptError.value = true
  const trimmedPrompt = prompt.value.trim()
  if (!trimmedPrompt || !selectedModel.value) return

  generating.value = true
  try {
    const blob = await speechWebChatAudio({
      model: selectedModel.value,
      input: trimmedPrompt,
    })
    const url = URL.createObjectURL(blob)
    // 最新一条排在最前；仅保存在内存中，刷新即清
    results.value = [
      {
        id: nextResultId++,
        text: trimmedPrompt,
        url,
        filename: `tts-${Date.now()}.${audioExtension(blob)}`,
      },
      ...results.value,
    ]
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('tts.generateFailed')))
  } finally {
    generating.value = false
  }
}

onMounted(() => {
  void loadTtsConfig()
})

/** 加载配置；store 内部吞错（失败时 loaded 仍为 false），据此判定加载失败并降级 */
async function loadTtsConfig(): Promise<void> {
  try {
    await webChatStore.loadConfig()
  } catch {
    // 防御未来 store 签名变化（当前实现不会 reject）
  }
  if (!webChatStore.loaded.value) {
    loadFailed.value = true
    appStore.showError(t('tts.loadFailed'))
  }
}

onUnmounted(() => {
  for (const item of results.value) {
    URL.revokeObjectURL(item.url)
  }
  results.value = []
})
</script>
