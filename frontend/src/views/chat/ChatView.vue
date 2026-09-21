<template>
  <!-- ChatGPT 式扁平布局：无卡片框架，消息直接铺在页面背景上（AppLayout flush 去内边距） -->
  <AppLayout flush>
  <div class="flex h-[calc(100vh-4rem)] flex-col">
    <!-- Main column -->
    <section class="flex h-full min-w-0 flex-1 flex-col">
      <!-- Loading state -->
      <div v-if="!initialLoadDone || retrying" class="flex flex-1 items-center justify-center">
        <LoadingSpinner size="lg" />
      </div>

      <!-- Disabled / load failed state -->
      <div v-else-if="!config || !config.enabled" class="flex flex-1 items-center justify-center p-6">
        <EmptyState
          :title="config ? t('chat.state.disabledTitle') : t('chat.state.loadFailedTitle')"
          :description="config ? t('chat.state.disabledDescription') : t('chat.state.loadFailedDescription')"
        >
          <template #icon>
            <Icon name="chatBubble" size="xl" class="text-gray-400 dark:text-dark-500" />
          </template>
          <template v-if="!config" #action>
            <button type="button" class="btn btn-primary" @click="retryLoadConfig">
              {{ t('chat.state.loadFailedRetry') }}
            </button>
          </template>
        </EmptyState>
      </div>

      <!-- Chat UI -->
      <template v-else>
        <!-- Top bar: frameless, ghost model chip -->
        <div class="relative flex h-14 shrink-0 items-center justify-center px-3">
          <!-- Current model chip -->
          <button
            type="button"
            class="mx-auto flex max-w-[70%] items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-60 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
            :disabled="chatModels.length === 0"
            @click="pickerOpen = true"
          >
            <Icon name="cube" size="sm" class="shrink-0 text-primary-500/80 dark:text-primary-400/80" />
            <span class="truncate">{{ modelLabel }}</span>
            <Icon name="chevronDown" size="xs" class="shrink-0 text-gray-400 dark:text-dark-500" />
          </button>
        </div>

        <!-- Messages -->
        <MessageList
          :messages="activeMessages"
          :streaming="streaming"
          :error-texts="runtimeErrorTexts"
          @regenerate="handleRegenerate"
          @retry="handleRetry"
          @suggest="handleSuggest"
        />

        <!-- Input -->
        <div class="shrink-0 px-4 pb-4 pt-2 md:pb-6">
          <div class="mx-auto w-full max-w-3xl">
            <ChatInput
              ref="chatInputRef"
              :streaming="streaming"
              :placeholder="inputPlaceholder"
              @send="handleSend"
              @stop="handleStop"
            />
          </div>
        </div>
      </template>
    </section>

    <!-- Model picker -->
    <ModelPickerModal
      :show="pickerOpen"
      :models="chatModels"
      :selected="currentModel"
      @select="handleSelectModel"
      @close="pickerOpen = false"
    />
  </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useWebChatStore } from '@/stores/webChat'
import { useChatHistoryStore } from '@/stores/chatHistory'
import { isAbortError, streamWebChatCompletion } from '@/api/webChat'
import type { WebChatModelInfo } from '@/api/webChat'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import MessageList from '@/components/chat/MessageList.vue'
import ChatInput from '@/components/chat/ChatInput.vue'
import ModelPickerModal from '@/components/chat/ModelPickerModal.vue'
import type { ChatConversation, ChatMessage } from '@/components/chat/types'

// ==================== State ====================

const { t } = useI18n()
const appStore = useAppStore()
const webChatStore = useWebChatStore()
const historyStore = useChatHistoryStore()

// Store properties are refs; this computed keeps `config.value` ergonomics in
// script and template unwrapping.
const config = computed(() => webChatStore.config.value)

const currentModel = ref('')
const pickerOpen = ref(false)
const streaming = ref(false)
const retrying = ref(false)
const initialLoadDone = ref(false)
const runtimeErrorTexts = ref<Record<number, string>>({})
const abortController = ref<AbortController | null>(null)
const chatInputRef = ref<InstanceType<typeof ChatInput> | null>(null)

// ==================== Computed ====================

/** All chat-type models (including api_only, which the picker renders disabled). */
const chatModels = computed<WebChatModelInfo[]>(() =>
  (config.value?.models ?? []).filter((model) => model.type === 'chat')
)

/** Models that can actually be used from the web UI. */
const usableModels = computed<WebChatModelInfo[]>(() =>
  chatModels.value.filter((model) => !model.api_only)
)

const currentModelInfo = computed<WebChatModelInfo | null>(
  () => chatModels.value.find((model) => model.model === currentModel.value) ?? null
)

const modelLabel = computed(() =>
  currentModelInfo.value?.display_name || currentModel.value || t('chat.state.noModels')
)

const inputPlaceholder = computed(() =>
  usableModels.value.length === 0 ? t('chat.state.noModelsHint') : t('chat.input.placeholder')
)

const currentConversation = computed<ChatConversation | null>(
  () => historyStore.currentConversation.value
)

const activeMessages = computed<ChatMessage[]>(() => currentConversation.value?.messages ?? [])

// ==================== Models ====================

function resolveDefaultModel(): string {
  const usable = usableModels.value
  if (usable.length === 0) {
    return ''
  }
  const preferred = config.value?.default_model
  if (preferred && usable.some((model) => model.model === preferred)) {
    return preferred
  }
  return usable[0].model
}

function handleSelectModel(model: string): void {
  currentModel.value = model
  const conversation = currentConversation.value
  if (conversation) {
    conversation.model = model
    historyStore.persist()
  }
}

