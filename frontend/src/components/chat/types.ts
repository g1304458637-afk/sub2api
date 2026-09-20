/**
 * Shared types for the web chat UI components.
 * The persisted shape in localStorage (`muc_chat_conversations`) must stay
 * compatible with ChatConversation / ChatMessage below.
 */

export type ChatRole = 'user' | 'assistant'

export interface ChatMessage {
  role: ChatRole
  content: string
  /** Marks a failed assistant message; renders an error bar with a retry action. */
  error?: boolean
}

export interface ChatConversation {
  id: string
  /** Truncated first user message (~20 chars). */
  title: string
  model: string
  createdAt: number
  updatedAt: number
  messages: ChatMessage[]
}
