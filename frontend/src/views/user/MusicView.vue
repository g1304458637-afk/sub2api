<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-3xl flex-col">
      <!-- 页头 -->
      <div class="pb-6 pt-2 text-center">
        <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('music.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('music.subtitle') }}</p>
      </div>

      <!-- 配置加载中 -->
      <div v-if="!loaded && !loadFailed" class="flex items-center justify-center py-24">
        <LoadingSpinner />
      </div>

      <!-- 功能未开放：无可用的网页音乐生成模型 -->
      <EmptyState
        v-else-if="!ready"
        :title="t('music.empty.title')"
        :description="t('music.empty.description')"
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
          <!-- 提示词 -->
          <textarea
            :value="prompt"
            rows="3"
            class="w-full resize-none bg-transparent px-2 pt-1.5 text-sm leading-relaxed text-gray-900 placeholder:text-gray-400 focus:outline-none disabled:cursor-not-allowed dark:text-gray-100 dark:placeholder:text-dark-500"
            :placeholder="t('music.form.promptPlaceholder')"
            :aria-label="t('music.form.prompt')"
            :disabled="submitting"
            @input="onPromptInput"
          ></textarea>

          <!-- 提示词必填错误 -->
          <p
            v-if="showPromptError && !prompt.trim()"
            class="mt-1 px-2 text-xs text-red-500 dark:text-red-400"
          >
            {{ t('music.form.promptRequired') }}
          </p>

          <!-- 控件行：模型 chip / 生成按钮 -->
          <div class="mt-1 flex flex-wrap items-center gap-2 px-1">
            <!-- 模型 chip（点击打开选择弹窗） -->
            <button
              type="button"
              class="flex max-w-[12rem] items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-60 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
              :aria-label="t('music.form.model')"
              :disabled="musicModels.length === 0"
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
              :disabled="submitting || musicModels.length === 0"
              @click="generate"
            >
              <Icon
                :name="submitting ? 'refresh' : 'sparkles'"
                size="sm"
                class="mr-1.5"
                :class="submitting ? 'animate-spin' : ''"
              />
              {{ submitting ? t('music.form.generating') : t('music.form.generate') }}
            </button>
          </div>

          <!-- 高级选项：可折叠（歌词 / 纯音乐） -->
          <div class="mt-1 px-1">
            <button
              type="button"
              class="text-xs font-medium text-gray-500 transition-colors hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-400 dark:hover:text-dark-200"
              @click="advancedOpen = !advancedOpen"
            >
              {{ advancedOpen ? t('music.form.advancedHide') : t('music.form.advanced') }}
            </button>

            <div v-if="advancedOpen" class="mt-2 space-y-3">
              <!-- 歌词（可选） -->
              <div>
                <label class="px-1 text-xs font-medium text-gray-500 dark:text-dark-400">
                  {{ t('music.form.lyrics') }}
                </label>
                <textarea
                  v-model="lyrics"
                  rows="3"
                  class="mt-1 w-full resize-none rounded-2xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm leading-relaxed text-gray-900 placeholder:text-gray-400 focus:border-primary-500/60 focus:outline-none disabled:cursor-not-allowed dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100 dark:placeholder:text-dark-500"
                  :placeholder="t('music.form.lyricsPlaceholder')"
                  :disabled="submitting"
                ></textarea>
              </div>

              <!-- 纯音乐开关 -->
              <div class="flex items-center justify-between gap-3 rounded-2xl border border-gray-200 px-3 py-2 dark:border-dark-600">
                <span class="text-sm text-gray-700 dark:text-gray-300">
                  {{ t('music.form.instrumental') }}
                </span>
                <Toggle v-model="instrumental" />
              </div>
            </div>
          </div>

          <!-- 模型选择弹窗（与聊天页共用） -->
          <ModelPickerModal
            :show="pickerOpen"
            :models="musicModels"
            :selected="selectedModel"
            @select="onSelectModel"
            @close="pickerOpen = false"
          />
        </div>

        <!-- 任务区：最新在前，仅保存在内存中 -->
        <div v-if="tasks.length > 0" class="mt-8">
          <h2 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('music.results.title') }}
          </h2>
          <div class="space-y-4">
            <div
              v-for="item in tasks"
              :key="item.id"
              class="rounded-2xl border border-gray-200/90 bg-white p-4 dark:border-dark-600 dark:bg-dark-900"
            >
              <div class="flex items-start justify-between gap-3">
                <p class="min-w-0 flex-1 truncate text-xs text-gray-500 dark:text-dark-400" :title="item.prompt">
                  {{ item.prompt }}
                </p>

                <!-- 成功：下载按钮 -->
                <a
                  v-if="item.status === 'succeeded' && audioSrc(item)"
                  :href="audioSrc(item)"
                  :download="downloadName(item)"
                  class="shrink-0 rounded-lg p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
                  :aria-label="t('music.results.download')"
                  :title="t('music.results.download')"
                >
                  <Icon name="download" size="sm" />
                </a>
              </div>

              <!-- 排队中 / 生成中 -->
              <div
                v-if="item.status === 'pending' || item.status === 'running'"
                class="mt-2 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400"
              >
                <Icon name="refresh" size="xs" class="animate-spin" />
                <span>{{ t(`music.status.${item.status}`) }}</span>
                <span class="text-gray-400 dark:text-dark-500">
                  {{ t('music.elapsed', { seconds: elapsedSeconds(item) }) }}
                </span>
              </div>

              <!-- 失败 -->
              <p v-else-if="item.status === 'failed'" class="mt-2 text-xs text-red-500 dark:text-red-400">
                {{ item.error || t('music.generateFailed') }}
              </p>

              <!-- 成功：音频播放器 -->
              <template v-else-if="item.status === 'succeeded'">
                <audio controls class="mt-2 w-full" :src="audioSrc(item)"></audio>
                <p class="mt-1.5 text-xs text-gray-400 dark:text-dark-500">
                  {{ resultMetaText(item) }}
                </p>
              </template>
            </div>
          </div>
        </div>

        <p class="mt-6 text-center text-xs text-gray-400 dark:text-gray-500">{{ t('music.saveHint') }}</p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * 音乐合成页（/music，路由由外部注册）。
 * - 音乐能力来自网页聊天配置（useWebChatStore）中 type=music 且非 api_only 的模型；
 * - 生成走 generateWebChatMusic（异步任务），提交后每 3s 轮询 getWebChatMusicTask
 *   （单任务最长 10 分钟，超时视为失败），终态后停止轮询；
 * - 不做历史持久化：任务仅保留在内存中，刷新即清。
 */
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import ModelPickerModal from '@/components/chat/ModelPickerModal.vue'
import { useWebChatStore } from '@/stores/webChat'
import { generateWebChatMusic, getWebChatMusicTask } from '@/api/webChat'
import type { WebChatModelInfo, WebChatMusicTask } from '@/api/webChat'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const webChatStore = useWebChatStore()

