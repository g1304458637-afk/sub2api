/**
 * MUC Harness API：桌面客户端一键连接
 * - connect-code：需登录态，签发 60s 一次性授权码（绑定当前用户）
 * - 授权码通过 muc://connect?code=... 唤起 MUC Desktop，绝不含 API Key
 */

import { apiClient } from './client'

export interface MucConnectCode {
  code: string
  expires_in: number
}

export async function createMucConnectCode(): Promise<MucConnectCode> {
  // apiClient.baseURL 已含 /api/v1 —— 这里必须用相对路径，否则拼出 /api/v1/api/v1/... 404
  const res = await apiClient.post('/muc/connect-code')
  return res.data?.data ?? res.data
}
