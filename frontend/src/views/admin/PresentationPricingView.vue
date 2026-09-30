<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-72">
            <input
              v-model="search"
              type="text"
              :placeholder="t('admin.presentationPricing.searchPlaceholder')"
              class="input"
            />
          </div>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <span class="hidden text-xs text-gray-400 xl:inline dark:text-dark-500">
              {{ t('admin.presentationPricing.headerNote') }}
            </span>
            <button
              class="btn btn-secondary"
              :disabled="loading"
              @click="reload"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
      <!-- 边界警示:展示价与真实计费彻底解耦,绝不能混同 -->
      <div
        class="mb-4 flex items-start gap-2.5 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
        data-testid="presentation-pricing-warning"
      >
        <Icon name="infoCircle" size="md" class="mt-0.5 h-4 w-4 flex-shrink-0" />
        <p>{{ t('admin.presentationPricing.warning') }}</p>
      </div>

      <!-- 加载 / 错误 / 空 -->
      <div v-if="loading" class="space-y-3 p-1">
        <div v-for="i in 4" :key="i" class="h-14 animate-pulse rounded-xl bg-gray-100 dark:bg-dark-800/60"></div>
      </div>
      <div
        v-else-if="loadError"
        class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
      >
        <p>{{ t('admin.presentationPricing.loadFailed') }}</p>
        <button class="btn btn-secondary mt-3" @click="reload">{{ t('common.refresh') }}</button>
      </div>
      <div
        v-else-if="plazaUnavailable"
        class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        {{ t('admin.presentationPricing.plazaUnavailable') }}
      </div>
      <div
        v-else-if="filteredModels.length === 0"
        class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        {{ search ? t('admin.presentationPricing.noSearchResult') : t('admin.presentationPricing.empty') }}
      </div>

      <template v-else>
        <!-- 桌面表格 -->
        <div class="hidden overflow-x-auto rounded-xl border border-gray-100 lg:block dark:border-dark-700/60">
          <table class="w-full min-w-[860px] text-sm" data-testid="presentation-pricing-table">
            <thead>
              <tr class="border-b border-gray-100 text-left text-xs uppercase tracking-wider text-gray-400 dark:border-dark-700 dark:text-dark-500">
                <th class="px-4 py-2.5 font-medium">{{ t('admin.presentationPricing.table.model') }}</th>
                <th class="px-4 py-2.5 font-medium">{{ t('admin.presentationPricing.table.displayPrice') }}</th>
                <th class="px-4 py-2.5 font-medium">{{ t('admin.presentationPricing.table.source') }}</th>
                <th class="px-4 py-2.5 font-medium">{{ t('admin.presentationPricing.table.officialPrice') }}</th>
                <th class="px-4 py-2.5 font-medium">{{ t('admin.presentationPricing.table.billingNote') }}</th>
                <th class="px-4 py-2.5 text-right font-medium">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="m in filteredModels"
                :key="`${m.platform}:${m.name}`"
                class="border-b border-gray-50 last:border-b-0 hover:bg-gray-50/60 dark:border-dark-800/70 dark:hover:bg-dark-800/40"
              >
                <td class="px-4 py-2.5">
                  <div class="flex items-center gap-2">
                    <span class="font-medium text-gray-900 dark:text-white">{{ m.name }}</span>
                    <span class="text-xs text-gray-400 dark:text-dark-500">{{ platformLabel(m.platform) }}</span>
                  </div>
                </td>
                <td class="px-4 py-2.5 font-mono text-xs">
                  <template v-if="m.display && m.display.mode !== 'token'">
                    <span class="font-semibold text-gray-900 dark:text-gray-50">{{ formatPerRequest(m.display.perRequestPrice) }}</span>
                    <span class="ml-1 text-gray-400 dark:text-dark-500">$ / {{ m.display.mode === 'image' ? t('admin.presentationPricing.perImage') : t('admin.presentationPricing.perRequest') }}</span>
                  </template>
                  <template v-else-if="m.display">
                    <span class="font-semibold text-gray-900 dark:text-gray-50">{{ formatMTok(m.display.inputPrice) }}</span>
                    <span class="text-gray-400"> / </span>
                    <span class="font-semibold text-gray-900 dark:text-gray-50">{{ formatMTok(m.display.outputPrice) }}</span>
                    <span class="ml-1 text-gray-400 dark:text-dark-500">$ / 1M</span>
                  </template>
                  <span v-else class="text-gray-400 dark:text-dark-500">{{ t('modelPlaza.table.priceNotPublished') }}</span>
                </td>
                <td class="px-4 py-2.5">
                  <span :class="['inline-flex items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium', sourceBadgeClass(m.source)]">
                    {{ t(`admin.presentationPricing.source.${m.source}`) }}
                  </span>
                  <span
                    v-if="overrideByModel.get(m.name)?.enabled === false"
                    class="ml-1 inline-flex items-center rounded-md bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-500 dark:bg-dark-700/70 dark:text-dark-400"
                  >
                    {{ t('admin.presentationPricing.disabledBadge') }}
                  </span>
                </td>
                <td class="px-4 py-2.5 font-mono text-xs text-gray-500 dark:text-dark-400">
                  <template v-if="m.official">
                    {{ formatMTok(m.official.inputPrice) }} / {{ formatMTok(m.official.outputPrice) }}
                    <span class="ml-1 text-gray-400 dark:text-dark-500">$ / 1M</span>
                  </template>
                  <span v-else>-</span>
                </td>
                <td class="px-4 py-2.5 text-xs text-gray-400 dark:text-dark-500">
                  {{ t('admin.presentationPricing.billingManagedByBilling') }}
                </td>
                <td class="px-4 py-2.5 text-right">
                  <button class="btn btn-secondary btn-sm" @click="openEditor(m)">
                    {{ t('admin.presentationPricing.edit') }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 移动端卡片 -->
        <div class="space-y-3 lg:hidden" data-testid="presentation-pricing-cards">
          <div
            v-for="m in filteredModels"
            :key="`m-${m.platform}:${m.name}`"
            class="rounded-xl border border-gray-100 bg-white p-4 dark:border-dark-700/60 dark:bg-dark-800/50"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="min-w-0">
                <p class="truncate font-medium text-gray-900 dark:text-white">{{ m.name }}</p>
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ platformLabel(m.platform) }}</p>
              </div>
              <span :class="['inline-flex flex-shrink-0 items-center rounded-md px-1.5 py-0.5 text-[11px] font-medium', sourceBadgeClass(m.source)]">
                {{ t(`admin.presentationPricing.source.${m.source}`) }}
              </span>
            </div>
            <dl class="mt-3 space-y-1.5 text-xs">
              <div class="flex items-center justify-between gap-3">
                <dt class="text-gray-400 dark:text-dark-500">{{ t('admin.presentationPricing.table.displayPrice') }}</dt>
                <dd class="font-mono text-gray-900 dark:text-gray-100">
                  <template v-if="!m.display">{{ t('modelPlaza.table.priceNotPublished') }}</template>
                  <template v-else-if="m.display.mode !== 'token'">{{ formatPerRequest(m.display.perRequestPrice) }} $/{{ m.display.mode === 'image' ? t('admin.presentationPricing.perImage') : t('admin.presentationPricing.perRequest') }}</template>
                  <template v-else>{{ formatMTok(m.display.inputPrice) }} / {{ formatMTok(m.display.outputPrice) }} $/1M</template>
                </dd>
              </div>
              <div class="flex items-center justify-between gap-3">
                <dt class="text-gray-400 dark:text-dark-500">{{ t('admin.presentationPricing.table.officialPrice') }}</dt>
                <dd class="font-mono text-gray-500 dark:text-dark-400">
                  {{ m.official ? `${formatMTok(m.official.inputPrice)} / ${formatMTok(m.official.outputPrice)} $/1M` : '-' }}
                </dd>
              </div>
            </dl>
            <button class="btn btn-secondary mt-3 w-full" @click="openEditor(m)">
              {{ t('admin.presentationPricing.edit') }}
            </button>
          </div>
        </div>
      </template>
      </template>
    </TablePageLayout>

    <!-- 编辑弹窗:用户展示(可编辑) 与 参考信息(只读) 视觉分区,严禁混排 -->
    <BaseDialog :show="editor != null" :title="t('admin.presentationPricing.edit')" width="wide" @close="closeEditor">
      <div v-if="editor" class="space-y-5 p-1">
        <div>
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ editor.modelName }}</h3>
          <p class="mt-0.5 text-xs text-gray-400 dark:text-dark-500">{{ platformLabel(editor.platform) }}</p>
        </div>

        <!-- 【用户展示】独立区域:唯一可编辑区 -->
        <section class="rounded-xl border border-primary-200/70 bg-primary-50/40 p-4 dark:border-primary-500/25 dark:bg-primary-500/5">
          <header class="flex items-center justify-between gap-2">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.presentationPricing.editor.displaySection') }}
            </h4>
            <label class="flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
              <input v-model="editor.form.enabled" type="checkbox" class="h-3.5 w-3.5 accent-primary-600" />
              {{ t('admin.presentationPricing.editor.enableOverride') }}
            </label>
          </header>
          <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
            {{ t('admin.presentationPricing.editor.displaySectionHint') }}
          </p>

          <div class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <template v-if="editor.form.billingMode === 'token'">
              <label class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.presentationPricing.editor.inputPrice') }} ($/1M)
                <input
                  v-model="editor.form.inputPrice"
                  type="number"
                  min="0"
                  step="any"
                  class="input mt-1 font-mono"
                />
              </label>
              <label class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.presentationPricing.editor.outputPrice') }} ($/1M)
                <input
                  v-model="editor.form.outputPrice"
                  type="number"
                  min="0"
                  step="any"
                  class="input mt-1 font-mono"
                />
              </label>
              <label class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.presentationPricing.editor.cacheWritePrice') }} ($/1M)
                <input
                  v-model="editor.form.cacheWritePrice"
                  type="number"
                  min="0"
                  step="any"
                  class="input mt-1 font-mono"
                />
              </label>
              <label class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.presentationPricing.editor.cacheWrite1hPrice') }} ($/1M)
                <input
                  v-model="editor.form.cacheWrite1hPrice"
                  type="number"
                  min="0"
                  step="any"
                  class="input mt-1 font-mono"
                />
              </label>
              <label class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.presentationPricing.editor.cacheReadPrice') }} ($/1M)
                <input
                  v-model="editor.form.cacheReadPrice"
                  type="number"
                  min="0"
                  step="any"
                  class="input mt-1 font-mono"
                />
              </label>
            </template>
            <template v-else>
              <label class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.presentationPricing.editor.perRequestPrice') }} ($)
                <input
                  v-model="editor.form.perRequestPrice"
                  type="number"
                  min="0"
                  step="any"
                  class="input mt-1 font-mono"
                />
              </label>
            </template>
            <label class="text-xs text-gray-500 dark:text-dark-400 sm:col-span-2">
              {{ t('admin.presentationPricing.editor.remark') }}
              <input v-model="editor.form.remark" type="text" class="input mt-1" />
            </label>
          </div>
          <p
            v-if="editor.form.enabled"
            class="mt-3 flex items-center gap-1.5 text-xs font-medium text-amber-600 dark:text-amber-400"
          >
            <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5" />
            {{ t('admin.presentationPricing.editor.saveHint') }}
          </p>
        </section>

        <!-- 【参考信息】只读区:官方价 + 实际计费去向 -->
        <section class="rounded-xl border border-gray-100 bg-gray-50/60 p-4 dark:border-dark-700/60 dark:bg-dark-800/40">
          <h4 class="text-sm font-semibold text-gray-500 dark:text-dark-300">
            {{ t('admin.presentationPricing.editor.referenceSection') }}
          </h4>
          <dl class="mt-2 space-y-1.5 text-xs">
            <div class="flex items-center justify-between gap-3">
              <dt class="text-gray-400 dark:text-dark-500">{{ t('admin.presentationPricing.table.officialPrice') }}</dt>
              <dd class="font-mono text-gray-600 dark:text-dark-300">
                {{ editor.official ? `${formatMTok(editor.official.inputPrice)} / ${formatMTok(editor.official.outputPrice)} $/1M` : '-' }}
              </dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-gray-400 dark:text-dark-500">{{ t('admin.presentationPricing.editor.resolvedDisplay') }}</dt>
              <dd class="font-mono text-gray-600 dark:text-dark-300">
                <template v-if="!editor.resolved">{{ t('modelPlaza.table.priceNotPublished') }}</template>
                <template v-else-if="editor.resolved.mode !== 'token'">{{ formatPerRequest(editor.resolved.perRequestPrice) }} $/{{ editor.resolved.mode === 'image' ? t('admin.presentationPricing.perImage') : t('admin.presentationPricing.perRequest') }}</template>
                <template v-else>{{ formatMTok(editor.resolved.inputPrice) }} / {{ formatMTok(editor.resolved.outputPrice) }} $/1M</template>
                <span class="ml-1 font-sans text-gray-400">({{ t(`admin.presentationPricing.source.${editor.source}`) }})</span>
              </dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-gray-400 dark:text-dark-500">{{ t('admin.presentationPricing.table.billingNote') }}</dt>
              <dd class="text-gray-600 dark:text-dark-300">{{ t('admin.presentationPricing.billingManagedByBilling') }}</dd>
            </div>
          </dl>
        </section>
      </div>

      <template #footer>
        <div class="flex flex-wrap items-center justify-between gap-2">
          <button
            class="btn btn-secondary"
            :disabled="saving"
            @click="clearOverride"
          >
            {{ t('admin.presentationPricing.editor.restoreDefault') }}
          </button>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary" :disabled="saving" @click="closeEditor">
              {{ t('common.cancel') }}
            </button>
            <button class="btn btn-primary" :disabled="saving" @click="saveOverride">
              <Icon v-if="saving" name="refresh" size="sm" class="mr-1.5 animate-spin" />
              {{ t('admin.presentationPricing.editor.save') }}
            </button>
          </div>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { getModelPlaza, type ModelPlazaResponse, type PlazaModel } from '@/api/modelPlaza'