// ── 可用音乐模型 ──
const loaded = computed(() => webChatStore.loaded.value)
const loadFailed = ref(false)

const musicModels = computed<WebChatModelInfo[]>(() =>
  (webChatStore.config.value?.models ?? []).filter(
    (m) => m.type === 'music' && m.api_only === false,
  ),
)

/** config 缺失 / 未启用 / 无可用的非 API 音乐模型时视为功能未开放 */
const ready = computed(
  () => webChatStore.config.value?.enabled === true && musicModels.value.length > 0,
)

// ── 表单状态 ──
const selectedModel = ref('')
const prompt = ref('')
const lyrics = ref('')
const instrumental = ref(false)
const advancedOpen = ref(false)
const showPromptError = ref(false)
const pickerOpen = ref(false)

watch(prompt, (value) => {
  if (value.trim()) showPromptError.value = false
})

// 默认模型：优先配置中的 default_model（需在可用列表内），否则取第一个
watch(
  musicModels,
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
  if (!selectedModel.value) return t('music.form.modelPlaceholder')
  return musicModels.value.find((m) => m.model === selectedModel.value)?.display_name || selectedModel.value
})

function onPromptInput(event: Event): void {
  prompt.value = String((event.target as HTMLTextAreaElement | null)?.value ?? '')
}

function onSelectModel(value: string): void {
  selectedModel.value = value
  pickerOpen.value = false
}

// ── 任务列表与轮询 ──
interface MusicTaskItem {
  id: number
  taskId: string
  prompt: string
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  /** 提交时间，用于等待时长提示与 10 分钟超时判定 */
  startedAt: number
  /** 终态结果（succeeded 时携带音频信息） */
  result: WebChatMusicTask | null
  error: string
}

const POLL_INTERVAL_MS = 3_000
const POLL_TIMEOUT_MS = 10 * 60 * 1_000

