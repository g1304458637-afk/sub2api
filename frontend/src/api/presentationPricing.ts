/**
 * 模型展示价格（Presentation Pricing）管理 API（仅管理员）。
 *
 * 边界：本能力只影响用户端「模型与价格」页面的标准价格展示，不影响任何真实
 * 计费。价格以 USD per token 存储；「$/MTok」输入换算由前端 mTokToPerToken 完成。
 */

import { apiClient } from './client'
import type { BillingMode } from '@/constants/channel'

export interface PresentationPricingItem {
  id: number
  model_name: string
  billing_mode: BillingMode | string
  currency: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_write_1h_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
  enabled: boolean
  remark: string
  updated_by: number | null
  updated_at: string
}

export interface PresentationPricingUpsertPayload {
  model_name: string
  billing_mode: BillingMode | string
  currency?: string
  input_price?: number | null
  output_price?: number | null
  cache_write_price?: number | null
  cache_write_1h_price?: number | null
  cache_read_price?: number | null
  per_request_price?: number | null
  enabled: boolean
  remark?: string
}

export const presentationPricingAPI = {
  async list(options?: { signal?: AbortSignal }): Promise<PresentationPricingItem[]> {
    const { data } = await apiClient.get<{ items: PresentationPricingItem[] }>(
      '/admin/model-presentation-pricing',
      { signal: options?.signal }
    )
    return data.items ?? []
  },

  async upsert(payload: PresentationPricingUpsertPayload): Promise<PresentationPricingItem> {
    const { data } = await apiClient.put<{ data: PresentationPricingItem }>(
      '/admin/model-presentation-pricing',
      payload
    )
    return data.data
  },

  async remove(modelName: string): Promise<void> {
    await apiClient.delete('/admin/model-presentation-pricing', {
      params: { model_name: modelName }
    })
  }
}

export default presentationPricingAPI
