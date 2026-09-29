<template>
  <div class="plaza-pricing-table" :style="accentStyle">
    <!-- 桌面/平板:完整表格 -->
    <div class="hidden overflow-x-auto lg:block">
      <table class="w-full min-w-[900px] table-auto border-collapse text-sm tabular-nums">
      <colgroup>
        <col class="w-[25%]" />
        <col class="w-[13%]" />
        <col class="w-[14%]" />
        <col class="w-[14%]" />
        <col class="w-[11%]" />
        <col class="w-[11%]" />
        <col class="w-[12%]" />
      </colgroup>
      <thead>
        <tr
          class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400"
        >
          <th
            rowspan="2"
            class="border-r border-gray-100 py-2.5 pl-5 pr-4 text-left align-middle dark:border-dark-700/60"
          >
            {{ t('modelPlaza.table.model') }}
          </th>
          <th colspan="3" class="pz-bg pt-2 text-center">
            <div class="pz-title border-b pb-2 font-semibold">
              {{ t('modelPlaza.table.standardPrice') }}
              <span class="pz-unit ml-1 normal-case font-normal">{{ t('modelPlaza.table.unitPerMillion') }}</span>
            </div>
          </th>
          <th
            colspan="3"
            class="border-l border-gray-100 pt-2 text-center dark:border-dark-700/60"
          >
            <div class="border-b border-gray-200 pb-2 text-gray-400 dark:border-dark-600 dark:text-dark-500">
              {{ t('modelPlaza.table.officialPrice') }}
              <span class="ml-1 normal-case font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.unitPerMillion') }}</span>
            </div>
          </th>
        </tr>
        <tr
          class="border-b border-gray-200 text-left text-[11px] font-medium uppercase leading-4 tracking-wide text-gray-400 dark:border-dark-700 dark:text-dark-500"
        >
          <th class="pz-bg px-3 py-2 font-medium">{{ t('modelPlaza.table.input') }}</th>
          <th class="pz-bg px-3 py-2 font-medium">{{ t('modelPlaza.table.output') }}</th>
          <th class="pz-bg px-3 py-2 font-medium">{{ t('modelPlaza.table.cache') }}</th>
          <th class="border-l border-gray-100 px-3 py-2 font-medium dark:border-dark-700/60">
            {{ t('modelPlaza.table.input') }}
          </th>
          <th class="px-3 py-2 font-medium">{{ t('modelPlaza.table.output') }}</th>
          <th class="px-3 py-2 font-medium">{{ t('modelPlaza.table.cache') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="m in sortedModels"
          :key="`${m.platform}:${m.name}`"
          class="border-b border-gray-100 transition-colors last:border-b-0 hover:bg-gray-50/70 dark:border-dark-800 dark:hover:bg-dark-800/50"
        >
          <!-- 模型名 + 平台徽章;Composite 分组中同名模型按具体平台分别成行 -->
          <td class="border-r border-gray-100 py-2.5 pl-5 pr-4 align-middle dark:border-dark-700/60">
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="font-medium text-gray-900 dark:text-white">{{ m.name }}</span>
              <span
                v-if="platform && m.platform !== platform"
                :class="[
                  'inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium',
                  platformBadgeLightClass(m.platform)
                ]"
              >
                {{ platformLabel(m.platform) }}
              </span>
              <span
                v-if="standardBillingMode(m) !== BILLING_MODE_TOKEN && hasStandardPricing(m)"
                class="rounded-md bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700/70 dark:text-dark-300"
              >
                {{ billingModeLabel(m) }}
              </span>
            </div>
          </td>

          <!-- 标准价格（展示价，绝对值；token 模式带阶梯时每档一行） -->
          <template v-if="standardBillingMode(m) === BILLING_MODE_TOKEN">
            <td class="pz-cell px-3 py-2.5 align-middle font-mono font-semibold text-gray-900 dark:text-gray-50">
              <template v-if="hasStandardPricing(m)">
                <template v-if="standardIntervals(m).length">
                  <div
                    v-for="(iv, idx) in standardIntervals(m)"
                    :key="idx"
                    class="whitespace-nowrap text-xs leading-5"
                  >
                    <span class="mr-1 font-sans font-normal text-gray-400 dark:text-dark-500" :title="tierHint(m)">{{ tierLabel(iv) }}</span>
                    {{ standardPerMillion(iv.input_price) }}
                  </div>
                </template>
                <template v-else>{{ standardPerMillion(m.display_pricing?.input_price) }}</template>
              </template>
              <span v-else class="font-sans text-xs font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.priceNotPublished') }}</span>
            </td>
            <td class="pz-cell px-3 py-2.5 align-middle font-mono font-semibold text-gray-900 dark:text-gray-50">
              <template v-if="hasStandardPricing(m)">
                <template v-if="standardIntervals(m).length">
                  <div
                    v-for="(iv, idx) in standardIntervals(m)"
                    :key="idx"
                    class="whitespace-nowrap text-xs leading-5"
                    :title="tierHint(m)"
                  >
                    {{ standardPerMillion(iv.output_price) }}
                  </div>
                </template>
                <template v-else>{{ standardPerMillion(m.display_pricing?.output_price) }}</template>
              </template>
            </td>
            <td class="pz-cell px-3 py-2.5 align-middle">
              <template v-if="hasStandardPricing(m)">
                <template v-if="hasTierCachePricing(standardIntervals(m))">
                  <div
                    v-for="(iv, idx) in standardIntervals(m)"
                    :key="idx"
                    class="whitespace-nowrap font-mono text-xs leading-5 text-gray-800 dark:text-gray-200"
                    :title="tierHint(m)"
                  >
                    <template v-if="iv.cache_write_price != null || iv.cache_write_1h_price != null || iv.cache_read_price != null">
                      <span class="font-sans font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheWriteShort') }}</span>
                      {{ standardPerMillion(iv.cache_write_price) }}
                      <template v-if="iv.cache_write_1h_price != null"
                        ><span class="font-sans font-normal text-gray-400 dark:text-dark-500"> (1h </span>{{ standardPerMillion(iv.cache_write_1h_price)
                        }}<span class="font-sans font-normal text-gray-400 dark:text-dark-500">)</span></template
                      >
                      <span class="ml-1 font-sans font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheReadShort') }}</span>
                      {{ standardPerMillion(iv.cache_read_price) }}
                    </template>
                    <span v-else class="text-gray-400 dark:text-dark-500">-</span>
                  </div>
                </template>
                <div
                  v-else-if="hasStandardCache(m)"
                  class="space-y-0.5 font-mono text-xs text-gray-800 dark:text-gray-200"
                >
                  <div>
                    <span class="mr-1 font-sans font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheWrite') }}</span>
                    {{ standardPerMillion(m.display_pricing?.cache_write_price)
                    }}<template v-if="m.display_pricing?.cache_write_1h_price != null"
                      ><span class="font-sans font-normal text-gray-400 dark:text-dark-500"> (1h </span>{{ standardPerMillion(m.display_pricing?.cache_write_1h_price)
                      }}<span class="font-sans font-normal text-gray-400 dark:text-dark-500">)</span></template
                    >
                  </div>
                  <div>
                    <span class="mr-1 font-sans font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheRead') }}</span>
                    {{ standardPerMillion(m.display_pricing?.cache_read_price) }}
                  </div>
                </div>
                <span v-else class="text-gray-400 dark:text-dark-500">-</span>
              </template>
              <span v-else class="text-gray-400 dark:text-dark-500">-</span>
            </td>
          </template>

          <!-- 按次 / 按图片计费：标准价区整体合并，阶梯芯片或单一按次价 -->
          <template v-else>
            <td colspan="3" class="pz-cell px-3 py-2.5 align-middle">
              <div
                v-if="standardRequestIntervals(m).length"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  v-for="(iv, idx) in standardRequestIntervals(m)"
                  :key="idx"
                  class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-800 dark:bg-dark-700/60 dark:text-gray-200"
                >
                  <span class="font-sans text-gray-400 dark:text-dark-500">{{ tierLabel(iv) }}</span>
                  {{ standardRequestPrice(iv.per_request_price) }}<span class="font-sans text-gray-400 dark:text-dark-500">{{ perUnitSuffix(m) }}</span>
                </span>
              </div>
              <template v-else-if="m.display_pricing?.per_request_price != null">
                <span class="font-mono font-semibold text-gray-900 dark:text-gray-50">
                  {{ standardRequestPrice(m.display_pricing.per_request_price) }}
                </span>
                <span class="ml-1 text-xs text-gray-400 dark:text-dark-500">{{ perUnitSuffix(m) }}</span>
              </template>
              <span v-else class="font-sans text-xs text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.priceNotPublished') }}</span>
            </td>
          </template>

          <!-- 官方参考价（仅供参考；官方有阶梯时每档一行） -->
          <td
            class="border-l border-gray-100 px-3 py-2.5 align-middle font-mono text-xs text-gray-500 dark:border-dark-700/60 dark:text-dark-400"
          >
            <template v-if="officialIntervals(m).length">
              <div
                v-for="(iv, idx) in officialIntervals(m)"
                :key="idx"
                class="whitespace-nowrap leading-5"
              >
                <span class="mr-1 font-sans text-gray-400 dark:text-dark-500" :title="t('modelPlaza.table.tierHint')">{{ tierLabel(iv) }}</span>
                {{ official(iv.input_price) }}
              </div>
            </template>
            <template v-else>{{ official(m.official_pricing?.input_price) }}</template>
          </td>
          <td class="px-3 py-2.5 align-middle font-mono text-xs text-gray-500 dark:text-dark-400">
            <template v-if="officialIntervals(m).length">
              <div
                v-for="(iv, idx) in officialIntervals(m)"
                :key="idx"
                class="whitespace-nowrap leading-5"
                :title="t('modelPlaza.table.tierHint')"
              >
                {{ official(iv.output_price) }}
              </div>
            </template>
            <template v-else>{{ official(m.official_pricing?.output_price) }}</template>
          </td>
          <td class="px-3 py-2.5 align-middle">
            <template v-if="hasTierCachePricing(officialIntervals(m))">
              <div
                v-for="(iv, idx) in officialIntervals(m)"
                :key="idx"
                class="whitespace-nowrap font-mono text-xs leading-5 text-gray-500 dark:text-dark-400"
                :title="t('modelPlaza.table.tierHint')"
              >
                <template v-if="iv.cache_write_price != null || iv.cache_write_1h_price != null || iv.cache_read_price != null">
                  <span class="font-sans text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheWriteShort') }}</span>
                  {{ official(iv.cache_write_price) }}
                  <template v-if="iv.cache_write_1h_price != null"
                    ><span class="font-sans text-gray-400 dark:text-dark-500"> (1h </span>{{ official(iv.cache_write_1h_price)
                    }}<span class="font-sans text-gray-400 dark:text-dark-500">)</span></template
                  >
                  <span class="ml-1 font-sans text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheReadShort') }}</span>
                  {{ official(iv.cache_read_price) }}
                </template>
                <span v-else class="text-gray-400 dark:text-dark-500">-</span>
              </div>
            </template>
            <div
              v-else-if="m.official_pricing && hasOfficialCache(m.official_pricing)"
              class="space-y-0.5 font-mono text-xs text-gray-500 dark:text-dark-400"
            >
              <div>
                <span class="mr-1 font-sans font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheWrite') }}</span>
                {{ official(m.official_pricing.cache_write_price)
                }}<template v-if="m.official_pricing.cache_write_1h_price != null"
                  ><span class="font-sans font-normal text-gray-400 dark:text-dark-500"> (1h </span>{{ official(m.official_pricing.cache_write_1h_price)
                  }}<span class="font-sans font-normal text-gray-400 dark:text-dark-500">)</span></template
                >
              </div>
              <div>
                <span class="mr-1 font-sans font-normal text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.cacheRead') }}</span>
                {{ official(m.official_pricing.cache_read_price) }}
              </div>
            </div>
            <span v-else class="text-gray-400 dark:text-dark-500">-</span>
          </td>
        </tr>
      </tbody>
    </table>
    </div>

    <!-- 移动端:卡片布局(禁止桌面宽表格横向滚动) -->
    <div class="space-y-3 lg:hidden" data-testid="plaza-model-cards">
      <div
        v-for="m in sortedModels"
        :key="`card-${m.platform}:${m.name}`"
        class="overflow-hidden rounded-xl border border-gray-100 bg-white dark:border-dark-700/60 dark:bg-dark-800/40"
      >
        <div class="border-b border-gray-50 px-4 py-3 dark:border-dark-800">
          <div class="flex flex-wrap items-center gap-1.5">
            <span class="font-medium text-gray-900 dark:text-white">{{ m.name }}</span>
            <span
              v-if="platform && m.platform !== platform"
              :class="[
                'inline-flex items-center rounded-md px-1.5 py-0.5 text-[10px] font-medium',
                platformBadgeLightClass(m.platform)
              ]"
            >
              {{ platformLabel(m.platform) }}
            </span>
            <span
              v-if="standardBillingMode(m) !== BILLING_MODE_TOKEN && hasStandardPricing(m)"
              class="rounded-md bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700/70 dark:text-dark-300"
            >
              {{ billingModeLabel(m) }}
            </span>
          </div>
        </div>

        <!-- 标准价格 -->
        <div class="pz-bg px-4 py-3">
          <p class="pz-title text-[11px] font-semibold uppercase tracking-wider">
            {{ t('modelPlaza.table.standardPrice') }}
            <span class="pz-unit ml-1 font-normal normal-case">{{ t('modelPlaza.table.unitPerMillion') }}</span>
          </p>
          <template v-if="hasStandardPricing(m)">
            <template v-if="standardBillingMode(m) === BILLING_MODE_TOKEN">
              <dl class="mt-2 space-y-1.5 text-sm">
                <div
                  v-for="(iv, idx) in standardCardRows(m)"
                  :key="idx"
                  class="flex items-baseline justify-between gap-3"
                >
                  <dt class="flex-shrink-0 text-xs text-gray-400 dark:text-dark-500">
                    <template v-if="iv.tier">{{ iv.tier }} </template>{{ iv.label }}
                  </dt>
                  <dd class="text-right">
                    <span class="font-mono font-semibold text-gray-900 dark:text-gray-50">{{ iv.value }}</span>
                    <span
                      v-if="iv.cache"
                      class="ml-1 font-mono text-xs text-gray-500 dark:text-dark-400"
                    >{{ iv.cache }}</span>
                  </dd>
                </div>
              </dl>
            </template>
            <div v-else class="mt-2">
              <template v-if="standardRequestIntervals(m).length">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span
                    v-for="(iv, idx) in standardRequestIntervals(m)"
                    :key="idx"
                    class="inline-flex items-center gap-1 rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-800 dark:bg-dark-700/60 dark:text-gray-200"
                  >
                    <span class="font-sans text-gray-400 dark:text-dark-500">{{ tierLabel(iv) }}</span>
                    {{ standardRequestPrice(iv.per_request_price) }}<span class="font-sans text-gray-400 dark:text-dark-500">{{ perUnitSuffix(m) }}</span>
                  </span>
                </div>
              </template>
              <p v-else-if="m.display_pricing?.per_request_price != null" class="text-sm">
                <span class="font-mono font-semibold text-gray-900 dark:text-gray-50">{{ standardRequestPrice(m.display_pricing.per_request_price) }}</span>
                <span class="ml-1 text-xs text-gray-400 dark:text-dark-500">{{ perUnitSuffix(m) }}</span>
              </p>
            </div>
          </template>
          <p v-else class="mt-2 text-xs text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.table.priceNotPublished') }}
          </p>
        </div>

        <!-- 官方参考价(折叠为紧凑两行) -->
        <div
          v-if="m.official_pricing"
          class="border-t border-gray-50 px-4 py-2.5 dark:border-dark-800"
        >
          <p class="text-[11px] uppercase tracking-wider text-gray-400 dark:text-dark-500">
            {{ t('modelPlaza.table.officialPrice') }}
          </p>
          <p class="mt-1 font-mono text-xs text-gray-500 dark:text-dark-400">
            {{ official(m.official_pricing.input_price) }} <span class="text-gray-400">/</span> {{ official(m.official_pricing.output_price) }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatScaled, resolveIntervalPrices } from '@/utils/pricing'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'
