/**
 * Admin Web Chat API endpoints
 * 网页聊天：可用模型候选（全部分组模型并集）与 web_chat_* 设置的序列化辅助函数。
 * 设置本体仍走通用 GET/PUT /admin/settings（web_chat_enabled / web_chat_models / web_chat_default_model）。
 */

import { apiClient } from "../client";

/**
 * GET /api/v1/admin/web-chat/available-models
 * 返回全部分组可用模型的并集（去重后的模型 ID 列表）。
 */
export async function getWebChatAvailableModels(): Promise<string[]> {
  const { data } = await apiClient.get<{ models: string[] }>(
    "/admin/web-chat/available-models",
  );
  return Array.isArray(data?.models) ? data.models.filter((m) => typeof m === "string" && m) : [];
}

/** web_chat_models 设置（JSON 数组字符串）中的单个模型条目 */
export interface WebChatModelSetting {
  model: string;
  display_name: string;
  vendor: string;
  description: string;
  type: "chat" | "image";
  api_only: boolean;
}

/**
 * 解析 web_chat_models（JSON 字符串）为模型列表。
 * 空串/解析失败/非数组一律视为空数组。
 */
export function parseWebChatModels(raw: unknown): WebChatModelSetting[] {
  if (typeof raw !== "string" || !raw.trim()) return [];
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .filter(
        (item): item is Record<string, unknown> =>
          !!item && typeof item === "object" && !Array.isArray(item),
      )
      .map((item) => ({
        model: typeof item.model === "string" ? item.model : "",
        display_name: typeof item.display_name === "string" ? item.display_name : "",
        vendor: typeof item.vendor === "string" ? item.vendor : "",
        description: typeof item.description === "string" ? item.description : "",
        type: item.type === "image" ? ("image" as const) : ("chat" as const),
        api_only: item.api_only === true,
      }));
  } catch {
    return [];
  }
}

/**
 * 序列化模型列表为 web_chat_models 字符串（保持条目原样，便于行内编辑）。
 * 空数组存空串。
 */
export function serializeWebChatModels(models: WebChatModelSetting[]): string {
  if (!Array.isArray(models) || models.length === 0) return "";
  return JSON.stringify(
    models.map((m) => ({
      model: typeof m.model === "string" ? m.model : "",
      display_name: typeof m.display_name === "string" ? m.display_name : "",
      vendor: typeof m.vendor === "string" ? m.vendor : "",
      description: typeof m.description === "string" ? m.description : "",
      type: m.type === "image" ? ("image" as const) : ("chat" as const),
      api_only: m.api_only === true,
    })),
  );
}

/**
 * 保存时清洗：剔除未填模型 ID 的行后重新序列化。
 * 供 SettingsView 的保存链路展开进 UpdateSettingsRequest（后端为通用 JSON 设置名，
 * 前端 SystemSettings 类型暂未声明这三个字段，故返回独立类型再展开）。
 */
export function buildWebChatSettingsPayload(form: unknown): {
  web_chat_enabled: boolean;
  web_chat_models: string;
  web_chat_default_model: string;
} {
  const f = (form ?? {}) as Record<string, unknown>;
  return {
    web_chat_enabled: f.web_chat_enabled === true,
    web_chat_models: serializeWebChatModels(
      parseWebChatModels(f.web_chat_models).filter((m) => m.model.trim()),
    ),
    web_chat_default_model:
      typeof f.web_chat_default_model === "string" ? f.web_chat_default_model.trim() : "",
  };
}
