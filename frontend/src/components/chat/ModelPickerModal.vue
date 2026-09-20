<template>
  <BaseDialog
    :show="show"
    :title="t('chat.modelPicker.title')"
    width="normal"
    :close-on-click-outside="true"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <!-- Search -->
      <SearchInput v-model="keyword" :placeholder="t('chat.modelPicker.searchPlaceholder')" />

      <!-- Grouped model list -->
      <div class="max-h-[50vh] space-y-4 overflow-y-auto pr-1">
        <p
          v-if="groupedModels.length === 0"
          class="py-10 text-center text-sm text-gray-400 dark:text-dark-500"
        >
          {{ t('chat.modelPicker.noResults') }}
        </p>

        <div v-for="group in groupedModels" :key="group.vendor">
          <!-- Group header: vendor + count -->
          <div class="mb-1.5 flex items-center justify-between px-1">
            <span class="text-xs font-semibold uppercase tracking-wide text-gray-500 dark:text-dark-400">
              {{ group.vendor }}
            </span>
            <span class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('chat.modelPicker.modelCount', { count: group.items.length }) }}
            </span>
          </div>

          <div class="space-y-1">
            <button
              v-for="item in group.items"
              :key="item.model"
              type="button"
              class="flex w-full items-center justify-between gap-3 rounded-xl border px-3.5 py-2.5 text-left transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30"
              :class="
                item.api_only
                  ? 'cursor-not-allowed border-transparent bg-gray-50 opacity-60 dark:bg-dark-800/60'
                  : item.model === selected
                    ? 'border-primary-500/60 bg-primary-50 hover:bg-primary-50 dark:border-primary-500/50 dark:bg-primary-900/20 dark:hover:bg-primary-900/30'
                    : 'border-gray-200 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-800'
              "
              :disabled="item.api_only"
              @click="!item.api_only && select(item.model)"
            >
              <span class="min-w-0">
                <span class="flex flex-wrap items-center gap-2">
                  <span
                    class="text-sm font-semibold"
                    :class="
                      item.api_only
                        ? 'text-gray-400 dark:text-dark-500'
                        : 'text-gray-900 dark:text-gray-100'
                    "
                  >
                    {{ item.display_name }}
                  </span>
                  <span
                    v-if="item.api_only"
                    class="rounded-full bg-gray-200 px-2 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-400"
                  >
                    {{ t('chat.modelPicker.apiOnlyBadge') }}
                  </span>
                </span>
                <span
                  class="mt-0.5 line-clamp-2 block text-xs text-gray-500 dark:text-dark-400"
                  :title="item.description"
                >
                  {{ item.description }}
                </span>
              </span>
              <Icon
                v-if="item.model === selected && !item.api_only"
                name="check"
                size="sm"
                class="shrink-0 text-primary-600 dark:text-primary-400"
              />
            </button>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Icon from '@/components/icons/Icon.vue'
import type { WebChatModelInfo } from '@/api/webChat'

const props = withDefaults(
  defineProps<{
    show: boolean
    models: WebChatModelInfo[]
    selected?: string
  }>(),
  {
    selected: '',
  }
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', model: string): void
}>()

const { t } = useI18n()

const keyword = ref('')

// Reset the filter every time the picker opens
watch(
  () => props.show,
  (show) => {
    if (show) {
      keyword.value = ''
    }
  }
)

const groupedModels = computed<{ vendor: string; items: WebChatModelInfo[] }[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  const groups = new Map<string, WebChatModelInfo[]>()

  for (const item of props.models) {
    if (kw) {
      const haystack = `${item.display_name} ${item.model} ${item.vendor}`.toLowerCase()
      if (!haystack.includes(kw)) {
        continue
      }
    }
    const existing = groups.get(item.vendor)
    if (existing) {
      existing.push(item)
    } else {
      groups.set(item.vendor, [item])
    }
  }

  return Array.from(groups.entries()).map(([vendor, items]) => ({ vendor, items }))
})

function select(model: string): void {
  emit('select', model)
  emit('close')
}
</script>
