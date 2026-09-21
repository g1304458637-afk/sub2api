<template>
  <div class="pricing-bg" aria-hidden="true">
    <!-- 背景视频：assets/muc/ 下放入任意 .mp4/.webm 即自动启用（boomerang 往返播放，不循环跳变）。
         无视频资产时呈现同语言的 CSS 极光 + 红色辉光动态背景，保持原始深色视觉（不铺红色 overlay）。 -->
    <video
      v-if="videoSrc"
      ref="videoRef"
      class="pricing-bg__video"
      :src="videoSrc"
      muted
      playsinline
      autoplay
      preload="auto"
    ></video>
    <template v-else>
      <div class="pricing-bg__aurora pricing-bg__aurora--a"></div>
      <div class="pricing-bg__aurora pricing-bg__aurora--b"></div>
      <div class="pricing-bg__aurora pricing-bg__aurora--c"></div>
    </template>
    <div class="pricing-bg__glow pricing-bg__glow--top"></div>
    <div class="pricing-bg__glow pricing-bg__glow--bottom"></div>
    <div class="pricing-bg__vignette"></div>
    <div class="pricing-bg__grain"></div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

// Drop-in 视频发现：构建期无对应文件时返回空 map，不报错；命中则异步解析资源 URL。
const videoModules = import.meta.glob('../../assets/muc/*.{mp4,webm,mov}', {
  query: '?url',
  import: 'default'
}) as Record<string, () => Promise<string>>

const videoSrc = ref('')
const videoRef = ref<HTMLVideoElement | null>(null)
let reverseRaf = 0
let reachedEnd = false

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

// boomerang：正向 play() 到结尾附近后暂停，用 rAF 逐帧倒放 currentTime，回到起点再正向播放。
function startReverseSeek(video: HTMLVideoElement) {
  if (reverseRaf) return
  video.pause()
  const step = () => {
    if (!videoRef.value) {
      reverseRaf = 0
      return
    }
    const next = video.currentTime - 1 / 30
    if (next <= 0) {
      reverseRaf = 0
      video.currentTime = 0
      reachedEnd = false
      void video.play().catch(() => {})
      return
    }
    video.currentTime = next
    reverseRaf = requestAnimationFrame(step)
  }
  reverseRaf = requestAnimationFrame(step)
}

function onTimeUpdate() {
  const video = videoRef.value
  if (!video || !video.duration) return
  if (!reachedEnd && video.currentTime >= video.duration - 0.06) {
    reachedEnd = true
    startReverseSeek(video)
  }
}

function onEnded() {
  const video = videoRef.value
  if (video) startReverseSeek(video)
}

function bindVideo(video: HTMLVideoElement) {
  if (window.matchMedia(REDUCED_MOTION_QUERY).matches) {
    video.pause()
    return
  }
  video.addEventListener('timeupdate', onTimeUpdate)
  video.addEventListener('ended', onEnded)
  void video.play().catch(() => {})
}

watch(videoSrc, async (src) => {
  if (!src) return
  await nextTick()
  if (videoRef.value) bindVideo(videoRef.value)
})

onMounted(async () => {
  const loader = Object.values(videoModules)[0]
  if (!loader) return
  try {
    videoSrc.value = await loader()
  } catch {
    videoSrc.value = ''
  }
})

onBeforeUnmount(() => {
  const video = videoRef.value
  if (video) {
    video.removeEventListener('timeupdate', onTimeUpdate)
    video.removeEventListener('ended', onEnded)
  }
  if (reverseRaf) cancelAnimationFrame(reverseRaf)
})
</script>

<style scoped>
.pricing-bg {
  position: absolute;
  inset: 0;
  overflow: hidden;
  background: var(--muc-bg-primary);
}

.pricing-bg__video {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  /* 视频保持原始视觉，只压暗到可读层级，不叠红色 */
  filter: brightness(0.72) saturate(0.9);
}

/* ---- 无视频时的 CSS 极光降级：大面积深色 + 极弱红移 ---- */
.pricing-bg__aurora {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.5;
  will-change: transform;
  animation: pricing-aurora-drift 26s ease-in-out infinite alternate;
}

.pricing-bg__aurora--a {
  width: 62vw;
  height: 42vh;
  left: -14vw;
  top: -16vh;
  background: radial-gradient(closest-side, rgba(255, 255, 255, 0.06), transparent 70%);
}

.pricing-bg__aurora--b {
  width: 52vw;
  height: 46vh;
  right: -16vw;
  bottom: -18vh;
  background: radial-gradient(closest-side, var(--muc-red-soft), transparent 72%);
  animation-delay: -9s;
}

.pricing-bg__aurora--c {
  width: 34vw;
  height: 30vh;
  left: 30vw;
  top: 40vh;
  background: radial-gradient(closest-side, rgba(214, 180, 106, 0.05), transparent 70%);
  animation-delay: -17s;
}

@keyframes pricing-aurora-drift {
  from {
    transform: translate3d(0, 0, 0) scale(1);
  }
  to {
    transform: translate3d(4vw, -3vh, 0) scale(1.08);
  }
}

/* ---- 克制的红色辉光：局部 radial，不做整面红 ---- */
.pricing-bg__glow {
  position: absolute;
  pointer-events: none;
}

.pricing-bg__glow--top {
  left: 50%;
  top: -22vh;
  width: 72vw;
  height: 46vh;
  transform: translateX(-50%);
  background: radial-gradient(closest-side, var(--muc-red-glow), transparent 70%);
  opacity: 0.55;
}

.pricing-bg__glow--bottom {
  left: -10vw;
  bottom: -26vh;
  width: 50vw;
  height: 44vh;
  background: radial-gradient(closest-side, rgba(127, 22, 34, 0.22), transparent 72%);
  opacity: 0.5;
}

.pricing-bg__vignette {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(120% 90% at 50% 10%, transparent 55%, rgba(0, 0, 0, 0.55) 100%),
    linear-gradient(to bottom, rgba(7, 7, 8, 0.35), transparent 30%, transparent 62%, rgba(7, 7, 8, 0.72));
}

.pricing-bg__grain {
  position: absolute;
  inset: 0;
  opacity: 0.05;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
}

@media (prefers-reduced-motion: reduce) {
  .pricing-bg__aurora {
    animation: none;
  }
}
</style>