import {
  presentationPricingAPI,
  type PresentationPricingItem
} from '@/api/presentationPricing'
import { platformLabel } from '@/utils/platformColors'
import { mTokToPerToken, perTokenToMTok } from '@/components/admin/channel/types'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const toast = {
  success: (m: string) => appStore.showSuccess(m),
  error: (m: string) => appStore.showError(m)
}

interface AdminPlazaModel {
  name: string
  platform: string
  source: string
  display: { inputPrice: number | null; outputPrice: number | null; perRequestPrice: number | null; mode: string } | null
  official: { inputPrice: number | null; outputPrice: number | null } | null
  billingMode: string
}

const models = ref<AdminPlazaModel[]>([])
const overrides = ref<PresentationPricingItem[]>([])
const loading = ref(true)
const loadError = ref(false)
const plazaUnavailable = ref(false)
const search = ref('')

const overrideByModel = computed(() => {
  const map = new Map<string, PresentationPricingItem>()
  for (const o of overrides.value) map.set(o.model_name, o)
  return map
})

const filteredModels = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return models.value
  return models.value.filter(
    (m) => m.name.toLowerCase().includes(q) || m.platform.toLowerCase().includes(q)
  )
})

/** 统一价格格式:保底 2 位小数;有效精度超过 2 位时原样保留(避免 $0.003 被抹成 0)。 */
function formatPrice(value: number): string {
  const fixed = value.toFixed(2)
  const precise = parseFloat(value.toPrecision(6))
  return Number(fixed) === precise ? fixed : String(precise)
}