// ==================== Conversations ====================

function clearRuntimeErrors(): void {
  runtimeErrorTexts.value = {}
}

// 欢迎页建议问题：填入输入框并聚焦，不直接发送
function handleSuggest(text: string): void {
  chatInputRef.value?.setText(text)
  chatInputRef.value?.focus()
}

// ==================== Messaging ====================

function buildRequestMessages(
  conversation: ChatConversation
): { role: string; content: string }[] {
  return conversation.messages
    .filter(
      (message) => !(message.role === 'assistant' && message.error === true && !message.content.trim())
    )
    .map((message) => ({ role: message.role, content: message.content }))
}

function removeMessage(conversation: ChatConversation, message: ChatMessage): void {
  const index = conversation.messages.indexOf(message)
  if (index !== -1) {
    conversation.messages.splice(index, 1)
  }
}

function markAssistantError(
  conversation: ChatConversation,
  message: ChatMessage,
  text: string
): void {
  message.error = true
  const index = conversation.messages.indexOf(message)
  if (index !== -1) {
    runtimeErrorTexts.value = { ...runtimeErrorTexts.value, [index]: text }
  }
}

function startAssistantRun(conversation: ChatConversation): void {
  if (!conversation.model) {
    conversation.model = currentModel.value || resolveDefaultModel()
  }
  const assistantMessage: ChatMessage = { role: 'assistant', content: '' }
  conversation.messages.push(assistantMessage)
  // 取回数组内的响应式代理引用：直接持有原始对象时，流式追加 content 不会触发视图更新
  const reactiveMessage = conversation.messages[conversation.messages.length - 1] as ChatMessage
  conversation.updatedAt = Date.now()
  void runCompletion(conversation, reactiveMessage)
}

async function runCompletion(
  conversation: ChatConversation,
  assistantMessage: ChatMessage
): Promise<void> {
  const controller = new AbortController()
  abortController.value = controller
  streaming.value = true

  try {
    await streamWebChatCompletion(
      {
        model: conversation.model || currentModel.value,
        messages: buildRequestMessages(conversation),
      },
      controller.signal,
      (delta: string) => {
        assistantMessage.content += delta
      }
    )

    if (!assistantMessage.content.trim()) {
      markAssistantError(conversation, assistantMessage, t('chat.error.emptyResponse'))
    }
  } catch (error) {
    if (isAbortError(error)) {
      // User stopped: keep partial content; drop the placeholder when empty
      if (!assistantMessage.content) {
        removeMessage(conversation, assistantMessage)
      }
    } else {
      const detail = error instanceof Error && error.message ? error.message : ''
      markAssistantError(
        conversation,
        assistantMessage,
        detail || t('chat.error.responseFailed')
      )
    }
  } finally {
    streaming.value = false
    if (abortController.value === controller) {
      abortController.value = null
    }
    conversation.updatedAt = Date.now()
    historyStore.persist()
  }
}

function handleSend(text: string): void {
  if (streaming.value) {
    return
  }
  if (!currentModel.value) {
    currentModel.value = resolveDefaultModel()
  }
  if (!currentModel.value) {
    appStore.showWarning(t('chat.state.noModelsHint'))
    return
  }
  const conversation = historyStore.ensureConversation(
    text,
    t('chat.title'),
    currentModel.value || resolveDefaultModel()
  )
  conversation.messages.push({ role: 'user', content: text })
  clearRuntimeErrors()
  startAssistantRun(conversation)
}

function handleStop(): void {
  abortController.value?.abort()
}

function handleRegenerate(): void {
  const conversation = currentConversation.value
  if (!conversation || streaming.value) {
    return
  }
  const last = conversation.messages[conversation.messages.length - 1]
  if (!last || last.role !== 'assistant') {
    return
  }
  conversation.messages.pop()
  clearRuntimeErrors()
  startAssistantRun(conversation)
}

function handleRetry(index: number): void {
  const conversation = currentConversation.value
  if (!conversation || streaming.value) {
    return
  }
  const message = conversation.messages[index]
  if (!message || message.role !== 'assistant') {
    return
  }
  conversation.messages.splice(index, 1)
  clearRuntimeErrors()
  startAssistantRun(conversation)
}

// ==================== Config ====================

async function retryLoadConfig(): Promise<void> {
  retrying.value = true
  await webChatStore.loadConfig(true)
  retrying.value = false
  if (!currentModel.value) {
    currentModel.value = resolveDefaultModel()
  }
}

// ==================== Lifecycle ====================

onMounted(() => {
  historyStore.ensureHydrated()
  if (!currentModel.value) {
    currentModel.value = resolveDefaultModel()
  }
  const restored = currentConversation.value
  if (restored?.model) {
    currentModel.value = restored.model
  }
  void webChatStore.loadConfig().then(() => {
    initialLoadDone.value = true
    if (currentConversation.value?.model) {
      currentModel.value = currentConversation.value.model
    } else if (!currentModel.value) {
      currentModel.value = resolveDefaultModel()
    }
  })
})

onBeforeUnmount(() => {
  // Leaving the page stops the in-flight stream; its finally block persists state
  abortController.value?.abort()
})

// 会话切换（含侧边栏发起的 新建/选择/删除）：同步错误条、模型与输入焦点
watch(
  () => historyStore.currentId.value,
  () => {
    clearRuntimeErrors()
    const conversation = currentConversation.value
    if (conversation?.model) {
      currentModel.value = conversation.model
    } else if (!conversation && !streaming.value) {
      currentModel.value = resolveDefaultModel()
    }
    chatInputRef.value?.focus()
  }
)
</script>
