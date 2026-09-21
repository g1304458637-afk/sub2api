/**
 * Admin Subscription Reset API（Subscription V1：Direct Reset 事件 + Reset Card 发放）
 * 与后端 /admin/subscription-resets、/admin/subscription-reset-cards 对齐。
 */

import { apiClient } from '../client'

// ── Direct Reset 事件 ──

export type ResetTargetMode = 'subscription_ids' | 'users' | 'groups' | 'all_active'

export interface ResetEventSummary {
  id: number
  status: string
  target_mode: ResetTargetMode
  effective_at: string
  reason?: string
  total_targeted: number
  applied_count: number
  skipped_count: number
  failed_count: number
  pending_count: number
  created_at: string
  started_at?: string
  completed_at?: string
}

export interface CreateResetEventRequest {
  target_mode: ResetTargetMode
  subscription_ids?: number[]
  user_ids?: number[]
  group_ids?: number[]
  effective_at?: string
  reason?: string
  idempotency_key: string
}

/** 预览目标返回（真实合同：计数 + 分组分布 + 样本） */
export interface ResetTargetPreview {
  target_mode: string
  subscription_count: number
  unique_user_count: number
  group_breakdown?: Array<{ group_id: number; name: string; subscription_count: number }>
  sample?: Array<{ subscription_id: number; user_id: number; display_name: string }>
}

export const resetEventsAPI = {
  /** 预览目标（同 create 的 target 请求；返回目标计数与样本） */
  preview(request: Omit<CreateResetEventRequest, 'idempotency_key'>) {
    return apiClient.post<ResetTargetPreview>('/admin/subscription-resets/preview', request)
  },
  create(request: CreateResetEventRequest) {
    return apiClient.post<ResetEventSummary>('/admin/subscription-resets', request, {
      headers: { 'Idempotency-Key': request.idempotency_key }
    })
  },
  list(params?: { page?: number; page_size?: number; status?: string }) {
    return apiClient.get<{ items: ResetEventSummary[]; total: number }>('/admin/subscription-resets', { params })
  },
  get(id: number) {
    return apiClient.get<ResetEventSummary>(`/admin/subscription-resets/${id}`)
  },
  /** 重试失败项（不重复已成功项） */
  retry(id: number) {
    return apiClient.post<ResetEventSummary>(`/admin/subscription-resets/${id}/retry`)
  }
}

// ── Reset Card 发放 ──

export type CardTargetMode = 'users' | 'groups' | 'all_active_users'

export interface GrantResetCardsRequest {
  target_mode: CardTargetMode
  user_ids?: number[]
  group_ids?: number[]
  quantity_per_user: number
  expires_at?: string
  reason?: string
  campaign?: string
  source_type?: string
  idempotency_key: string
}

export interface GrantResetCardsResult {
  EventID: number
  UniqueUsers: number
  TotalCards: number
  Replayed: boolean
}

export interface ResetCardRow {
  id: number
  user_id: number
  status: 'available' | 'used' | 'expired' | 'revoked' | string
  granted_at?: string
  expires_at?: string
  used_at?: string
  used_subscription_id?: number
  reason?: string
  campaign?: string
}

// ── Reward 发放审计 / 套餐变更审计（Final Frontend CLOSURE）──

export interface RewardGrantView {
  id: number
  user_id: number
  source_type: string
  campaign: string
  amount: number
  granted_by?: number
  created_at: string
}

export interface RewardStats {
  today_count: number
  today_sum: number
  month_count: number
  month_sum: number
}

export const rewardsAPI = {
  list(params?: { user_id?: number; page?: number; page_size?: number }) {
    return apiClient.get<{ items: RewardGrantView[]; total: number }>('/admin/rewards', { params })
  },
  stats(params?: { user_id?: number }) {
    return apiClient.get<RewardStats>('/admin/rewards/stats', { params })
  }
}

export interface AdminPlanChangeRow {
  ID: number
  UserID: number
  SubscriptionID: number
  ChangeType: 'upgrade' | 'scheduled_downgrade' | string
  FromPlanID: number | null
  ToPlanID: number
  ToGroupID: number
  FromTier: number
  ToTier: number
  NewPriceSnapshot: number
  Currency: string
  AmountDue: number
  Status: string
  OrderID: number | null
  EffectiveAt: string | null
  CreatedAt: string
}

export const planChangesAPI = {
  list(params?: {
    user_id?: number
    subscription_id?: number
    status?: string
    page?: number
    page_size?: number
  }) {
    return apiClient.get<{ items: AdminPlanChangeRow[]; total: number }>('/admin/plan-changes', { params })
  }
}

export const resetCardsAPI = {
  grantPreview(request: Omit<GrantResetCardsRequest, 'idempotency_key'>) {
    return apiClient.post<{ unique_users: number; total_cards: number }>(
      '/admin/subscription-reset-cards/grants/preview',
      request
    )
  },
  grant(request: GrantResetCardsRequest) {
    return apiClient.post<GrantResetCardsResult>('/admin/subscription-reset-cards/grants', request)
  },
  list(params?: { user_id?: number; status?: string; page?: number; page_size?: number }) {
    return apiClient.get<{ items: ResetCardRow[]; total: number }>(
      '/admin/subscription-reset-cards',
      { params }
    )
  },
  revoke(id: number) {
    return apiClient.post(`/admin/subscription-reset-cards/${id}/revoke`)
  },
  count(userId: number) {
    return apiClient.get<{ available: number }>('/admin/subscription-reset-cards/count', {
      params: { user_id: userId }
    })
  }
}
