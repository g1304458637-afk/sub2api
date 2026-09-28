<template>
  <AppLayout>
    <div class="muc-scope muc-spend">
      <div class="muc-spend__content">
        <header class="muc-spend__header">
          <h1 class="muc-spend__title">{{ t('modelSpend.title') }}</h1>
          <p class="muc-spend__subtitle">{{ t('modelSpend.subtitle') }}</p>
        </header>

        <!-- 时间范围：当前周期（有订阅周期时）| 本月 | 近30天 | 全部 -->
        <div class="muc-spend__ranges" role="tablist" :aria-label="t('modelSpend.rangeLabel')">
          <button
            v-for="option in rangeOptions"
            :key="option.key"
            type="button"
            role="tab"
            class="muc-spend__range"
            :class="{ 'muc-spend__range--active': activeRange === option.key }"
            :aria-selected="activeRange === option.key ? 'true' : 'false'"
            :data-testid="`spend-range-${option.key}`"
            @click="setRange(option.key)"
          >
            {{ option.label }}
          </button>
        </div>
        <p v-if="rangeCaption" class="muc-spend__range-caption">{{ rangeCaption }}</p>

        <!-- 顶部核心指标：本期累计模型消费（真实 Billing 口径 actual_cost） -->
        <MucGlassCard variant="strong" class="muc-spend__hero" data-testid="spend-total">
          <span class="muc-spend__hero-label">{{ t('modelSpend.totalLabel') }}</span>
          <span v-if="spendLoading" class="muc-spend__hero-value muc-spend__hero-value--loading">
            <MucSkeleton width="180px" height="40px" />
          </span>
          <span v-else class="muc-spend__hero-value">{{ totalDisplay }}</span>
          <span class="muc-spend__hero-note">{{ t('modelSpend.totalNote') }}</span>
        </MucGlassCard>

        <!-- 余额/额度摘要：钱包与套餐是两个权益来源，永不错误相加 -->
        <div v-if="showAccountStrip" class="muc-spend__accounts">
          <router-link
            v-if="walletBalance !== null"
            to="/wallet"
            class="muc-spend__account"
            data-testid="spend-wallet"
          >
            <span class="muc-spend__account-label">{{ t('modelSpend.walletLabel') }}</span>
            <span class="muc-spend__account-value">{{ walletDisplay }}</span>
            <span class="muc-spend__account-hint">{{ t('modelSpend.walletHint') }}</span>
          </router-link>
          <router-link
            v-if="subscriptionSummary"
            to="/subscriptions"
            class="muc-spend__account"
            data-testid="spend-subscription"
          >
            <span class="muc-spend__account-label">{{ t('modelSpend.subLabel') }}</span>
            <span class="muc-spend__account-value">{{ subscriptionSummary.percent }}</span>
            <span class="muc-spend__account-hint">{{ subscriptionSummary.name }}</span>
          </router-link>
        </div>

        <!-- 按模型：消费排名（Spend by Model，cost DESC） -->
        <MucSectionHeader :title="t('modelSpend.byModelTitle')" :description="t('modelSpend.byModelDesc')" />

        <MucGlassCard class="muc-spend__models" data-testid="spend-models">
          <div v-if="spendLoading" class="muc-spend__skeleton">
            <MucSkeleton v-for="i in 4" :key="i" height="30px" />
          </div>
          <MucState v-else-if="modelRows.length === 0" :message="t('modelSpend.emptyModels')" icon="inbox" />
          <ul v-else class="muc-spend__model-list">
            <li v-for="row in modelRows" :key="row.model" class="muc-spend__model-row">
              <div class="muc-spend__model-head">
                <span class="muc-spend__model-name" :title="row.model">{{ row.model }}</span>
                <span class="muc-spend__model-amount">{{ currencyStore.formatUSD(row.actualCost) }}</span>
                <span class="muc-spend__model-share">{{ row.sharePercent }}</span>
              </div>
              <div class="muc-spend__model-bar-track">
                <div
                  class="muc-spend__model-bar"
                  :style="{ width: row.barPercent + '%' }"
                  aria-hidden="true"
                ></div>
              </div>
            </li>
          </ul>
        </MucGlassCard>

        <!-- 每日消费：轻量趋势（仅短时间范围展示；全部 → 隐藏避免密度失控） -->
        <template v-if="!spendLoading && showTrend">
          <MucSectionHeader :title="t('modelSpend.dailyTitle')" :description="t('modelSpend.dailyDesc')" />
          <MucGlassCard class="muc-spend__daily" data-testid="spend-daily">
            <MucState v-if="dailyPoints.length === 0" :message="t('modelSpend.emptyDaily')" icon="inbox" />
            <div v-else class="muc-spend__daily-chart" role="img" :aria-label="t('modelSpend.dailyTitle')">
              <div v-for="point in dailyPoints" :key="point.date" class="muc-spend__daily-col">
                <div class="muc-spend__daily-bar-area">
                  <div
                    class="muc-spend__daily-bar"
                    :class="{ 'muc-spend__daily-bar--zero': point.barPercent <= 0 }"
                    :style="{ height: point.barHeight + '%' }"
                    :title="`${point.label} · ${currencyStore.formatUSD(point.actualCost)}`"
                  ></div>
                </div>
                <span class="muc-spend__daily-label">{{ point.label }}</span>
              </div>
            </div>
          </MucGlassCard>
        </template>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import '@/components/pricing/muc-tokens.css'
