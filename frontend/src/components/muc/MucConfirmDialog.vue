<template>
  <MucModal
    :open="open"
    :title="title"
    max-width="440px"
    :closable="!loading"
    :close-on-overlay="!loading"
    @close="emit('cancel')"
  >
    <p class="muc-confirm__body">{{ body }}</p>
    <template #footer>
      <button type="button" class="muc-confirm__dismiss" :disabled="loading" @click="emit('cancel')">
        {{ cancelText }}
      </button>
      <MucButton :variant="danger ? 'danger' : 'primary'" :loading="loading" @click="emit('confirm')">
        {{ confirmText }}
      </MucButton>
    </template>
  </MucModal>
</template>

<script setup lang="ts">
import MucModal from '@/components/muc/MucModal.vue'
import MucButton from '@/components/muc/MucButton.vue'

withDefaults(
  defineProps<{
    open: boolean
    title: string
    body: string
    confirmText: string
    cancelText: string
    danger?: boolean
    loading?: boolean
  }>(),
  { danger: false, loading: false }
)

const emit = defineEmits<{ confirm: []; cancel: [] }>()
</script>

<style scoped>
.muc-confirm__body {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.65;
  color: var(--muc-text-secondary);
}

.muc-confirm__dismiss {
  border: none;
  background: none;
  padding: 10px 14px;
  border-radius: 12px;
  font-size: 14px;
  color: var(--muc-text-muted);
  cursor: pointer;
}

.muc-confirm__dismiss:hover:not(:disabled) {
  color: var(--muc-text-primary);
}

.muc-confirm__dismiss:disabled {
  opacity: 0.5;
  cursor: wait;
}
</style>