import { platformAccentColor, platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_IMAGE,
  type BillingMode
} from '@/constants/channel'
import type { PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'

const props = defineProps<{
  models: PlazaModel[]
  /** 分组平台；标准价分区底色随平台着色，未知平台回退品牌青。 */
  platform?: string
}>()

const { t } = useI18n()

/** 标准价分区只从平台拿一个主色，浅底/标题/下划线全部由 scoped CSS 用 color-mix 派生。 */
const accentStyle = computed(() => ({ '--plaza-accent': platformAccentColor(props.platform ?? '') }))

const PER_MILLION = 1_000_000

/**
 * 展示顺序：
 * 1. token 计费的排在前，按图/按次计费的沉到末尾——量纲不同不混排；
 * 2. 组内按标准价输出价从高到低，无标准价的排最后；
 * 3. 同价按名称降序（新版本号在前，如 gpt-5.6 先于 gpt-5.5）。
 */
const sortedModels = computed(() => {
  return [...props.models].sort((a, b) => {
    const ta = standardBillingMode(a) === BILLING_MODE_TOKEN
    const tb = standardBillingMode(b) === BILLING_MODE_TOKEN
    if (ta !== tb) return ta ? -1 : 1
    const pa = a.display_pricing?.output_price ?? null
    const pb = b.display_pricing?.output_price ?? null
    if (pa != null && pb != null && pa !== pb) return pb - pa
    if (pa != null && pb == null) return -1
    if (pa == null && pb != null) return 1
    return b.name.localeCompare(a.name)
  })
})

function standardBillingMode(m: PlazaModel): BillingMode {
  return (m.display_pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
}

function hasStandardPricing(m: PlazaModel): boolean {
  return m.display_pricing != null
}

function billingModeLabel(m: PlazaModel): string {
  return standardBillingMode(m) === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perImage')
    : t('modelPlaza.table.perRequest')
}

/** 价格统一保底 2 位小数，更长的有效小数原样保留（如 $0.003 不能显示成 $0.00）。 */
const MIN_DECIMALS = 2

/** 展示币种换算：CNY 展示按配置的 USD/CNY 汇率换算，账本本身保持 USD。 */
const currencyStore = useCurrencyDisplayStore()
const displayCurrencyFactor = computed(() =>
  currencyStore.displayCurrency === 'CNY' ? currencyStore.usdToCnyRate : 1
)
const displayCurrencySymbol = computed(() =>
  currencyStore.displayCurrency === 'CNY' ? '¥' : '$'
)

/** 标准价格 = 展示价原值（绝对口径，不乘任何倍率），按展示币种/1M token 展示。 */
function standardPerMillion(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * displayCurrencyFactor.value, PER_MILLION, MIN_DECIMALS, displayCurrencySymbol.value)
}

/** 按次 / 按图片标准单价（不乘倍率，不换算 1M）。 */
function standardRequestPrice(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * displayCurrencyFactor.value, 1, MIN_DECIMALS, displayCurrencySymbol.value)
}