const submitting = ref(false)
const tasks = ref<MusicTaskItem[]>([])
let nextTaskId = 1
let pollTimer: ReturnType<typeof setInterval> | null = null

function hasActiveTasks(): boolean {
  return tasks.value.some((item) => item.status === 'pending' || item.status === 'running')
}

function ensurePolling(): void {
  if (pollTimer !== null) return
  pollTimer = setInterval(() => {
    void pollActiveTasks()
  }, POLL_INTERVAL_MS)
}

function stopPolling(): void {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function pollActiveTasks(): Promise<void> {
  if (!hasActiveTasks()) {
    stopPolling()
    return
  }

  const active = tasks.value.filter((item) => item.status === 'pending' || item.status === 'running')
  await Promise.all(
    active.map(async (item) => {
      // 超过 10 分钟仍未终态：停止跟踪该任务，按失败处理
      if (Date.now() - item.startedAt > POLL_TIMEOUT_MS) {
        item.status = 'failed'
        item.error = t('music.timeout')
        return
      }
      try {
        const result = await getWebChatMusicTask(item.taskId)
        if (result.status === 'succeeded') {
          item.status = 'succeeded'
          item.result = result
        } else if (result.status === 'failed') {
          item.status = 'failed'
          item.error = result.error ?? ''
        } else {
          item.status = result.status
        }
      } catch {
        // 单次轮询失败视为瞬时错误，下一轮继续
      }
    }),
  )

  if (!hasActiveTasks()) stopPolling()
}

async function generate(): Promise<void> {
  if (submitting.value) return
  showPromptError.value = true
  const trimmedPrompt = prompt.value.trim()
  if (!trimmedPrompt || !selectedModel.value) return

  const trimmedLyrics = lyrics.value.trim()

  submitting.value = true
  try {
    const task = await generateWebChatMusic({
      model: selectedModel.value,
      prompt: trimmedPrompt,
      lyrics: trimmedLyrics || undefined,
      instrumental: instrumental.value,
    })

    const item: MusicTaskItem = {
      id: nextTaskId++,
      taskId: task.task_id,
      prompt: trimmedPrompt,
      status: task.status === 'succeeded' || task.status === 'failed' ? task.status : 'pending',
      startedAt: Date.now(),
      result: task.status === 'succeeded' ? task : null,
      error: task.status === 'failed' ? (task.error ?? '') : '',
    }
    // 最新一条排在最前；仅保存在内存中，刷新即清
    tasks.value = [item, ...tasks.value]
    if (item.status === 'pending' || item.status === 'running') ensurePolling()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('music.generateFailed')))
  } finally {
    submitting.value = false
  }
}

// ── 结果展示辅助 ──

/** 音频来源：优先 http URL，其次 base64 data URL */
function audioSrc(item: MusicTaskItem): string {
  return item.result?.audio_url || item.result?.audio_base64 || ''
}

function downloadName(item: MusicTaskItem): string {
  const ext = (item.result?.format || 'mp3').replace(/^\./, '')
  return `music-${item.id}.${ext}`
}

/** 等待秒数（模板内随轮询刷新；轮询间隔 3s，粒度足够提示用） */
function elapsedSeconds(item: MusicTaskItem): number {
  return Math.floor((Date.now() - item.startedAt) / 1_000)
}

/** 时长 m:ss + 格式徽标；缺省时逐项隐藏 */
function resultMetaText(item: MusicTaskItem): string {
  const parts: string[] = []
  const duration = item.result?.duration_sec
  if (typeof duration === 'number' && Number.isFinite(duration) && duration > 0) {
    const minutes = Math.floor(duration / 60)
    const seconds = Math.floor(duration % 60)
    parts.push(`${minutes}:${String(seconds).padStart(2, '0')}`)
  }
  const format = item.result?.format?.replace(/^\./, '')
  if (format) parts.push(format.toUpperCase())
  return parts.join(' · ')
}

onMounted(() => {
  void loadMusicConfig()
})

/** 加载配置；store 内部吞错（失败时 loaded 仍为 false），据此判定加载失败并降级 */
async function loadMusicConfig(): Promise<void> {
  try {
    await webChatStore.loadConfig()
  } catch {
    // 防御未来 store 签名变化（当前实现不会 reject）
  }
  if (!webChatStore.loaded.value) {
    loadFailed.value = true
    appStore.showError(t('music.loadFailed'))
  }
}

onUnmounted(() => {
  stopPolling()
})
</script>
