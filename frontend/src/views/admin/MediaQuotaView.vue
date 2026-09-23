<template>
  <AppLayout>
    <div class="space-y-6" data-test="media-quota-view">
      <!-- Header -->
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ t('admin.mediaQuota.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.mediaQuota.description') }}</p>
        </div>
        <button type="button" class="btn btn-secondary" data-test="refresh" :disabled="loading" @click="refreshAll">
          <Icon name="refresh" size="sm" class="mr-1.5" />
          {{ t('admin.mediaQuota.refresh') }}
        </button>
      </div>

      <!-- Global load error -->
      <div
        v-if="loadError"
        data-test="load-error"
        class="card border border-red-200 bg-red-50 p-4 text-sm text-red-600 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-400"
      >
        {{ t('admin.mediaQuota.loadFailed') }}
      </div>

      <!-- Initial loading skeleton -->
      <div v-if="initialLoading" data-test="initial-loading" class="card p-8">
        <div class="flex flex-col items-center justify-center gap-3">
          <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.mediaQuota.loading') }}</p>
        </div>
      </div>

      <template v-else>
        <!-- Top summary: three media today total cost -->
        <div class="card p-4 sm:p-6" data-test="today-summary">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <div>
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.mediaQuota.summary.todayTotal') }}</p>
              <p class="mt-1 text-2xl font-bold text-gray-900 dark:text-white" data-test="today-total">
                {{ currencyStore.formatUSD(totalTodayCost) }}
              </p>
            </div>
            <div class="flex flex-wrap gap-6">
              <div>
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.summary.image') }}</p>
                <p class="font-mono font-medium text-gray-800 dark:text-gray-200" data-test="image-today-cost">
                  {{ currencyStore.formatUSD(mediaStats.image.cost) }}
                </p>
              </div>
              <div>
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.summary.music') }}</p>
                <p class="font-mono font-medium text-gray-800 dark:text-gray-200" data-test="music-today-cost">
                  {{ currencyStore.formatUSD(mediaStats.music.cost) }}
                </p>
              </div>
              <div>
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.summary.audio') }}</p>
                <p class="font-mono font-medium text-gray-800 dark:text-gray-200" data-test="audio-today-cost">
                  {{ currencyStore.formatUSD(mediaStats.audio.cost) }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <!-- Image section -->
        <section class="card p-4 sm:p-6" data-test="image-section">
          <div class="mb-4 flex flex-wrap items-center gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.mediaQuota.image.title') }}
            </h2>
            <span class="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-semibold text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
              {{ t('admin.mediaQuota.billing.quota') }}
            </span>
          </div>

          <!-- Quota card: only when enabled and reachable -->
          <div
            v-if="quota && quota.enabled && quota.reachable"
            data-test="image-quota-card"
            class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
          >
            <div class="flex flex-wrap items-center gap-3">
              <span
                data-test="auth-dot"
                class="inline-block h-2.5 w-2.5 rounded-full"
                :class="{
                  'bg-emerald-500': authState === 'ok',
                  'bg-amber-500': authState === 'bad',
                  'bg-gray-300 dark:bg-dark-600': authState === 'unknown'
                }"
                :title="t(`admin.mediaQuota.quota.auth_${authState}`)"
              ></span>
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t(`admin.mediaQuota.quota.auth_${authState}`) }}
              </span>
              <span
                v-if="quota.busy"
                data-test="busy-badge"
                class="inline-flex items-center rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
              >
                {{ t('admin.mediaQuota.quota.busy') }}
              </span>
              <span v-if="quota.version" class="text-xs text-gray-400 dark:text-dark-500">
                {{ t('admin.mediaQuota.quota.version') }}: {{ quota.version }}
              </span>
              <span v-if="quota.latency" class="text-xs text-gray-400 dark:text-dark-500">
                {{ t('admin.mediaQuota.quota.latency') }}: p50 {{ quota.latency.p50_ms }}ms / p95 {{ quota.latency.p95_ms }}ms
              </span>
            </div>

            <div v-if="quota.budget" class="mt-4" data-test="quota-progress">
              <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
                <span class="text-gray-500 dark:text-dark-400">{{ t('admin.mediaQuota.quota.used') }}</span>
                <span class="font-mono text-gray-800 dark:text-gray-200">
                  {{ formatNumber(quota.budget.used) }} / {{ formatNumber(quota.budget.limit) }}
                </span>
              </div>
              <div class="mt-2 h-2 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                <div
                  class="h-full rounded-full bg-primary-500"
                  :class="{ 'bg-red-500': usagePercent >= 90 }"
                  :style="{ width: `${usagePercent}%` }"
                ></div>
              </div>
              <div class="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-gray-400 dark:text-dark-500">
                <span>{{ t('admin.mediaQuota.quota.remaining') }}: {{ formatNumber(quota.budget.remaining) }}</span>
                <span data-test="reset-countdown">
                  {{ t('admin.mediaQuota.quota.resetIn') }}: {{ resetCountdown ?? '—' }}
                </span>
                <span>{{ windowNote }}</span>
              </div>
            </div>
          </div>

          <!-- Unreachable: error banner -->
          <div
            v-else-if="quota && quota.enabled && !quota.reachable"
            data-test="image-quota-unreachable"
            class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-600 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-400"
          >
            <p class="font-medium">{{ t('admin.mediaQuota.quota.unreachable') }}</p>
            <p v-if="quota.error" class="mt-1 break-all font-mono text-xs">{{ quota.error }}</p>
          </div>

          <!-- Not configured: guide -->
          <div
            v-else
            data-test="image-quota-unconfigured"
            class="rounded-lg border border-dashed border-gray-300 p-4 dark:border-dark-600"
          >
            <p class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.mediaQuota.quota.unconfigured') }}
            </p>
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.mediaQuota.quota.unconfiguredHint') }}
            </p>
          </div>

          <!-- Image pricing -->
          <div class="mt-6">
            <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
              {{ t('admin.mediaQuota.pricing.title') }}
            </h3>
            <div v-if="pricing" data-test="image-pricing" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div
                v-for="tier in imagePricingTiers"
                :key="tier.key"
                class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"
              >
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ tier.label }}</p>
                <p class="mt-1 font-mono text-sm font-medium text-gray-800 dark:text-gray-200">
                  <template v-if="tier.value !== null">{{ currencyStore.formatUSD(tier.value) }}</template>
                  <template v-else>{{ t('admin.mediaQuota.pricing.defaultPrice') }}</template>
                </p>
              </div>
            </div>
            <p v-else class="text-sm text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.pricing.unavailable') }}</p>
          </div>

          <!-- Image recent generations -->
          <div class="mt-6">
            <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
              {{ t('admin.mediaQuota.recent.title') }}
            </h3>
            <div v-if="imageRecent.length > 0" class="overflow-x-auto" data-test="image-recent">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-dark-400">
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.time') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.duration') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.count') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.size') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.cost') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                  <tr v-for="row in imageRecent" :key="row.id" class="text-gray-700 dark:text-gray-300">
                    <td class="whitespace-nowrap px-3 py-2">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ formatDuration(row.duration_ms) }}</td>
                    <td class="whitespace-nowrap px-3 py-2">{{ row.image_count }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ row.image_size || '—' }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ currencyStore.formatUSD(row.total_cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else data-test="image-recent-empty" class="text-sm text-gray-400 dark:text-dark-500">
              {{ t('admin.mediaQuota.recent.empty') }}
            </p>
          </div>

          <!-- Bridge recent requests (proxy-side log, when available) -->
          <div v-if="bridgeRecent.length > 0" class="mt-6">
            <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
              {{ t('admin.mediaQuota.quota.bridgeRecent') }}
            </h3>
            <div class="overflow-x-auto" data-test="bridge-recent">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-dark-400">
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.time') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.status') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.duration') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.count') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.size') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                  <tr v-for="item in bridgeRecent" :key="item.request_id" class="text-gray-700 dark:text-gray-300">
                    <td class="whitespace-nowrap px-3 py-2">{{ formatDateTime(normalizeTimestampIso(item.ts)) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ item.status }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ formatDuration(item.duration_ms) }}</td>
                    <td class="whitespace-nowrap px-3 py-2">{{ item.n }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ item.size || '—' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- Music section -->
        <section class="card p-4 sm:p-6" data-test="music-section">
          <div class="mb-4 flex flex-wrap items-center gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.mediaQuota.music.title') }}
            </h2>
            <span class="inline-flex items-center rounded-full bg-purple-100 px-2.5 py-0.5 text-xs font-semibold text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">
              {{ t('admin.mediaQuota.billing.payAsYouGo') }}
            </span>
            <span class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.mediaQuota.billing.noQuota') }}
            </span>
          </div>

          <div class="flex flex-wrap gap-8">
            <div>
              <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.music.todayTracks') }}</p>
              <p class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="music-today-requests">
                {{ formatNumber(mediaStats.music.requests) }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.music.todayCost') }}</p>
              <p class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="music-today-cost-summary">
                {{ currencyStore.formatUSD(mediaStats.music.cost) }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.pricing.musicPerTrack') }}</p>
              <p class="mt-1 font-mono text-sm font-medium text-gray-800 dark:text-gray-200" data-test="music-pricing">
                <template v-if="pricing && pricing.musicPricePerTrack !== null">
                  {{ currencyStore.formatUSD(pricing.musicPricePerTrack) }}
                </template>
                <template v-else>{{ t('admin.mediaQuota.pricing.defaultMusicPrice', { price: currencyStore.formatUSD(0.5) }) }}</template>
              </p>
            </div>
          </div>

          <div class="mt-6">
            <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
              {{ t('admin.mediaQuota.recent.title') }}
            </h3>
            <div v-if="musicRecent.length > 0" class="overflow-x-auto" data-test="music-recent">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-dark-400">
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.time') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.model') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.user') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.duration') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.cost') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                  <tr v-for="row in musicRecent" :key="row.id" class="text-gray-700 dark:text-gray-300">
                    <td class="whitespace-nowrap px-3 py-2">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ row.model }}</td>
                    <td class="max-w-[200px] truncate px-3 py-2">{{ row.user?.email || `#${row.user_id}` }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ formatDuration(row.duration_ms) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ currencyStore.formatUSD(row.total_cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else data-test="music-recent-empty" class="text-sm text-gray-400 dark:text-dark-500">
              {{ t('admin.mediaQuota.recent.empty') }}
            </p>
          </div>
        </section>

        <!-- Audio section -->
        <section class="card p-4 sm:p-6" data-test="audio-section">
          <div class="mb-4 flex flex-wrap items-center gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.mediaQuota.audio.title') }}
            </h2>
            <span class="inline-flex items-center rounded-full bg-purple-100 px-2.5 py-0.5 text-xs font-semibold text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">
              {{ t('admin.mediaQuota.billing.payAsYouGo') }}
            </span>
            <span class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.mediaQuota.billing.noQuota') }}
            </span>
          </div>

          <div class="flex flex-wrap gap-8">
            <div>
              <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.audio.todayRequests') }}</p>
              <p class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="audio-today-requests">
                {{ formatNumber(mediaStats.audio.requests) }}
              </p>
            </div>
            <div>
              <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.audio.todayCost') }}</p>
              <p class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="audio-today-cost-summary">
                {{ currencyStore.formatUSD(mediaStats.audio.cost) }}
              </p>
            </div>
          </div>

          <div class="mt-6">
            <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
              {{ t('admin.mediaQuota.pricing.title') }}
            </h3>
            <div v-if="pricing" data-test="audio-pricing" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div
                v-for="tier in audioPricingTiers"
                :key="tier.key"
                class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800"
              >
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ tier.label }}</p>
                <p class="mt-1 font-mono text-sm font-medium text-gray-800 dark:text-gray-200">
                  <template v-if="tier.value !== null">{{ currencyStore.formatUSD(tier.value) }}</template>
                  <template v-else>{{ t('admin.mediaQuota.pricing.defaultPrice') }}</template>
                </p>
              </div>
            </div>
            <p v-else class="text-sm text-gray-400 dark:text-dark-500">{{ t('admin.mediaQuota.pricing.unavailable') }}</p>
          </div>

          <div class="mt-6">
            <h3 class="mb-2 text-sm font-semibold text-gray-800 dark:text-gray-200">
              {{ t('admin.mediaQuota.recent.title') }}
            </h3>
            <div v-if="audioRecent.length > 0" class="overflow-x-auto" data-test="audio-recent">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs uppercase tracking-wide text-gray-500 dark:text-dark-400">
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.time') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.model') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.user') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.duration') }}</th>
                    <th class="px-3 py-2">{{ t('admin.mediaQuota.recent.cost') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700/60">
                  <tr v-for="row in audioRecent" :key="row.id" class="text-gray-700 dark:text-gray-300">
                    <td class="whitespace-nowrap px-3 py-2">{{ formatDateTime(row.created_at) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ row.model }}</td>
                    <td class="max-w-[200px] truncate px-3 py-2">{{ row.user?.email || `#${row.user_id}` }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ formatDuration(row.duration_ms) }}</td>
                    <td class="whitespace-nowrap px-3 py-2 font-mono">{{ currencyStore.formatUSD(row.total_cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p v-else data-test="audio-recent-empty" class="text-sm text-gray-400 dark:text-dark-500">
              {{ t('admin.mediaQuota.recent.empty') }}
            </p>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import {
  MEDIA_MODELS,
  aggregateMediaStats
} from '@/api/admin/mediaQuota'
import type {
  MediaImageQuotaResponse,
  MediaImageQuotaRecentItem,
  MediaModelStat,
  MediaPricing
} from '@/api/admin/mediaQuota'
import type { AdminUsageLog } from '@/types'
import { formatNumber } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'

const { t } = useI18n()
const appStore = useAppStore()
const currencyStore = useCurrencyDisplayStore()

// ==================== State ====================

const loading = ref(false)
const initialLoading = ref(true)
const loadError = ref(false)

const quota = ref<MediaImageQuotaResponse | null>(null)
const statsModels = ref<MediaModelStat[]>([])
const pricing = ref<MediaPricing | null>(null)
const imageRecent = ref<AdminUsageLog[]>([])
const musicRecent = ref<AdminUsageLog[]>([])
const audioRecent = ref<AdminUsageLog[]>([])

/** 最近记录表行数 */
const RECENT_LIMIT = 10
/** 音乐/语音前端前缀过滤的拉取页大小 */
const RECENT_PAGE_SIZE = 100

// ==================== Derived ====================

const mediaStats = computed(() => aggregateMediaStats(statsModels.value))
const totalTodayCost = computed(
  () => mediaStats.value.image.cost + mediaStats.value.music.cost + mediaStats.value.audio.cost
)

const bridgeRecent = computed<MediaImageQuotaRecentItem[]>(() => quota.value?.recent ?? [])

const authState = computed<'ok' | 'bad' | 'unknown'>(() => {
  if (quota.value?.auth_ok === true) return 'ok'
  if (quota.value?.auth_ok === false) return 'bad'
  return 'unknown'
})

const usagePercent = computed(() => {
  const budget = quota.value?.budget
  if (!budget || !budget.limit || budget.limit <= 0) return 0
  return Math.min(100, Math.max(0, (budget.used / budget.limit) * 100))
})

const windowNote = computed(() => {
  const hours = quota.value?.budget?.window_ms
  if (hours && hours > 0) {
    return t('admin.mediaQuota.quota.windowNote', { hours: Math.round((hours / 3600000) * 10) / 10 })
  }
  return t('admin.mediaQuota.quota.windowDefault')
})

const imagePricingTiers = computed(() => [
  { key: '1k', label: t('admin.mediaQuota.pricing.image1k'), value: pricing.value?.imagePrice1k ?? null },
  { key: '2k', label: t('admin.mediaQuota.pricing.image2k'), value: pricing.value?.imagePrice2k ?? null },
  { key: '4k', label: t('admin.mediaQuota.pricing.image4k'), value: pricing.value?.imagePrice4k ?? null }
])

const audioPricingTiers = computed(() => [
  { key: 'tts', label: t('admin.mediaQuota.pricing.audioTts'), value: pricing.value?.audioTtsPricePerMillionChars ?? null },
  { key: 'realtime', label: t('admin.mediaQuota.pricing.audioRealtime'), value: pricing.value?.audioRealtimePricePerMin ?? null },
  { key: 'stt', label: t('admin.mediaQuota.pricing.audioStt'), value: pricing.value?.audioSttPricePerHour ?? null }
])

// ==================== Countdown ====================

const nowTick = ref(Date.now())
let tickTimer: ReturnType<typeof setInterval> | null = null

/** reset_at 兼容 ISO 字符串与 unix 秒/毫秒，归一化为毫秒时间戳 */
function normalizeTimestamp(value: string | number | null | undefined): number | null {
  if (value === null || value === undefined) return null
  if (typeof value === 'number') {
    return value > 1e12 ? value : value * 1000
  }
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? null : parsed
}

function normalizeTimestampIso(value: string | number | null | undefined): string {
  const ms = normalizeTimestamp(value)
  return ms === null ? '' : new Date(ms).toISOString()
}

const resetCountdown = computed(() => {
  const resetMs = normalizeTimestamp(quota.value?.budget?.reset_at)
  if (resetMs === null) return null
  const remainMs = resetMs - nowTick.value
  if (remainMs <= 0) return t('admin.mediaQuota.quota.resetExpired')
  const totalSeconds = Math.floor(remainMs / 1000)
  const h = String(Math.floor(totalSeconds / 3600)).padStart(2, '0')
  const m = String(Math.floor((totalSeconds % 3600) / 60)).padStart(2, '0')
  const s = String(totalSeconds % 60).padStart(2, '0')
  return `${h}:${m}:${s}`
})

// ==================== Data loading ====================

async function loadQuota() {
  try {
    quota.value = await adminAPI.mediaQuota.getImageQuota()
  } catch {
    loadError.value = true
    appStore.showError(t('admin.mediaQuota.loadFailed'))
  }
}

async function loadStats() {
  try {
    statsModels.value = await adminAPI.mediaQuota.getMediaModelStats()
  } catch {
    appStore.showError(t('admin.mediaQuota.loadFailed'))
  }
}

async function loadPricing() {
  try {
    const result = await adminAPI.mediaQuota.getMediaPricing()
    pricing.value = result.pricing
  } catch {
    appStore.showError(t('admin.mediaQuota.loadFailed'))
  }
}

async function loadRecent() {
  const [imageResult, allResult] = await Promise.allSettled([
    // 图片：按模型精确匹配走服务端过滤
    adminAPI.usage.list({ page: 1, page_size: RECENT_LIMIT, model: MEDIA_MODELS.imageExact[0] }),
    // 音乐/语音：拉回后前端按前缀过滤
    adminAPI.usage.list({ page: 1, page_size: RECENT_PAGE_SIZE })
  ])

  if (imageResult.status === 'fulfilled') {
    imageRecent.value = imageResult.value.items ?? []
  } else {
    appStore.showError(t('admin.mediaQuota.loadFailed'))
  }

  if (allResult.status === 'fulfilled') {
    const items = allResult.value.items ?? []
    musicRecent.value = items.filter((row) => row.model.startsWith(MEDIA_MODELS.musicPrefix)).slice(0, RECENT_LIMIT)
    audioRecent.value = items.filter((row) => row.model.startsWith(MEDIA_MODELS.audioPrefix)).slice(0, RECENT_LIMIT)
  } else {
    appStore.showError(t('admin.mediaQuota.loadFailed'))
  }
}

async function refreshAll() {
  loading.value = true
  nowTick.value = Date.now()
  await Promise.all([loadQuota(), loadStats(), loadPricing(), loadRecent()])
  loading.value = false
  initialLoading.value = false
}

// ==================== Format helpers ====================

function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

function formatDuration(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '—'
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${ms}ms`
}

// ==================== Lifecycle ====================

onMounted(() => {
  tickTimer = setInterval(() => {
    nowTick.value = Date.now()
  }, 1000)
  void refreshAll()
})

onBeforeUnmount(() => {
  if (tickTimer) {
    clearInterval(tickTimer)
    tickTimer = null
  }
})
</script>
