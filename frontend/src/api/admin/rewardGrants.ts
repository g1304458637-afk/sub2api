/**
 * Admin reward grants ledger API (read-only).
 *
 * reward_grants is the single source of truth for "system reward balance"
 * credits (student verification rewards today, future campaigns later).
 * This admin endpoint is read-only: grants are written exclusively through
 * the idempotent reward grant service.
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface RewardGrantRecord {
  id: number
  user_id: number
  email: string
  username: string
  idempotency_key: string
  source_type: string
  source_id: number | null
  campaign: string
  /** USD decimal (not cents) */
  amount: number
  granted_by: number | null
  granted_by_email: string
  created_at: string
}

export interface RewardGrantQuery {
  page?: number
  page_size?: number
  /** Exact user id filter */
  user_id?: number
  /** Exact campaign filter */
  campaign?: string
  /** Exact source type filter; empty means all */
  source_type?: string
}

export type RewardGrantListResponse = PaginatedResponse<RewardGrantRecord>

/**
 * List reward grants (paginated, exact-match filters).
 */
export async function listRewardGrants(query: RewardGrantQuery): Promise<RewardGrantListResponse> {
  const { data } = await apiClient.get<RewardGrantListResponse>('/admin/reward-grants', {
    params: {
      page: query.page ?? 1,
      page_size: query.page_size ?? 20,
      user_id: query.user_id || undefined,
      campaign: query.campaign?.trim() || undefined,
      source_type: query.source_type || undefined
    }
  })
  return data
}

export const adminRewardGrantsAPI = {
  list: listRewardGrants
}

export default adminRewardGrantsAPI
