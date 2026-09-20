<template>
  <div class="card p-6">
    <div class="mb-4 flex items-center justify-between">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">
        {{ t('draw.gallery.title') }}
      </h2>
      <span
        v-if="items.length > 0"
        class="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300"
      >
        {{ items.length }}
      </span>
    </div>

    <div
      v-if="items.length === 0"
      class="flex flex-col items-center justify-center rounded-xl border border-dashed border-gray-200 px-6 py-16 text-center dark:border-dark-600"
    >
      <div class="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800">
        <Icon name="sparkles" size="lg" class="text-gray-400 dark:text-gray-500" />
      </div>
      <p class="text-sm text-gray-400 dark:text-gray-500">{{ t('draw.gallery.empty') }}</p>
    </div>

    <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <figure
        v-for="item in items"
        :key="item.id"
        class="group relative overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
      >
        <img
          :src="item.src"
          :alt="item.prompt"
          loading="lazy"
          class="aspect-square w-full cursor-zoom-in object-cover"
          @click="preview = item"
        />
        <figcaption
          class="absolute inset-x-0 bottom-0 flex items-center justify-between gap-2 bg-gradient-to-t from-black/70 to-transparent px-3 pb-2 pt-8 opacity-0 transition group-hover:opacity-100"
        >
          <span class="min-w-0 flex-1 truncate text-xs text-white/90" :title="item.prompt">
            {{ item.prompt }}
          </span>
          <span class="flex shrink-0 items-center gap-1">
            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-full bg-white/20 text-white transition hover:bg-white/35"
              :aria-label="t('draw.gallery.preview')"
              :title="t('draw.gallery.preview')"
              @click="preview = item"
            >
              <Icon name="eye" size="sm" />
            </button>
            <button
              type="button"
              class="inline-flex h-8 w-8 items-center justify-center rounded-full bg-white/20 text-white transition hover:bg-white/35"
              :aria-label="t('draw.gallery.download')"
              :title="t('draw.gallery.download')"
              @click="download(item)"
            >
              <Icon name="download" size="sm" />
            </button>
          </span>
        </figcaption>
      </figure>
    </div>

    <DrawLightbox :src="preview?.src ?? ''" :alt="preview?.prompt ?? ''" @close="preview = null" />
  </div>
</template>

<script setup lang="ts">
/**
 * 结果画廊：网格展示 + 点击放大（lightbox）+ 下载。
 * url 型经 fetch→blob 保下载名（跨域失败时退化为新窗口打开）；
 * b64 型直接用 data URL 下载。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import DrawLightbox from './DrawLightbox.vue'
import type { DrawImageItem } from './types'

interface Props {
  items: DrawImageItem[]
}

defineProps<Props>()

const { t } = useI18n()
const appStore = useAppStore()

const preview = ref<DrawImageItem | null>(null)

function triggerDownload(href: string, name: string): void {
  const anchor = document.createElement('a')
  anchor.href = href
  anchor.download = name
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
}

async function download(item: DrawImageItem): Promise<void> {
  const name = `${item.id}.png`
  // b64：data URL 直接下载；url：fetch→blob 以保留自定义文件名
  if (item.kind === 'dataUrl' && item.dataUrl) {
    triggerDownload(item.dataUrl, name)
    return
  }
  if (item.kind === 'url' && item.url) {
    try {
      const response = await fetch(item.url)
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const blob = await response.blob()
      const objectUrl = URL.createObjectURL(blob)
      try {
        triggerDownload(objectUrl, name)
      } finally {
        URL.revokeObjectURL(objectUrl)
      }
      return
    } catch {
      // 跨域等导致 fetch 失败：退化为新窗口打开原图
      window.open(item.url, '_blank', 'noopener')
      appStore.showError(t('draw.gallery.downloadFailed'))
    }
  }
}
</script>
