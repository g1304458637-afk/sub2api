<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-4">
      <!-- 两个 Tab：直接重置 ≠ 重置卡，绝对不能混 -->
      <div class="card p-1">
        <div class="flex gap-1" role="tablist">
          <button
            v-for="tab in ['direct', 'cards'] as const"
            :key="tab"
            role="tab"
            :aria-selected="activeTab === tab"
            class="flex-1 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors"
            :class="activeTab === tab
              ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
              : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700'"
            @click="activeTab = tab"
          >
            {{ tab === 'direct' ? t('resetCenter.directTab') : t('resetCenter.cardsTab') }}
          </button>
        </div>
      </div>

      <!-- ============ Tab 1: 直接重置 ============ -->
      <template v-if="activeTab === 'direct'">
        <div class="card space-y-4 p-5">
          <div>
            <h3 class="text-sm font-semibold">{{ t('resetCenter.directTitle') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('resetCenter.directDesc') }}</p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <label class="block text-sm">
              <span class="input-label">{{ t('resetCenter.targetMode') }}</span>
              <Select v-model="directForm.target_mode" :options="directModeOptions" class="mt-1 w-full" />
            </label>
            <label class="block text-sm">
              <span class="input-label">{{ t('resetCenter.reason') }}</span>
              <input v-model="directForm.reason" class="input mt-1 w-full" :placeholder="t('resetCenter.reasonPlaceholder')" />
            </label>
          </div>

          <label v-if="needsIds(directForm.target_mode)" class="block text-sm">
            <span class="input-label">{{ directForm.target_mode === 'users' ? t('resetCenter.userIds') : t('resetCenter.subIds') }}</span>
            <input v-model="directForm.ids" class="input mt-1 w-full" placeholder="1, 2, 3" />
          </label>

          <div class="flex flex-wrap gap-2">
            <button class="btn btn-secondary" :disabled="previewing" @click="previewDirect">
              {{ previewing ? t('common.processing') : t('resetCenter.preview') }}
            </button>
            <button class="btn btn-primary" :disabled="!previewSummary || executing" @click="executeDirect">
              {{ executing ? t('common.processing') : t('resetCenter.execute') }}
            </button>
          </div>

          <!-- Preview 结果 -->
          <div v-if="previewSummary" class="rounded-xl border border-gray-200 p-4 text-sm dark:border-dark-700">
            <p class="font-medium">{{ t('resetCenter.previewResult') }}</p>
            <div class="mt-2 grid grid-cols-2 gap-2 text-xs sm:grid-cols-3">
              <div><span class="text-gray-500">{{ t('resetCenter.totalTargeted') }}</span> {{ previewSummary.subscription_count }}</div>
              <div><span class="text-gray-500">{{ t('resetCenter.uniqueUsers') }}</span> {{ previewSummary.unique_user_count }}</div>
              <div v-if="previewSummary.group_breakdown?.length">
                <span class="text-gray-500">{{ t('resetCenter.breakdown') }}</span>
                {{ previewSummary.group_breakdown.map((g) => `${g.name}×${g.subscription_count}`).join('、') }}
              </div>
            </div>
            <ul v-if="previewSummary.sample?.length" class="mt-2 space-y-0.5 text-xs text-gray-500 dark:text-dark-400">
              <li v-for="s in previewSummary.sample.slice(0, 5)" :key="s.subscription_id">
                #{{ s.subscription_id }} · {{ s.display_name }}
              </li>
            </ul>
            <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('resetCenter.directConfirmNote') }}</p>
          </div>

          <!-- 批量进度 -->
          <div v-if="activeEvent" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
            <div class="mb-1 flex items-center justify-between text-sm">
              <span class="font-medium">#{{ activeEvent.id }} {{ t('resetCenter.progress') }}</span>
              <span>{{ activeEvent.applied_count }} / {{ activeEvent.total_targeted }} · {{ activeEvent.status }}</span>
            </div>
            <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
              <div
                class="h-2 rounded-full bg-gray-900 transition-all dark:bg-white"
                :style="{ width: `${progressPct}%` }"
              ></div>
            </div>
            <div class="mt-2 flex items-center justify-between text-xs">
              <span class="text-gray-500">
                {{ t('resetCenter.applied') }} {{ activeEvent.applied_count }} · {{ t('resetCenter.skipped') }} {{ activeEvent.skipped_count }}
                · <span :class="activeEvent.failed_count > 0 ? 'text-red-500' : ''">{{ t('resetCenter.failed') }} {{ activeEvent.failed_count }}</span>
              </span>
              <button
                v-if="activeEvent.failed_count > 0 && activeEvent.status !== 'running'"
                class="text-xs underline decoration-dotted"
                @click="retryEvent(activeEvent.id)"
              >
                {{ t('resetCenter.retryFailed') }}
              </button>
            </div>
          </div>
        </div>

        <!-- 最近事件列表 -->
        <div class="card p-5">
          <h3 class="mb-3 text-sm font-semibold">{{ t('resetCenter.recentEvents') }}</h3>
          <p v-if="recentEvents.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('resetCenter.noEvents') }}</p>
          <ul v-else class="space-y-2">
            <li v-for="ev in recentEvents" :key="ev.id" class="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-gray-100 px-3 py-2 text-sm dark:border-dark-700">
              <span>#{{ ev.id }} · {{ t('resetCenter.mode.' + ev.target_mode, ev.target_mode) }} <span class="text-xs text-gray-400">{{ ev.reason }}</span></span>
              <span class="text-xs text-gray-500">
                {{ ev.applied_count }}/{{ ev.total_targeted }}
                <span v-if="ev.failed_count > 0" class="text-red-500">· {{ t('resetCenter.failed') }} {{ ev.failed_count }}</span>
                · {{ fmtTime(ev.created_at) }}
              </span>
            </li>
          </ul>
        </div>
      </template>

      <!-- ============ Tab 2: 重置卡 ============ -->
      <template v-else>
        <div class="card space-y-4 p-5">
          <div>
            <h3 class="text-sm font-semibold">{{ t('resetCenter.cardsTitle') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('resetCenter.cardsDesc') }}</p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <label class="block text-sm">
              <span class="input-label">{{ t('resetCenter.targetMode') }}</span>
              <Select v-model="cardForm.target_mode" :options="cardModeOptions" class="mt-1 w-full" />
            </label>
            <label class="block text-sm">
              <span class="input-label">{{ t('resetCenter.perUser') }}</span>
              <input v-model.number="cardForm.quantity_per_user" type="number" min="1" class="input mt-1 w-full" />
            </label>
            <label class="block text-sm">
              <span class="input-label">{{ t('resetCenter.expiresAt') }}</span>
              <input v-model="cardForm.expires_at" type="date" class="input mt-1 w-full" />
            </label>
            <label class="block text-sm">
              <span class="input-label">{{ t('resetCenter.campaign') }}</span>
              <input v-model="cardForm.campaign" class="input mt-1 w-full" :placeholder="t('resetCenter.campaignPlaceholder')" />
            </label>
          </div>

          <label v-if="needsIds(cardForm.target_mode)" class="block text-sm">
            <span class="input-label">{{ cardForm.target_mode === 'users' ? t('resetCenter.userIds') : t('resetCenter.groupIds') }}</span>
            <input v-model="cardForm.ids" class="input mt-1 w-full" placeholder="1, 2, 3" />
          </label>

          <div class="flex flex-wrap gap-2">
            <button class="btn btn-secondary" :disabled="previewing" @click="previewCards">
              {{ previewing ? t('common.processing') : t('resetCenter.preview') }}
            </button>
            <button class="btn btn-primary" :disabled="!cardPreview || grantingCards" @click="grantCards">
              {{ grantingCards ? t('common.processing') : t('resetCenter.grant') }}
            </button>
          </div>

          <div v-if="cardPreview" class="rounded-xl border border-gray-200 p-4 text-sm dark:border-dark-700">
            <p class="font-medium">{{ t('resetCenter.previewResult') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
              {{ t('resetCenter.cardPreviewLine', { users: cardPreview.unique_users, cards: cardPreview.total_cards }) }}
            </p>
            <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('resetCenter.cardsNote') }}</p>
          </div>
        </div>

        <!-- 卡记录 -->
        <div class="card p-5">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 class="text-sm font-semibold">{{ t('resetCenter.cardRecords') }}</h3>
            <div class="flex items-center gap-2">
              <input
                v-model.number="cardQueryUserId"
                type="number"
                min="1"
                :placeholder="t('resetCenter.userId')"
                class="input w-32"
              />
              <Select v-model="cardStatusFilter" :options="cardStatusOptions" class="w-40" @change="loadCards" />
            </div>
          </div>
          <p v-if="cardRows.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('resetCenter.noCards') }}</p>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b border-gray-100 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
                  <th class="py-2 pr-4">#</th>
                  <th class="py-2 pr-4">{{ t('resetCenter.userId') }}</th>
                  <th class="py-2 pr-4">{{ t('resetCenter.cardStatus') }}</th>
                  <th class="py-2 pr-4">{{ t('resetCenter.expiresAt') }}</th>
                  <th class="py-2 pr-4">{{ t('resetCenter.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="card in cardRows" :key="card.id" class="border-b border-gray-50 dark:border-dark-700/50">
                  <td class="py-2 pr-4 font-mono text-xs">{{ card.id }}</td>
                  <td class="py-2 pr-4">{{ card.user_id }}</td>
                  <td class="py-2 pr-4">
                    <span
                      class="rounded-full border px-2 py-0.5 text-[11px]"
                      :class="card.status === 'available'
                        ? 'border-emerald-200 text-emerald-600'
                        : card.status === 'revoked'
                          ? 'border-red-200 text-red-500'
                          : 'border-gray-200 text-gray-400 dark:border-dark-600 dark:text-dark-400'"
                    >
                      {{ card.status }}
                    </span>
                  </td>
                  <td class="py-2 pr-4 text-xs">{{ fmtDate(card.expires_at) }}</td>
                  <td class="py-2 pr-4">
                    <button
                      v-if="card.status === 'available'"
                      class="text-xs text-red-500 underline decoration-dotted"
                      @click="revokeCard(card.id)"
                    >
                      {{ t('resetCenter.revoke') }}
                    </button>
                    <span v-else class="text-xs text-gray-300 dark:text-dark-600">—</span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import { adminAPI } from '@/api/admin'
import type { ResetEventSummary, ResetTargetPreview } from '@/api/admin/subscriptionReset'
import { useAppStore } from '@/stores'

/**
 * Admin Reset Center：一个页面两个 Tab。
 * [直接重置] 立即重置目标订阅当前周期（Preview → Execute → 进度 → 重试失败项）；
 * [重置卡] 发放后不立即重置，用户自行决定使用时机（Preview → 发放 → 记录/撤销 Available）。
 * 两者语义严格分离，绝不混用。
 */
const { t } = useI18n()
const appStore = useAppStore()

const activeTab = ref<'direct' | 'cards'>('direct')
const previewing = ref(false)
const executing = ref(false)
const grantingCards = ref(false)

const directForm = ref({ target_mode: 'users' as 'users' | 'groups' | 'all_active', ids: '', reason: '' })
const cardForm = ref({
  target_mode: 'users' as 'users' | 'groups' | 'all_active_users',
  ids: '',
  quantity_per_user: 1,
  expires_at: '',
  campaign: ''
})

const previewSummary = ref<ResetTargetPreview | null>(null)
const activeEvent = ref<ResetEventSummary | null>(null)
const recentEvents = ref<ResetEventSummary[]>([])
const cardPreview = ref<{ unique_users: number; total_cards: number } | null>(null)
const cardRows = ref<Array<{ id: number; user_id: number; status: string; expires_at?: string }>>([])
const cardStatusFilter = ref('')
const cardQueryUserId = ref<number>()

const directModeOptions = computed(() => [
  { value: 'users', label: t('resetCenter.m.users') },
  { value: 'groups', label: t('resetCenter.m.groups') },
  { value: 'all_active', label: t('resetCenter.m.all_active') }
])

const cardModeOptions = computed(() => [
  { value: 'users', label: t('resetCenter.m.users') },
  { value: 'groups', label: t('resetCenter.m.groups') },
  { value: 'all_active_users', label: t('resetCenter.m.all_active_users') }
])

const cardStatusOptions = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'available', label: 'available' },
  { value: 'used', label: 'used' },
  { value: 'expired', label: 'expired' },
  { value: 'revoked', label: 'revoked' }
])

