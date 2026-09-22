<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { campusScene, campusVideos } from '@/brand/scenes'
import CampusParticles from './CampusParticles.vue'

const route = useRoute()
const scene = computed(() => campusScene(route.path))
const source = computed(() => scene.value in campusVideos ? campusVideos[scene.value as keyof typeof campusVideos] : undefined)
const video = ref<HTMLVideoElement>()
const reduced = ref(window.matchMedia('(prefers-reduced-motion: reduce)').matches)
const hidden = ref(document.hidden)
const failed = ref(false)
const paused = ref(false)
const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
const staticScene = computed(() => reduced.value || paused.value || hidden.value)

function syncPlayback() {
  if (!video.value) return
  if (staticScene.value) video.value.pause()
  else void video.value.play().catch(() => { /* Autoplay can be denied; retain the poster. */ })
}
function onMotionChange() { reduced.value = motionQuery.matches }
function onVisibilityChange() { hidden.value = document.hidden }
watch(staticScene, syncPlayback)
watch(scene, () => { failed.value = false })
onMounted(() => {
  motionQuery.addEventListener('change', onMotionChange)
  document.addEventListener('visibilitychange', onVisibilityChange)
})
onBeforeUnmount(() => {
  motionQuery.removeEventListener('change', onMotionChange)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  video.value?.pause()
})
</script>

<template>
  <div class="campus-backdrop" :data-scene="scene" aria-hidden="true">
    <video
      v-if="source && !failed"
      :key="source"
      ref="video"
      :src="source"
      :autoplay="!staticScene"
      :poster="scene === 'globe' ? 'https://d2ol7oe51mr4n9.cloudfront.net/user_38xzZboKViGWJOttwIXH07lWA1P/82e7eb75-c65f-490a-99b5-f3d1cad54200.webp' : undefined"
      muted loop playsinline disablepictureinpicture preload="metadata"
      @loadeddata="syncPlayback" @error="failed = true"
    />
    <CampusParticles v-if="scene === 'particles'" :paused="staticScene" />
    <div class="campus-backdrop__veil" />
  </div>
  <button
    v-if="scene !== 'quiet' && !reduced && !failed"
    type="button" class="campus-motion liquid-glass"
    :aria-pressed="paused" @click="paused = !paused"
  >{{ paused ? '播放背景' : '暂停背景' }}</button>
</template>

<style scoped>
.campus-backdrop { position: fixed; inset: 0; z-index: -1; pointer-events: none; overflow: hidden; background: hsl(260 87% 3%); }
.campus-backdrop video, .campus-backdrop iframe { width: 100%; height: 100%; border: 0; object-fit: cover; }
.campus-backdrop[data-scene="globe"] video { object-position: 51% 8%; }
.campus-backdrop[data-scene="particles"] { background: #010a16; }
.campus-backdrop[data-scene="orbit"] { background: #060d19; }
.campus-backdrop__veil { position: absolute; inset: 0; background: linear-gradient(180deg, rgba(5,3,12,.28), rgba(5,3,12,.50) 50%, rgba(5,3,12,.82)); }
.campus-backdrop[data-scene="particles"] .campus-backdrop__veil { background: rgba(2,6,16,.48); }
.campus-backdrop[data-scene="orbit"] .campus-backdrop__veil { background: linear-gradient(180deg, rgba(3,5,14,.35), rgba(3,5,14,.66) 50%, rgba(3,5,14,.65)); }
.campus-motion { position: fixed; right: 18px; bottom: 14px; z-index: 25; padding: 8px 14px; border-radius: 999px; color: #eee; font-size: 12px; min-height: 36px; background-color: rgba(8,8,15,.8); }
.campus-motion:focus-visible { outline: 2px solid #87fb89; outline-offset: 3px; }
@media (max-width: 640px) { .campus-motion { right: 12px; bottom: 10px; min-height: 44px; } }
</style>