/** 按次/按图标准价原值展示(不做 1M 换算)。 */
function formatPerRequest(value: number | null | undefined): string {
  if (value == null) return '-'
  return `$${formatPrice(value)}`
}

/** 参考价统一 $/1M 展示(USD per token → $/MTok)。 */
function formatMTok(perToken: number | null | undefined): string {
  if (perToken == null) return '-'
  const v = perTokenToMTok(perToken)
  if (v == null) return '-'
  return `$${formatPrice(v)}`
}

async function reload() {
  loading.value = true
  loadError.value = false
  plazaUnavailable.value = false
  try {
    const [plaza, overrideList] = await Promise.allSettled([
      getModelPlaza(),
      presentationPricingAPI.list()
    ])
    if (plaza.status === 'rejected') {
      // 404 = 模型广场未启用;其余按加载失败处理
      plazaUnavailable.value = true
      models.value = []
    } else {
      models.value = flattenPlazaModels(plaza.value)
    }
    if (overrideList.status === 'fulfilled') {
      overrides.value = overrideList.value
    } else {
      overrides.value = []
    }
    if (plaza.status === 'fulfilled' && overrideList.status === 'rejected') {
      loadError.value = true
    }
  } finally {
    loading.value = false
  }
}

/** 广场响应按 (name, platform) 去重平铺;展示价/来源/官方价全部来自同一响应。 */
function flattenPlazaModels(resp: ModelPlazaResponse): AdminPlazaModel[] {
  const seen = new Set<string>()
  const out: AdminPlazaModel[] = []
  for (const group of resp.groups) {
    for (const m of group.models) {
      const key = `${m.platform}:${m.name}`
      if (seen.has(key)) continue
      seen.add(key)
      out.push(toAdminModel(m))
    }
  }
  out.sort((a, b) => a.name.localeCompare(b.name) || a.platform.localeCompare(b.platform))
  return out
}

