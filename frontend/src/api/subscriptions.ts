/**
 * User Subscription API
 * API for regular users to view their own subscriptions and progress
 */

import { apiClient } from './client'
import type { UserSubscription, SubscriptionProgress } from '@/types'

/**
 * Subscription summary for user dashboard
 */
export interface SubscriptionSummary {
  active_count: number
  subscriptions: Array<{
    id: number
    group_name: string
    status: string
    daily_progress: number | null
    weekly_progress: number | null
    monthly_progress: number | null
    expires_at: string | null
    days_remaining: number | null
  }>
}

/**
 * Get list of current user's subscriptions
 */
export async function getMySubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions')
  return response.data
}

/**
 * Get current user's active subscriptions
 */
export async function getActiveSubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions/active')
  return response.data
}

/**
 * Get progress for all user's active subscriptions
 */
export async function getSubscriptionsProgress(): Promise<SubscriptionProgress[]> {
  const response = await apiClient.get<SubscriptionProgress[]>('/subscriptions/progress')
  return response.data
}

/**
 * Get subscription summary for dashboard display
 */
export async function getSubscriptionSummary(): Promise<SubscriptionSummary> {
  const response = await apiClient.get<SubscriptionSummary>('/subscriptions/summary')
  return response.data
}

/**
 * Get progress for a specific subscription
 */
export async function getSubscriptionProgress(
  subscriptionId: number
): Promise<SubscriptionProgress> {
  const response = await apiClient.get<SubscriptionProgress>(
    `/subscriptions/${subscriptionId}/progress`
  )
  return response.data
}

// ── Phase 4/4.1 统一账户状态合同（净化视图：无内部 USD 额度/倍率） ──

export type AccountWalletStatus = {
  /** users.balance，NUMERIC(20,8)，8 位小数字符串 */
  balance: string
  canonical_currency: string
}

export type AccountResetCardsStatus = {
  /** 账户级可用 Reset Card 数（Reset Card 属于 User，非单条订阅） */
  available: number
}

export type UsageStatus = 'unmetered' | 'normal' | 'high' | 'near_limit' | 'exhausted'

export type AccountSubscriptionStatus = {
  id: number
  group_id: number
  /** 权益展示身份 = Group 名（非购买 SKU 名） */
  display_name: string
  /** 服务端计算并 clamp 0..100 的整数；unmetered 时为 null */
  weekly_usage_percent: number | null
  usage_status: UsageStatus
  weekly_period_started_at: string | null
  weekly_period_ends_at: string | null
  expires_at: string
  payg_fallback: boolean
  /** Reset Card Runtime 未启用，恒为 0 */
  reset_cards_available: number
}

export type AccountStatus = {
  wallet: AccountWalletStatus
  reset_cards: AccountResetCardsStatus
  subscriptions: AccountSubscriptionStatus[]
}

/**
 * Get the unified account status (wallet + all active subscriptions, sanitized).
 * Percent / usage status / period are computed server-side — clients must not
 * recompute them from internal USD values.
 */
export async function getAccountStatus(): Promise<AccountStatus> {
  const response = await apiClient.get<AccountStatus>('/subscriptions/status')
  return response.data
}

export default {
  getMySubscriptions,
  getActiveSubscriptions,
  getSubscriptionsProgress,
  getSubscriptionSummary,
  getSubscriptionProgress,
  getAccountStatus
}
