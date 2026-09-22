<template>
  <component :is="tag" class="muc-glass-card" :class="[`muc-glass-card--${variant}`]">
    <slot></slot>
  </component>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    /** strong=更深更实的玻璃（弹窗/主卡），default=普通玻璃 */
    variant?: 'default' | 'strong'
    tag?: string
  }>(),
  { variant: 'default', tag: 'div' }
)
</script>

<style scoped>
.muc-glass-card {
  border-radius: 24px;
  border: 1px solid var(--muc-glass-border);
  background: var(--muc-glass);
  backdrop-filter: blur(20px) saturate(1.1);
  -webkit-backdrop-filter: blur(20px) saturate(1.1);
  color: var(--muc-text-primary);
}

.muc-glass-card--strong {
  background: var(--muc-glass-strong);
  border-color: var(--muc-glass-border-strong);
}
</style>
