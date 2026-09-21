/**
 * Web Chat API
 * JWT-authenticated web chat endpoints. The chat completion endpoint streams
 * OpenAI-style SSE chunks, so it uses native fetch instead of axios (whose
 * default 30s timeout would kill long-running streams).
 */

import { apiClient } from './client'
import { getAPIBaseURL } from './url'
import { refreshAuthTokens } from './tokenRefresh'

// ==================== Types ====================

export interface WebChatModelInfo {
  model: string
  display_name: string
  vendor: string
  description: string
  type: 'chat' | 'image' | 'tts' | 'music'
  api_only: boolean
}

export interface WebChatEntrances {
  chat: boolean
  draw: boolean
  tts: boolean
  music: boolean
}

export interface WebChatConfig {
  enabled: boolean
  default_model: string
  models: WebChatModelInfo[]
  /** 大模型服务分组各入口显示开关（后台可配；缺省全显示） */
  entrances?: Partial<WebChatEntrances>
}

export interface WebChatImageParams {
  model: string
  prompt: string
  size: string
  n: number
}

export interface WebChatImageResult {
  url?: string
  b64_json?: string
}

export interface WebChatImageEditsParams {
  model: string
  prompt: string
  /** Base images as data URLs ("data:image/png;base64,...") */
  image: string[]
  size?: string
  n?: number
}

export interface WebChatSpeechParams {
  model: string
  input: string
}

export type WebChatMusicTaskStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export interface WebChatMusicTask {
  task_id: string
  status: WebChatMusicTaskStatus
  /** Present on the 202 submit response */
  poll_url?: string
  /** Present once status is "succeeded" */
  audio_url?: string
  audio_base64?: string
  duration_sec?: number
  format?: string
  error?: string
}

export interface WebChatMusicParams {
  model: string
  prompt: string
  lyrics?: string
  instrumental?: boolean
}

interface WebChatCompletionParams {
  model: string
  messages: { role: string; content: string }[]
}

interface WebChatCompletionChunk {
  choices?: {
    delta?: {
      content?: string | null
    }
  }[]
}

// ==================== Config ====================

export function fetchWebChatConfig(): Promise<WebChatConfig> {
  return apiClient.get<WebChatConfig>('/web-chat/config').then(({ data }) => data)
}

// ==================== Image Generations ====================

export function generateWebChatImages(params: WebChatImageParams): Promise<WebChatImageResult[]> {
  return apiClient
    .post<{ created: number; data: WebChatImageResult[] }>(
      '/web-chat/images/generations',
      params,
      { timeout: 300_000 }
    )
    .then(({ data }) => data?.data ?? [])
}

// ==================== Image Edits ====================

export function generateWebChatImageEdits(
  params: WebChatImageEditsParams
): Promise<WebChatImageResult[]> {
  return apiClient
    .post<{ created: number; data: WebChatImageResult[] }>(
      '/web-chat/images/edits',
      params,
      { timeout: 300_000 }
    )
    .then(({ data }) => data?.data ?? [])
}

// ==================== Music Generations (task + poll) ====================

export function generateWebChatMusic(params: WebChatMusicParams): Promise<WebChatMusicTask> {
  return apiClient.post<WebChatMusicTask>('/web-chat/music/generations', params).then(({ data }) => data)
}

export function getWebChatMusicTask(taskId: string): Promise<WebChatMusicTask> {
  return apiClient
    .get<WebChatMusicTask>(`/web-chat/music/tasks/${encodeURIComponent(taskId)}`)
    .then(({ data }) => data)
}

// ==================== Chat Completions (SSE) ====================

const AUTH_TOKEN_KEY = 'auth_token'

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

async function readStreamErrorMessage(response: Response): Promise<string> {
  try {
    const data = (await response.json()) as { message?: unknown }
    if (data && typeof data.message === 'string' && data.message) {
      return data.message
    }
  } catch {
    // Body was not JSON; fall through to generic message
  }
  return ''
}

/**
 * Parse one SSE event block (already split on blank lines) and forward
 * assistant delta content to onDelta. Returns true once [DONE] is seen.
 */
