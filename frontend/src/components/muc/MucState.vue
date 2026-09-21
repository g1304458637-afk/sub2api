<template>
  <div class="muc-state" :class="{ 'muc-state--error': error }">
    <Icon :name="error ? 'exclamationCircle' : icon" size="xl" class="muc-state__icon" />
    <p class="muc-state__text">{{ message }}</p>
    <div v-if="$slots.actions" class="muc-state__actions">
      <slot name="actions"></slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

withDefaults(
  defineProps<{
    message: string
    error?: boolean
    icon?: 'inbox' | 'exclamationCircle' | 'infoCircle'
  }>(),
  { error: false, icon: 'inbox' }
)
</script>

<style scoped>
.muc-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 44px 20px;
  text-align: center;
  color: var(--muc-text-muted);
}

.muc-state--error .muc-state__icon {
  color: var(--muc-danger);
}

.muc-state__icon {
  opacity: 0.7;
}

.muc-state__text {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.6;
  color: var(--muc-text-secondary);
}

.muc-state__actions {
  display: flex;
  gap: 10px;
}
</style>