const progressPct = computed(() => {
  const ev = activeEvent.value
  if (!ev || ev.total_targeted <= 0) return 0
  return Math.round((((ev.applied_count ?? 0) + (ev.skipped_count ?? 0) + (ev.failed_count ?? 0)) / ev.total_targeted) * 100)
})

function needsIds(mode: string): boolean {
  return mode === 'users' || mode === 'groups'
}

function parseIds(raw: string): number[] {
  return raw
    .split(/[,，\s]+/)
    .map((s) => Number.parseInt(s.trim(), 10))
    .filter((n) => Number.isFinite(n) && n > 0)
}

function targetPayload(form: { target_mode: 'users' | 'groups' | 'all_active'; ids: string }) {
  const payload: {
    target_mode: import('@/api/admin/subscriptionReset').ResetTargetMode
    user_ids?: number[]
    group_ids?: number[]
  } = {
    target_mode: form.target_mode
  }
  const ids = parseIds(form.ids)
  if (form.target_mode === 'users') payload.user_ids = ids
  if (form.target_mode === 'groups') payload.group_ids = ids
  return payload
}

function extractMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = String((err as { message?: unknown }).message ?? '')
    if (m && m !== 'Unknown error') return m
  }
  return fallback
}

// ── 直接重置 ──
async function previewDirect() {
  previewing.value = true
  previewSummary.value = null
  try {
    const { data } = await adminAPI.resetEvents.preview(targetPayload(directForm.value))
    previewSummary.value = data
  } catch (err) {
    appStore.showError(extractMessage(err, t('resetCenter.previewFailed')))
  } finally {
    previewing.value = false
  }
}

