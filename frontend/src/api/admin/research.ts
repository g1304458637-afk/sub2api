/**
 * Admin research application API endpoints
 * Review research discount applications submitted by users
 */

import { apiClient } from '../client'
import {
  downloadFileByUrl,
  type ResearchApplicationStatus,
  type ResearchAttachment,
  type ResearchRequestOptions
} from '../research'

export interface AdminResearchApplicant {
  id: string
  email: string
  username: string
}

export interface AdminResearchApplication {
  id: string
  user: AdminResearchApplicant
  description: string
  status: ResearchApplicationStatus
  review_notes: string | null
  reward_amount: number | null
  created_at: string
  reviewed_at: string | null
  attachments: ResearchAttachment[]
}

export interface AdminResearchApplicationListResponse {
  items: AdminResearchApplication[]
  total: number
  page: number
  page_size: number
}

export interface AdminResearchListParams {
  /** Filter by status; empty string means all */
  status?: ResearchApplicationStatus | ''
  page?: number
  page_size?: number
}

/**
 * List research applications with pagination and status filter
 */
export async function list(
  params?: AdminResearchListParams,
  options?: ResearchRequestOptions
): Promise<AdminResearchApplicationListResponse> {
  const { data } = await apiClient.get<AdminResearchApplicationListResponse>(
    '/admin/research-applications',
    {
      params: {
        status: params?.status || undefined,
        page: params?.page ?? 1,
        page_size: params?.page_size ?? 20
      },
      signal: options?.signal
    }
  )
  return data
}

/**
 * Approve a research application and grant the reward amount to the user's balance
 */
export async function approve(
  applicationId: string,
  payload: { amount: number; notes?: string }
): Promise<AdminResearchApplication> {
  const { data } = await apiClient.post<AdminResearchApplication>(
    `/admin/research-applications/${encodeURIComponent(applicationId)}/approve`,
    payload
  )
  return data
}

/**
 * Reject a research application with a required reason
 */
export async function reject(
  applicationId: string,
  payload: { notes: string }
): Promise<AdminResearchApplication> {
  const { data } = await apiClient.post<AdminResearchApplication>(
    `/admin/research-applications/${encodeURIComponent(applicationId)}/reject`,
    payload
  )
  return data
}

/**
 * Download one application attachment (admin namespace) via fetch + Blob
 * so that the Authorization header is sent.
 */
export async function downloadAttachment(
  applicationId: string,
  attachment: ResearchAttachment,
  options?: ResearchRequestOptions
): Promise<void> {
  const urlPath = `/admin/research-applications/${encodeURIComponent(
    applicationId
  )}/attachments/${encodeURIComponent(attachment.id)}`
  return downloadFileByUrl(urlPath, attachment.name, options)
}

export const adminResearchAPI = {
  list,
  approve,
  reject,
  downloadAttachment
}

export default adminResearchAPI
