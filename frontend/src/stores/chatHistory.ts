/**
 * Chat History Store
 * Module-level singleton holding the local conversation list, shared by the
 * chat page (messaging) and the sidebar history section (list / switch /
 * delete). Conversations persist to localStorage (`muc_chat_conversations`).
 *
 * Deliberately NOT a Pinia store, same rationale as webChat.ts: consumers read
 * state as refs (`store.currentId.value`), and a module singleton survives
 * route changes while staying in sync across components.
 */

import { computed, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import type { ChatConversation, ChatMessage } from '@/components/chat/types'

const CONVERSATIONS_KEY = 'muc_chat_conversations'
const CURRENT_ID_KEY = 'muc_chat_current_id'
const MAX_CONVERSATIONS = 50
const TITLE_MAX_LENGTH = 20

// ==================== Validation (localStorage input is untrusted) ====================

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

// ==================== Module-level singleton state ====================

const conversations = ref<ChatConversation[]>([])
const currentId = ref<string | null>(null)
let hydrated = false

const sortedConversations: ComputedRef<ChatConversation[]> = computed(() =>
  [...conversations.value].sort((a, b) => b.updatedAt - a.updatedAt)
)

const currentConversation: ComputedRef<ChatConversation | null> = computed(
  () => conversations.value.find((conversation) => conversation.id === currentId.value) ?? null
)

function loadFromStorage(): void {
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

// ==================== Store ====================

export function useChatHistoryStore(): {
  conversations: Ref<ChatConversation[]>
  currentId: Ref<string | null>
  sortedConversations: ComputedRef<ChatConversation[]>
  currentConversation: ComputedRef<ChatConversation | null>
  ensureHydrated: () => void
  persist: () => void
  newChat: () => void
  selectConversation: (id: string) => boolean
  deleteConversation: (id: string) => void
  ensureConversation: (content: string, fallbackTitle: string, model: string) => ChatConversation
} {
  /** Restore from localStorage once per app session; later calls are no-ops. */
  function ensureHydrated(): void {
    if (hydrated) {
      return
    }
    hydrated = true
    loadFromStorage()
  }

  function newChat(): void {
    currentId.value = null
    persist()
  }

  /** Returns false when the conversation no longer exists. */
  function selectConversation(id: string): boolean {
    if (!conversations.value.some((conversation) => conversation.id === id)) {
      return false
    }
    if (currentId.value !== id) {
      currentId.value = id
      persist()
    }
    return true
  }

  function deleteConversation(id: string): void {
    const index = conversations.value.findIndex((conversation) => conversation.id === id)
    if (index !== -1) {
      conversations.value.splice(index, 1)
    }
    if (currentId.value === id) {
      currentId.value = null
    }
    persist()
  }

  /**
   * Return the active conversation, or create one titled after the first user
   * message. Returns the reactive proxy inside the array — holding the raw
   * object breaks view updates when streaming appends content.
   */
  function ensureConversation(
    content: string,
    fallbackTitle: string,
    model: string
  ): ChatConversation {
    ensureHydrated()
    const existing = currentConversation.value
    if (existing) {
      return existing
    }
    const now = Date.now()
    const normalized = content.replace(/\s+/g, ' ').trim()
    const conversation: ChatConversation = {
      id: `chat_${now}_${Math.random().toString(36).slice(2, 10)}`,
      title: normalized
        ? normalized.length > TITLE_MAX_LENGTH
          ? `${normalized.slice(0, TITLE_MAX_LENGTH)}…`
          : normalized
        : fallbackTitle,
      model,
      createdAt: now,
      updatedAt: now,
      messages: [],
    }
    conversations.value.unshift(conversation)
    currentId.value = conversation.id
    return conversations.value.find((item) => item.id === conversation.id) ?? conversation
  }

  return {
    conversations,
    currentId,
    sortedConversations,
    currentConversation,
    ensureHydrated,
    persist,
    newChat,
    selectConversation,
    deleteConversation,
    ensureConversation,
  }
}
