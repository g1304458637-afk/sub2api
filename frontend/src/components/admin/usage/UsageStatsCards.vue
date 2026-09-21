<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div class="usage-stat">
      <div class="usage-stat__icon usage-stat__icon--red">
        <Icon name="document" size="md" />
      </div>
      <div class="min-w-0">
        <p class="usage-stat__label">{{ t('usage.totalRequests') }}</p>
        <p class="usage-stat__value">{{ stats?.total_requests?.toLocaleString() || '0' }}</p>
        <p class="usage-stat__hint">{{ t('usage.inSelectedRange') }}</p>
      </div>
    </div>
    <div class="usage-stat">
      <div class="usage-stat__icon usage-stat__icon--gold"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg></div>
      <div class="min-w-0">
        <p class="usage-stat__label">{{ t('usage.totalTokens') }}</p>
        <p class="usage-stat__value">{{ formatTokens(stats?.total_tokens || 0) }}</p>
        <p class="usage-stat__hint flex flex-wrap items-center gap-x-1">
          <span>{{ t('usage.in') }}: {{ formatTokens(stats?.total_input_tokens || 0) }}</span>
          <span>/</span>
          <span>{{ t('usage.out') }}: {{ formatTokens(stats?.total_output_tokens || 0) }}</span>
          <span>/</span>
          <span class="group relative inline-flex cursor-help items-center gap-0.5" tabindex="0">
            <span>{{ cacheLabel() }}: {{ formatTokens(stats?.total_cache_tokens || 0) }}</span>
            <svg
              class="h-3.5 w-3.5 opacity-60"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <span
              class="pointer-events-none absolute left-1/2 top-full z-30 mt-2 hidden w-56 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-3 text-left text-xs text-gray-700 shadow-lg group-hover:block group-focus:block dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200"
            >
              <span class="mb-2 block font-medium text-gray-900 dark:text-white">
                {{ cacheDetailLabel() }}
              </span>
              <span class="flex items-center justify-between gap-3">
                <span>{{ t('usage.cacheCreationTokensLabel') }}</span>
                <span class="tabular-nums">
                  {{ formatTokens(stats?.total_cache_creation_tokens || 0) }}
                </span>
              </span>
              <span class="mt-1 flex items-center justify-between gap-3">
                <span>{{ t('usage.cacheReadTokensLabel') }}</span>
                <span class="tabular-nums">
                  {{ formatTokens(stats?.total_cache_read_tokens || 0) }}
                </span>
              </span>
            </span>
          </span>
        </p>
      </div>
    </div>
    <div class="usage-stat">
      <div class="usage-stat__icon usage-stat__icon--green">
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="usage-stat__label">{{ t('usage.totalCost') }}</p>
        <p class="usage-stat__value">${{ (stats?.total_actual_cost || 0).toFixed(4) }}</p>
        <p class="usage-stat__hint">
          <template v-if="showAccountCost && totalAccountCost != null">
            <span class="text-orange-500">{{ t('usage.accountCost') }} ${{ totalAccountCost.toFixed(4) }}</span>
            <span> · </span>
          </template>
          <span>
            {{ t('usage.standardCost') }}
            <span :class="{ 'line-through opacity-60': strikeStandardCost }">${{ (stats?.total_cost || 0).toFixed(4) }}</span>
          </span>
        </p>
      </div>
    </div>
    <div class="usage-stat">
      <div class="usage-stat__icon usage-stat__icon--purple">
        <Icon name="clock" size="md" />
      </div>
      <div class="min-w-0">
        <p class="usage-stat__label">{{ t('usage.avgDuration') }}</p>
        <p class="usage-stat__value">{{ formatDuration(stats?.average_duration_ms || 0) }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
  showAccountCost?: boolean
  strikeStandardCost?: boolean
}>(), {
  showAccountCost: true,
  strikeStandardCost: false,
})

const { t } = useI18n()

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)

const formatDuration = (ms: number) =>
  ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`

const formatTokens = (value: number) => {
  if (value >= 1e9) return (value / 1e9).toFixed(2) + 'B'
  if (value >= 1e6) return (value / 1e6).toFixed(2) + 'M'
  if (value >= 1e3) return (value / 1e3).toFixed(2) + 'K'
  return value.toLocaleString()
}

const cacheLabel = () => t('usage.cacheTotal')
const cacheDetailLabel = () => t('usage.cacheBreakdown')
</script>

<style scoped>
/* MUC 统计卡：深色玻璃 + 24px 圆角（视觉规范 §5）；light 模式回退原 .card 材质 */
.usage-stat {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem;
  border-radius: 24px;
  border: 1px solid var(--muc-glass-border, rgba(255, 255, 255, 0.1));
  background: var(--muc-glass, rgba(15, 15, 17, 0.66));
  backdrop-filter: blur(14px);
  -webkit-backdrop-filter: blur(14px);
  color: var(--muc-text-primary, #fff);
  transition: border-color 0.2s var(--muc-ease, ease), transform 0.2s var(--muc-ease, ease);
}

:root:not(.dark) .usage-stat {
  border: 1px solid #f3f4f6;
  background: #fff;
  color: inherit;
  backdrop-filter: none;
  -webkit-backdrop-filter: none;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04), 0 1px 2px rgba(0, 0, 0, 0.06);
}

.usage-stat:hover {
  border-color: var(--muc-glass-border-strong, rgba(255, 255, 255, 0.16));
}

.usage-stat__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.75rem;
  height: 2.75rem;
  flex-shrink: 0;
  border-radius: 14px;
}

/* 低饱和数据区分色：红 / 金 / 绿 / 紫（不使用强品牌红铺满） */
.usage-stat__icon--red {
  color: #e86a74;
  background: rgba(200, 36, 51, 0.16);
}

.usage-stat__icon--gold {
  color: var(--muc-gold, #d6b46a);
  background: rgba(214, 180, 106, 0.14);
}

.usage-stat__icon--green {
  color: #5cc8a8;
  background: rgba(92, 200, 168, 0.13);
}

.usage-stat__icon--purple {
  color: #a78bdb;
  background: rgba(167, 139, 219, 0.14);
}

:root:not(.dark) .usage-stat__icon--red {
  color: #c82433;
  background: rgba(200, 36, 51, 0.1);
}

:root:not(.dark) .usage-stat__icon--gold {
  color: #a3801f;
  background: rgba(214, 180, 106, 0.18);
}

:root:not(.dark) .usage-stat__icon--green {
  color: #0f9d76;
  background: rgba(16, 185, 129, 0.12);
}

:root:not(.dark) .usage-stat__icon--purple {
  color: #7c5cd6;
  background: rgba(139, 92, 246, 0.12);
}

.usage-stat__label {
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--muc-text-secondary, rgba(255, 255, 255, 0.68));
}

:root:not(.dark) .usage-stat__label {
  color: rgb(107 114 128);
}

.usage-stat__value {
  margin-top: 1px;
  font-size: 1.25rem;
  line-height: 1.35;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--muc-text-primary, #fff);
}

:root:not(.dark) .usage-stat__value {
  color: rgb(17 24 39);
}

.usage-stat__hint {
  margin-top: 1px;
  font-size: 0.75rem;
  color: var(--muc-text-muted, rgba(255, 255, 255, 0.45));
}

:root:not(.dark) .usage-stat__hint {
  color: rgb(156 163 175);
}
</style>