async function executeDirect() {
  executing.value = true
  try {
    const { data } = await adminAPI.resetEvents.create({
      ...targetPayload(directForm.value),
      reason: directForm.value.reason || undefined,
      idempotency_key: `rc-${Date.now()}-${Math.random().toString(36).slice(2)}`
    })
    previewSummary.value = null
    activeEvent.value = data
    appStore.showSuccess(t('resetCenter.executed'))
    await loadEvents()
    pollEvent(data.id)
  } catch (err) {
    appStore.showError(extractMessage(err, t('resetCenter.executeFailed')))
  } finally {
    executing.value = false
  }
}

let pollTimer = 0
function pollEvent(id: number) {
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = window.setInterval(async () => {
    try {
      const { data } = await adminAPI.resetEvents.get(id)
      activeEvent.value = data
      if (data.status !== 'running' && data.status !== 'pending') {
        clearInterval(pollTimer)
        pollTimer = 0
        await loadEvents()
      }
    } catch {
      clearInterval(pollTimer)
      pollTimer = 0
    }
  }, 1500)
}

async function retryEvent(id: number) {
  try {
    const { data } = await adminAPI.resetEvents.retry(id)
    activeEvent.value = data
    pollEvent(id)
  } catch (err) {
    appStore.showError(extractMessage(err, t('resetCenter.retryFailedError')))
  }
}