function processSSEEvent(rawEvent: string, onDelta: (text: string) => void): boolean {
  for (const rawLine of rawEvent.split('\n')) {
    const line = rawLine.trim()
    if (!line.startsWith('data:')) {
      continue
    }
    const payload = line.slice('data:'.length).trim()
    if (!payload) {
      continue
    }
    if (payload === '[DONE]') {
      return true
    }
    try {
      const chunk = JSON.parse(payload) as WebChatCompletionChunk
      const deltaContent = chunk.choices?.[0]?.delta?.content
      if (typeof deltaContent === 'string' && deltaContent) {
        onDelta(deltaContent)
      }
    } catch {
      // Skip malformed JSON chunks instead of failing the whole stream
    }
  }
  return false
}

async function requestCompletionStream(
  params: WebChatCompletionParams,
  signal: AbortSignal,
  onDelta: (text: string) => void,
  allowAuthRetry: boolean
): Promise<void> {
  const failedAccessToken = localStorage.getItem(AUTH_TOKEN_KEY)
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (failedAccessToken) {
    headers.Authorization = `Bearer ${failedAccessToken}`
  }

  const response = await fetch(`${getAPIBaseURL()}/web-chat/chat/completions`, {
    method: 'POST',
    headers,
    body: JSON.stringify(params),
    signal,
  })

  if (!response.ok) {
    const apiMessage = await readStreamErrorMessage(response)

    // 401 before the stream starts: refresh tokens once and retry
    if (response.status === 401 && allowAuthRetry) {
      try {
        await refreshAuthTokens({ failedAccessToken })
      } catch {
        throw new Error(apiMessage || 'Unauthorized')
      }
      return requestCompletionStream(params, signal, onDelta, false)
    }

    throw new Error(apiMessage || `Request failed with status ${response.status}`)
  }

  if (!response.body) {
    throw new Error('Streaming is not supported in this browser')
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let done = false

  try {
    while (!done) {
      const { done: streamDone, value } = await reader.read()
      if (streamDone) {
        break
      }
      buffer += decoder.decode(value, { stream: true })
      buffer = buffer.replace(/\r\n/g, '\n')

      let separatorIndex = buffer.indexOf('\n\n')
      while (separatorIndex !== -1) {
        const event = buffer.slice(0, separatorIndex)
        buffer = buffer.slice(separatorIndex + 2)
        if (processSSEEvent(event, onDelta)) {
          done = true
          break
        }
        separatorIndex = buffer.indexOf('\n\n')
      }
    }
  } finally {
    reader.releaseLock()
  }

  // Flush a trailing event that was not terminated by a blank line
  if (!done && buffer.trim()) {
    processSSEEvent(buffer, onDelta)
  }
}

/**
 * Stream a web chat completion. Resolves when the stream finishes ([DONE] or
 * connection close); rejects with Error(message) on HTTP errors and lets
 * AbortError propagate when the caller aborts via the signal.
 */
export function streamWebChatCompletion(
  params: WebChatCompletionParams,
  signal: AbortSignal,
  onDelta: (text: string) => void
): Promise<void> {
  return requestCompletionStream(params, signal, onDelta, true)
}

// ==================== Audio Speech (raw audio blob) ====================

async function requestSpeechAudio(
  params: WebChatSpeechParams,
  allowAuthRetry: boolean
): Promise<Blob> {
  const failedAccessToken = localStorage.getItem(AUTH_TOKEN_KEY)
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (failedAccessToken) {
    headers.Authorization = `Bearer ${failedAccessToken}`
  }

  const response = await fetch(`${getAPIBaseURL()}/web-chat/audio/speech`, {
    method: 'POST',
    headers,
    body: JSON.stringify(params)
  })

  if (!response.ok) {
    const apiMessage = await readStreamErrorMessage(response)

    // 401 before the audio arrives: refresh tokens once and retry
    if (response.status === 401 && allowAuthRetry) {
      try {
        await refreshAuthTokens({ failedAccessToken })
      } catch {
        throw new Error(apiMessage || 'Unauthorized')
      }
      return requestSpeechAudio(params, false)
    }

    throw new Error(apiMessage || `Request failed with status ${response.status}`)
  }

  return response.blob()
}

/**
 * Synthesize speech via the web chat audio endpoint. Resolves with the raw
 * audio as a Blob; rejects with Error(message) on HTTP errors.
 */
export function speechWebChatAudio(params: WebChatSpeechParams): Promise<Blob> {
  return requestSpeechAudio(params, true)
}

export { isAbortError }
