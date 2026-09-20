<template>
  <AppLayout>
  <div
    class="relative flex h-[calc(100vh-6rem)] overflow-hidden rounded-2xl border border-gray-200/80 bg-white shadow-card md:h-[calc(100vh-7rem)] lg:h-[calc(100vh-8rem)] dark:border-dark-700/80 dark:bg-dark-900"
  >
    <!-- Sidebar: collapsed rail -->
    <div
      v-if="sidebarCollapsed"
      class="flex h-full w-12 shrink-0 flex-col items-center gap-2 border-r border-gray-200 bg-white py-3 dark:border-dark-700 dark:bg-dark-900"
    >
      <button
        type="button"
        class="rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-dark-300"
        :title="t('chat.sidebar.expand')"
        :aria-label="t('chat.sidebar.expand')"
        @click="sidebarCollapsed = false"
      >
        <Icon name="chevronRight" size="sm" />
      </button>
      <button
        type="button"
        class="rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 focus:outline-none dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-primary-400"
        :title="t('chat.sidebar.newChat')"
        :aria-label="t('chat.sidebar.newChat')"
        @click="handleNewChat"
      >
        <Icon name="plus" size="sm" />
      </button>
    </div>

    <!-- Sidebar: expanded (overlay on mobile, static on desktop) -->
    <template v-else>
      <div
        class="fixed inset-0 z-20 bg-black/30 md:hidden"
        aria-hidden="true"
        @click="sidebarCollapsed = true"
      ></div>
      <div class="absolute inset-y-0 left-0 z-30 h-full md:static md:z-auto">
        <ConversationSidebar
          :conversations="sortedConversations"
          :current-id="currentId"
          @new="handleNewChat"
          @select="handleSelectConversation"
          @delete="handleDeleteConversation"
          @collapse="sidebarCollapsed = true"
        />
      </div>
    </template>

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
          <button
            type="button"
            class="absolute left-3 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-300 md:hidden"
            :title="t('chat.sidebar.expand')"
            :aria-label="t('chat.sidebar.expand')"
            @click="sidebarCollapsed = false"
          >
            <Icon name="menu" size="sm" />
          </button>

          <!-- Current model chip -->
          <button
            type="button"
            class="mx-auto flex max-w-[70%] items-center gap-1.5 rounded-full border border-gray-200 px-4 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:border-primary-300 hover:text-primary-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:text-dark-300 dark:hover:border-primary-700 dark:hover:text-primary-300"
            :disabled="chatModels.length === 0"
            @click="pickerOpen = true"
          >
            <Icon name="cube" size="sm" class="shrink-0 text-primary-500/80 dark:text-primary-400/80" />
            <span class="truncate">{{ modelLabel }}</span>
            <Icon name="chevronDown" size="xs" class="shrink-0 text-gray-400 dark:text-dark-500" />
          </button>

          <button
            type="button"
            class="absolute right-3 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 focus:outline-none md:hidden dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-primary-400"
            :title="t('chat.sidebar.newChat')"
            :aria-label="t('chat.sidebar.newChat')"
            @click="handleNewChat"
          >
            <Icon name="plus" size="sm" />
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
        <div class="shrink-0 px-4 pb-4 pt-2">
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
import { isAbortError, streamWebChatCompletion } from '@/api/webChat'
import type { WebChatModelInfo } from '@/api/webChat'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConversationSidebar from '@/components/chat/ConversationSidebar.vue'
import MessageList from '@/components/chat/MessageList.vue'
import ChatInput from '@/components/chat/ChatInput.vue'
import ModelPickerModal from '@/components/chat/ModelPickerModal.vue'
import type { ChatConversation, ChatMessage } from '@/components/chat/types'

// ==================== Constants ====================

const CONVERSATIONS_KEY = 'muc_chat_conversations'
const CURRENT_ID_KEY = 'muc_chat_current_id'
const MAX_CONVERSATIONS = 50
const TITLE_MAX_LENGTH = 20
const DESKTOP_BREAKPOINT = 768

// ==================== State ====================

const { t } = useI18n()
const appStore = useAppStore()
const webChatStore = useWebChatStore()

// Store properties are refs; this computed keeps `config.value` ergonomics in
// script and template unwrapping.
const config = computed(() => webChatStore.config.value)

const conversations = ref<ChatConversation[]>([])
const currentId = ref<string | null>(null)
const currentModel = ref('')
const pickerOpen = ref(false)
const sidebarCollapsed = ref(
  typeof window !== 'undefined' ? window.innerWidth < DESKTOP_BREAKPOINT : false
)
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

const sortedConversations = computed<ChatConversation[]>(() =>
  [...conversations.value].sort((a, b) => b.updatedAt - a.updatedAt)
)

const currentConversation = computed<ChatConversation | null>(
  () => conversations.value.find((conversation) => conversation.id === currentId.value) ?? null
)

const activeMessages = computed<ChatMessage[]>(() => currentConversation.value?.messages ?? [])