async function loadEvents() {
  try {
    const res = await adminAPI.resetEvents.list({ page: 1, page_size: 8 })
    const payload = res.data as { items?: ResetEventSummary[] } | ResetEventSummary[]
    recentEvents.value = Array.isArray(payload) ? payload : (payload.items ?? [])
  } catch { /* 列表失败不阻塞 */ }
}

// ── 重置卡 ──
async function previewCards() {
  previewing.value = true
  cardPreview.value = null
  try {
    const { data } = await adminAPI.resetCards.grantPreview(cardPayload())
    cardPreview.value = data
  } catch (err) {
    appStore.showError(extractMessage(err, t('resetCenter.previewFailed')))
  } finally {
    previewing.value = false
  }
}

function cardPayload(): { target_mode: import('@/api/admin/subscriptionReset').CardTargetMode; quantity_per_user: number; user_ids?: number[]; group_ids?: number[]; campaign?: string; expires_at?: string } {
  const payload: { target_mode: import('@/api/admin/subscriptionReset').CardTargetMode; quantity_per_user: number; user_ids?: number[]; group_ids?: number[]; campaign?: string; expires_at?: string } = {
    target_mode: cardForm.value.target_mode,
    quantity_per_user: Math.max(cardForm.value.quantity_per_user || 1, 1),
    campaign: cardForm.value.campaign || undefined,
    expires_at: cardForm.value.expires_at ? new Date(cardForm.value.expires_at).toISOString() : undefined
  }
  const ids = parseIds(cardForm.value.ids)
  if (cardForm.value.target_mode === 'users') payload.user_ids = ids
  if (cardForm.value.target_mode === 'groups') payload.group_ids = ids
  return payload
}

async function grantCards() {
  grantingCards.value = true
  try {
    const { data } = await adminAPI.resetCards.grant({
      ...cardPayload(),
      idempotency_key: `card-${Date.now()}-${Math.random().toString(36).slice(2)}`
    })
    appStore.showSuccess(t('resetCenter.granted', { count: data.TotalCards }))
    cardPreview.value = null
    await loadCards()
  } catch (err) {
    appStore.showError(extractMessage(err, t('resetCenter.grantFailed')))
  } finally {
    grantingCards.value = false
  }
}

async function loadCards() {
  if (!cardQueryUserId.value) {
    cardRows.value = []
    return
  }
  try {
    const res = await adminAPI.resetCards.list({
      user_id: cardQueryUserId.value,
      page: 1,
      page_size: 30,
      status: cardStatusFilter.value || undefined
    })
    const payload = res.data as { items?: Array<{ id: number; user_id: number; status: string; expires_at?: string }> }
    cardRows.value = payload.items ?? []
  } catch { /* 列表失败不阻塞 */ }
}

async function revokeCard(id: number) {
  try {
    await adminAPI.resetCards.revoke(id)
    appStore.showSuccess(t('resetCenter.revoked'))
    await loadCards()
  } catch (err) {
    appStore.showError(extractMessage(err, t('resetCenter.revokeFailed')))
  }
}

function fmtTime(iso?: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString()
  } catch {
    return iso
  }
}

function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleDateString()
  } catch {
    return iso
  }
}

onMounted(() => {
  loadEvents()
  loadCards()
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>
