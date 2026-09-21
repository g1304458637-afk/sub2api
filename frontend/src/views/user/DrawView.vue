<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- 页头 -->
      <div>
        <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('draw.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('draw.subtitle') }}</p>
      </div>

      <!-- 配置加载中 -->
      <div v-if="!loaded && !loadFailed" class="flex items-center justify-center py-24">
        <LoadingSpinner />
      </div>

      <!-- 功能未开放：无可用的网页生图模型 -->
      <div v-else-if="!ready" class="card">
        <EmptyState
          :title="t('draw.empty.title')"
          :description="t('draw.empty.description')"
        >
          <template #icon>
            <Icon name="sparkles" size="xl" class="text-gray-400 dark:text-gray-500" />
          </template>
        </EmptyState>
      </div>

      <template v-else>
        <div class="grid items-start gap-6 lg:grid-cols-[360px_minmax(0,1fr)]">
          <!-- 表单区 -->
          <DrawForm
            v-model:model="selectedModel"
            v-model:size="selectedSize"
            v-model:count="selectedCount"
            v-model:prompt="prompt"
            :models="imageModels"
            :generating="generating"
            :show-prompt-error="showPromptError"
            @submit="generate"
          />

          <!-- 结果区 -->
          <DrawGallery :items="images" />
        </div>

        <p class="text-xs text-gray-400 dark:text-gray-500">{{ t('draw.saveHint') }}</p>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * 网页绘图页（/draw，路由由外部注册）。
 * - 生图能力来自网页聊天配置（useWebChatStore）中 type=image 且非 api_only 的模型；
 * - 生成走 generateWebChatImages（JWT 面板鉴权封装）；
 * - 不做绘图历史持久化：结果仅保留在内存中，刷新即清。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DrawForm from '@/components/draw/DrawForm.vue'
import DrawGallery from '@/components/draw/DrawGallery.vue'
import { toDrawImageItem, DRAW_RESULT_LIMIT, type DrawImageItem } from '@/components/draw/types'
import { useWebChatStore } from '@/stores/webChat'
import { generateWebChatImages } from '@/api/webChat'
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

// ── 生成 ──
const generating = ref(false)
const images = ref<DrawImageItem[]>([])

async function generate(): Promise<void> {
  if (generating.value) return
  showPromptError.value = true
  const trimmedPrompt = prompt.value.trim()
  if (!trimmedPrompt || !selectedModel.value) return

  generating.value = true
  try {
    const results = await generateWebChatImages({
      model: selectedModel.value,
      prompt: trimmedPrompt,
      size: selectedSize.value,
      n: selectedCount.value,
    })
    if (!Array.isArray(results) || results.length === 0) {
      appStore.showError(t('draw.generateFailed'))
      return
    }
    const items = results.map((result, index) =>
      toDrawImageItem(result, trimmedPrompt, index),
    )
    // 最新一批排在最前；仅保存在内存中，刷新即清，控制总量防止 b64 撑爆内存
    images.value = [...items, ...images.value].slice(0, DRAW_RESULT_LIMIT)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('draw.generateFailed')))
  } finally {
    generating.value = false
  }
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
