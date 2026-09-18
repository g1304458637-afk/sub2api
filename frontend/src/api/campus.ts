/**
 * 校园 Harness API（MUC / HUBU 共用）：桌面客户端一键连接
 * - connect-code：需登录态，签发 60s 一次性授权码（绑定当前用户）
 * - 授权码通过 {muc|hubu}://connect?code=... 唤起对应桌面端，绝不含 API Key
 * 历史 MUC 版本见 api/muc.ts（保持不变）。
 */

import { apiClient } from './client'
import type { BrandConfig } from '@/brand'

export interface CampusConnectCode {
  code: string
  expires_in: number
}

export async function createCampusConnectCode(brand: Pick<BrandConfig, 'pathSegment'>): Promise<CampusConnectCode> {
  const res = await apiClient.post(`/${brand.pathSegment}/connect-code`)
  return res.data?.data ?? res.data
}
