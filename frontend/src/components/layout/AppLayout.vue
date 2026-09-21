<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <!-- Background Decoration：功能页低饱和民大红 radial（dark 下更明显） -->
    <div class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <!-- Sidebar -->
    <AppSidebar />

    <!-- Main Content Area -->
    <div
      class="relative min-h-screen transition-all duration-300"
      :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
    >
      <!-- Header -->
      <AppHeader />

      <!-- Main Content：全站统一内容节奏（--muc-content-max；Admin 允许更宽） -->
      <main class="muc-content p-4 md:p-6 lg:p-8" :class="{ 'muc-content--wide': isAdmin }">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const isAdmin = computed(() => authStore.user?.role === 'admin')

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

<style scoped>
/* 全站统一内容边界：用户页 --muc-content-max，Admin 更宽（--muc-content-max-wide）。
   只约束 max-width，不改变任何页面的内部布局。 */
.muc-content {
  width: 100%;
  max-width: calc(var(--muc-content-max, 1200px) + 2 * var(--muc-page-padding, 24px) + 2rem);
  margin-left: auto;
  margin-right: auto;
}

.muc-content--wide {
  max-width: calc(var(--muc-content-max-wide, 1400px) + 2 * var(--muc-page-padding, 24px) + 2rem);
}
</style>