import MucGlassCard from '@/components/muc/MucGlassCard.vue'
import MucSectionHeader from '@/components/muc/MucSectionHeader.vue'
import MucState from '@/components/muc/MucState.vue'
import MucSkeleton from '@/components/muc/MucSkeleton.vue'
import { usageAPI } from '@/api/usage'
import { getAccountStatus, type AccountStatus } from '@/api/subscriptions'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'
import { getBrowserTimeZone } from '@/utils/format'

/**
 * 模型消费（Model Spend）：面向普通学生的极简消费视图。
 *
 * 只回答三个问题：这段时间总共花了多少钱、钱主要花在哪些模型、各花了多少。
 * - 金额一律来自真实 Billing 口径（usage_logs.actual_cost，订阅与钱包按量同一 USD 成本基准），
 *   经 currencyDisplay 统一换算展示；绝不由展示价（Presentation Pricing）重算。
 * - 不展示 token / cache / endpoint / group / channel / latency 等技术内部信息。
 * - 钱包余额与套餐剩余是两个权益来源，只分开展示，不合并且不参与排名金额。
 */

type RangeKey = 'period' | 'month' | '30d' | 'all'

const { t } = useI18n()
const currencyStore = useCurrencyDisplayStore()

const spendLoading = ref(true)
const totalActualCost = ref(0)
const modelRows = ref<{ model: string; actualCost: number; sharePercent: string; barPercent: number }[]>([])
const dailyPoints = ref<{ date: string; label: string; actualCost: number; barPercent: number; barHeight: number }[]>([])
const showTrend = ref(true)

const activeRange = ref<RangeKey>('30d')
const subscriptionPeriod = ref<{ start: string; end: string } | null>(null)
const walletBalance = ref<string | null>(null)
const walletCurrency = ref('')
const subscriptionInfo = ref<{ name: string; remainingPercent: number | null } | null>(null)

const ALL_RANGE_START = '2000-01-01'

const rangeOptions = computed(() => {
  const options: { key: RangeKey; label: string }[] = []
  if (subscriptionPeriod.value) {
    options.push({ key: 'period', label: t('modelSpend.rangePeriod') })
  }
  options.push(
    { key: 'month', label: t('modelSpend.rangeMonth') },
    { key: '30d', label: t('modelSpend.range30d') },
    { key: 'all', label: t('modelSpend.rangeAll') },
  )
  return options
})

const rangeCaption = computed(() => {
  const bounds = currentRangeBounds()
  if (!bounds) return ''
  return `${bounds.start} — ${bounds.end}`
})

const totalDisplay = computed(() => currencyStore.formatUSD(totalActualCost.value))

const walletDisplay = computed(() => {
  if (walletBalance.value === null) return '—'
  const value = parseFloat(walletBalance.value)
  if (!Number.isFinite(value)) return '—'
  if (walletCurrency.value.toUpperCase() === 'USD') return currencyStore.formatUSD(value)
  return currencyStore.formatCNY(value)
})

const showAccountStrip = computed(() => walletBalance.value !== null || subscriptionInfo.value !== null)

const subscriptionSummary = computed(() => {
  if (!subscriptionInfo.value) return null
  const percent = subscriptionInfo.value.remainingPercent
  return {
    name: subscriptionInfo.value.name,
    percent: percent === null ? t('modelSpend.subUnmetered') : t('modelSpend.subRemaining', { percent }),
  }
})

function currentRangeBounds(): { start: string; end: string } | null {
  const today = localDateString(new Date())
  switch (activeRange.value) {
    case 'period':
      return subscriptionPeriod.value ? { start: subscriptionPeriod.value.start, end: today } : null
    case 'month': {
      const now = new Date()
      return { start: localDateString(new Date(now.getFullYear(), now.getMonth(), 1)), end: today }
    }
    case '30d': {
      const start = new Date()
      start.setDate(start.getDate() - 29)
      return { start: localDateString(start), end: today }
    }
    case 'all':
      return { start: ALL_RANGE_START, end: today }
  }
  return null
}

