<template>
  <div class="muc-stat" :class="{ 'muc-stat--action': !!$slots.default }" :tabindex="$slots.default ? 0 : undefined" :role="$slots.default ? 'button' : undefined">
    <div class="muc-stat__head">
      <span class="muc-stat__label">{{ label }}</span>
      <MucBadge v-if="badge" :tone="badgeTone">{{ badge }}</MucBadge>
    </div>
    <div class="muc-stat__value" :class="{ 'muc-stat__value--gold': gold }">{{ value }}</div>
    <div v-if="hint" class="muc-stat__hint">{{ hint }}</div>
    <slot></slot>
  </div>
</template>

<script setup lang="ts">
import MucBadge from '@/components/muc/MucBadge.vue'
import type { MucBadgeTone } from '@/components/muc/MucBadge.vue'

withDefaults(
  defineProps<{
    label: string
    value: string | number
    badge?: string
    badgeTone?: MucBadgeTone
    hint?: string
    /** 暖金数值（Reward/金额类） */
    gold?: boolean
  }>(),
  { badge: '', badgeTone: 'muted', hint: '', gold: false }
)
</script>

<style scoped>
.muc-stat {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 16px 18px;
  border-radius: 16px;
  border: 1px solid var(--muc-glass-border);
  background: var(--muc-glass);
  color: var(--muc-text-primary);
}

.muc-stat--action {
  cursor: pointer;
  transition: border-color 0.2s ease, transform 0.15s ease;
}

.muc-stat--action:hover,
.muc-stat--action:focus-visible {
  border-color: var(--muc-red-border-hover);
  transform: translateY(-1px);
  outline: none;
}

.muc-stat__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.muc-stat__label {
  font-size: 12px;
  letter-spacing: 0.06em;
  color: var(--muc-text-muted);
}

.muc-stat__value {
  font-size: 26px;
  font-weight: 700;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.muc-stat__value--gold {
  color: var(--muc-gold);
}

.muc-stat__hint {
  font-size: 12px;
  color: var(--muc-text-secondary);
}
</style>
