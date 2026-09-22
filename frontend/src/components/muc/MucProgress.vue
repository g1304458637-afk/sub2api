<template>
  <div
    class="muc-progress"
    role="progressbar"
    :aria-valuenow="clamped"
    :aria-valuemin="0"
    :aria-valuemax="100"
    :aria-label="ariaLabel"
  >
    <div class="muc-progress__track">
      <div
        class="muc-progress__fill"
        :class="`muc-progress__fill--${tone}`"
        :style="{ width: `${clamped}%` }"
      ></div>
    </div>
    <span v-if="showLabel" class="muc-progress__label">{{ clamped }}%</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

/**
 * MUC 统一进度条。值来自后端（weekly_usage_percent 等），此处只 clamp 不重算。
 * tone=color tone：进度条颜色与状态色分层一致；normal 用柔白而不是品牌红。
 */
const props = withDefaults(
  defineProps<{
    /** 0-100 整数（后端已 clamp，这里兜底） */
    value: number | null
    tone?: 'normal' | 'high' | 'near_limit' | 'exhausted' | 'unmetered' | 'brand'
    showLabel?: boolean
    ariaLabel?: string
  }>(),
  { tone: 'normal', showLabel: false, ariaLabel: '' }
)

const clamped = computed(() => Math.min(Math.max(props.value ?? 0, 0), 100))
</script>

<style scoped>
.muc-progress {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.muc-progress__track {
  flex: 1;
  min-width: 0;
  height: 6px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.1);
  overflow: hidden;
}

.muc-progress__fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.5s cubic-bezier(0.22, 1, 0.36, 1);
}

.muc-progress__fill--normal {
  background: rgba(255, 255, 255, 0.72);
}

.muc-progress__fill--high {
  background: #ecc94b;
}

.muc-progress__fill--near_limit {
  background: #f08c3a;
}

.muc-progress__fill--exhausted {
  background: var(--muc-danger);
}

.muc-progress__fill--unmetered {
  background: linear-gradient(to right, var(--muc-gold), rgba(214, 180, 106, 0.35));
}

.muc-progress__fill--brand {
  background: linear-gradient(to right, var(--muc-red), var(--muc-red-bright));
}

.muc-progress__label {
  flex-shrink: 0;
  min-width: 38px;
  text-align: right;
  font-size: 12px;
  color: var(--muc-text-secondary);
  font-variant-numeric: tabular-nums;
}
</style>