function localDateString(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function setRange(key: RangeKey) {
  if (activeRange.value === key) return
  activeRange.value = key
}

watch(activeRange, () => {
  void loadSpend()
})

function shortDayLabel(dateStr: string): string {
  // trend 返回 YYYY-MM-DD（用户时区桶）；压缩为 MM-DD
  const parts = dateStr.split('-')
  return parts.length === 3 ? `${parts[1]}-${parts[2]}` : dateStr
}

async function loadSpend() {
  const bounds = currentRangeBounds()
  if (!bounds) return
  const timezone = getBrowserTimeZone()
  const params = { start_date: bounds.start, end_date: bounds.end, timezone }

  spendLoading.value = true
  try {
    const wantsTrend = activeRange.value !== 'all'
    const [stats, models, trend] = await Promise.all([
      usageAPI.getStatsByDateRange(bounds.start, bounds.end),
      usageAPI.getDashboardModels(params),
      wantsTrend
        ? usageAPI.getDashboardTrend({ ...params, granularity: 'day' })
        : Promise.resolve(null),
    ])

    totalActualCost.value = Number(stats.total_actual_cost) || 0

    // 排名只看真实消费（actual_cost），cost DESC；占比相对列表内合计，保证条形内部一致
    const ranked = [...models.models]
      .map((m) => ({ model: m.model, actualCost: Number(m.actual_cost) || 0 }))
      .filter((m) => m.actualCost > 0)
      .sort((a, b) => b.actualCost - a.actualCost)
    const modelSum = ranked.reduce((sum, m) => sum + m.actualCost, 0)
    modelRows.value = ranked.map((m) => ({
      ...m,
      sharePercent: modelSum > 0 ? `${((m.actualCost / modelSum) * 100).toFixed(0)}%` : '0%',
      barPercent: modelSum > 0 ? Math.max((m.actualCost / modelSum) * 100, 1.5) : 0,
    }))

    showTrend.value = wantsTrend
    const points = (trend?.trend ?? []).map((p) => ({
      date: p.date,
      label: shortDayLabel(p.date),
      actualCost: Number(p.actual_cost) || 0,
      barPercent: 0,
      barHeight: 0,
    }))
    const maxDaily = points.reduce((max, p) => Math.max(max, p.actualCost), 0)
    for (const p of points) {
      p.barPercent = maxDaily > 0 ? (p.actualCost / maxDaily) * 100 : 0
      p.barHeight = maxDaily > 0 ? Math.max((p.actualCost / maxDaily) * 100, p.actualCost > 0 ? 4 : 2) : 2
    }
    dailyPoints.value = points
  } catch {
    totalActualCost.value = 0
    modelRows.value = []
    dailyPoints.value = []
  } finally {
    spendLoading.value = false
  }
}

async function loadAccountStatus() {
  try {
    const status: AccountStatus = await getAccountStatus()
    walletBalance.value = status.wallet?.balance ?? null
    walletCurrency.value = status.wallet?.canonical_currency ?? ''

    // 订阅周期与剩余额度（服务端净化视图：仅百分比，无内部金额）
    const active = status.subscriptions?.[0]
    if (active) {
      subscriptionInfo.value = {
        name: active.display_name,
        remainingPercent: active.weekly_window ? active.weekly_window.remaining_percent : null,
      }
      const start = active.weekly_period_started_at
      if (start && active.weekly_period_ends_at) {
        subscriptionPeriod.value = {
          start: localDateString(new Date(start)),
          end: localDateString(new Date(active.weekly_period_ends_at)),
        }
      }
    }
  } catch {
    // 账户状态加载失败不阻塞消费数据；仅隐藏余额/额度摘要
    walletBalance.value = null
    subscriptionInfo.value = null
  }
}

onMounted(() => {
  void loadAccountStatus().then(() => {
    // 「当前周期」依赖订阅周期；拿到后再决定默认范围。
    // 范围变化由 watch 统一触发加载，仅在默认范围未变化时手动加载一次，避免双重请求。
    const defaultRange: RangeKey = subscriptionPeriod.value ? 'period' : '30d'
    if (defaultRange !== activeRange.value) {
      activeRange.value = defaultRange
    } else {
      void loadSpend()
    }
  })
})
</script>

<style scoped>
.muc-spend {
  position: relative;
  min-height: calc(100vh - 96px);
  /* 不画不透明底：让全局 CampusBackdrop 动画透出来，可读性由玻璃卡片承担 */
  color: var(--muc-text-primary);
}

.muc-spend__content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-width: 760px;
  margin: 0 auto;
  padding: clamp(28px, 5vh, 56px) clamp(16px, 4vw, 40px) 44px;
}

.muc-spend__header {
  text-align: center;
}

.muc-spend__title {
  font-size: clamp(22px, 3vw, 30px);
  font-weight: 700;
}

