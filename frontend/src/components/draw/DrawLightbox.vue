<template>
  <Teleport to="body">
    <Transition name="lightbox-fade">
      <div
        v-if="src"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4"
        role="dialog"
        aria-modal="true"
        :aria-label="alt || t('draw.gallery.preview')"
        @click.self="$emit('close')"
      >
        <!-- 关闭按钮 -->
        <button
          type="button"
          class="absolute right-4 top-4 inline-flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20"
          :aria-label="t('draw.gallery.close')"
          @click="$emit('close')"
        >
          <Icon name="x" size="md" />
        </button>

        <img
          :src="src"
          :alt="alt || ''"
          class="max-h-[85vh] max-w-[92vw] rounded-lg object-contain shadow-2xl"
        />
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
/**
 * 极简 lightbox：展示大图，点背景/关闭按钮/Esc 关闭。
 */
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  src: string
  alt?: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape' && props.src) emit('close')
}

function lockBodyScroll(): void {
  document.body.style.overflow = 'hidden'
}

function unlockBodyScroll(): void {
  document.body.style.overflow = ''
}

// 组件随画廊常驻挂载：仅在有大图展示时锁定背景滚动
watch(
  () => props.src,
  (src) => {
    if (src) lockBodyScroll()
    else unlockBodyScroll()
  },
)

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  unlockBodyScroll()
})
</script>

<style scoped>
.lightbox-fade-enter-active,
.lightbox-fade-leave-active {
  transition: opacity 0.18s ease;
}

.lightbox-fade-enter-from,
.lightbox-fade-leave-to {
  opacity: 0;
}
</style>
