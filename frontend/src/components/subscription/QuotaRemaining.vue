<script setup lang="ts">
import { computed } from 'vue'
import { formatRemainingPercent as percent } from '@/utils/quotaDisplay'
import type { AccountSubscriptionStatus } from '@/api/subscriptions'
const props = defineProps<{ subscription: AccountSubscriptionStatus }>()
const windows = computed(() => props.subscription.quota_policy === 'dual_window_v1'
  ? [{ label: '5 小时剩余', value: props.subscription.short_window }, { label: '本周剩余', value: props.subscription.weekly_window }]
  : [{ label: '本周剩余', value: props.subscription.weekly_usage_percent === null ? undefined : {
      remaining_percent: 100 - props.subscription.weekly_usage_percent,
      starts_at: props.subscription.weekly_period_started_at,
      resets_at: props.subscription.weekly_period_ends_at,
      exhausted: props.subscription.weekly_usage_percent >= 100
    } }])
</script>
<template>
  <div class="space-y-3" data-testid="quota-remaining">
    <div v-for="window in windows" :key="window.label">
      <div class="flex justify-between gap-3 text-sm"><span>{{ window.label }}</span><strong>{{ percent(window.value?.remaining_percent) }}</strong></div>
      <div class="my-1 h-1.5 overflow-hidden rounded-full bg-white/10" role="progressbar" :aria-label="window.label" :aria-valuenow="window.value?.remaining_percent" :aria-valuemin="0" :aria-valuemax="100">
        <div class="h-full rounded-full" :class="window.value?.exhausted ? 'bg-red-400' : 'bg-emerald-400'" :style="{ width: `${window.value?.remaining_percent ?? 0}%` }" />
      </div>
      <p class="text-xs opacity-60">{{ window.value?.resets_at ? `${new Date(window.value.resets_at).toLocaleString()} 恢复` : window.value?.starts_at ? '到期前不再恢复' : window.value ? '首次使用后开始计时' : subscription.quota_policy === 'dual_window_v1' ? '状态暂不可用' : '周不限额' }}</p>
    </div>
  </div>
</template>
