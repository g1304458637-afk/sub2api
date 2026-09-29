/**
 * 学生邮箱认证 API（品牌路径）：/api/v1/{pathSegment}/student-verification/*
 * - send-code：向学生邮箱发送验证码（受限流约束）
 * - verify：提交验证码完成认证，成功后由服务端自动发放学生专属会员权益（幂等：Idempotency-Key）
 * - status：查询当前用户的认证与权益发放状态
 * 品牌路径拼接方式与 api/campus.ts 一致（baseURL 已含 /api/v1）。
 */

import { apiClient } from './client'
import type { BrandConfig } from '@/brand'

/** GET status 响应：当前用户的认证与权益状态摘要 */
export interface StudentVerificationStatus {
  email_verified: boolean
  email?: string
  provider?: string
  /** 学生邮箱域名后缀（不含 @），如 stu.hubu.edu.cn */
  email_domain?: string
  benefit_code?: string
  benefit_days?: number
  verified_at?: string
  grant_status?: string
  grant_expires_at?: string
}

/** POST send-code 响应 */
export interface StudentVerificationCodeSent {
  sent: boolean
}

/** verify 成功响应中的权益发放结果 */
export interface StudentVerificationGrantOutcome {
  action: 'activated_new' | 'extended' | 'pending' | 'already_granted' | string
  grant_id?: string
  subscription_id?: string
  previous_expires?: string
  expires_at?: string
  message?: string
}

/** POST verify 响应：认证结果 + 权益发放详情 */
export interface StudentVerificationResult {
  verified: boolean
  email: string
  verified_at: string
  grant_status?: string
  outcome: StudentVerificationGrantOutcome
  benefit_code?: string
  duration_days?: number
  expires_at?: string
  activated_at?: string
  message?: string
}

/** 生成幂等键（与 subscriptions/batchImage 等模块同一兜底策略） */
function buildIdempotencyKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

/**
 * 向学生邮箱发送验证码。
 * @param brand 品牌配置（取 pathSegment 拼 API 路径）
 * @param email 学生邮箱地址
 */
export async function sendStudentVerificationCode(
  brand: Pick<BrandConfig, 'pathSegment'>,
  email: string
): Promise<StudentVerificationCodeSent> {
  const res = await apiClient.post(`/${brand.pathSegment}/student-verification/send-code`, { email })
  return res.data?.data ?? res.data
}

/**
 * 提交验证码完成学生身份认证（幂等：Idempotency-Key）。
 * @param brand 品牌配置（取 pathSegment 拼 API 路径）
 * @param email 学生邮箱地址
 * @param code 邮箱验证码
 */
export async function verifyStudentVerification(
  brand: Pick<BrandConfig, 'pathSegment'>,
  email: string,
  code: string
): Promise<StudentVerificationResult> {
  const res = await apiClient.post(
    `/${brand.pathSegment}/student-verification/verify`,
    { email, code },
    { headers: { 'Idempotency-Key': buildIdempotencyKey() } }
  )
  return res.data?.data ?? res.data
}

/**
 * 查询当前用户的认证与权益发放状态。
 * @param brand 品牌配置（取 pathSegment 拼 API 路径）
 */
export async function getStudentVerificationStatus(
  brand: Pick<BrandConfig, 'pathSegment'>
): Promise<StudentVerificationStatus> {
  const res = await apiClient.get(`/${brand.pathSegment}/student-verification/status`)
  return res.data?.data ?? res.data
}
