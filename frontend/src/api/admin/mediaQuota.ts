/**
 * Admin Media Quota API endpoints
 *
 * 媒体（图片 / 音乐 / 语音）额度与计费监控页面所需的数据聚合：
 * - 图片额度来自媒体代理端点 GET /admin/media/image-quota（后端桥接 MUC 图片代理）
 * - 按模型聚合的用量统计来自既有端点 GET /admin/dashboard/models（返回
 *   {model, requests, cost, actual_cost} 列表；本页取"今日"范围做聚合）
 * - 分组定价来自既有端点 GET /admin/groups/all?platform=openai
 *
 * 模型匹配规则集中定义在 MEDIA_MODELS：
 * - 图片：模型精确等于 gpt-image-2
 * - 音乐：模型以 "suno" 开头的条目求和
 * - 语音：模型以 "audio" 开头的条目求和
 */

import { apiClient } from '../client'
import { groupsAPI } from './groups'
import { formatDateLocalInput } from '@/utils/format'
import type { AdminGroup } from '@/types'

// ==================== 模型匹配常量（集中定义） ====================

/** 三类媒体的模型匹配规则：图片精确匹配、音乐/语音前缀匹配 */
export const MEDIA_MODELS = {
  imageExact: ['gpt-image-2'],
  musicPrefix: 'suno',
  audioPrefix: 'audio'
} as const

/** 图片精确匹配（任一命中即为图片模型） */
export function isImageModel(model: string): boolean {
  return (MEDIA_MODELS.imageExact as readonly string[]).includes(model)
}

/** 音乐前缀匹配 */
export function isMusicModel(model: string): boolean {
  return model.startsWith(MEDIA_MODELS.musicPrefix)
}

/** 语音前缀匹配 */
export function isAudioModel(model: string): boolean {
  return model.startsWith(MEDIA_MODELS.audioPrefix)
}

// ==================== 图片额度端点类型 ====================

/** 图片额度窗口（3 小时滚动窗口） */
export interface MediaImageQuotaBudget {
  limit: number
  used: number
  remaining: number
  /** 窗口长度（毫秒），如 3 小时 = 10800000 */
  window_ms: number
  /** 窗口重置时间：ISO 字符串或 unix 秒 */
  reset_at: string | number | null
}

/** 图片代理最近请求记录 */
export interface MediaImageQuotaRecentItem {
  request_id: string
  ts: string | number
  duration_ms: number | null
  status: string
  /** 张数 */
  n: number
  /** 尺寸，如 "1024x1024" */
  size: string | null
  /** 文件数 */
  files: number
}

export interface MediaImageQuotaResponse {
  /** 未配置 MUC_IMAGE_BRIDGE_URL 时为 false */
  enabled: boolean
  /** enabled 且代理健康检查通过时为 true */
  reachable?: boolean
  /** reachable=false 时的错误信息 */
  error?: string
  version?: string
  /** 代理运行时长（秒） */
  uptime_s?: number
  budget?: MediaImageQuotaBudget
  /** 当前是否有生成任务在执行 */
  busy?: boolean
  totals?: {
    today: number
    last_24h: number
    success: number
    failed: number
  }
  recent?: MediaImageQuotaRecentItem[]
  /** 代理鉴权是否正常 */
  auth_ok?: boolean
  latency?: {
    p50_ms: number
    p95_ms: number
  }
}

/**
 * 获取图片额度（媒体代理状态）。
 * 三态：{enabled:false}（未配置）/ {enabled,reachable:false,error} / 完整数据。
 */
export async function getImageQuota(): Promise<MediaImageQuotaResponse> {
  const { data } = await apiClient.get<MediaImageQuotaResponse>('/admin/media/image-quota')
  return data
}

// ==================== 按模型聚合统计 ====================

/** 按模型聚合的用量统计条目（GET /admin/dashboard/models 返回列表的子集） */
export interface MediaModelStat {
  model: string
  requests: number
  cost: number
  actual_cost: number
}

export interface MediaUsageSummaryItem {
  requests: number
  cost: number
}

export interface MediaUsageSummary {
  image: MediaUsageSummaryItem
  music: MediaUsageSummaryItem
  audio: MediaUsageSummaryItem
}