function toAdminModel(m: PlazaModel): AdminPlazaModel {
  const billingMode = m.display_pricing?.billing_mode ?? m.pricing?.billing_mode ?? 'token'
  return {
    name: m.name,
    platform: m.platform,
    source: m.presentation_source ?? 'none',
    billingMode,
    display:
      m.display_pricing != null
        ? {
            inputPrice: m.display_pricing.input_price,
            outputPrice: m.display_pricing.output_price,
            perRequestPrice: m.display_pricing.per_request_price,
            mode: m.display_pricing.billing_mode ?? 'token'
          }
        : null,
    official:
      m.official_pricing != null
        ? {
            inputPrice: m.official_pricing.input_price,
            outputPrice: m.official_pricing.output_price
          }
        : null
  }
}

// ---- 编辑器 ----

interface EditorState {
  modelName: string
  platform: string
  source: string
  resolved: { inputPrice: number | null; outputPrice: number | null; perRequestPrice: number | null; mode: string } | null
  official: { inputPrice: number | null; outputPrice: number | null } | null
  form: {
    billingMode: string
    enabled: boolean
    inputPrice: string
    outputPrice: string
    cacheWritePrice: string
    cacheWrite1hPrice: string
    cacheReadPrice: string
    perRequestPrice: string
    remark: string
  }
}

