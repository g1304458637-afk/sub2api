<template>
  <span :class="['badge', badgeClass]">
    {{ t(`research.status.${status}`) }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ResearchApplicationStatus } from '@/api/research'

const props = defineProps<{
  status: ResearchApplicationStatus
}>()

const { t } = useI18n()

const badgeClass = computed(() => {
  switch (props.status) {
    case 'approved':
      return 'badge-success'
    case 'rejected':
      return 'badge-danger'
    default:
      return 'badge-warning'
  }
})
</script>