function emptySummaryItem(): MediaUsageSummaryItem {
  return { requests: 0, cost: 0 }
}

/**
 * 按媒体类型聚合按模型统计：图片精确匹配、音乐/语音前缀匹配求和。
 * 其它模型（如 gpt-4o）不参与任何汇总。
 */
export function aggregateMediaStats(models: MediaModelStat[]): MediaUsageSummary {
  const summary: MediaUsageSummary = {
    image: emptySummaryItem(),
    music: emptySummaryItem(),
    audio: emptySummaryItem()
  }
  for (const stat of models) {
    if (isImageModel(stat.model)) {
      summary.image.requests += stat.requests
      summary.image.cost += stat.cost
    } else if (isMusicModel(stat.model)) {
      summary.music.requests += stat.requests
      summary.music.cost += stat.cost
    } else if (isAudioModel(stat.model)) {
      summary.audio.requests += stat.requests
      summary.audio.cost += stat.cost
    }
  }
  return summary
}

/**
 * 获取"今日"按模型聚合统计（本地时区当日 0 点起）。
 * 复用既有 admin dashboard model-stats 端点。
 */
export async function getMediaModelStats(): Promise<MediaModelStat[]> {
  const today = formatDateLocalInput(new Date())
  const { data } = await apiClient.get<{ models?: MediaModelStat[] }>('/admin/dashboard/models', {
    params: { start_date: today, end_date: today }
  })
  return data.models ?? []
}

// ==================== 分组定价 ====================

/** openai 平台分组的媒体定价字段（部分字段为分组级可选覆盖，null = 使用默认价） */
export interface MediaPricing {
  /** null = 默认价 */
  imagePrice1k: number | null
  imagePrice2k: number | null
  imagePrice4k: number | null
  /** null = 默认 $0.5/首 */
  musicPricePerTrack: number | null
  /** null = 默认价 */
  audioTtsPricePerMillionChars: number | null
  audioRealtimePricePerMin: number | null
  audioSttPricePerHour: number | null
}

/** 音乐每首默认价（USD/首） */
export const DEFAULT_MUSIC_PRICE_PER_TRACK = 0.5

type MediaPricingGroup = AdminGroup & { music_price_per_track?: number | null }

function firstNonNull<T>(values: Array<T | null | undefined>): T | null {
  for (const value of values) {
    if (value !== null && value !== undefined) return value
  }
  return null
}

/**
 * 读取 openai 平台分组的媒体定价。
 * 多个 openai 分组时按字段取第一个非 null 值；无 openai 分组时返回全 null。
 */
export async function getMediaPricing(): Promise<{
  pricing: MediaPricing
  /** 定价来源分组名（取第一个含非 null 定价的分组） */
  groupName: string | null
  found: boolean
}> {
  const groups = (await groupsAPI.getAll('openai')) as MediaPricingGroup[]
  const pricing: MediaPricing = {
    imagePrice1k: firstNonNull(groups.map((g) => g.image_price_1k)),
    imagePrice2k: firstNonNull(groups.map((g) => g.image_price_2k)),
    imagePrice4k: firstNonNull(groups.map((g) => g.image_price_4k)),
    musicPricePerTrack: firstNonNull(groups.map((g) => g.music_price_per_track)),
    audioTtsPricePerMillionChars: firstNonNull(groups.map((g) => g.audio_tts_price_per_million_chars)),
    audioRealtimePricePerMin: firstNonNull(groups.map((g) => g.audio_realtime_price_per_min)),
    audioSttPricePerHour: firstNonNull(groups.map((g) => g.audio_stt_price_per_hour))
  }
  const sourceGroup = groups.find(
    (g) =>
      g.image_price_1k != null ||
      g.image_price_2k != null ||
      g.image_price_4k != null ||
      g.music_price_per_track != null ||
      g.audio_tts_price_per_million_chars != null ||
      g.audio_realtime_price_per_min != null ||
      g.audio_stt_price_per_hour != null
  )
  return { pricing, groupName: sourceGroup?.name ?? null, found: groups.length > 0 }
}

export const adminMediaQuotaAPI = {
  getImageQuota,
  getMediaModelStats,
  getMediaPricing,
  aggregateMediaStats
}

export default adminMediaQuotaAPI
