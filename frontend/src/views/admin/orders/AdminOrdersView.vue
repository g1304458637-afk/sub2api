<template>
  <AppLayout>
    <div class="space-y-4">
      <!-- Finance Center: 订单 / 套餐变更 / 钱包流水（Final Frontend C8） -->
      <div class="card p-1">
        <div class="flex gap-1" role="tablist">
          <button
            v-for="tab in ['orders', 'changes', 'ledger'] as const"
            :key="tab"
            role="tab"
            :aria-selected="financeTab === tab"
            class="flex-1 rounded-xl px-4 py-2.5 text-sm font-medium transition-colors"
            :class="financeTab === tab
              ? 'bg-gray-900 text-white dark:bg-white/10 dark:text-white'
              : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700'"
            @click="switchFinanceTab(tab)"
          >
            {{ t('payment.admin.financeTab.' + tab) }}
          </button>
        </div>
      </div>

      <template v-if="financeTab === 'orders'">
      <!-- Filters -->
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input v-model="orderSearch" type="text" :placeholder="t('payment.admin.searchOrders')" class="input" @input="debounceLoadOrders" />
          </div>
          <Select v-model="orderFilters.status" :options="statusFilterOptions" class="w-36" @change="loadOrders" />
          <Select v-model="orderFilters.payment_type" :options="paymentTypeFilterOptions" class="w-40" @change="loadOrders" />
          <Select v-model="orderFilters.order_type" :options="orderTypeFilterOptions" class="w-36" @change="loadOrders" />
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadOrders" :disabled="ordersLoading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="ordersLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </div>

      <!-- Table -->
      <OrderTable :orders="orders" :loading="ordersLoading" show-user>
        <template #actions="{ row }">
          <div class="flex items-center gap-1">
            <button @click="showOrderDetail(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-600">
              <Icon name="eye" size="sm" />
              {{ t('common.view') }}
            </button>
            <button v-if="row.status === 'PENDING'" @click="handleCancelOrder(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-yellow-600 hover:bg-yellow-50 dark:text-yellow-400 dark:hover:bg-yellow-900/20">
              <Icon name="x" size="sm" />
              {{ t('payment.orders.cancel') }}
            </button>
            <button v-if="row.status === 'FAILED'" @click="handleRetryOrder(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20">
              <Icon name="refresh" size="sm" />
              {{ t('payment.admin.retry') }}
            </button>
            <template v-if="row.status === 'REFUND_REQUESTED'">
              <span v-if="row.refund_amount" class="rounded-full bg-purple-100 px-1.5 py-0.5 text-xs font-medium text-purple-700 dark:bg-purple-900/30 dark:text-purple-300">{{ creditedAmountSymbol }}{{ row.refund_amount.toFixed(2) }}</span>
              <button @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50 dark:text-purple-400 dark:hover:bg-purple-900/20">
                <Icon name="check" size="sm" />
                {{ t('payment.admin.approveRefund') }}
              </button>
            </template>
            <button v-else-if="row.status === 'REFUND_FAILED'" @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-purple-600 hover:bg-purple-50 dark:text-purple-400 dark:hover:bg-purple-900/20">
              <Icon name="refresh" size="sm" />
              {{ t('payment.admin.retryRefund') }}
            </button>
            <button v-else-if="row.status === 'REFUND_PENDING'" :disabled="refundQueryingIds.has(row.id)" @click="handleQueryRefund(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-orange-600 hover:bg-orange-50 disabled:opacity-60 dark:text-orange-400 dark:hover:bg-orange-900/20">
              <Icon name="refresh" size="sm" :class="refundQueryingIds.has(row.id) ? 'animate-spin' : ''" />
              {{ t('payment.admin.queryRefundStatus') }}
            </button>
            <button v-else-if="row.status === 'COMPLETED' || row.status === 'PARTIALLY_REFUNDED'" @click="openRefundDialog(row)" class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20">
              <Icon name="dollar" size="sm" />
              {{ t('payment.admin.refund') }}
            </button>
          </div>
        </template>
      </OrderTable>
      <Pagination v-if="orderPagination.total > 0" :page="orderPagination.page" :total="orderPagination.total" :page-size="orderPagination.page_size" @update:page="handleOrderPageChange" @update:pageSize="handleOrderPageSizeChange" />
      </template>

      <!-- Tab 2: 套餐变更（admin /admin/plan-changes 全量审计：升级 + 预约降级） -->
      <template v-else-if="financeTab === 'changes'">
        <div class="card p-4">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <Select v-model="planChangeStatusFilter" :options="planChangeStatusOptions" class="w-44" @change="loadPlanChanges" />
            <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('payment.admin.financeChangesTotal', { total: planChangeTotal }) }}</span>
          </div>
          <p v-if="!changesLoading && planChangeRows.length === 0" class="text-sm text-gray-500 dark:text-dark-400">{{ t('payment.admin.financeChangesEmpty') }}</p>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead>
                <tr class="border-b border-gray-100 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
                  <th class="py-2 pr-4">ID</th>
                  <th class="py-2 pr-4">{{ t('payment.admin.pcUser') }}</th>
                  <th class="py-2 pr-4">{{ t('payment.admin.pcType') }}</th>
                  <th class="py-2 pr-4">{{ t('payment.admin.pcTier') }}</th>
                  <th class="py-2 pr-4">{{ t('payment.admin.pcAmount') }}</th>
                  <th class="py-2 pr-4">{{ t('payment.admin.pcStatus') }}</th>
                  <th class="py-2 pr-4">{{ t('payment.orders.createdAt') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in planChangeRows" :key="row.ID" class="border-b border-gray-50 dark:border-dark-700/50">
                  <td class="py-2 pr-4 font-mono text-xs">{{ row.ID }}</td>
                  <td class="py-2 pr-4">{{ row.UserID }}</td>
                  <td class="py-2 pr-4">{{ row.ChangeType === 'upgrade' ? t('payment.orders.planChangeUpgrade') : t('payment.orders.planChangeDowngrade') }}</td>
                  <td class="py-2 pr-4">{{ row.FromTier }} → {{ row.ToTier }}</td>
                  <td class="py-2 pr-4">{{ row.ChangeType === 'upgrade' ? `$${row.AmountDue.toFixed(2)}` : '—' }}</td>
                  <td class="py-2 pr-4">
                    <span
                      class="rounded-full border px-2 py-0.5 text-[11px]"
                      :class="row.Status === 'fulfilled'
                        ? 'border-emerald-200 text-emerald-600'
                        : row.Status === 'cancelled'
                          ? 'border-gray-200 text-gray-400 dark:border-dark-600 dark:text-dark-400'
                          : 'border-amber-200 text-amber-600'"
                    >
                      {{ t('payment.orders.planChangeStatus.' + row.Status, row.Status) }}
                    </span>
                  </td>
                  <td class="py-2 pr-4 text-xs text-gray-500">{{ fmtLedgerDate(row.CreatedAt) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>

      <!-- Tab 3: 钱包流水（全局 ledger 后端暂缺 — BLOCKED #1；先提供按用户查询） -->
      <template v-else>
        <div class="card p-4 space-y-3">
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('payment.admin.financeLedgerNote') }}</p>
          <div class="flex flex-wrap items-center gap-2">
            <input v-model.number="ledgerUserId" type="number" min="1" :placeholder="t('payment.admin.ledgerUserId')" class="input w-48" />
            <button class="btn btn-secondary" :disabled="!ledgerUserId || ledgerLoading" @click="loadLedger">
              {{ ledgerLoading ? t('common.processing') : t('common.view') }}
            </button>
          </div>
          <p v-if="ledgerError" class="text-sm text-red-500">{{ ledgerError }}</p>
          <ul v-if="ledgerRows.length" class="space-y-1.5 text-sm">
            <li v-for="(row, i) in ledgerRows" :key="i" class="flex items-center justify-between rounded-xl border border-gray-100 px-3 py-2 dark:border-dark-700">
              <span class="truncate font-mono text-xs">{{ row.code || '#' + (row.id ?? i) }}</span>
              <span class="text-xs text-gray-500">{{ fmtLedgerDate(row.created_at) }}</span>
            </li>
          </ul>
        </div>
      </template>
    </div>

    <!-- Order Detail Dialog -->
    <BaseDialog :show="showDetailDialog" :title="t('payment.admin.orderDetail')" width="wide" @close="showDetailDialog = false">
      <div v-if="selectedOrder" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderId') }}</p><p class="font-mono text-sm font-medium text-gray-900 dark:text-white">#{{ selectedOrder.id }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.orderNo') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">{{ selectedOrder.out_trade_no }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.status') }}</p><OrderStatusBadge :status="selectedOrder.status" /></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.amount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">{{ creditedAmountSymbol }}{{ selectedOrder.amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.payAmount') }}</p><p class="text-sm font-medium text-gray-900 dark:text-white">{{ paymentAmountSymbol(selectedOrder) }}{{ selectedOrder.pay_amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.paymentMethod') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + selectedOrder.payment_type, selectedOrder.payment_type) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.feeRate') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.fee_rate }}%</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.orders.createdAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.created_at) }}</p></div>
          <div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.expiresAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.expires_at) }}</p></div>
          <div v-if="selectedOrder.paid_at"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.paidAt') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.paid_at) }}</p></div>
          <div v-if="selectedOrder.refund_amount"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundAmount') }}</p><p class="text-sm font-medium text-red-600 dark:text-red-400">{{ creditedAmountSymbol }}{{ selectedOrder.refund_amount.toFixed(2) }}</p></div>
          <div v-if="selectedOrder.refund_reason" class="col-span-2"><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundReason') }}</p><p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_reason }}</p></div>
          <!-- Refund request info -->
          <div v-if="selectedOrder.refund_requested_at" class="col-span-2 border-t border-gray-200 pt-3 dark:border-dark-600">
            <p class="mb-2 text-xs font-medium text-purple-600 dark:text-purple-400">{{ t('payment.admin.refundRequestInfo') }}</p>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestedAt') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">{{ formatDateTime(selectedOrder.refund_requested_at) }}</p>
              </div>
              <div>
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestedBy') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">#{{ selectedOrder.refund_requested_by }}</p>
              </div>
              <div class="col-span-2">
                <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.refundRequestReason') }}</p>
                <p class="text-sm text-gray-700 dark:text-gray-300">{{ selectedOrder.refund_request_reason }}</p>
              </div>
            </div>
          </div>
        </div>
        <!-- Audit Logs -->
        <div v-if="orderAuditLogs.length > 0" class="border-t border-gray-200 pt-4 dark:border-dark-600">
          <p class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.auditLogs') }}</p>
          <div class="max-h-48 space-y-2 overflow-y-auto">
            <div v-for="log in orderAuditLogs" :key="log.id" class="rounded-lg border border-gray-100 bg-gray-50 p-2.5 dark:border-dark-600 dark:bg-dark-800">
              <div class="flex items-center justify-between">
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ log.action }}</span>
                <span class="text-xs text-gray-400">{{ formatDateTime(log.created_at) }}</span>
              </div>
              <div v-if="log.detail" class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">{{ log.detail }}</div>
              <div v-if="log.operator" class="mt-1 text-xs text-gray-400">{{ t('payment.admin.operator') }}: {{ log.operator }}</div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>

    <AdminRefundDialog :show="showRefundDialog" :order="selectedOrder" :submitting="refundSubmitting" :require-force="refundRequireForce" :warning="refundWarning" @confirm="handleRefund" @cancel="closeRefundDialog" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { adminAPI } from '@/api/admin'
