<template>
  <!-- ChatGPT 式会话区：标题行 + 内部滚动列表，夹在「大模型服务」与「我的」两组之间 -->
  <div class="flex min-h-0 flex-1 flex-col py-1">
    <div class="flex items-center justify-between gap-2 pl-[1.0625rem] pr-2.5 pb-0.5">
      <span
        class="text-[11px] font-semibold uppercase tracking-wider text-gray-400 dark:text-dark-500"
      >{{ t('chat.sidebar.history') }}</span>
      <button
        type="button"
        class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-primary-400"
        :title="t('chat.sidebar.newChat')"
        :aria-label="t('chat.sidebar.newChat')"
        @click="handleNewChat"
      >
        <Icon name="plus" size="sm" />
      </button>
    </div>

    <div class="scrollbar-hide min-h-0 flex-1 overflow-y-auto px-2.5">
      <p
        v-if="conversations.length === 0"
        class="px-2 py-3 text-xs leading-relaxed text-gray-400 dark:text-dark-500"
      >
        {{ t('chat.sidebar.empty') }}
      </p>
      <div v-else class="space-y-0.5 pb-1">
        <div
          v-for="conversation in conversations"
          :key="conversation.id"
          class="group relative flex items-center rounded-xl transition-colors"
          :class="
            conversation.id === currentId
              ? 'bg-gray-100 dark:bg-dark-800'
              : 'hover:bg-gray-100/70 dark:hover:bg-dark-800/60'
          "
        >
          <button
            type="button"
            class="min-w-0 flex-1 rounded-xl px-3 py-2 text-left focus:outline-none"
            @click="handleSelect(conversation.id)"
          >
            <p
              class="truncate text-[13px]"
              :class="
                conversation.id === currentId
                  ? 'font-medium text-primary-600 dark:text-primary-400'
                  : 'text-gray-600 dark:text-dark-300'
              "
            >
              {{ conversation.title || t('chat.title') }}
            </p>
          </button>
          <button
            type="button"
            class="mr-1 hidden rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-gray-200 hover:text-red-600 focus:outline-none group-hover:block dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-red-400"
            :title="t('chat.sidebar.delete')"
            :aria-label="t('chat.sidebar.delete')"
            @click.stop="handleDelete(conversation.id)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <p class="px-4 pb-1 pt-1.5 text-[11px] text-gray-400 dark:text-dark-500">
      {{ t('chat.sidebar.localOnly') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useChatHistoryStore } from '@/stores/chatHistory'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const historyStore = useChatHistoryStore()

const { conversations, currentId } = historyStore

onMounted(() => {
  historyStore.ensureHydrated()
})

function handleNewChat(): void {
  historyStore.newChat()
  if (route.path !== '/chat') {
    void router.push('/chat')
  }
}

function handleSelect(id: string): void {
  if (!historyStore.selectConversation(id)) {
    return
  }
  if (route.path !== '/chat') {
    void router.push('/chat')
  }
}

function handleDelete(id: string): void {
  historyStore.deleteConversation(id)
}
</script>
