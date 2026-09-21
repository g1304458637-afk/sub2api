<template>
  <button
    v-if="!href"
    type="button"
    class="muc-btn"
    :class="[`muc-btn--${variant}`, { 'muc-btn--pill': pill }]"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
  >
    <span v-if="loading" class="muc-btn__spinner" aria-hidden="true"></span>
    <slot></slot>
  </button>
  <a v-else class="muc-btn" :class="[`muc-btn--${variant}`, { 'muc-btn--pill': pill }]" :href="href">
    <slot></slot>
  </a>
</template>

<script setup lang="ts">
/**
 * MUC 统一按钮：primary=白底黑字（可读性/高级感来源），secondary=透明描边，
 * danger=警示红描边（仅危险操作），gold=暖金描边（Reward 场景）。
 * 品牌红不用于按钮底色，只出现在 hover 细节与 selected 态。
 */
withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'gold'
    pill?: boolean
    disabled?: boolean
    loading?: boolean
    href?: string
  }>(),
  { variant: 'primary', pill: false, disabled: false, loading: false, href: '' }
)
</script>

<style scoped>
.muc-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 10px 18px;
  border-radius: 12px;
  border: 1px solid transparent;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.03em;
  line-height: 1.2;
  cursor: pointer;
  text-decoration: none;
  transition: box-shadow 0.2s ease, transform 0.15s ease, border-color 0.2s ease,
    background 0.2s ease, color 0.2s ease;
}

.muc-btn--pill {
  border-radius: 999px;
}

.muc-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.muc-btn--primary {
  background: #fff;
  color: #0a0a0b;
}

.muc-btn--primary:hover:not(:disabled) {
  box-shadow: 0 6px 26px rgba(238, 56, 72, 0.3);
  transform: translateY(-1px);
}

.muc-btn--secondary {
  background: transparent;
  color: var(--muc-text-secondary);
  border-color: var(--muc-glass-border-strong);
}

.muc-btn--secondary:hover:not(:disabled) {
  border-color: var(--muc-red-border-hover);
  color: var(--muc-text-primary);
}

.muc-btn--danger {
  background: transparent;
  color: var(--muc-danger);
  border-color: rgba(255, 69, 80, 0.45);
}

.muc-btn--danger:hover:not(:disabled) {
  border-color: var(--muc-danger);
  background: rgba(255, 69, 80, 0.08);
}

.muc-btn--gold {
  background: transparent;
  color: var(--muc-gold);
  border-color: rgba(214, 180, 106, 0.45);
}

.muc-btn--gold:hover:not(:disabled) {
  border-color: var(--muc-gold);
  background: rgba(214, 180, 106, 0.08);
}

.muc-btn__spinner {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  border-radius: 50%;
  border: 2px solid currentColor;
  border-top-color: transparent;
  animation: muc-btn-spin 0.7s linear infinite;
}

@keyframes muc-btn-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