/** 官方参考价不乘倍率。 */
function official(value: number | null | undefined): string {
  if (value == null) return '-'
  return formatScaled(value * displayCurrencyFactor.value, PER_MILLION, MIN_DECIMALS, displayCurrencySymbol.value)
}

/** 非 token 计费的单位后缀：按图片 → “/ 张”，按次 → “/ 次”。 */
function perUnitSuffix(m: PlazaModel): string {
  return standardBillingMode(m) === BILLING_MODE_IMAGE
    ? t('modelPlaza.table.perUnitImage')
    : t('modelPlaza.table.perUnitRequest')
}

function hasStandardCache(m: PlazaModel): boolean {
  return (
    m.display_pricing?.cache_write_price != null ||
    m.display_pricing?.cache_write_1h_price != null ||
    m.display_pricing?.cache_read_price != null
  )
}

function hasOfficialCache(o: NonNullable<PlazaModel['official_pricing']>): boolean {
  return o.cache_write_price != null || o.cache_read_price != null || o.cache_write_1h_price != null
}

/** 上下文档位按下限升序展示（后端已升序，此处兜底）。 */
function sortByContext(intervals: UserPricingInterval[]): UserPricingInterval[] {
  return [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)
}

/** 标准价的阶梯定价（内联进输入/输出/缓存列）。 */
function standardIntervals(m: PlazaModel): UserPricingInterval[] {
  return sortByContext(m.display_pricing?.intervals ?? []).map(iv => resolveIntervalPrices(iv, m.display_pricing!))
}

