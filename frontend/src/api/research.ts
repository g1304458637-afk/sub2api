/**
 * Research discount application API endpoints (user side)
 * Submit research discount applications with supporting attachments
 * and track the review status.
 */

import { apiClient } from './client'
import { getAPIBaseURL } from './url'

export type ResearchApplicationStatus = 'pending' | 'approved' | 'rejected'

export interface ResearchAttachment {
  id: string
  name: string
  mime: string
  size: number
}

export interface ResearchApplication {
  id: string
  description: string
  status: ResearchApplicationStatus
  review_notes: string | null
  reward_amount: number | null
  created_at: string
  reviewed_at: string | null
  attachments: ResearchAttachment[]
}

export interface ResearchApplicationListResponse {
  items: ResearchApplication[]
}

export interface SubmitResearchApplicationRequest {
  description: string
  attachment_ids: string[]
}

/** Maximum size of a single attachment: 5MB (enforced by backend as well) */
export const RESEARCH_MAX_ATTACHMENT_SIZE = 5 * 1024 * 1024

/** Maximum number of attachments per application */
export const RESEARCH_MAX_ATTACHMENT_COUNT = 5

/** Attachment MIME types accepted by the backend */
export const RESEARCH_ALLOWED_ATTACHMENT_MIME_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
  'application/pdf'
] as const

/** Maximum length of the application description */
export const RESEARCH_MAX_DESCRIPTION_LENGTH = 2000

export interface ResearchRequestOptions {
  signal?: AbortSignal
}

/**
 * Upload one attachment for a research application
 * @param file - File to upload (multipart form field "file")
 * @param onProgress - Optional progress callback (0-100)
 * @returns The uploaded attachment metadata
 */
export async function uploadAttachment(
  file: File,
  onProgress?: (percent: number) => void,
  options?: ResearchRequestOptions
): Promise<ResearchAttachment> {
  const formData = new FormData()
  formData.append('file', file)

  const { data } = await apiClient.post<ResearchAttachment>(
    '/research-applications/attachments',
    formData,
    {
      headers: { 'Content-Type': 'multipart/form-data' },
      signal: options?.signal,
      onUploadProgress: (progressEvent) => {
        if (!onProgress) return
        const total = progressEvent.total ?? 0
        if (total > 0) {
          onProgress(Math.min(100, Math.round((progressEvent.loaded / total) * 100)))
        }
      }
    }
  )
  return data
}

/**
 * Submit a research discount application
 * @param description - Application description (max 2000 chars)
 * @param attachmentIds - IDs of previously uploaded attachments
 * @returns The created application
 */
export async function submitApplication(
  description: string,
  attachmentIds: string[]
): Promise<ResearchApplication> {
  const payload: SubmitResearchApplicationRequest = {
    description,
    attachment_ids: attachmentIds
  }
  const { data } = await apiClient.post<ResearchApplication>('/research-applications', payload)
  return data
}

/**
 * List the current user's research applications (newest first)
 * @returns List of applications
 */
export async function listMyApplications(
  options?: ResearchRequestOptions
): Promise<ResearchApplication[]> {
  const { data } = await apiClient.get<ResearchApplicationListResponse>('/research-applications', {
    signal: options?.signal
  })
  return data.items
}

/**
 * Extract the filename from a Content-Disposition header,
 * supporting both `filename=` and RFC 5987 `filename*=UTF-8''...` forms.
 */
function parseContentDispositionFilename(disposition: string | null): string | null {
  if (!disposition) return null

  const utf8Match = disposition.match(/filename\*\s*=\s*([^;]+)/i)
  if (utf8Match?.[1]) {
    try {
      const encoded = utf8Match[1]
        .trim()
        .replace(/^UTF-8''/i, '')
        .replace(/^"(.*)"$/, '$1')
      const decoded = decodeURIComponent(encoded)
      if (decoded) return decoded
    } catch {
      // Malformed RFC 5987 encoding: fall through to plain filename parsing
    }
  }

  const plainMatch = disposition.match(/filename\s*=\s*"?([^";]+)"?/i)
  return plainMatch?.[1] ? plainMatch[1].trim() : null
}

/**
 * Fetch an attachment as a Blob with the Authorization header attached
 * (plain <a href> downloads cannot send the auth header).
 */
export async function fetchAttachmentBlob(
  urlPath: string,
  options?: ResearchRequestOptions
): Promise<{ blob: Blob; filename: string | null }> {
  const baseURL = getAPIBaseURL().replace(/\/+$/, '')
  const url = `${baseURL}${urlPath.startsWith('/') ? urlPath : `/${urlPath}`}`
  const token = localStorage.getItem('auth_token')

  const response = await fetch(url, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    signal: options?.signal
  })
  if (!response.ok) {
    throw new Error(`Failed to download attachment (HTTP ${response.status})`)
  }

  const blob = await response.blob()
  return {
    blob,
    filename: parseContentDispositionFilename(response.headers.get('Content-Disposition'))
  }
}

function triggerBrowserDownload(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = objectUrl
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(objectUrl)
}

/**
 * Download an attachment file through fetch + Blob so that the
 * Authorization header is sent, then trigger a browser download.
 * @param urlPath - API path of the attachment (user or admin namespace)
 * @param fallbackName - Fallback filename when Content-Disposition is absent
 */
export async function downloadFileByUrl(
  urlPath: string,
  fallbackName: string,
  options?: ResearchRequestOptions
): Promise<void> {
  const { blob, filename } = await fetchAttachmentBlob(urlPath, options)
  triggerBrowserDownload(blob, filename || fallbackName || 'attachment')
}

/**
 * Download one of the current user's application attachments
 * @param applicationId - Owning application ID
 * @param attachment - Attachment metadata (id + name)
 */
export async function downloadAttachment(
  applicationId: string,
  attachment: ResearchAttachment,
  options?: ResearchRequestOptions
): Promise<void> {
  const urlPath = `/research-applications/${encodeURIComponent(
    applicationId
  )}/attachments/${encodeURIComponent(attachment.id)}`
  return downloadFileByUrl(urlPath, attachment.name, options)
}

export const researchAPI = {
  uploadAttachment,
  submitApplication,
  listMyApplications,
  downloadAttachment
}

export default researchAPI