import type { AdminPlanChangeRow } from '@/api/admin/subscriptionReset'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatOrderDateTime } from '@/components/payment/orderUtils'
import type { PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AdminRefundDialog from '@/components/admin/payment/AdminRefundDialog.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import OrderTable from '@/components/payment/OrderTable.vue'
import { currencySymbol } from '@/components/payment/currency'

interface AuditLog {
  id: number
  action: string
  detail: string | null
  operator: string | null
  created_at: string
}

const { t } = useI18n()
const appStore = useAppStore()

const ordersLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const orderSearch = ref('')
const orderFilters = reactive({ status: '', payment_type: '', order_type: '' })

// ── Finance Center tabs（Final Frontend C8）──
const financeTab = ref<'orders' | 'changes' | 'ledger'>('orders')
const planChangeRows = ref<AdminPlanChangeRow[]>([])
const planChangeTotal = ref(0)
const planChangeStatusFilter = ref('')
const planChangeStatusOptions = [
  { value: '', label: 'All' },
  { value: 'quoted', label: 'quoted' },
  { value: 'scheduled', label: 'scheduled' },
  { value: 'paid', label: 'paid' },
  { value: 'fulfilled', label: 'fulfilled' },
  { value: 'cancelled', label: 'cancelled' }
]
const changesLoading = ref(false)

async function loadPlanChanges() {
  changesLoading.value = true
  try {
    const res = await adminAPI.planChanges.list({
      page: 1,
      page_size: 30,
      status: planChangeStatusFilter.value || undefined
    })
    planChangeRows.value = res.data.items ?? []
    planChangeTotal.value = res.data.total ?? 0
  } finally {
    changesLoading.value = false
  }
}
const ledgerUserId = ref<number>()
const ledgerLoading = ref(false)
const ledgerRows = ref<Array<{ id?: number; code?: string; created_at?: string }>>([])
const ledgerError = ref('')

async function switchFinanceTab(tab: 'orders' | 'changes' | 'ledger') {
  financeTab.value = tab
  if (tab === 'changes') {
    await loadPlanChanges()
  }
}

async function loadLedger() {
  if (!ledgerUserId.value) return
  ledgerLoading.value = true
  ledgerError.value = ''
  ledgerRows.value = []
  try {
    const res = await adminPaymentAPI.getOrders({ page: 1, page_size: 1, user_id: ledgerUserId.value })
    void res
    const { getUserBalanceHistory } = await import('@/api/admin/users')
    const history = await getUserBalanceHistory(ledgerUserId.value, 1, 20)
    ledgerRows.value = (history.items ?? []).slice(0, 20)
    if (ledgerRows.value.length === 0) ledgerError.value = t('payment.admin.ledgerEmpty')
  } catch {
    ledgerError.value = t('payment.admin.ledgerFailed')
  } finally {
    ledgerLoading.value = false
  }
}

function fmtLedgerDate(iso?: string): string {
  if (!iso) return '—'
  try { return new Date(iso).toLocaleString() } catch { return iso }
}
const orderPagination = reactive({ page: 1, page_size: 20, total: 0 })
const selectedOrder = ref<PaymentOrder | null>(null)
const showDetailDialog = ref(false)
const showRefundDialog = ref(false)
const refundSubmitting = ref(false)
const refundRequireForce = ref(false)
const refundWarning = ref('')
const refundQueryingIds = ref(new Set<number>())
const orderAuditLogs = ref<AuditLog[]>([])
const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder | null | undefined): string {
  return currencySymbol(order?.currency)
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function debounceLoadOrders() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => loadOrders(), 300)
}