/** 按次/按图模式的标准价阶梯（仅保留配了按次价的档位）。 */
function standardRequestIntervals(m: PlazaModel): UserPricingInterval[] {
  return (m.display_pricing?.intervals ?? []).filter((iv) => iv.per_request_price != null)
}

interface StandardCardRow {
  /** 行标签:输入 / 输出 / 缓存写入 / 缓存读取。 */
  label: string
  /** 阶梯档位标签(仅多档时出现在输入行)。 */
  tier?: string
  value: string
  /** 缓存行随附的另一半(如 1h 写入价)。 */
  cache?: string
}

/** 移动端卡片的标准价行:平价 2 行 + 缓存 2 行;多档时输入/输出按档展开。 */
function standardCardRows(m: PlazaModel): StandardCardRow[] {
  const p = m.display_pricing
  if (!p) return []
  const intervals = standardIntervals(m)
  if (intervals.length) {
    const rows: StandardCardRow[] = []
    for (const iv of intervals) {
      const tier = tierLabel(iv)
      rows.push({ label: t('modelPlaza.table.input'), tier, value: standardPerMillion(iv.input_price) })
      rows.push({ label: t('modelPlaza.table.output'), value: standardPerMillion(iv.output_price) })
    }
    return rows
  }
  const rows: StandardCardRow[] = [
    { label: t('modelPlaza.table.input'), value: standardPerMillion(p.input_price) },
    { label: t('modelPlaza.table.output'), value: standardPerMillion(p.output_price) }
  ]
  if (p.cache_write_price != null) {
    let cache = ''
    if (p.cache_write_1h_price != null) {
      cache = `(1h ${standardPerMillion(p.cache_write_1h_price)})`
    }
    rows.push({ label: t('modelPlaza.table.cacheWrite'), value: standardPerMillion(p.cache_write_price), cache })
  }
  if (p.cache_read_price != null) {
    rows.push({ label: t('modelPlaza.table.cacheRead'), value: standardPerMillion(p.cache_read_price) })
  }
  return rows
}

