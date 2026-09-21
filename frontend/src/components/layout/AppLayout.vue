<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <!-- Background Decoration -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Mobile floating menu button (顶栏移除后，移动端从悬浮键打开侧边栏) -->
    <button
      type="button"
      class="fixed left-3 top-3 z-30 rounded-xl bg-white/90 p-2 text-gray-600 shadow-sm backdrop-blur transition-colors hover:bg-gray-100 lg:hidden dark:bg-dark-900/90 dark:text-dark-300 dark:hover:bg-dark-800"
      :aria-label="t('common.toggleMenu')"
      @click="appStore.toggleMobileSidebar()"
    >
      <svg
        class="h-5 w-5"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="1.5"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
      </svg>
    </button>

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Main Content -->
      <!-- flush：去掉内边距，交给页面自己铺满（聊天页满幅布局用）；pt-16 给移动端悬浮菜单键让位 -->
      <main :class="flush ? '' : 'p-4 pt-16 lg:p-8 lg:pt-8'">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

withDefaults(defineProps<{ flush?: boolean }>(), { flush: false })

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
