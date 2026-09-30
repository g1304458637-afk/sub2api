/**
 * Admin subscription grants API (订阅赠送).
 *
 * All mutating endpoints are idempotent and require an `Idempotency-Key`
 * header. Keys are generated client-side with crypto.randomUUID(); reuse the
 * same key when retrying the same logical operation so the backend can
 * deduplicate.
 */

import { apiClient } from '../client'

/** How the granted duration is applied to the user's current subscription. */
export type GrantEffectivePolicy = 'immediate' | 'end_of_term'

/** Why the grant was issued. */
export type GrantSource =
  | 'admin_grant'
  | 'compensation'
  | 'campaign'
  | 'school_bulk'
  | 'teacher_verification'
  | 'invitation'
  | 'internal'
  | 'other'

export const GRANT_SOURCES: GrantSource[] = [
  'admin_grant',
  'compensation',
  'campaign',
  'school_bulk',
  'teacher_verification',
  'invitation',
  'internal',
  'other'
]

/** Preview outcome. `conflict` blocks submission (cross-group conflict). */
export type GrantPreviewOutcome = 'will_activate_new' | 'will_extend' | 'will_be_pending' | 'conflict'

/** Result action reported after a grant is created. */
export type GrantAction = 'activated_new' | 'extended' | 'pending' | 'already_granted'

/** Ledger status of a grant record. */
export type GrantStatus = 'pending' | 'fulfilled' | 'expired' | 'revoked' | 'failed'

export interface SubscriptionGrantPreviewRequest {
  user_id: number
  group_id: number
  plan_id?: number
  duration_days: number
  effective_policy: GrantEffectivePolicy
  source: GrantSource
}

export interface SubscriptionGrantPreview {
  outcome: GrantPreviewOutcome
  current_group_id?: number
  current_plan_name?: string
  current_expires?: string
  predicted_expires?: string
  extension_base?: string
  /** Chinese ops-facing hint produced by the backend; shown as-is. */
  message: string
}

export interface SubscriptionGrantCreateRequest {
  user_id: number
  group_id: number
  plan_id?: number
  duration_days: number
  effective_policy: GrantEffectivePolicy
  source: GrantSource
  reason?: string
  notes?: string
}

export interface SubscriptionGrantOutcome {
  action: GrantAction
  grant_id: number
  subscription_id?: number
  previous_expires?: string
  expires_at?: string
  /** Chinese ops-facing hint produced by the backend; shown as-is. */
  message: string
}

export interface SubscriptionGrantCreateResult {
  grant: SubscriptionGrantRecord
  outcome: SubscriptionGrantOutcome
}

export interface SubscriptionGrantBulkRequest {
  user_ids: number[]
  group_id: number
  duration_days: number
  effective_policy: GrantEffectivePolicy
  source: GrantSource
  reason?: string
  notes?: string
}

export interface SubscriptionGrantBulkItem {
  user_id: number
  success: boolean
  action?: GrantAction
  grant_id?: number
  expires_at?: string
  error?: string
}

export interface SubscriptionGrantBulkResult {
  items: SubscriptionGrantBulkItem[]
  total: number
  success_count: number
  failed_count: number
}

/** Ledger row of a single subscription grant. */
export interface SubscriptionGrantRecord {
  id: number
  user_id: number
  group_id: number
  plan_id?: number
  source: GrantSource | string
  source_key?: string
  benefit_code?: string
  identity_type?: string
  identity_key?: string
  idempotency_key?: string
  status: GrantStatus
  effective_policy: GrantEffectivePolicy
  duration_days: number
  reason?: string
  notes?: string
  operator_user_id?: number
  linked_subscription_id?: number
  contribution_start?: string
  contribution_end?: string
  activated_at?: string
  revoked_at?: string
  revoked_by?: number
  revoke_reason?: string
  failure_reason?: string
  created_at: string
  updated_at: string
  user_email?: string
  username?: string
  operator_email?: string
  group_name?: string
}

export interface SubscriptionGrantListQuery {
  page?: number
  page_size?: number
  user_id?: number
  source?: string
  status?: string
  benefit_code?: string
  group_id?: number
}

export interface SubscriptionGrantListResponse {
  items: SubscriptionGrantRecord[]
  total: number
  page: number
  page_size: number
}

export interface SubscriptionGrantRevokeRequest {
  reason: string
}

/** Generate a fresh idempotency key for one dialog/operation session. */
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  // Fallback for non-secure contexts without crypto.randomUUID.
  return `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`.replace(/[xy]/g, (ch) => {
    const rand = (Math.random() * 16) | 0
    const value = ch === 'x' ? rand : (rand & 0x3) | 0x8
    return value.toString(16)
  })
}

/**
 * Preview the outcome of a grant without side effects.
 */
export async function preview(
  request: SubscriptionGrantPreviewRequest
): Promise<SubscriptionGrantPreview> {
  const { data } = await apiClient.post<SubscriptionGrantPreview>(
    '/admin/subscription-grants/preview',
    request
  )
  return data
}

/**
 * Grant a subscription benefit to a single user (idempotent).
 */
export async function create(
  request: SubscriptionGrantCreateRequest,
  idempotencyKey: string
): Promise<SubscriptionGrantCreateResult> {
  const { data } = await apiClient.post<SubscriptionGrantCreateResult>(
    '/admin/subscription-grants',
    request,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  return data
}

/**
 * Grant the same subscription benefit to up to 100 users (idempotent).
 */
export async function bulk(
  request: SubscriptionGrantBulkRequest,
  idempotencyKey: string
): Promise<SubscriptionGrantBulkResult> {
  const { data } = await apiClient.post<SubscriptionGrantBulkResult>(
    '/admin/subscription-grants/bulk',
    request,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  return data
}

/**
 * List the grant ledger with pagination and exact-match filters.
 */
export async function list(
  query: SubscriptionGrantListQuery = {},
  options?: {
    signal?: AbortSignal
  }
): Promise<SubscriptionGrantListResponse> {
  const { data } = await apiClient.get<SubscriptionGrantListResponse>(
    '/admin/subscription-grants',
    {
      params: {
        page: query.page ?? 1,
        page_size: query.page_size ?? 20,
        user_id: query.user_id || undefined,
        source: query.source || undefined,
        status: query.status || undefined,
        benefit_code: query.benefit_code?.trim() || undefined,
        group_id: query.group_id || undefined
      },
      signal: options?.signal
    }
  )
  return data
}

/**
 * Revoke a pending/fulfilled grant (idempotent). Reason is required, <= 500 chars.
 */
export async function revoke(
  id: number,
  request: SubscriptionGrantRevokeRequest,
  idempotencyKey: string
): Promise<{ grant?: SubscriptionGrantRecord; message?: string }> {
  const { data } = await apiClient.post<{ grant?: SubscriptionGrantRecord; message?: string }>(
    `/admin/subscription-grants/${id}/revoke`,
    request,
    { headers: { 'Idempotency-Key': idempotencyKey } }
  )
  return data
}

export const subscriptionGrantsAPI = {
  preview,
  create,
  bulk,
  list,
  revoke,
  newIdempotencyKey
}

export default subscriptionGrantsAPI
