<template>
  <Teleport to="body">
    <Transition name="muc-reset-fade">
      <div
        v-if="open"
        class="muc-scope muc-reset-overlay"
        role="status"
        :aria-label="t('pricing.reset.overlayLabel')"
      >
        <div class="muc-reset-overlay__glow" aria-hidden="true"></div>
        <div class="muc-reset-overlay__card">
          <p class="muc-reset-overlay__title">{{ t('pricing.reset.title') }}</p>

          <div class="muc-reset-overlay__ring-wrap">
            <svg class="muc-reset-overlay__ring" viewBox="0 0 200 200" aria-hidden="true">
              <circle cx="100" cy="100" r="84" class="muc-reset-overlay__ring-track" />
              <circle
                cx="100"
                cy="100"
                r="84"
                class="muc-reset-overlay__ring-fill"
                :style="{ strokeDashoffset: ringOffset }"
              />
            </svg>
            <div class="muc-reset-overlay__number">
              <span class="muc-reset-overlay__percent">{{ displayPercent }}</span>
              <span class="muc-reset-overlay__unit">%</span>
            </div>
          </div>

          <p class="muc-reset-overlay__available">{{ t('pricing.reset.available') }}</p>

          <div class="muc-reset-overlay__facts">
            <p>{{ t('pricing.reset.periodStarted', { date: nextEndDisplay }) }}</p>
            <p v-if="remainingCards !== null">
              {{ t('pricing.reset.remainingCards', { count: remainingCards }) }}
            </p>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/components/pricing/muc-tokens.css'
import { availableQuotaAfterReset, mucTween, MUC_MOTION } from '@/components/subscription/resetAnimation'

/**
 * Reset Card 使用成功的「额度已恢复」动画层。
 * 语义：后端保持 weekly_usage_percent=0 不动；本层只表达派生的
 * AVAILABLE QUOTA（= 100 − used）从当前值补满到 100% 的视觉效果。
 * 必须在 Reset API 成功之后才挂载展示；动画结束自动 emit('done')。
 * prefers-reduced-motion：不做数字滚动，直接淡入完成态。
 */
const props = defineProps<{
  open: boolean
  /** Reset 前的 weekly_usage_percent（后端权威值） */
  usedPercentBefore: number | null
  /** 新周期结束时间（Reset API 返回的 weekly_period_ends_at） */
  nextEndDate: string
  /** 剩余重置卡张数（Reset 成功后重新拉取的账户状态） */
  remainingCards: number | null
}>()

const emit = defineEmits<{ done: [] }>()

const { t } = useI18n()

const RING_LENGTH = 2 * Math.PI * 84
const displayPercent = ref(0)

const nextEndDisplay = computed(() => {
  if (!props.nextEndDate) return '—'
  try {
    return new Date(props.nextEndDate).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  } catch {
    return props.nextEndDate
  }
})
let cancelTween: (() => void) | null = null
let doneTimer = 0

const ringOffset = computed(() => {
  const p = displayPercent.value / 100
  return String(RING_LENGTH * (1 - p))
})

watch(
  () => props.open,
  (open) => {
    cancelTween?.()
    cancelTween = null
    if (doneTimer) {
      window.clearTimeout(doneTimer)
      doneTimer = 0
    }
    if (!open) return
    const { from, to } = availableQuotaAfterReset(props.usedPercentBefore)
    displayPercent.value = from
    cancelTween = mucTween({
      from,
      to,
      durationMs: MUC_MOTION.success,
      onUpdate: (v) => {
        displayPercent.value = Math.round(v)
      },
      onDone: () => {
        // 结束态短暂停留后回归正常 UI，不长时间遮挡页面
        doneTimer = window.setTimeout(() => emit('done'), 700)
      }
    })
  }
)

onBeforeUnmount(() => {
  cancelTween?.()
  if (doneTimer) window.clearTimeout(doneTimer)
})
</script>

<style scoped>
.muc-reset-overlay {
  position: fixed;
  inset: 0;
  z-index: 95;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(4, 4, 5, 0.78);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

.muc-reset-overlay__glow {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 46vmin;
  height: 46vmin;
  transform: translate(-50%, -50%);
  border-radius: 50%;
  background: radial-gradient(closest-side, var(--muc-red-glow), transparent 70%);
  animation: muc-reset-glow 1.6s cubic-bezier(0.22, 1, 0.36, 1) both;
}

@keyframes muc-reset-glow {
  from {
    opacity: 0.2;
    transform: translate(-50%, -50%) scale(0.7);
  }
  to {
    opacity: 0.85;
    transform: translate(-50%, -50%) scale(1.12);
  }
}

.muc-reset-overlay__card {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 34px 44px;
  border-radius: 26px;
  border: 1px solid var(--muc-red-border);
  background: var(--muc-glass-strong);
  box-shadow: 0 30px 90px rgba(0, 0, 0, 0.55);
  max-width: min(92vw, 380px);
}

.muc-reset-overlay__title {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.06em;
}

.muc-reset-overlay__ring-wrap {
  position: relative;
  width: 168px;
  height: 168px;
}

.muc-reset-overlay__ring {
  width: 100%;
  height: 100%;
  transform: rotate(-90deg);
}

.muc-reset-overlay__ring-track {
  fill: none;
  stroke: rgba(255, 255, 255, 0.1);
  stroke-width: 10;
}

.muc-reset-overlay__ring-fill {
  fill: none;
  stroke: var(--muc-red-bright);
  stroke-width: 10;
  stroke-linecap: round;
  stroke-dasharray: 527.79;
  filter: drop-shadow(0 0 10px var(--muc-red-glow));
}

.muc-reset-overlay__number {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
}

.muc-reset-overlay__percent {
  font-size: 44px;
  font-weight: 800;
  line-height: 1;
  font-variant-numeric: tabular-nums;
}

.muc-reset-overlay__unit {
  font-size: 18px;
  color: var(--muc-text-secondary);
  align-self: center;
  margin-top: 14px;
}

.muc-reset-overlay__available {
  margin: 0;
  font-size: 12px;
  letter-spacing: 0.28em;
  text-indent: 0.28em;
  color: var(--muc-text-muted);
}

.muc-reset-overlay__facts {
  display: flex;
  flex-direction: column;
  gap: 4px;
  text-align: center;
  font-size: 12.5px;
  color: var(--muc-text-secondary);
}

.muc-reset-overlay__facts p {
  margin: 0;
}

.muc-reset-fade-enter-active,
.muc-reset-fade-leave-active {
  transition: opacity 0.28s cubic-bezier(0.22, 1, 0.36, 1);
}

.muc-reset-fade-enter-from,
.muc-reset-fade-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .muc-reset-overlay__glow {
    animation: none;
    opacity: 0.5;
  }
}
</style>