async function loadOrders() {
  ordersLoading.value = true
  try {
    const res = await adminPaymentAPI.getOrders({
      page: orderPagination.page, page_size: orderPagination.page_size,
      keyword: orderSearch.value || undefined, status: orderFilters.status || undefined,
      payment_type: orderFilters.payment_type || undefined, order_type: orderFilters.order_type || undefined,
    })
    orders.value = res.data.items || []
    orderPagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally { ordersLoading.value = false }
}

function handleOrderPageChange(page: number) { orderPagination.page = page; loadOrders() }
function handleOrderPageSizeChange(size: number) { orderPagination.page_size = size; orderPagination.page = 1; loadOrders() }

const statusFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allStatuses') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'PAID', label: t('payment.status.paid') },
  { value: 'COMPLETED', label: t('payment.status.completed') },
  { value: 'EXPIRED', label: t('payment.status.expired') },
  { value: 'CANCELLED', label: t('payment.status.cancelled') },
  { value: 'FAILED', label: t('payment.status.failed') },
  { value: 'REFUNDED', label: t('payment.status.refunded') },
  { value: 'REFUND_REQUESTED', label: t('payment.status.refund_requested') },
  { value: 'REFUND_PENDING', label: t('payment.status.refund_pending') },
  { value: 'REFUND_FAILED', label: t('payment.status.refund_failed') },
])

const paymentTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allPaymentTypes') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
  { value: 'airwallex', label: t('payment.methods.airwallex') },
])

const orderTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allOrderTypes') },
  { value: 'balance', label: t('payment.admin.balanceOrder') },
  { value: 'subscription', label: t('payment.admin.subscriptionOrder') },
])

async function showOrderDetail(order: PaymentOrder) {
  selectedOrder.value = order
  orderAuditLogs.value = []
  showDetailDialog.value = true
  try {
    const res = await adminPaymentAPI.getOrder(order.id)
    const data = res.data as unknown as Record<string, unknown>
    if (data.order) selectedOrder.value = data.order as PaymentOrder
    orderAuditLogs.value = ((data.auditLogs || data.audit_logs || []) as unknown) as AuditLog[]
  } catch (_err: unknown) { /* keep cached order data */ }
}

async function handleCancelOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.cancelOrder(order.id); appStore.showSuccess(t('payment.admin.orderCancelled')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

async function handleRetryOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.retryRecharge(order.id); appStore.showSuccess(t('payment.admin.retrySuccess')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

function openRefundDialog(order: PaymentOrder) {
  selectedOrder.value = order
  refundRequireForce.value = false
  refundWarning.value = ''
  showRefundDialog.value = true
}

function closeRefundDialog() {
  showRefundDialog.value = false
  refundRequireForce.value = false
  refundWarning.value = ''
}

function isRefundPendingWarning(warning: string | undefined): boolean {
  return /pending|处理中|待/.test(String(warning || '').toLowerCase())
}

async function handleRefund(data: { amount: number; reason: string; deduct_balance: boolean; force: boolean }) {
  if (!selectedOrder.value) return
  refundSubmitting.value = true
  try {
    const res = await adminPaymentAPI.refundOrder(selectedOrder.value.id, { amount: data.amount, reason: data.reason, deduct_balance: data.deduct_balance, force: data.force })
    if (res.data.success) {
      appStore.showSuccess(t('payment.admin.refundSuccess'))
      closeRefundDialog()
      loadOrders()
      return
    }
    if (isRefundPendingWarning(res.data.warning)) {
      appStore.showSuccess(t('payment.admin.refundPending'))
      closeRefundDialog()
      loadOrders()
      return
    }
    if (res.data.require_force) {
      // Backend needs an explicit force confirmation (e.g. the user spent their
      // balance after requesting the refund). Keep the dialog open and surface
      // the force checkbox instead of dropping the admin back to the list.
      refundRequireForce.value = true
      refundWarning.value = res.data.warning || ''
      return
    }
    appStore.showError(res.data.warning || t('common.error'))
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { refundSubmitting.value = false }
}

async function handleQueryRefund(order: PaymentOrder) {
  refundQueryingIds.value = new Set(refundQueryingIds.value).add(order.id)
  try {
    const res = await adminPaymentAPI.queryRefund(order.id)
    if (res.data.success) {
      appStore.showSuccess(t('payment.admin.refundSuccess'))
    } else if (isRefundPendingWarning(res.data.warning)) {
      appStore.showSuccess(t('payment.admin.refundPending'))
    } else {
      appStore.showError(res.data.warning || t('common.error'))
    }
    loadOrders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    const next = new Set(refundQueryingIds.value)
    next.delete(order.id)
    refundQueryingIds.value = next
  }
}

function formatDateTime(dateStr: string): string { return formatOrderDateTime(dateStr) }

onMounted(() => loadOrders())
</script>
