<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-3xl flex-col">
      <!-- 页头 -->
      <div class="pb-6 pt-2 text-center">
        <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('draw.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('draw.subtitle') }}</p>
      </div>

      <!-- 配置加载中 -->
      <div v-if="!loaded && !loadFailed" class="flex items-center justify-center py-24">
        <LoadingSpinner />
      </div>

      <!-- 功能未开放：无可用的网页生图模型 -->
      <EmptyState
        v-else-if="!ready"
        :title="t('draw.empty.title')"
        :description="t('draw.empty.description')"
      >
        <template #icon>
          <Icon name="sparkles" size="xl" class="text-gray-400 dark:text-gray-500" />
        </template>
      </EmptyState>

      <template v-else>
        <DrawForm
          v-model:model="selectedModel"
          v-model:size="selectedSize"
          v-model:count="selectedCount"
          v-model:prompt="prompt"
          :models="imageModels"
          :generating="generating"
          :show-prompt-error="showPromptError"
          :context-image="contextImage"
          @submit="generate"
          @clear-context="clearContext"
        />

        <!-- 会话线程 -->
        <div class="mt-8">
          <DrawThread
            :turns="turns"
            :pending-prompt="pendingPrompt"
            :context-image-id="contextImage?.id ?? null"
            @select-context="selectContext"
            @retry="retryTurn"
          />
        </div>

        <p class="mt-6 text-center text-xs text-gray-400 dark:text-gray-500">{{ t('draw.saveHint') }}</p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * 网页绘图页（/draw，路由由外部注册）——会话式多轮改图。
 * - 生图能力来自网页聊天配置（useWebChatStore）中 type=image 且非 api_only 的模型；
 * - 全新生成走 generateWebChatImages；设置编辑上下文后走 generateWebChatImageEdits（固定 n=1）；
 * - 点击线程中任意图片将其选为上下文（编辑 chaining：生成成功后自动以新图作为下一轮上下文）；
 * - 不做绘图历史持久化：会话仅保留在内存中，刷新即清；图片总量受 DRAW_RESULT_LIMIT 约束。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DrawForm from '@/components/draw/DrawForm.vue'
import DrawThread from '@/components/draw/DrawThread.vue'
import {
  toDrawImageItem,
  DRAW_RESULT_LIMIT,
  type DrawImageItem,
  type DrawTurn,
  type DrawTurnRequest,
} from '@/components/draw/types'
import { useWebChatStore } from '@/stores/webChat'
import { generateWebChatImages, generateWebChatImageEdits } from '@/api/webChat'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const webChatStore = useWebChatStore()

// ── 可用生图模型 ──
const loaded = computed(() => webChatStore.loaded.value)
const loadFailed = ref(false)

const imageModels = computed(() =>
  (webChatStore.config.value?.models ?? []).filter(
    (m) => m.type === 'image' && m.api_only === false,
  ),
)

/** config 缺失 / 未启用 / 无可用的非 API 生图模型时视为功能未开放 */
const ready = computed(
  () => webChatStore.config.value?.enabled === true && imageModels.value.length > 0,
)

// ── 表单状态 ──
const selectedModel = ref('')
const selectedSize = ref('1024x1024')
const selectedCount = ref(1)
const prompt = ref('')
const showPromptError = ref(false)

watch(prompt, (value) => {
  if (value.trim()) showPromptError.value = false
})

// 默认模型：优先配置中的 default_model（需在可用列表内），否则取第一个
watch(
  imageModels,
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

// ── 会话状态（仅内存） ──
const turns = ref<DrawTurn[]>([])
const pendingPrompt = ref<string | null>(null)
/** 编辑上下文：线程中点击的图片；生成成功后自动指向本轮首图（chaining） */
const contextImage = ref<DrawImageItem | null>(null)

let turnSeq = 0

function selectContext(item: DrawImageItem): void {
  contextImage.value = item
}

function clearContext(): void {
  contextImage.value = null
}

/** 追加一轮并把总图片数裁剪到 DRAW_RESULT_LIMIT（最老的先被丢弃） */
function appendTurn(turn: DrawTurn): void {
  turns.value.push(turn)
  let total = turns.value.reduce((sum, entry) => sum + entry.images.length, 0)
  while (total > DRAW_RESULT_LIMIT) {
    const oldest = turns.value[0]
    if (!oldest || oldest.images.length === 0) break
    oldest.images.shift()
    total -= 1
    if (oldest.images.length === 0) {
      turns.value.shift()
    }
  }
}

// ── 生成 ──
const generating = ref(false)

async function generate(): Promise<void> {
  if (generating.value) return
  showPromptError.value = true
  const trimmedPrompt = prompt.value.trim()
  if (!trimmedPrompt || !selectedModel.value) return

  // 提示词随轮次上屏，输入框进入下一轮（会话式）
  showPromptError.value = false
  prompt.value = ''
  await runGenerate({
    prompt: trimmedPrompt,
    model: selectedModel.value,
    size: selectedSize.value,
    n: selectedCount.value,
    contextImage: contextImage.value,
  })
}

/** 失败轮重试：移除失败轮并按原参数重新提交 */
async function retryTurn(turn: DrawTurn): Promise<void> {
  if (generating.value || !turn.request) return
  turns.value = turns.value.filter((entry) => entry.id !== turn.id)
  await runGenerate(turn.request)
}

async function runGenerate(request: DrawTurnRequest): Promise<void> {
  if (generating.value) return
  generating.value = true
  pendingPrompt.value = request.prompt
  try {
    // 编辑模式：以参考图发起 edits（固定 n=1）；否则全新生成
    const contextSrc = request.contextImage
      ? (request.contextImage.dataUrl ?? request.contextImage.src)
      : null
    const results = contextSrc
      ? await generateWebChatImageEdits({
          model: request.model,
          prompt: request.prompt,
          image: [contextSrc],
          size: request.size,
          n: 1,
        })
      : await generateWebChatImages({
          model: request.model,
          prompt: request.prompt,
          size: request.size,
          n: request.n,
        })

    if (!Array.isArray(results) || results.length === 0) {
      appendTurn({
        id: nextTurnId(),
        prompt: request.prompt,
        images: [],
        error: t('draw.generateFailed'),
        request,
      })
      return
    }

    const items = results.map((result, index) => toDrawImageItem(result, request.prompt, index))
    const firstImage = items[0]
    appendTurn({
      id: firstImage ? `${firstImage.id}-turn` : nextTurnId(),
      prompt: request.prompt,
      images: items,
      request,
    })
    // 编辑 chaining：新图自动成为下一轮的编辑上下文
    contextImage.value = firstImage ?? null
  } catch (err) {
    appendTurn({
      id: nextTurnId(),
      prompt: request.prompt,
      images: [],
      error: extractApiErrorMessage(err, t('draw.generateFailed')),
      request,
    })
  } finally {
    pendingPrompt.value = null
    generating.value = false
  }
}

function nextTurnId(): string {
  turnSeq += 1
  return `draw-turn-${Date.now()}-${turnSeq}`
}

onMounted(() => {
  void loadDrawConfig()
})

/** 加载配置；store 内部吞错（失败时 loaded 仍为 false），据此判定加载失败并降级 */
async function loadDrawConfig(): Promise<void> {
  try {
    await webChatStore.loadConfig()
  } catch {
    // 防御未来 store 签名变化（当前实现不会 reject）
  }
  if (!webChatStore.loaded.value) {
    loadFailed.value = true
    appStore.showError(t('draw.loadFailed'))
  }
}
</script>