// ==================== Persistence ====================

function isChatConversation(value: unknown): value is ChatConversation {
  if (!value || typeof value !== 'object') {
    return false
  }
  const candidate = value as Partial<ChatConversation>
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.title === 'string' &&
    typeof candidate.model === 'string' &&
    typeof candidate.createdAt === 'number' &&
    typeof candidate.updatedAt === 'number' &&
    Array.isArray(candidate.messages)
  )
}

function sanitizeMessage(value: unknown): ChatMessage | null {
  if (!value || typeof value !== 'object') {
    return null
  }
  const candidate = value as Partial<ChatMessage>
  if (
    (candidate.role !== 'user' && candidate.role !== 'assistant') ||
    typeof candidate.content !== 'string'
  ) {
    return null
  }
  const message: ChatMessage = { role: candidate.role, content: candidate.content }
  if (candidate.error === true) {
    message.error = true
  }
  return message
}

function loadConversations(): void {
  try {
    const raw = localStorage.getItem(CONVERSATIONS_KEY)
    if (!raw) {
      return
    }
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return
    }
    const restored: ChatConversation[] = []
    for (const item of parsed) {
      if (!isChatConversation(item)) {
        continue
      }
      const messages = item.messages
        .map(sanitizeMessage)
        .filter((message): message is ChatMessage => message !== null)
      restored.push({
        id: item.id,
        title: item.title,
        model: item.model,
        createdAt: item.createdAt,
        updatedAt: item.updatedAt,
        messages,
      })
    }
    restored.sort((a, b) => b.updatedAt - a.updatedAt)
    conversations.value = restored.slice(0, MAX_CONVERSATIONS)

    const savedId = localStorage.getItem(CURRENT_ID_KEY)
    if (savedId && conversations.value.some((conversation) => conversation.id === savedId)) {
      currentId.value = savedId
      const conversation = currentConversation.value
      if (conversation?.model) {
        currentModel.value = conversation.model
      }
    }
  } catch {
    // Corrupted storage: start fresh with in-memory state
  }
}

function persist(): void {
  try {
    const sorted = [...conversations.value]
      .sort((a, b) => b.updatedAt - a.updatedAt)
      .slice(0, MAX_CONVERSATIONS)
    conversations.value = sorted
    localStorage.setItem(CONVERSATIONS_KEY, JSON.stringify(sorted))
    if (currentId.value) {
      localStorage.setItem(CURRENT_ID_KEY, currentId.value)
    } else {
      localStorage.removeItem(CURRENT_ID_KEY)
    }
  } catch {
    // localStorage unavailable or full: keep working in memory
  }
}

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
    persist()
  }
}

// ==================== Conversations ====================

function createTitle(content: string): string {
  const normalized = content.replace(/\s+/g, ' ').trim()
  if (!normalized) {
    return t('chat.title')
  }
  return normalized.length > TITLE_MAX_LENGTH
    ? `${normalized.slice(0, TITLE_MAX_LENGTH)}…`
    : normalized
}

function ensureConversation(content: string): ChatConversation {
  const existing = currentConversation.value
  if (existing) {
    return existing
  }
  const now = Date.now()
  const conversation: ChatConversation = {
    id: `chat_${now}_${Math.random().toString(36).slice(2, 10)}`,
    title: createTitle(content),
    model: currentModel.value || resolveDefaultModel(),
    createdAt: now,
    updatedAt: now,
    messages: [],
  }
  conversations.value.unshift(conversation)
  currentId.value = conversation.id
  // 返回数组内的响应式代理而非局部原始对象，否则后续流式写入不触发视图更新
  return conversations.value.find((item) => item.id === conversation.id) ?? conversation
}

function clearRuntimeErrors(): void {
  runtimeErrorTexts.value = {}
}

function handleNewChat(): void {
  if (streaming.value) {
    return
  }
  currentId.value = null
  clearRuntimeErrors()
  currentModel.value = resolveDefaultModel()
}

// 欢迎页建议问题：填入输入框并聚焦，不直接发送
function handleSuggest(text: string): void {
  chatInputRef.value?.setText(text)
  chatInputRef.value?.focus()
}

function handleSelectConversation(id: string): void {
  if (id === currentId.value) {
    return
  }
  currentId.value = id
  clearRuntimeErrors()
  const conversation = currentConversation.value
  if (conversation?.model) {
    currentModel.value = conversation.model
  }
}

function handleDeleteConversation(id: string): void {
  const index = conversations.value.findIndex((conversation) => conversation.id === id)
  if (index !== -1) {
    conversations.value.splice(index, 1)
  }
  if (currentId.value === id) {
    currentId.value = null
    clearRuntimeErrors()
    currentModel.value = resolveDefaultModel()
  }
  persist()
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
    persist()
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
  const conversation = ensureConversation(text)
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
  loadConversations()
  if (!currentModel.value) {
    currentModel.value = resolveDefaultModel()
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

watch(currentId, () => {
  chatInputRef.value?.focus()
})
</script>