/** 官方阶梯（后端按目录规则合成）。 */
function officialIntervals(m: PlazaModel): UserPricingInterval[] {
  return sortByContext(m.official_pricing?.intervals ?? [])
}

/** 任一档带缓存价才按档渲染缓存列；否则沿用平价的写入/读取两行。 */
function hasTierCachePricing(intervals: UserPricingInterval[]): boolean {
  return intervals.some((iv) =>
    iv.cache_write_price != null || iv.cache_write_1h_price != null || iv.cache_read_price != null ||
    iv.cache_write_multiplier != null || iv.cache_read_multiplier != null
  )
}

/** 档位说明：整单按档计价，或（平台旧规则）仅超出部分按档计价。 */
function tierHint(m: PlazaModel): string {
  return m.long_context_basis === 'marginal'
    ? t('modelPlaza.table.tierHintMarginal')
    : t('modelPlaza.table.tierHint')
}

/**
 * 档位标签：优先后端给出的 tier_label，否则按区间生成统一形态——
 * 有上限为「≤上限」，末档为「>下限」；档位升序排列，相邻的 ≤100K / ≤200K 即表示 (100K,200K]。
 */
function tierLabel(iv: UserPricingInterval): string {
  if (iv.tier_label) return iv.tier_label
  const { min_tokens: min, max_tokens: max } = iv
  return max == null ? `>${formatTokenCount(min)}` : `≤${formatTokenCount(max)}`
}

