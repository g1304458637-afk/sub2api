<template>
  <article
    class="plan-card"
    :class="[`plan-card--${variant}`, { 'plan-card--current': isCurrent }]"
  >
    <!-- Pro：红边高光 Popular 徽标；Max：红 + 极少量暖金层级点缀 -->
    <span v-if="variant === 'max'" class="plan-card__gold-hairline" aria-hidden="true"></span>

    <header class="plan-card__head">
      <div class="plan-card__name-row">
        <h3 class="plan-card__name">{{ display.name }}</h3>
        <span v-if="popular" class="plan-card__badge">{{ t('pricing.popularBadge') }}</span>
        <span v-else-if="isCurrent" class="plan-card__badge plan-card__badge--current">
          {{ t('pricing.currentPlanBadge') }}
        </span>
      </div>
      <p v-if="display.description" class="plan-card__desc">{{ display.description }}</p>
    </header>

    <div class="plan-card__price-row">
      <div class="plan-card__price-wrap">
        <span v-if="display.originalPrice" class="plan-card__price-original">
          {{ display.originalPrice }}
        </span>
        <span class="plan-card__price">{{ display.price }}</span>
      </div>
      <span v-if="display.validitySuffix" class="plan-card__validity">
        {{ display.validitySuffix }}
      </span>
    </div>

    <ul v-if="display.features.length" class="plan-card__features">
      <li v-for="(feature, i) in display.features" :key="i" class="plan-card__feature">
        <Icon name="check" size="sm" class="plan-card__feature-icon" />
        <span>{{ feature }}</span>
      </li>
    </ul>

    <!-- 当前套餐卡片：预约切换横幅 -->
    <div v-if="isCurrent && scheduledPlanName" class="plan-card__scheduled-banner">
      <Icon name="clock" size="sm" class="plan-card__banner-icon" />
      <div class="plan-card__banner-text">
        <p>{{ t('pricing.scheduledBanner.on', { plan: scheduledPlanName }) }}</p>
        <p v-if="scheduledEffectiveText" class="plan-card__banner-at">
          {{ t('pricing.scheduledBanner.at', { date: scheduledEffectiveText }) }}
        </p>
      </div>
      <button
        type="button"
        class="plan-card__banner-cancel"
        :disabled="busy"
        @click="emit('cancelScheduled')"
      >
        {{ t('pricing.scheduledBanner.cancel') }}
      </button>
    </div>

    <div class="plan-card__cta-row">
      <button
        v-if="ctaKind === 'buy' || ctaKind === 'upgrade'"
        type="button"
        class="plan-card__cta plan-card__cta--primary"
        :disabled="busy"
        @click="emit('cta')"
      >
        {{ ctaLabel }}
      </button>
      <button
        v-else-if="ctaKind === 'downgrade'"
        type="button"
        class="plan-card__cta plan-card__cta--ghost"
        :disabled="busy"
        @click="emit('cta')"
      >
        {{ ctaLabel }}
      </button>
      <span v-else-if="ctaKind === 'scheduled'" class="plan-card__cta plan-card__cta--done">
        <Icon name="check" size="sm" />
        {{ ctaLabel }}
      </span>
      <span v-else-if="ctaKind === 'current'" class="plan-card__cta plan-card__cta--current">
        {{ ctaLabel }}
      </span>
      <span v-else class="plan-card__cta plan-card__cta--disabled">{{ ctaLabel }}</span>
    </div>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

export interface MucPlanCardDisplay {
  name: string
  description?: string
  /** 已格式化金额（含币种符号） */
  price: string
  originalPrice?: string
  validitySuffix: string
  features: string[]
}

type CtaKind = 'buy' | 'upgrade' | 'downgrade' | 'scheduled' | 'current' | 'unavailable'

defineProps<{
  display: MucPlanCardDisplay
  variant: 'basic' | 'pro' | 'max'
  ctaKind: CtaKind
  ctaLabel: string
  popular?: boolean
  isCurrent?: boolean
  /** 当前套餐卡上的到期切换横幅（目标套餐名） */
  scheduledPlanName?: string | null
  scheduledEffectiveText?: string | null
  busy?: boolean
}>()

const emit = defineEmits<{
  cta: []
  cancelScheduled: []
}>()

const { t } = useI18n()
</script>

<style scoped>
.plan-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 18px;
  padding: 26px 24px 24px;
  border-radius: 20px;
  border: 1px solid var(--muc-glass-border);
  background: var(--muc-glass);
  backdrop-filter: blur(22px) saturate(1.1);
  -webkit-backdrop-filter: blur(22px) saturate(1.1);
  color: var(--muc-text-primary);
  transition: border-color 0.25s ease, box-shadow 0.25s ease, transform 0.25s ease;
}

.plan-card:hover {
  border-color: var(--muc-red-border-hover);
  box-shadow:
    0 18px 60px rgba(0, 0, 0, 0.4),
    0 0 32px rgba(238, 56, 72, 0.14);
  transform: translateY(-2px);
}

