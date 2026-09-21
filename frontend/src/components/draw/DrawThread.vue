<template>
  <div>
    <!-- 空状态：无任何会话轮 -->
    <div
      v-if="turns.length === 0 && !pendingPrompt"
      class="flex flex-col items-center justify-center px-6 py-16 text-center"
    >
      <div class="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800">
        <Icon name="sparkles" size="lg" class="text-gray-400 dark:text-gray-500" />
      </div>
      <p class="text-sm text-gray-400 dark:text-gray-500">{{ t('draw.gallery.empty') }}</p>
    </div>

    <div v-else class="flex flex-col gap-8">
      <!-- 已完成的会话轮 -->
      <div v-for="turn in turns" :key="turn.id" class="flex flex-col gap-3">
        <!-- 用户提示词：与聊天页一致的右对齐气泡 -->
        <div class="flex justify-end">
          <div
            class="max-w-[85%] whitespace-pre-wrap break-words rounded-2xl bg-gradient-to-r from-primary-500 to-primary-600 px-4 py-2.5 text-sm leading-relaxed text-white shadow-sm"
          >
            {{ turn.prompt }}
          </div>
        </div>

        <!-- 生成失败：错误信息 + 重试 -->
        <div v-if="turn.error" class="flex flex-col items-start gap-1.5">
          <p class="text-sm text-red-500 dark:text-red-400">{{ turn.error }}</p>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-200"
            @click="$emit('retry', turn)"
          >
            <Icon name="refresh" size="xs" />
            {{ t('draw.thread.turnRetry') }}
          </button>
        </div>

        <!-- 结果图：点击图片选为编辑上下文（选中带主色描边） -->
        <div v-else-if="turn.images.length > 0" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <figure
            v-for="item in turn.images"
            :key="item.id"
            class="group relative overflow-hidden rounded-2xl bg-gray-50 transition-shadow dark:bg-dark-800"
            :class="contextImageId === item.id ? 'ring-2 ring-primary-500 dark:ring-primary-400' : ''"
          >
            <img
              :src="item.src"
              :alt="item.prompt"
              loading="lazy"
              class="aspect-square w-full cursor-pointer object-cover"
              :title="t('draw.form.editWithContext')"
              @click="$emit('select-context', item)"
            />
            <figcaption
              class="pointer-events-none absolute inset-x-0 bottom-0 flex items-center justify-between gap-2 bg-gradient-to-t from-black/70 to-transparent px-3 pb-2 pt-8 opacity-0 transition group-hover:opacity-100"
            >
              <span class="min-w-0 flex-1 truncate text-xs text-white/90" :title="item.prompt">
                {{ item.prompt }}
              </span>
              <span class="pointer-events-auto flex shrink-0 items-center gap-1">
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
      </div>

      <!-- 生成中占位：提示词先上屏 + 占位卡片 -->
      <div v-if="pendingPrompt" class="flex flex-col gap-3">
        <div class="flex justify-end">
          <div
            class="max-w-[85%] whitespace-pre-wrap break-words rounded-2xl bg-gradient-to-r from-primary-500 to-primary-600 px-4 py-2.5 text-sm leading-relaxed text-white shadow-sm"
          >
            {{ pendingPrompt }}
          </div>
        </div>
        <div
          class="flex items-center gap-2 rounded-2xl bg-gray-50 px-4 py-3 text-sm text-gray-400 dark:bg-dark-800 dark:text-dark-500"
          role="status"
        >
          <Icon name="refresh" size="sm" class="animate-spin" />
          {{ t('draw.thread.generatingPlaceholder') }}
        </div>
      </div>
    </div>

    <DrawLightbox :src="preview?.src ?? ''" :alt="preview?.prompt ?? ''" @close="preview = null" />
  </div>
</template>

<script setup lang="ts">
/**
 * 绘图会话线程：按时间顺序渲染每一轮（右对齐用户提示词气泡 + 结果图网格）。
 * - 点击图片将其选为编辑上下文（选中的图带主色描边），选中逻辑由 DrawView 持有；
 * - 失败的轮展示错误与重试；生成中的轮由 pendingPrompt 驱动占位；
 * - url 型下载 fetch→blob 保下载名，b64 型直接 data URL 下载（沿用原画廊逻辑）。
 */
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import DrawLightbox from './DrawLightbox.vue'
import type { DrawImageItem, DrawTurn } from './types'

interface Props {
  turns: DrawTurn[]
  /** 正在生成中的轮次提示词；null 表示当前没有进行中的生成 */
  pendingPrompt: string | null
  /** 当前选为编辑上下文的图片 id */
  contextImageId: string | null
}

defineProps<Props>()

defineEmits<{
  (e: 'select-context', item: DrawImageItem): void
  (e: 'retry', turn: DrawTurn): void
}>()

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
