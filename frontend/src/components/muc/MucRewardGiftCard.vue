<template>
  <div class="muc-gift" :class="{ 'muc-gift--mini': mini }" role="status" :aria-label="title">
    <!-- 光扫（左上 → 右下） -->
    <span class="muc-gift__sweep" aria-hidden="true"></span>
    <!-- 红金小粒子（6~12 个，纯 CSS keyframes，不用粒子引擎） -->
    <span
      v-for="p in particles"
      :key="p.id"
      class="muc-gift__particle"
      :style="p.style"
      aria-hidden="true"
    ></span>

    <div class="muc-gift__brand">MUCODE</div>
    <p class="muc-gift__title">{{ title }}</p>
    <div class="muc-gift__amount" :class="{ 'muc-gift__amount--done': countDone }">
      {{ displayAmount }}
    </div>
    <p v-if="amountCnyDisplay" class="muc-gift__cny">≈ {{ amountCnyDisplay }}</p>
    <p class="muc-gift__usage">{{ usageLabel }}</p>
    <p v-if="campaign" class="muc-gift__campaign">{{ campaign }}</p>
    <p class="muc-gift__foot">{{ t('rewardGift.depositNote') }}</p>

    <div v-if="!mini" class="muc-gift__actions">
      <MucButton variant="secondary" pill @click="emit('viewWallet')">
        {{ t('rewardGift.viewWallet') }}
      </MucButton>
      <MucButton pill @click="emit('acknowledge')">{{ t('rewardGift.ok') }}</MucButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/components/pricing/muc-tokens.css'
import MucButton from '@/components/muc/MucButton.vue'
import { mucTween } from '@/components/subscription/resetAnimation'

/**
 * MucRewardGiftCard —— 奖励到账「额度卡」（黑红玻璃 + 暖金边，纯 HTML/CSS/RAF）。
 * Reward 后端已自动入账，因此绝无「领取」按钮：mini 模式（Admin 预览/流水记录）无按钮，
 * 完整模式只有「查看钱包 / 知道了」。金额 count-up 仅是视觉表达，canonical 金额来自后端。
 * prefers-reduced-motion：跳过 count-up 与光扫，直接呈现终态。
 */
const props = withDefaults(
  defineProps<{
    /** canonical 金额显示串（如 "+$5.00"，由调用方用后端数值格式化） */
    amount: string
    /** 可选的 CNY 近似显示串（如 "¥35.90"），仅展示 */
    amountCny?: string
    title?: string
    campaign?: string
    usageLabel?: string
    /** mini=Admin 预览/记录样式（无按钮、无动画） */
    mini?: boolean
    /** 完整模式挂载即播放 count-up */
    animate?: boolean
  }>(),
  {
    amountCny: '',
    title: '',
    campaign: '',
    usageLabel: '',
    mini: false,
    animate: true
  }
)

const emit = defineEmits<{ viewWallet: []; acknowledge: [] }>()

const { t } = useI18n()

const title = computed(() => props.title || t('rewardGift.studentTitle'))
const usageLabel = computed(() => props.usageLabel || t('rewardGift.usage'))
const amountCnyDisplay = computed(() => props.amountCny)

const countDone = ref(false)
const animatedValue = ref(props.mini || !props.animate ? 1 : 0)
let cancelTween: (() => void) | null = null

// amount 形如 "+$5.00" → 取数值部分做 count-up
const numericAmount = computed(() => {
  const match = props.amount.replace(/,/g, '').match(/-?\d+(\.\d+)?/)
  return match ? parseFloat(match[0]) : 0
})

const displayAmount = computed(() => {
  if (countDone.value || props.mini || !props.animate) return props.amount
  const prefix = props.amount.trim().startsWith('+') ? '+' : ''
  const decimals = (props.amount.split('.')[1] || '').replace(/\D/g, '').length
  const value = numericAmount.value * animatedValue.value
  return `${prefix}${value.toFixed(decimals)}`
})

// 6~12 个粒子：随机位置/延迟/颜色（红或金）
const particles = computed(() => {
  const total = 6 + Math.floor(Math.random() * 7)
  return Array.from({ length: total }, (_, id) => {
    const gold = Math.random() > 0.5
    return {
      id,
      style: {
        left: `${4 + Math.random() * 92}%`,
        top: `${6 + Math.random() * 88}%`,
        background: gold ? 'var(--muc-gold)' : 'var(--muc-red-bright)',
        animationDelay: `${(Math.random() * 0.9).toFixed(2)}s`,
        animationDuration: `${(1.1 + Math.random() * 1.2).toFixed(2)}s`
      }
    }
  })
})

