<template>
  <AppLayout>
    <div class="mx-auto max-w-2xl space-y-6">
      <!-- Introduction Card -->
      <div
        class="card border-primary-200 bg-primary-50 dark:border-primary-800/50 dark:bg-primary-900/20"
      >
        <div class="p-6">
          <div class="flex items-start gap-4">
            <div
              class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 dark:bg-primary-900/30"
            >
              <Icon name="beaker" size="md" class="text-primary-600 dark:text-primary-400" />
            </div>
            <div class="flex-1">
              <h3 class="text-sm font-semibold text-primary-800 dark:text-primary-300">
                {{ t('research.intro.title') }}
              </h3>
              <ul
                class="mt-2 list-inside list-disc space-y-1 text-sm text-primary-700 dark:text-primary-400"
              >
                <li>{{ t('research.intro.rule1') }}</li>
                <li>{{ t('research.intro.rule2') }}</li>
                <li>{{ t('research.intro.rule3') }}</li>
                <li>{{ t('research.intro.rule4') }}</li>
              </ul>
            </div>
          </div>
        </div>
      </div>

      <!-- Application Form -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('research.form.title') }}
          </h2>
        </div>
        <div class="p-6">
          <ResearchApplicationForm @submitted="fetchApplications" />
        </div>
      </div>

      <!-- My Applications -->
      <div class="card">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('research.list.title') }}
          </h2>
        </div>
        <div class="p-6">
          <!-- Loading State -->
          <div v-if="loadingApplications" class="flex items-center justify-center py-8">
            <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
          </div>

          <!-- Application List -->
          <div v-else-if="applications.length > 0" class="space-y-4">
            <ResearchApplicationCard
              v-for="application in applications"
              :key="application.id"
              :application="application"
            />
          </div>

          <!-- Empty State -->
          <div v-else class="empty-state py-8">
            <div
              class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gray-100 dark:bg-dark-800"
            >
              <Icon name="document" size="xl" class="text-gray-400 dark:text-dark-500" />
            </div>
            <p class="text-sm font-medium text-gray-500 dark:text-dark-400">
              {{ t('research.list.empty') }}
            </p>
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
              {{ t('research.list.emptyHint') }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ResearchApplicationForm from '@/components/research/ResearchApplicationForm.vue'
import ResearchApplicationCard from '@/components/research/ResearchApplicationCard.vue'
import { researchAPI, type ResearchApplication } from '@/api/research'

const { t } = useI18n()
const appStore = useAppStore()

const applications = ref<ResearchApplication[]>([])
const loadingApplications = ref(false)

const fetchApplications = async () => {
  loadingApplications.value = true
  try {
    applications.value = await researchAPI.listMyApplications()
  } catch (error) {
    console.error('Failed to fetch research applications:', error)
    appStore.showError(t('research.loadFailed'))
  } finally {
    loadingApplications.value = false
  }
}

onMounted(() => {
  fetchApplications()
})
</script>