function formatTokenCount(n: number): string {
  if (n >= 1_000_000) return `${trimZero(n / 1_000_000)}M`
  if (n >= 1_000) return `${trimZero(n / 1_000)}K`
  return String(n)
}

function trimZero(n: number): string {
  return String(Math.round(n * 100) / 100)
}
</script>

<style scoped>
/* 标准价分区配色统一从 --plaza-accent（平台主色）派生，新增平台无需扩展样式 */
.plaza-pricing-table {
  --pz-title: color-mix(in srgb, var(--plaza-accent) 88%, black);
  --pz-bg: color-mix(in srgb, var(--plaza-accent) 7%, transparent);
  --pz-bg-hover: color-mix(in srgb, var(--plaza-accent) 13%, transparent);
}

.dark .plaza-pricing-table {
  --pz-title: color-mix(in srgb, var(--plaza-accent) 70%, white);
  --pz-bg: color-mix(in srgb, var(--plaza-accent) 6%, transparent);
  --pz-bg-hover: color-mix(in srgb, var(--plaza-accent) 10%, transparent);
}

.pz-bg,
.pz-cell {
  background-color: var(--pz-bg);
}

.pz-cell {
  transition: background-color 150ms cubic-bezier(0.4, 0, 0.2, 1);
}

tbody tr:hover .pz-cell {
  background-color: var(--pz-bg-hover);
}

.pz-title {
  /* color-mix 不可用的老浏览器回退为平台原色 */
  color: var(--plaza-accent);
  color: var(--pz-title);
  border-color: color-mix(in srgb, var(--pz-title) 30%, transparent);
}

.pz-unit {
  color: color-mix(in srgb, var(--pz-title) 62%, transparent);
}
</style>
