<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
const props = defineProps<{ paused: boolean }>()
const canvas = ref<HTMLCanvasElement>()
let scene: { resume(): void; pause(): void; dispose(): void } | undefined
let disposed = false
watch(() => props.paused, (paused) => paused ? scene?.pause() : scene?.resume())
onMounted(async () => {
  try {
    const artwork = await import('@/assets/backgrounds/particles.js')
    if (disposed || !canvas.value) return
    scene = artwork.mountScene(canvas.value)
    if (!props.paused) scene?.resume()
  } catch {
    // Unsupported WebGL retains the dark static background; the app stays usable.
  }
})
onBeforeUnmount(() => { disposed = true; scene?.dispose() })
</script>
<template><canvas ref="canvas" class="campus-particles" aria-hidden="true" /></template>
<style scoped>.campus-particles { width: 100%; height: 100%; display: block; }</style>
