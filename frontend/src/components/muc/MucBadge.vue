<template>
  <span class="muc-badge" :class="[`muc-badge--${tone}`]">
    <slot></slot>
  </span>
</template>

<script lang="ts">
// 独立 script 块导出类型（script setup 内不允许 export）
export type MucBadgeTone =
  | 'brand'
  | 'normal'
  | 'high'
  | 'near_limit'
  | 'exhausted'
  | 'unmetered'
  | 'reward'
  | 'success'
  | 'muted'
</script>

<script setup lang="ts">
/**
 * MUC 统一徽标/状态 chip。tone 与业务状态色板严格对应：
 * normal=柔白、high=暖黄、near_limit=橙、exhausted=警示红（≠品牌红）、
 * unmetered/reward=暖金、brand=民大红（身份/badge）、muted=弱化。
 */
withDefaults(defineProps<{ tone?: MucBadgeTone }>(), { tone: 'muted' })
</script>

<style scoped>
.muc-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  border: 1px solid transparent;
  white-space: nowrap;
}

.muc-badge--brand {
  color: #fff;
  background: linear-gradient(120deg, var(--muc-red), var(--muc-red-bright));
  box-shadow: 0 2px 14px var(--muc-red-glow);
}

.muc-badge--normal {
  color: rgba(255, 255, 255, 0.85);
  border-color: rgba(255, 255, 255, 0.25);
  background: rgba(255, 255, 255, 0.06);
}

.muc-badge--high {
  color: #ecc94b;
  border-color: rgba(236, 201, 75, 0.4);
  background: rgba(236, 201, 75, 0.1);
}

.muc-badge--near_limit {
  color: #f08c3a;
  border-color: rgba(240, 140, 58, 0.45);
  background: rgba(240, 140, 58, 0.1);
}

.muc-badge--exhausted {
  color: var(--muc-danger);
  border-color: rgba(255, 69, 80, 0.55);
  background: rgba(255, 69, 80, 0.12);
}

.muc-badge--unmetered,
.muc-badge--reward {
  color: var(--muc-gold);
  border-color: rgba(214, 180, 106, 0.4);
  background: rgba(214, 180, 106, 0.08);
}

.muc-badge--success {
  color: #6fd598;
  border-color: rgba(111, 213, 152, 0.4);
  background: rgba(111, 213, 152, 0.08);
}

.muc-badge--muted {
  color: var(--muc-text-muted);
  border-color: var(--muc-glass-border-strong);
  background: rgba(255, 255, 255, 0.03);
}
</style>