.muc-spend__subtitle {
  margin-top: 8px;
  font-size: 13.5px;
  color: var(--muc-text-secondary);
}

.muc-spend__ranges {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
}

.muc-spend__range {
  padding: 7px 16px;
  border-radius: 999px;
  border: 1px solid var(--muc-glass-border);
  background: var(--muc-glass-bg);
  color: var(--muc-text-secondary);
  font-size: 13px;
  transition: color 0.16s ease, border-color 0.16s ease, background 0.16s ease;
}

.muc-spend__range:hover {
  color: var(--muc-text-primary);
}

.muc-spend__range--active {
  background: rgba(var(--muc-red-rgb, 200, 36, 51), 0.16);
  border-color: rgba(var(--muc-red-rgb, 200, 36, 51), 0.45);
  color: var(--muc-text-primary);
  font-weight: 600;
}

.muc-spend__range-caption {
  margin: -6px 0 0;
  text-align: center;
  font-size: 12px;
  color: var(--muc-text-muted);
  font-variant-numeric: tabular-nums;
}

.muc-spend__hero {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 26px 28px;
  text-align: center;
}

.muc-spend__hero-label {
  font-size: 11px;
  letter-spacing: 0.1em;
  color: var(--muc-text-muted);
}

.muc-spend__hero-value {
  font-size: 40px;
  font-weight: 800;
  line-height: 1.05;
  font-variant-numeric: tabular-nums;
}

.muc-spend__hero-value--loading {
  display: flex;
  justify-content: center;
}

.muc-spend__hero-note {
  font-size: 12px;
  color: var(--muc-text-muted);
}

.muc-spend__accounts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.muc-spend__account {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 16px 20px;
  border-radius: 20px;
  border: 1px solid var(--muc-glass-border);
  background: var(--muc-glass-bg);
  transition: border-color 0.16s ease, transform 0.16s ease;
}

.muc-spend__account:hover {
  border-color: rgba(var(--muc-red-rgb, 200, 36, 51), 0.4);
  transform: translateY(-1px);
}

.muc-spend__account-label {
  font-size: 11px;
  letter-spacing: 0.08em;
  color: var(--muc-text-muted);
}

.muc-spend__account-value {
  font-size: 20px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.muc-spend__account-hint {
  font-size: 11.5px;
  color: var(--muc-text-muted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.muc-spend__models {
  padding: 8px;
}

.muc-spend__skeleton {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
}

.muc-spend__model-list {
  margin: 0;
  padding: 0;
  list-style: none;
}

.muc-spend__model-row {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 12px 14px;
}

.muc-spend__model-row + .muc-spend__model-row {
  border-top: 1px solid var(--muc-glass-border);
}

.muc-spend__model-head {
  display: flex;
  align-items: baseline;
  gap: 12px;
}

.muc-spend__model-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
  font-weight: 600;
}

.muc-spend__model-amount {
  flex-shrink: 0;
  font-size: 14px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.muc-spend__model-share {
  flex-shrink: 0;
  min-width: 3em;
  text-align: right;
  font-size: 12px;
  color: var(--muc-text-muted);
  font-variant-numeric: tabular-nums;
}

.muc-spend__model-bar-track {
  height: 6px;
  border-radius: 999px;
  background: rgba(var(--muc-red-rgb, 200, 36, 51), 0.08);
  overflow: hidden;
}

.muc-spend__model-bar {
  height: 100%;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(var(--muc-red-rgb, 200, 36, 51), 0.75), rgba(var(--muc-red-rgb, 200, 36, 51), 0.95));
}

.muc-spend__daily {
  padding: 18px;
}

.muc-spend__daily-chart {
  display: flex;
  align-items: stretch;
  gap: 4px;
  height: 120px;
}

.muc-spend__daily-col {
  flex: 1 1 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.muc-spend__daily-bar-area {
  flex: 1 1 auto;
  display: flex;
  align-items: flex-end;
  justify-content: center;
}

.muc-spend__daily-bar {
  width: 100%;
  max-width: 26px;
  border-radius: 6px 6px 2px 2px;
  background: linear-gradient(180deg, rgba(var(--muc-red-rgb, 200, 36, 51), 0.85), rgba(var(--muc-red-rgb, 200, 36, 51), 0.45));
}

.muc-spend__daily-bar--zero {
  background: var(--muc-glass-border);
}

.muc-spend__daily-label {
  flex-shrink: 0;
  font-size: 9px;
  color: var(--muc-text-muted);
  text-align: center;
  overflow: hidden;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .muc-spend__hero-value {
    font-size: 32px;
  }

  .muc-spend__model-head {
    flex-wrap: wrap;
    gap: 6px;
  }

  .muc-spend__model-name {
    flex-basis: 100%;
  }

  .muc-spend__daily-label {
    font-size: 8px;
  }
}
</style>
