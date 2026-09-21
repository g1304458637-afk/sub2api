<template>
  <AppLayout>
    <div class="muc-scope muc-wallet">
      <div class="muc-wallet__bg" aria-hidden="true"></div>

      <div class="muc-wallet__content">
        <header class="muc-wallet__header">
          <h1 class="muc-wallet__title">{{ t('wallet.title') }}</h1>
          <p class="muc-wallet__subtitle">{{ t('wallet.subtitle') }}</p>
        </header>

        <!-- 余额 hero -->
        <MucGlassCard variant="strong" class="muc-wallet__hero" data-testid="wallet-balance">
          <div class="muc-wallet__hero-main">
            <span class="muc-wallet__hero-label">{{ t('wallet.balance') }}</span>
            <span class="muc-wallet__hero-value">{{ walletDisplay }}</span>
            <span v-if="cnyApproxDisplay" class="muc-wallet__hero-cny">
              ≈ {{ cnyApproxDisplay }}
            </span>
          </div>
          <MucButton pill @click="goRecharge">{{ t('wallet.recharge') }}</MucButton>
        </MucGlassCard>

        <!-- 说明条：一个钱包，来源见流水 -->
        <p class="muc-wallet__note">{{ t('wallet.singleWalletNote') }}</p>

        <!-- 流水：当前后端无用户侧 ledger 端点（见 FINAL_FRONTEND_REFERENCE_LOG.md BLOCKED #1），
             先呈现空态；奖励到账卡片样式见下方记录区。 -->
        <MucSectionHeader :title="t('wallet.historyTitle')" :description="t('wallet.historyDesc')" />

        <MucGlassCard class="muc-wallet__history">
          <MucState :message="t('wallet.historyEmpty')" icon="inbox" />
        </MucGlassCard>

        <!-- 奖励记录：学生认证奖励等（Reward → Wallet 自动入账）。
             后端暂无用户侧 reward 查询端点（BLOCKED #2），先提供静态卡片样式记录位。 -->
        <MucSectionHeader :title="t('wallet.rewardTitle')" :description="t('wallet.rewardDesc')" />

        <MucGlassCard class="muc-wallet__history muc-wallet__rewards">
          <MucRewardGiftCard
            v-if="showRewardSample"
            mini
            :amount="t('wallet.rewardSampleAmount')"
            :campaign="t('wallet.rewardSampleCampaign')"
            :animate="false"
          />
          <MucState v-else :message="t('wallet.rewardEmpty')" icon="inbox" />
        </MucGlassCard>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import '@/components/pricing/muc-tokens.css'
import MucGlassCard from '@/components/muc/MucGlassCard.vue'
import MucButton from '@/components/muc/MucButton.vue'
import MucSectionHeader from '@/components/muc/MucSectionHeader.vue'
import MucState from '@/components/muc/MucState.vue'
import MucRewardGiftCard from '@/components/muc/MucRewardGiftCard.vue'
import { getAccountStatus } from '@/api/subscriptions'
import { paymentAPI } from '@/api/payment'
import type { PaymentConfig } from '@/types/payment'
import {
  formatPaymentAmount,
  normalizePaymentCurrency
} from '@/components/payment/currency'
import { useAppStore } from '@/stores'

/**
 * Wallet：充值 + Reward + PAYG 统一账户钱包。
 * 余额来自 /subscriptions/status 合同（users.balance，canonical USD）；
 * ≈¥ 仅在后台配置了展示汇率（usd_to_cny_display_rate > 0）时显示。
 * 流水区当前无用户端 ledger API（FRONTEND_BLOCKED_BY_API #1），空态先行。
 */
const router = useRouter()
const { t, locale } = useI18n()
const appStore = useAppStore()

const balance = ref<string | null>(null)
const canonicalCurrency = ref('')
const displayRate = ref(0)
const showRewardSample = ref(true)

const loc = computed(() => (typeof locale.value === 'string' ? locale.value : undefined))

const walletDisplay = computed(() => {
  if (balance.value === null) return '—'
  const value = parseFloat(balance.value)
  return formatPaymentAmount(
    Number.isFinite(value) ? value : 0,
    normalizePaymentCurrency(canonicalCurrency.value),
    loc.value
  )
})

const cnyApproxDisplay = computed(() => {
  if (balance.value === null || displayRate.value <= 0) return ''
  const value = parseFloat(balance.value)
  if (!Number.isFinite(value)) return ''
  return formatPaymentAmount(value * displayRate.value, 'CNY', loc.value)
})

onMounted(async () => {
  try {
    const [status, config] = await Promise.all([
      getAccountStatus(),
      paymentAPI.getConfig().catch(() => null)
    ])
    balance.value = status.wallet.balance
    canonicalCurrency.value = status.wallet.canonical_currency
    const cfg = config?.data as PaymentConfig | null
    displayRate.value = cfg?.usd_to_cny_display_rate ?? 0
  } catch (err) {
    appStore.showError(
      err && typeof err === 'object' && 'message' in err
        ? String((err as { message?: unknown }).message)
        : t('wallet.loadError')
    )
  }
})

function goRecharge() {
  void router.push('/purchase')
}
</script>

<style scoped>
.muc-wallet {
  position: relative;
  overflow: hidden;
  min-height: calc(100vh - 96px);
  border-radius: 24px;
  background: var(--muc-bg-primary);
  color: var(--muc-text-primary);
}

.muc-wallet__bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(60% 36% at 50% 0%, rgba(200, 36, 51, 0.1), transparent 70%),
    linear-gradient(180deg, var(--muc-bg-primary), var(--muc-bg-secondary));
  pointer-events: none;
}

.muc-wallet__content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-width: 760px;
  margin: 0 auto;
  padding: clamp(28px, 5vh, 56px) clamp(16px, 4vw, 40px) 44px;
}

.muc-wallet__header {
  text-align: center;
}

.muc-wallet__title {
  font-size: clamp(22px, 3vw, 30px);
  font-weight: 700;
}

.muc-wallet__subtitle {
  margin-top: 8px;
  font-size: 13.5px;
  color: var(--muc-text-secondary);
}

.muc-wallet__hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 26px 28px;
}

.muc-wallet__hero-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.muc-wallet__hero-label {
  font-size: 11px;
  letter-spacing: 0.1em;
  color: var(--muc-text-muted);
}

.muc-wallet__hero-value {
  font-size: 40px;
  font-weight: 800;
  line-height: 1.05;
  font-variant-numeric: tabular-nums;
}

.muc-wallet__hero-cny {
  font-size: 13px;
  color: var(--muc-text-secondary);
}

.muc-wallet__note {
  margin: 0;
  text-align: center;
  font-size: 12px;
  color: var(--muc-text-muted);
}

.muc-wallet__history {
  padding: 8px;
}

.muc-wallet__rewards {
  display: flex;
  justify-content: center;
  padding: 20px;
}
</style>
