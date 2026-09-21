<template>
  <div class="pricing-bg" aria-hidden="true">
    <!-- 背景视频：assets/muc/ 下放入任意 .mp4/.webm 即自动启用（boomerang 往返播放，不循环跳变）。
         加载失败或无资产时回退同语言 CSS 极光背景，Pricing 功能不受影响；不铺红色 overlay。 -->
    <video
      v-if="videoActive"
      ref="videoRef"
      class="pricing-bg__video"
      :src="videoUrl"
      muted
      playsinline
      autoplay
      preload="auto"
      @timeupdate="onTimeUpdate"
      @ended="onEnded"
      @seeked="onSeeked"
      @error="onError"
      @loadedmetadata="onReady"
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
import { computed, onBeforeUnmount, ref } from 'vue'
// 背景视频资产：同名替换 src/assets/muc/pricing-bg.mp4 即可。
import pricingBgVideoUrl from '../../assets/muc/pricing-bg.mp4?url'

const videoUrl = ref(pricingBgVideoUrl)
const videoFailed = ref(false)
const videoActive = computed(() => !!videoUrl.value && !videoFailed.value)
const videoRef = ref<HTMLVideoElement | null>(null)

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

// ── Boomerang throttled seek ──
// 正向 play() 播到结尾附近后暂停，按步倒放 currentTime，回到起点再正向播放。
// seekPending 防止上一帧 seek 未落地时叠加新 seek（避免 seek 堆积）。
// 用 setInterval 而非 rAF 驱动：后台/遮挡标签页 rAF 会停发导致倒放冻结；
// interval 在后台被节流到 ≥1s 仍能渐进，可见时 33ms 平滑，且无常驻高 CPU。
let direction = 1 // 1=正向播放中，-1=倒放 seek 中
let rafId = 0 // 倒放循环句柄（interval id，沿用原名供卸载清理）
let seekPending = false
let reachedEnd = false
let lastTs = 0
const SEEK_STEP_SECONDS = 1 / 30
const SEEK_INTERVAL_MS = 33

function cancelSeekLoop() {
  if (rafId) {
    clearInterval(rafId)
    rafId = 0
  }
  seekPending = false
}

function stepReverse() {
  const video = videoRef.value
  if (!video || direction !== -1) {
    cancelSeekLoop()
    return
  }
  if (seekPending) return
  const now = performance.now()
  if (!lastTs) lastTs = now
  if (now - lastTs < SEEK_INTERVAL_MS) return
  lastTs = now
  const next = video.currentTime - SEEK_STEP_SECONDS
  if (next <= 0) {
    video.currentTime = 0
    direction = 1
    reachedEnd = false
    cancelSeekLoop()
    void video.play().catch(() => {})
    return
  }
  seekPending = true
  video.currentTime = next
}

function startReverseSeek() {
  const video = videoRef.value
  if (!video || direction === -1) return
  video.pause()
  direction = -1
  lastTs = 0
  seekPending = false
  rafId = window.setInterval(stepReverse, SEEK_INTERVAL_MS)
}

function onSeeked() {
  seekPending = false
}

function onTimeUpdate() {
  const video = videoRef.value
  if (!video || !video.duration || direction !== 1 || reachedEnd) return
  if (video.currentTime >= video.duration - 0.06) {
    reachedEnd = true
    startReverseSeek()
  }
}

function onEnded() {
  if (direction === 1) startReverseSeek()
}

function onError() {
  videoFailed.value = true
  videoUrl.value = ''
}

// 事件经模板绑定；元数据就绪后按 reduce-motion 决定播放或静帧
function onReady() {
  const video = videoRef.value
  if (!video) return
  direction = 1
  reachedEnd = false
  if (window.matchMedia(REDUCED_MOTION_QUERY).matches) {
    // reduce-motion：停在当前帧，不做往返
    video.pause()
  }
}

onBeforeUnmount(() => {
  cancelSeekLoop()
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

/* Mobile：关闭高成本噪点纹理 */
@media (max-width: 768px) {
  .pricing-bg__grain {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .pricing-bg__aurora {
    animation: none;
  }
}
</style>
