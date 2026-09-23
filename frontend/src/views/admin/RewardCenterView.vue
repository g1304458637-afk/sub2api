<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-4">
      <!-- 学生认证奖励配置 -->
      <div class="card space-y-4 p-5">
        <div>
          <h3 class="text-sm font-semibold">{{ t('rewardCenter.configTitle') }}</h3>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('rewardCenter.configDesc') }}</p>
        </div>

        <div v-if="configLoading" class="space-y-3">
          <div class="h-10 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-800"></div>
          <div class="h-10 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-800"></div>
        </div>

        <template v-else>
          <div class="flex items-center justify-between rounded-xl border border-gray-100 p-3 dark:border-dark-700">
            <div>
              <p class="text-sm font-medium">{{ t('rewardCenter.enabled') }}</p>
              <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('rewardCenter.enabledHint') }}</p>
            </div>
            <button
              type="button"
              role="switch"
              :aria-checked="form.enabled"
              class="relative h-6 w-11 rounded-full transition-colors"
              :class="form.enabled ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-dark-600'"
              @click="form.enabled = !form.enabled"
            >
              <span
                class="absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all"
                :style="{ left: form.enabled ? '22px' : '2px' }"
              ></span>
            </button>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <label class="block text-sm">
              <span class="input-label">{{ t('rewardCenter.amount') }}</span>
              <input v-model.number="form.amount" type="number" step="0.01" min="0" class="input mt-1 w-full" />
            </label>
            <label class="block text-sm">
              <span class="input-label">{{ t('rewardCenter.campaign') }}</span>
              <input v-model="form.campaign" class="input mt-1 w-full" placeholder="2026-fall-student" />
            </label>
          </div>

          <div class="flex items-center gap-3">
            <button class="btn btn-primary" :disabled="saving" @click="save">
              {{ saving ? t('common.processing') : t('common.save') }}
            </button>
            <span v-if="savedMessage" class="text-xs text-emerald-600">{{ savedMessage }}</span>
          </div>

          <div class="rounded-xl bg-gray-50 p-3 text-xs leading-relaxed text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            <p>· {{ t('rewardCenter.noteAuto') }}</p>
            <p>· {{ t('rewardCenter.noteIdempotent') }}</p>
          </div>
        </template>
      </div>

      <!-- 奖励发放记录（admin /admin/rewards + /stats，CLOSURE 已解锁） -->
      <div class="card space-y-3 p-5">
        <h3 class="text-sm font-semibold">{{ t('rewardCenter.recordsTitle') }}</h3>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div class="rounded-xl border border-gray-100 p-3 text-center dark:border-dark-700">
            <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('rewardCenter.todayUsers') }}</p>
            <p class="text-lg font-bold">{{ stats.today_count }}</p>
          </div>
          <div class="rounded-xl border border-gray-100 p-3 text-center dark:border-dark-700">
            <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('rewardCenter.todayAmount') }}</p>
            <p class="text-lg font-bold">{{ currencyStore.formatCNY(stats.today_sum) }}</p>
          </div>
          <div class="rounded-xl border border-gray-100 p-3 text-center dark:border-dark-700">
            <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('rewardCenter.monthUsers') }}</p>
            <p class="text-lg font-bold">{{ stats.month_count }}</p>
          </div>
          <div class="rounded-xl border border-gray-100 p-3 text-center dark:border-dark-700">
            <p class="text-[11px] text-gray-500 dark:text-dark-400">{{ t('rewardCenter.monthAmount') }}</p>
            <p class="text-lg font-bold">{{ currencyStore.formatCNY(stats.month_sum) }}</p>
          </div>
        </div>

        <div v-if="grants.length" class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-gray-100 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">
                <th class="py-2 pr-4">ID</th>
                <th class="py-2 pr-4">{{ t('rewardCenter.userId') }}</th>
                <th class="py-2 pr-4">{{ t('rewardCenter.sourceType') }}</th>
                <th class="py-2 pr-4">{{ t('rewardCenter.campaignCol') }}</th>
                <th class="py-2 pr-4">{{ t('rewardCenter.amountCol') }}</th>
                <th class="py-2 pr-4">{{ t('rewardCenter.grantedAt') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="g in grants" :key="g.id" class="border-b border-gray-50 dark:border-dark-700/50">
                <td class="py-2 pr-4 font-mono text-xs">{{ g.id }}</td>
                <td class="py-2 pr-4">{{ g.user_id }}</td>
                <td class="py-2 pr-4">{{ t('rewardCenter.source.' + g.source_type, g.source_type) }}</td>
                <td class="py-2 pr-4 text-xs">{{ g.campaign || '—' }}</td>
                <td class="py-2 pr-4 font-medium text-amber-600 dark:text-[#d6b46a]">+{{ currencyStore.formatCNY(g.amount) }}</td>
                <td class="py-2 pr-4 text-xs text-gray-500">{{ fmtDate(g.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="text-sm text-gray-500 dark:text-dark-400">{{ t('rewardCenter.noGrants') }}</p>

        <!-- 用户端 Gift Card 样式预览（管理员预览学生会看到什么；无动画） -->
        <div class="flex justify-center rounded-xl bg-gradient-to-b from-[#070708] to-[#101012] p-8">
          <MucRewardGiftCard
            mini
            :amount="previewAmount"
            :campaign="form.campaign || '2026-fall-student'"
            :animate="false"
          />
        </div>
        <p class="text-center text-[11px] text-gray-400">{{ t('rewardCenter.previewNote') }}</p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import MucRewardGiftCard from '@/components/muc/MucRewardGiftCard.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores'
import { useCurrencyDisplayStore } from '@/stores/currencyDisplay'

/**
 * Admin Reward Center：只管 Reward → Wallet（学生如何认证不属于本项目）。
 * 配置经 admin settings（student_verification_reward_enabled/amount/campaign）。
 * 发放记录：后端暂无 reward 查询端点（REFERENCE_LOG BLOCKED #2）——统计区留空态，
 * 展示与用户端同风格的 Gift Card 静态预览，等真实触发点接入。
 */
const { t, locale } = useI18n()
const appStore = useAppStore()
const currencyStore = useCurrencyDisplayStore()

const configLoading = ref(true)
const saving = ref(false)
const savedMessage = ref('')
const stats = ref({ today_count: 0, today_sum: 0, month_count: 0, month_sum: 0 })
const grants = ref<Array<{ id: number; user_id: number; source_type: string; campaign: string; amount: number; created_at: string }>>([])

function fmtDate(iso: string): string {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleDateString()
  } catch {
    return iso
  }
}
const form = ref({ enabled: false, amount: 0, campaign: '' })

const previewAmount = computed(() => `+${currencyStore.formatCNY(Number(form.value.amount || 0))}`)
const loc = computed(() => (typeof locale.value === 'string' ? locale.value : undefined))
void loc.value

onMounted(async () => {
  try {
    const [data, rewardsRes, statsRes] = await Promise.all([
      adminAPI.settings.getSettings(),
      adminAPI.rewards.list({ page: 1, page_size: 20 }).catch(() => null),
      adminAPI.rewards.stats().catch(() => null)
    ])
    if (rewardsRes) grants.value = rewardsRes.data.items ?? []
    if (statsRes) stats.value = statsRes.data
    form.value = {
      enabled: (data as unknown as Record<string, unknown>).student_verification_reward_enabled === true,
      amount: Number((data as unknown as Record<string, unknown>).student_verification_reward_amount ?? 0),
      campaign: String((data as unknown as Record<string, unknown>).student_verification_reward_campaign ?? '')
    }
  } catch {
    appStore.showError(t('rewardCenter.loadFailed'))
  } finally {
    configLoading.value = false
  }
})

async function save() {
  saving.value = true
  savedMessage.value = ''
  try {
    await adminAPI.settings.updateSettings({
      student_verification_reward_enabled: form.value.enabled,
      student_verification_reward_amount: form.value.amount,
      student_verification_reward_campaign: form.value.campaign
    })
    savedMessage.value = t('rewardCenter.saved')
    setTimeout(() => (savedMessage.value = ''), 2500)
  } catch {
    appStore.showError(t('rewardCenter.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>
