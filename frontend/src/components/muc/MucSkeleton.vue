<template>
  <div class="muc-skeleton" :style="{ height: typeof height === 'number' ? `${height}px` : height }" aria-hidden="true"></div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ height?: number | string }>(), { height: 16 })
</script>

<style scoped>
.muc-skeleton {
  width: 100%;
  border-radius: 10px;
  background: linear-gradient(
    100deg,
    rgba(255, 255, 255, 0.05) 40%,
    rgba(255, 255, 255, 0.11) 50%,
    rgba(255, 255, 255, 0.05) 60%
  );
  background-size: 200% 100%;
  animation: muc-skeleton-wave 1.4s ease infinite;
}

@keyframes muc-skeleton-wave {
  from {
    background-position: 120% 0;
  }
  to {
    background-position: -80% 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .muc-skeleton {
    animation: none;
  }
}
</style>
