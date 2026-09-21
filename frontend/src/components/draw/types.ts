/**
 * 网页绘图共享类型与常量
 */

/** 画廊中的一张结果图：统一展示 src，按 kind 区分下载策略 */
export interface DrawImageItem {
  id: string
  /** 展示用 src（远程 url 或 data URL） */
  src: string
  /** url: 下载需 fetch→blob；dataUrl: 直接以 data URL 下载 */
  kind: 'url' | 'dataUrl'
  url?: string
  dataUrl?: string
  prompt: string
  createdAt: number
}

/** 尺寸档位（后端契约固定三档） */
export interface DrawSizeOption {
  value: string
  /** i18n key（draw.form.* 下） */
  labelKey: string
}

export const DRAW_SIZE_OPTIONS: DrawSizeOption[] = [
  { value: '1024x1024', labelKey: 'draw.form.sizeSquare' },
  { value: '1536x1024', labelKey: 'draw.form.sizeLandscape' },
  { value: '1024x1536', labelKey: 'draw.form.sizePortrait' },
]

/** 单次生成张数范围 */
export const DRAW_COUNT_MIN = 1
export const DRAW_COUNT_MAX = 4

/** 会话内最多保留的结果张数（b64 data URL 体积大，防止无限累积撑爆内存） */
export const DRAW_RESULT_LIMIT = 24

/** 由后端返回的图片结果转换为画廊条目 */
export function toDrawImageItem(
  result: { url?: string; b64_json?: string },
  prompt: string,
  index: number,
): DrawImageItem {
  const createdAt = Date.now()
  if (result.b64_json) {
    const dataUrl = result.b64_json.startsWith('data:')
      ? result.b64_json
      : `data:image/png;base64,${result.b64_json}`
    return { id: `draw-${createdAt}-${index}`, src: dataUrl, kind: 'dataUrl', dataUrl, prompt, createdAt }
  }
  const url = result.url ?? ''
  return { id: `draw-${createdAt}-${index}`, src: url, kind: 'url', url, prompt, createdAt }
}