/* Pro：民大红主强调 */
.plan-card--pro {
  border-color: var(--muc-red-border);
  background: linear-gradient(170deg, rgba(200, 36, 51, 0.14), rgba(15, 15, 17, 0.62) 46%),
    var(--muc-glass);
  box-shadow: 0 14px 50px rgba(0, 0, 0, 0.38), 0 0 26px var(--muc-red-glow);
}

.plan-card--pro:hover {
  border-color: var(--muc-red-border-hover);
  box-shadow:
    0 18px 60px rgba(0, 0, 0, 0.4),
    0 0 40px rgba(238, 56, 72, 0.2);
}

/* Max：深色玻璃 + 极少量暖金点缀（整卡仍是深色，不做金卡） */
.plan-card--max .plan-card__feature-icon {
  color: var(--muc-gold);
}

.plan-card--max .plan-card__price {
  text-shadow: 0 0 24px rgba(214, 180, 106, 0.25);
}

.plan-card__gold-hairline {
  position: absolute;
  top: 0;
  left: 24px;
  right: 24px;
  height: 1px;
  background: linear-gradient(to right, transparent, rgba(214, 180, 106, 0.65), transparent);
}

.plan-card--current {
  border-color: var(--muc-red-border);
}

.plan-card__head {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.plan-card__name-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.plan-card__name {
  font-size: 20px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.plan-card__badge {
  flex-shrink: 0;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  color: #fff;
  background: linear-gradient(120deg, var(--muc-red), var(--muc-red-bright));
  box-shadow: 0 2px 14px var(--muc-red-glow);
}

.plan-card__badge--current {
  color: var(--muc-red-bright);
  background: var(--muc-red-soft);
  box-shadow: none;
  border: 1px solid var(--muc-red-border);
}

.plan-card__desc {
  font-size: 13px;
  line-height: 1.55;
  color: var(--muc-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.plan-card__price-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.plan-card__price-wrap {
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.plan-card__price {
  font-size: 34px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.01em;
}

.plan-card__price-original {
  font-size: 14px;
  color: var(--muc-text-muted);
  text-decoration: line-through;
}

.plan-card__validity {
  font-size: 12px;
  color: var(--muc-text-muted);
}

.plan-card__features {
  display: flex;
  flex-direction: column;
  gap: 9px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.plan-card__feature {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--muc-text-secondary);
}

.plan-card__feature-icon {
  flex-shrink: 0;
  margin-top: 2px;
  color: var(--muc-red-bright);
}

.plan-card--basic .plan-card__feature-icon {
  color: var(--muc-text-muted);
}

.plan-card__scheduled-banner {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  border: 1px solid var(--muc-red-border);
  background: var(--muc-red-soft);
  font-size: 12px;
}

.plan-card__banner-icon {
  flex-shrink: 0;
  margin-top: 1px;
  color: var(--muc-red-bright);
}

.plan-card__banner-text {
  flex: 1;
  min-width: 0;
  color: var(--muc-text-primary);
}

.plan-card__banner-at {
  margin-top: 2px;
  font-size: 11px;
  color: var(--muc-text-muted);
}

.plan-card__banner-cancel {
  flex-shrink: 0;
  border: none;
  background: none;
  padding: 2px 4px;
  font-size: 12px;
  color: var(--muc-red-bright);
  cursor: pointer;
  text-decoration: underline;
  text-underline-offset: 3px;
}

.plan-card__banner-cancel:disabled {
  opacity: 0.5;
  cursor: wait;
}

.plan-card__cta-row {
  margin-top: auto;
}

.plan-card__cta {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 11px 16px;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.04em;
  border: 1px solid transparent;
  transition: transform 0.15s ease, box-shadow 0.2s ease, background 0.2s ease,
    border-color 0.2s ease;
}

/* 主 CTA：白底黑字（可读性与高级感来源），红只用于 hover 细节 */
.plan-card__cta--primary {
  background: #fff;
  color: #0a0a0b;
  cursor: pointer;
}

.plan-card__cta--primary:hover:not(:disabled) {
  box-shadow: 0 6px 26px rgba(238, 56, 72, 0.3);
  transform: translateY(-1px);
}

.plan-card__cta--primary:disabled {
  opacity: 0.6;
  cursor: wait;
}

.plan-card__cta--ghost {
  background: transparent;
  color: var(--muc-text-secondary);
  border-color: var(--muc-glass-border-strong);
  cursor: pointer;
}

.plan-card__cta--ghost:hover:not(:disabled) {
  border-color: var(--muc-red-border-hover);
  color: var(--muc-text-primary);
}

.plan-card__cta--ghost:disabled {
  opacity: 0.6;
  cursor: wait;
}

.plan-card__cta--done {
  background: var(--muc-red-soft);
  border-color: var(--muc-red-border);
  color: var(--muc-red-bright);
}

.plan-card__cta--current {
  background: rgba(255, 255, 255, 0.05);
  border-color: var(--muc-glass-border-strong);
  color: var(--muc-text-muted);
}

.plan-card__cta--disabled {
  background: rgba(255, 255, 255, 0.03);
  border-color: var(--muc-glass-border);
  color: var(--muc-text-muted);
}
</style>