const editor = ref<EditorState | null>(null)
const saving = ref(false)

function priceToForm(perToken: number | null | undefined, scale1M: boolean): string {
  if (perToken == null) return ''
  const v = scale1M ? perTokenToMTok(perToken) : perToken
  return v == null ? '' : String(v)
}

function openEditor(m: AdminPlazaModel) {
  const o = overrideByModel.value.get(m.name)
  const isToken = m.billingMode === 'token'
  editor.value = {
    modelName: m.name,
    platform: m.platform,
    source: m.source,
    resolved: m.display,
    official: m.official,
    form: {
      billingMode: m.billingMode,
      enabled: o ? o.enabled : false,
      inputPrice: priceToForm(o?.input_price ?? null, isToken),
      outputPrice: priceToForm(o?.output_price ?? null, isToken),
      cacheWritePrice: priceToForm(o?.cache_write_price ?? null, isToken),
      cacheWrite1hPrice: priceToForm(o?.cache_write_1h_price ?? null, isToken),
      cacheReadPrice: priceToForm(o?.cache_read_price ?? null, isToken),
      perRequestPrice: priceToForm(o?.per_request_price ?? null, false),
      remark: o?.remark ?? ''
    }
  }
}

function closeEditor() {
  if (saving.value) return
  editor.value = null
}

function parseFormPrice(raw: string, scale1M: boolean): number | null {
  const trimmed = raw.trim()
  if (trimmed === '') return null
  const n = Number(trimmed)
  if (!Number.isFinite(n) || n < 0) return null
  return scale1M ? mTokToPerToken(n) : n
}

async function saveOverride() {
  if (!editor.value || saving.value) return
  const isToken = editor.value.form.billingMode === 'token'
  const inputPrice = parseFormPrice(editor.value.form.inputPrice, isToken)
  const outputPrice = parseFormPrice(editor.value.form.outputPrice, isToken)
  const perRequestPrice = parseFormPrice(editor.value.form.perRequestPrice, false)
  if (isToken && inputPrice == null && outputPrice == null) {
    toast.error(t('admin.presentationPricing.editor.priceRequired'))
    return
  }
  if (!isToken && perRequestPrice == null) {
    toast.error(t('admin.presentationPricing.editor.priceRequired'))
    return
  }
  saving.value = true
  try {
    await presentationPricingAPI.upsert({
      model_name: editor.value.modelName,
      billing_mode: editor.value.form.billingMode,
      enabled: editor.value.form.enabled,
      input_price: isToken ? inputPrice : null,
      output_price: isToken ? outputPrice : null,
      cache_write_price: isToken ? parseFormPrice(editor.value.form.cacheWritePrice, true) : null,
      cache_write_1h_price: isToken
        ? parseFormPrice(editor.value.form.cacheWrite1hPrice, true)
        : null,
      cache_read_price: isToken ? parseFormPrice(editor.value.form.cacheReadPrice, true) : null,
      per_request_price: isToken ? null : perRequestPrice,
      remark: editor.value.form.remark
    })
    toast.success(t('admin.presentationPricing.saved'))
    editor.value = null
    await reload()
  } catch {
    toast.error(t('admin.presentationPricing.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function clearOverride() {
  if (!editor.value || saving.value) return
  saving.value = true
  try {
    await presentationPricingAPI.remove(editor.value.modelName)
    toast.success(t('admin.presentationPricing.cleared'))
    editor.value = null
    await reload()
  } catch {
    toast.error(t('admin.presentationPricing.saveFailed'))
  } finally {
    saving.value = false
  }
}

function sourceBadgeClass(source: string): string {
  switch (source) {
    case 'manual':
      return 'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'
    case 'official':
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700/70 dark:text-dark-300'
    case 'billing':
      return 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300'
    default:
      return 'bg-gray-100 text-gray-400 dark:bg-dark-700/50 dark:text-dark-500'
  }
}

onMounted(reload)
</script>