watch(
  () => props.animate,
  (animate) => {
    if (props.mini || !animate) return
    cancelTween?.()
    countDone.value = false
    animatedValue.value = 0
    cancelTween = mucTween({
      from: 0,
      to: 1,
      durationMs: 900,
      onUpdate: (v) => {
        animatedValue.value = v
      },
      onDone: () => {
        countDone.value = true
      }
    })
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  cancelTween?.()
})
</script>

<style scoped>
.muc-gift {
  --gift-accent: var(--muc-gold);
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  width: min(92vw, 380px);
  padding: 30px 30px 26px;
  border-radius: 26px;
  border: 1px solid rgba(214, 180, 106, 0.55);
  background:
    radial-gradient(120% 90% at 50% 0%, rgba(200, 36, 51, 0.22), transparent 60%),
    var(--muc-glass-strong);
  box-shadow:
    0 24px 80px rgba(0, 0, 0, 0.55),
    0 0 40px rgba(214, 180, 106, 0.12);
  color: var(--muc-text-primary);
  text-align: center;
  animation: muc-gift-enter 0.6s cubic-bezier(0.22, 1, 0.36, 1) both;
}

@keyframes muc-gift-enter {
  from {
    opacity: 0;
    transform: translateY(24px) scale(0.94);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

/* 暖金光扫 */
.muc-gift__sweep {
  position: absolute;
  inset: -40%;
  background: linear-gradient(
    115deg,
    transparent 42%,
    rgba(214, 180, 106, 0.16) 50%,
    transparent 58%
  );
  transform: translateX(-70%);
  animation: muc-gift-sweep 2.6s cubic-bezier(0.22, 1, 0.36, 1) 0.4s 2;
  pointer-events: none;
}

@keyframes muc-gift-sweep {
  to {
    transform: translateX(70%);
  }
}

/* 红金小粒子：升起点灭 */
.muc-gift__particle {
  position: absolute;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  opacity: 0;
  animation: muc-gift-particle ease-out infinite alternate;
  pointer-events: none;
}

@keyframes muc-gift-particle {
  0% {
    opacity: 0;
    transform: translateY(6px) scale(0.6);
  }
  35% {
    opacity: 0.9;
  }
  100% {
    opacity: 0;
    transform: translateY(-26px) scale(1);
  }
}

.muc-gift__brand {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.5em;
  text-indent: 0.5em;
  color: rgba(255, 226, 228, 0.6);
}

.muc-gift__title {
  margin: 4px 0 0;
  font-size: 16px;
  font-weight: 700;
}

.muc-gift__amount {
  font-size: 44px;
  font-weight: 800;
  line-height: 1.1;
  color: var(--gift-accent);
  text-shadow: 0 0 30px rgba(214, 180, 106, 0.3);
  font-variant-numeric: tabular-nums;
}

.muc-gift__cny {
  margin: 0;
  font-size: 13px;
  color: var(--muc-text-secondary);
}

.muc-gift__usage {
  margin: 2px 0 0;
  font-size: 12.5px;
  letter-spacing: 0.12em;
  color: var(--muc-text-secondary);
}

.muc-gift__campaign {
  margin: 0;
  font-size: 11px;
  color: var(--muc-text-muted);
}

.muc-gift__foot {
  margin: 6px 0 0;
  font-size: 11.5px;
  color: var(--muc-text-muted);
}

.muc-gift__actions {
  display: flex;
  gap: 10px;
  margin-top: 12px;
}

/* mini（Admin 预览/记录）：无动画无按钮，静态呈现 */
.muc-gift--mini {
  width: min(100%, 300px);
  padding: 20px 20px 18px;
  gap: 5px;
  animation: none;
  border-radius: 20px;
}

.muc-gift--mini .muc-gift__sweep,
.muc-gift--mini .muc-gift__particle {
  display: none;
}

.muc-gift--mini .muc-gift__amount {
  font-size: 30px;
}

@media (prefers-reduced-motion: reduce) {
  .muc-gift {
    animation: none;
  }

  .muc-gift__sweep,
  .muc-gift__particle {
    display: none;
  }
}
</style>
