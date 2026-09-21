<template>
  <Teleport to="body">
    <Transition name="muc-modal-anim">
      <div
        v-if="open"
        class="muc-scope muc-modal-root"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
        ref="overlayRef"
        tabindex="-1"
        @click.self="onOverlayClick"
        @keydown.esc="onEsc"
      >
        <div class="muc-modal-root__panel" :style="{ width: `min(${maxWidth}, 100%)` }">
          <header class="muc-modal-root__head">
            <h3 class="muc-modal-root__title">{{ title }}</h3>
            <button
              v-if="closable"
              type="button"
              class="muc-modal-root__close"
              :aria-label="t('common.close')"
              @click="emit('close')"
            >
              <Icon name="x" size="md" />
            </button>
          </header>
          <div class="muc-modal-root__body" :class="{ 'muc-modal-root__body--flush': flush }">
            <slot></slot>
          </div>
          <footer v-if="$slots.footer" class="muc-modal-root__foot">
            <slot name="footer"></slot>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import '@/components/pricing/muc-tokens.css'
import Icon from '@/components/icons/Icon.vue'

/**
 * MUC 统一深色玻璃弹窗基座（Teleport 到 body，token 经 .muc-scope 生效）。
 * a11y：role=dialog + aria-label + ESC 关闭 + 打开时焦点移入面板 + 关闭后归还。
 * 业务弹窗（升级报价/重置卡确认等）一律基于此组件，不再各写一套 overlay。
 */
const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    maxWidth?: string
    closable?: boolean
    /** 点击遮罩是否关闭 */
    closeOnOverlay?: boolean
    /** body 是否去掉内边距（自定义内容自带布局时） */
    flush?: boolean
  }>(),
  { maxWidth: '520px', closable: true, closeOnOverlay: true, flush: false }
)

const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const overlayRef = ref<HTMLElement | null>(null)
let lastFocused: HTMLElement | null = null

function onOverlayClick() {
  if (props.closeOnOverlay && props.closable) emit('close')
}

function onEsc() {
  if (props.closable) emit('close')
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      lastFocused = document.activeElement as HTMLElement | null
      await nextTick()
      overlayRef.value?.focus()
    } else {
      lastFocused?.focus?.()
      lastFocused = null
    }
  }
)

onBeforeUnmount(() => {
  if (props.open) lastFocused?.focus?.()
})
</script>

<style scoped>
.muc-modal-root {
  position: fixed;
  inset: 0;
  z-index: 90;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(4, 4, 5, 0.72);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  outline: none;
}

.muc-modal-root__panel {
  display: flex;
  max-height: min(86vh, 780px);
  flex-direction: column;
  border-radius: 20px;
  border: 1px solid var(--muc-glass-border-strong);
  background: linear-gradient(175deg, rgba(24, 24, 27, 0.92), rgba(12, 12, 14, 0.96));
  box-shadow: 0 30px 90px rgba(0, 0, 0, 0.55), 0 0 40px rgba(238, 56, 72, 0.1);
  color: var(--muc-text-primary);
}

.muc-modal-root__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 22px 12px;
}

.muc-modal-root__title {
  font-size: 17px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.muc-modal-root__close {
  border: none;
  background: none;
  padding: 6px;
  border-radius: 10px;
  color: var(--muc-text-muted);
  cursor: pointer;
}

.muc-modal-root__close:hover {
  color: var(--muc-text-primary);
  background: rgba(255, 255, 255, 0.06);
}

.muc-modal-root__body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 4px 22px 16px;
  overflow-y: auto;
}

.muc-modal-root__body--flush {
  padding: 0;
  overflow: visible;
}

.muc-modal-root__foot {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 14px 22px 18px;
  border-top: 1px solid var(--muc-glass-border);
}

.muc-modal-anim-enter-active,
.muc-modal-anim-leave-active {
  transition: opacity 0.22s ease;
}

.muc-modal-anim-enter-active .muc-modal-root__panel,
.muc-modal-anim-leave-active .muc-modal-root__panel {
  transition: transform 0.22s cubic-bezier(0.22, 1, 0.36, 1), opacity 0.22s ease;
}

.muc-modal-anim-enter-from,
.muc-modal-anim-leave-to {
  opacity: 0;
}

.muc-modal-anim-enter-from .muc-modal-root__panel,
.muc-modal-anim-leave-to .muc-modal-root__panel {
  transform: translateY(10px) scale(0.98);
  opacity: 0;
}
</style>
