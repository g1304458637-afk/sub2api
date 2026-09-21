/**
 * MUC 统一动效 helper（Website 与 MUCODE 客户端共享同一套语义参数）。
 * 这里只放纯函数，不依赖 DOM —— MUCODE（SolidJS）按相同语义自行实现渲染层。
 */

/** 主 easing：cubic-bezier(0.22, 1, 0.36, 1)，与全站 CSS transition 保持一致。 */
export function mucEase(p1x = 0.22, p1y = 1, p2x = 0.36, p2y = 1): (t: number) => number {
  // Newton-Raphson 求 bezier x(t)=t 处的参数，再取 y(t)。系数为常数，直接实现。
  const cx = 3 * p1x
  const bx = 3 * (p2x - p1x) - cx
  const ax = 1 - cx - bx
  const cy = 3 * p1y
  const by = 3 * (p2y - p1y) - cy
  const ay = 1 - cy - by

  const sampleX = (t: number) => ((ax * t + bx) * t + cx) * t
  const sampleY = (t: number) => ((ay * t + by) * t + cy) * t
  const sampleDX = (t: number) => (3 * ax * t + 2 * bx) * t + cx

  return (x: number): number => {
    if (x <= 0) return 0
    if (x >= 1) return 1
    let t = x
    for (let i = 0; i < 6; i++) {
      const dx = sampleX(t) - x
      if (Math.abs(dx) < 1e-5) break
      const d = sampleDX(t)
      if (Math.abs(d) < 1e-6) break
      t -= dx / d
    }
    return sampleY(t)
  }
}

export const MUC_MOTION_EASE = mucEase()

/** 统一 motion durations（ms）。 */
export const MUC_MOTION = {
  fast: 180,
  normal: 320,
  hero: 620,
  /** 特殊成功动画（Reset 恢复等） */
  success: 1400
} as const

export interface TweenOptions {
  from: number
  to: number
  durationMs: number
  ease?: (t: number) => number
  onUpdate: (value: number) => void
  onDone?: () => void
}

/**
 * RAF 数字/进度 tween。返回 cancel 函数（组件卸载必须调用，防泄漏）。
 * prefers-reduced-motion 时调用方应跳过动画直接呈现终态。
 */
export function mucTween(options: TweenOptions): () => void {
  const { from, to, durationMs, ease = MUC_MOTION_EASE, onUpdate, onDone } = options
  if (typeof window === 'undefined') return () => {}
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches || durationMs <= 0) {
    onUpdate(to)
    onDone?.()
    return () => {}
  }
  let rafId = 0
  let start = 0
  let cancelled = false
  const step = (ts: number) => {
    if (cancelled) return
    if (!start) start = ts
    const progress = Math.min((ts - start) / durationMs, 1)
    onUpdate(from + (to - from) * ease(progress))
    if (progress < 1) {
      rafId = requestAnimationFrame(step)
    } else {
      onDone?.()
    }
  }
  rafId = requestAnimationFrame(step)
  return () => {
    cancelled = true
    if (rafId) cancelAnimationFrame(rafId)
  }
}

/**
 * Reset 成功动画的共享语义：
 * 视觉指标 = AVAILABLE QUOTA = 100 − weekly_usage_percent（后端字段保持 used 语义不动）。
 * Reset 成功后：used → 0，因此 available 从 (100 − usedBefore) 补满到 100。
 */
export function availableQuotaAfterReset(usedPercentBefore: number | null): {
  from: number
  to: number
} {
  const used = Math.min(Math.max(usedPercentBefore ?? 0, 0), 100)
  return { from: Math.round(100 - used), to: 100 }
}
