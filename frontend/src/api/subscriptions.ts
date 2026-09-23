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

export type QuotaWindow = {
  remaining_percent: number
  starts_at: string | null
  resets_at: string | null
  exhausted: boolean
}

export type AccountSubscriptionStatus = {
  quota_policy?: string
  short_window?: QuotaWindow
  weekly_window?: QuotaWindow
  blocking_windows?: string[]

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

/** 已预约、未生效的套餐变更（additive 合同；升级不预约，只会是 scheduled_downgrade） */
export type AccountPendingPlanChange = {
  change_type: string
  to_plan_id: number
  to_plan_name: string
  effective_at: string
  current_period_ends_at: string
}

/**
 * 上一份已结束的套餐周期（Phase 11B 预付费固定周期制）。
 * 过期即 EXPIRED，无自动续费；仅用于"上次套餐"展示。
 */
export type AccountLastSubscription = {
  group_id: number
  display_name: string
  expires_at: string
}

export type AccountStatus = {
  wallet: AccountWalletStatus
  reset_cards: AccountResetCardsStatus
  /** 单主套餐不变量生效后至多一个元素；0 个是合法状态（到期未续费）。保留数组形态兼容 legacy 消费端 */
  subscriptions: AccountSubscriptionStatus[]
  /**
   * 兼容字段：表达"下次续费套餐变更"。
   * effective_at = 最近一条订阅的 expires_at（最早续费切换点）——
   * 系统绝不自动激活目标套餐，客户端不得如此解读。
   */
  pending_change?: AccountPendingPlanChange
  /** 下次续费的默认目标（Phase 11B additive，与 pending_change 同源） */
  next_renewal_plan_id?: number
  next_renewal_plan?: string
  last_subscription?: AccountLastSubscription
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

export type WalletLedgerEntry = {
  id: string
  type: 'wallet_recharge' | 'redeem_balance' | 'reward' | 'payg_usage' | 'refund' | 'manual_adjustment'
  amount: number
  currency?: string
  ref?: string
  created_at: string
}

export async function getWalletLedger(limit = 50, page = 1): Promise<{ entries: WalletLedgerEntry[]; total: number }> {
  const response = await apiClient.get<{ entries: WalletLedgerEntry[]; total: number }>('/wallet/ledger', {
    params: { page_size: limit, page }
  })
  return response.data
}

// ── Phase 10 Plan Change（升级/到期切换）；金额全部来自服务端权威报价 ──

/** 服务端权威报价。金额字段为 decimal 字符串（后端 json:",string"），禁止用 float 重算。 */
export type PlanChangeQuote = {
  subscription_id: number
  change_type: 'upgrade' | 'scheduled_downgrade'
  from_plan_id: number
  to_plan_id: number
  from_display_name: string
  to_display_name: string
  effective_at: string
  current_expiry: string
  /** 升级=不变；降级=term 末 */
  new_expiry: string
  remaining_seconds: number
  /** decimal 字符串 */
  unused_credit: string
  /** decimal 字符串 */
  prorated_charge: string
  /** decimal 字符串 */
  amount_due: string
  currency: string
  short_remaining_percent_before?: number
  short_remaining_percent_after?: number
  weekly_remaining_percent_after?: number
  weekly_usage_percent_before: number | null
  weekly_usage_percent_after: number | null
  usage_status_after: UsageStatus
  keys_to_migrate_count: number
}

/**
 * GET /subscriptions/:id/changes 审计记录。后端返回无 json tag 的 Go 结构体，
 * 键为 PascalCase——保持原样，勿"纠正"为 snake_case。
 */
export type PlanChangeRecordDto = {
  ID: number
  UserID: number
  SubscriptionID: number
  ChangeType: 'upgrade' | 'scheduled_downgrade' | string
  FromPlanID: number | null
  ToPlanID: number
  FromGroupID: number | null
  ToGroupID: number
  FromTier: number
  ToTier: number
  OldPriceSnapshot: number | null
  NewPriceSnapshot: number
  Currency: string
  TermStart: string | null
  TermEnd: string | null
  RemainingSeconds: number
  UnusedCredit: number
  ProratedCharge: number
  AmountDue: number
  QuoteCreatedAt: string | null
  QuoteExpiresAt: string | null
  EffectiveAt: string | null
  Status: 'quoted' | 'scheduled' | 'paid' | 'fulfilled' | 'cancelled' | string
  CancelReason: string | null
  OrderID: number | null
  IdempotencyKey: string | null
  PaidAt: string | null
  FulfilledAt: string | null
  CancelledAt: string | null
  CreatedAt: string
  UpdatedAt: string
}

/** 升级下单返回（金额来自冻结报价行，客户端不传 amount）。 */
export type PlanUpgradeOrder = {
  order_id: number
  plan_change_id: number
  amount: number
  pay_amount: number
  payment_type: string
  status: string
  pay_url?: string
  qr_code?: string
}

export async function previewUpgrade(
  subscriptionId: number,
  targetPlanId: number
): Promise<PlanChangeQuote> {
  const response = await apiClient.post<PlanChangeQuote>(
    `/subscriptions/${subscriptionId}/change/preview`,
    { target_plan_id: targetPlanId }
  )
  return response.data
}

export async function createUpgrade(
  subscriptionId: number,
  targetPlanId: number,
  paymentType: string,
  idempotencyKey: string
): Promise<PlanUpgradeOrder> {
  const response = await apiClient.post<PlanUpgradeOrder>(
    `/subscriptions/${subscriptionId}/upgrade`,
    { target_plan_id: targetPlanId, payment_type: paymentType },
    { headers: { 'Idempotency-Key': idempotencyKey, 'X-Quota-Contract': '2' } }
  )
  return response.data
}

export async function scheduleDowngrade(
  subscriptionId: number,
  targetPlanId: number,
  idempotencyKey: string
): Promise<PlanChangeRecordDto> {
  const response = await apiClient.post<PlanChangeRecordDto>(
    `/subscriptions/${subscriptionId}/schedule-downgrade`,
    { target_plan_id: targetPlanId },
    { headers: { 'Idempotency-Key': idempotencyKey, 'X-Quota-Contract': '2' } }
  )
  return response.data
}

export async function cancelScheduledDowngrade(subscriptionId: number): Promise<void> {
  await apiClient.delete(`/subscriptions/${subscriptionId}/schedule-downgrade`)
}

export async function getSubscriptionChanges(subscriptionId: number): Promise<PlanChangeRecordDto[]> {
  const response = await apiClient.get<PlanChangeRecordDto[]>(
    `/subscriptions/${subscriptionId}/changes`
  )
  return response.data
}

/** 使用重置卡（幂等：Idempotency-Key 必带；重放只消费一张卡）。 */
export async function resetWithCard(
  subscriptionId: number,
  idempotencyKey: string
): Promise<{ subscription_id: number; weekly_period_ends_at: string }> {
  const response = await apiClient.post<{ subscription_id: number; weekly_period_ends_at: string }>(
    `/subscriptions/${subscriptionId}/reset-with-card`,
    {},
    { headers: { 'Idempotency-Key': idempotencyKey, 'X-Quota-Contract': '2' } }
  )
  return response.data
}

/** 开关「额度用完后继续使用」（后端语义：PAYG fallback）。 */
export async function updatePaygFallback(
  subscriptionId: number,
  enabled: boolean
): Promise<{ subscription_id: number; payg_fallback: boolean }> {
  const response = await apiClient.patch<{ subscription_id: number; payg_fallback: boolean }>(
    `/subscriptions/${subscriptionId}/payg-fallback`,
    { enabled }
  )
  return response.data
}

export default {
  getMySubscriptions,
  getActiveSubscriptions,
  getSubscriptionsProgress,
  getSubscriptionSummary,
  getSubscriptionProgress,
  getAccountStatus,
  previewUpgrade,
  createUpgrade,
  scheduleDowngrade,
  cancelScheduledDowngrade,
  getSubscriptionChanges,
  getWalletLedger,
  resetWithCard,
  updatePaygFallback
}
