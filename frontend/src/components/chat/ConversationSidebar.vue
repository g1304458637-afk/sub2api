<template>
  <aside
    class="flex h-full w-64 shrink-0 flex-col border-r border-gray-100 bg-gray-50/70 dark:border-dark-700/60 dark:bg-dark-950/40"
  >
    <!-- Top: collapse + new chat -->
    <div class="flex items-center gap-1.5 p-3">
      <button
        type="button"
        class="rounded-xl p-2 text-gray-300 transition-colors hover:bg-gray-200/60 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-600 dark:hover:bg-dark-800 dark:hover:text-dark-300"
        :title="t('chat.sidebar.collapse')"
        :aria-label="t('chat.sidebar.collapse')"
        @click="emit('collapse')"
      >
        <Icon name="chevronLeft" size="sm" />
      </button>
      <button
        type="button"
        class="flex flex-1 items-center justify-center gap-1.5 rounded-xl border border-primary-200 bg-white/60 py-2 text-sm font-medium text-primary-700 transition-colors hover:border-primary-300 hover:bg-primary-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:border-primary-800 dark:bg-transparent dark:text-primary-300 dark:hover:border-primary-700 dark:hover:bg-primary-900/20"
        @click="emit('new')"
      >
        <Icon name="plus" size="sm" />
        <span class="truncate">{{ t('chat.sidebar.newChat') }}</span>
      </button>
    </div>

    <!-- Conversation list -->
    <div class="min-h-0 flex-1 overflow-y-auto px-3 pb-2">
      <p
        v-if="conversations.length === 0"
        class="px-2 py-6 text-center text-xs text-gray-400 dark:text-dark-500"
      >
        {{ t('chat.sidebar.empty') }}
      </p>
      <div v-else class="space-y-0.5">
        <div
          v-for="conversation in conversations"
          :key="conversation.id"
          class="group relative flex items-center rounded-xl transition-colors"
          :class="
            conversation.id === currentId
              ? 'bg-white shadow-sm ring-1 ring-gray-200/80 dark:bg-dark-800 dark:ring-dark-700'
              : 'hover:bg-gray-200/50 dark:hover:bg-dark-800/60'
          "
        >
          <button
            type="button"
            class="min-w-0 flex-1 rounded-xl px-3 py-2.5 text-left focus:outline-none"
            @click="emit('select', conversation.id)"
          >
            <p
              class="truncate text-[13px] font-medium"
              :class="
                conversation.id === currentId
                  ? 'text-primary-700 dark:text-primary-300'
                  : 'text-gray-600 dark:text-dark-300'
              "
            >
              {{ conversation.title || t('chat.title') }}
            </p>
            <p class="mt-0.5 truncate text-xs text-gray-400 dark:text-dark-500">
              {{ formatRelativeTime(new Date(conversation.updatedAt)) }}
            </p>
          </button>
          <button
            type="button"
            class="mr-1.5 hidden rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-200 hover:text-red-600 focus:outline-none group-hover:block dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-red-400"
            :title="t('chat.sidebar.delete')"
            :aria-label="t('chat.sidebar.delete')"
            @click.stop="emit('delete', conversation.id)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <!-- Footer hint -->
    <div class="px-4 pb-3 pt-2">
      <p class="text-[11px] leading-relaxed text-gray-400 dark:text-dark-500">
        {{ t('chat.sidebar.localOnly') }}
      </p>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatRelativeTime } from '@/utils/format'
import type { ChatConversation } from './types'

defineProps<{
  conversations: ChatConversation[]
  currentId: string | null
}>()

const emit = defineEmits<{
  (e: 'new'): void
  (e: 'select', id: string): void
  (e: 'delete', id: string): void
  (e: 'collapse'): void
}>()

const { t } = useI18n()
</script>
