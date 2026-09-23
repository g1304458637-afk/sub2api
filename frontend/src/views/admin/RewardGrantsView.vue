<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Filters -->
      <template #filters>
        <div class="card p-4 sm:p-6">
          <div class="flex flex-wrap items-end justify-between gap-4">
            <!-- Left: filter fields -->
            <div class="flex flex-1 flex-wrap items-end gap-4">
              <div class="w-full sm:w-auto sm:min-w-[180px]">
                <label class="input-label">{{ t('admin.rewardGrants.filters.userId') }}</label>
                <input
                  v-model.trim="filters.user_id"
                  type="text"
                  class="input"
                  :placeholder="t('admin.rewardGrants.filters.userIdPlaceholder')"
                  @keyup.enter="search"
                />
              </div>

              <div class="w-full sm:w-auto sm:min-w-[200px]">
                <label class="input-label">{{ t('admin.rewardGrants.filters.campaign') }}</label>
                <input
                  v-model.trim="filters.campaign"
                  type="text"
                  class="input"
                  :placeholder="t('admin.rewardGrants.filters.campaignPlaceholder')"
                  @keyup.enter="search"
                />
              </div>

              <div class="w-full sm:w-auto sm:min-w-[180px]">
                <label class="input-label">{{ t('admin.rewardGrants.filters.sourceType') }}</label>
                <Select v-model="filters.source_type" :options="sourceTypeOptions" @change="search" />
              </div>
            </div>

            <!-- Right: actions -->
            <div class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
              <button type="button" class="btn btn-primary" :disabled="loading" @click="search">
                {{ t('common.search') }}
              </button>
              <button type="button" class="btn btn-secondary" :disabled="loading" @click="resetFilters">
                {{ t('common.reset') }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <!-- Table -->
      <template #table>
        <p v-if="loadError" role="alert" class="p-4 text-red-500">{{ loadError }}</p>
        <DataTable v-else :columns="columns" :data="grants" :loading="loading" row-key="id">
          <template #cell-created_at="{ value }">
            <span class="whitespace-nowrap text-gray-600 dark:text-gray-300">{{ formatTime(value) }}</span>
          </template>

          <template #cell-user="{ row }">
            <div class="min-w-0 max-w-[220px]">
              <div class="truncate font-medium text-gray-900 dark:text-white" :title="row.email">
                {{ row.email || '—' }}
              </div>
              <div class="mt-0.5 truncate text-xs text-gray-400">
                {{ row.username || `#${row.user_id}` }}
              </div>
            </div>
          </template>

          <template #cell-amount="{ value }">
            <span
              class="whitespace-nowrap font-mono font-medium"
              :class="value >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'"
            >
              {{ formatAmount(value) }}
            </span>
          </template>

          <template #cell-source_type="{ value }">
            <span class="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-semibold text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
              {{ sourceTypeLabel(value) }}
            </span>
          </template>

          <template #cell-campaign="{ value }">
            <span class="whitespace-nowrap text-gray-600 dark:text-gray-300">{{ value || '—' }}</span>
          </template>

          <template #cell-granted_by_email="{ value }">
            <span class="whitespace-nowrap text-gray-600 dark:text-gray-300">
              {{ value || t('admin.rewardGrants.grantedBySystem') }}
            </span>
          </template>

          <template #cell-idempotency_key="{ value }">
            <div class="min-w-0 max-w-[240px]">
              <div class="truncate font-mono text-xs text-gray-500 dark:text-gray-400" :title="value">
                {{ value }}
              </div>
            </div>
          </template>

          <template #empty>
            <div class="flex flex-col items-center py-8">
              <div class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800">
                <Icon name="gift" size="xl" class="text-gray-400 dark:text-dark-500" />
              </div>
              <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
                {{ t('admin.rewardGrants.empty') }}
              </p>
              <p v-if="hasActiveFilters" class="mt-1 text-xs text-gray-400 dark:text-dark-500">
                {{ t('admin.rewardGrants.emptyHint') }}
              </p>
            </div>
          </template>
        </DataTable>
      </template>

      <!-- Pagination -->
      <template #pagination>
        <Pagination
          v-if="total > 0"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="onPageChange"
          @update:pageSize="onPageSizeChange"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { RewardGrantRecord } from '@/api/admin/rewardGrants'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const currencyStore = useCurrencyDisplayStore()

// ==================== List state ====================

const loading = ref(false)
const loadError = ref('')
const grants = ref<RewardGrantRecord[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(getPersistedPageSize())

const filters = reactive({
  user_id: '',
  campaign: '',
  source_type: ''
})

const sourceTypeOptions = computed(() => [
  { value: '', label: t('admin.rewardGrants.filters.sourceTypeAll') },
  { value: 'student_verification', label: t('admin.rewardGrants.sourceType.student_verification') }
])

const columns = computed<Column[]>(() => [
  { key: 'created_at', label: t('admin.rewardGrants.columns.time') },
  { key: 'user', label: t('admin.rewardGrants.columns.user') },
  { key: 'amount', label: t('admin.rewardGrants.columns.amount') },
  { key: 'source_type', label: t('admin.rewardGrants.columns.sourceType') },
  { key: 'campaign', label: t('admin.rewardGrants.columns.campaign') },
  { key: 'granted_by_email', label: t('admin.rewardGrants.columns.grantedBy') },
  { key: 'idempotency_key', label: t('admin.rewardGrants.columns.idempotencyKey') }
])

const hasActiveFilters = computed(
  () => filters.user_id !== '' || filters.campaign !== '' || filters.source_type !== ''
)

// ==================== Data loading ====================

function buildQuery() {
  const parsedUserId = Number(filters.user_id)
  return {
    page: page.value,
    page_size: pageSize.value,
    user_id: filters.user_id !== '' && Number.isInteger(parsedUserId) && parsedUserId > 0 ? parsedUserId : undefined,
    campaign: filters.campaign || undefined,
    source_type: filters.source_type || undefined
  }
}

let requestId = 0
onBeforeUnmount(() => { ++requestId })

async function fetchGrants() {
  const request = ++requestId
  loadError.value = ''
  loading.value = true
  try {
    const res = await adminAPI.rewardGrants.list(buildQuery())
    if (request !== requestId) return
    grants.value = res.items
    total.value = res.total
  } catch (err: any) {
    if (request !== requestId) return
    grants.value = []
    total.value = 0
    loadError.value = err?.message || t('admin.rewardGrants.loadFailed')
    appStore.showError(loadError.value)
  } finally {
    if (request === requestId) loading.value = false
  }
}

function search() {
  page.value = 1
  fetchGrants()
}

function resetFilters() {
  filters.user_id = ''
  filters.campaign = ''
  filters.source_type = ''
  search()
}

function onPageChange(p: number) {
  page.value = p
  fetchGrants()
}

function onPageSizeChange(ps: number) {
  pageSize.value = ps
  page.value = 1
  fetchGrants()
}

// ==================== Helpers ====================

function formatTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

// 金额为 USD 小数（非分）；正数带 + 前缀，与余额历史弹窗口径一致
function formatAmount(value: number): string {
  const sign = value > 0 ? '+' : ''
  return `${sign}${currencyStore.formatCNY(value)}`
}

function sourceTypeLabel(value: string): string {
  const found = sourceTypeOptions.value.find((o) => o.value === value)
  return found && found.value ? found.label : value
}

onMounted(fetchGrants)
</script>
