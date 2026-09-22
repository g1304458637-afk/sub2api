<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6 px-4 py-6">
      <!-- 品牌横幅：主视觉 + 校名（cover 裁切，不拉伸） -->
      <section class="relative overflow-hidden rounded-2xl shadow-lg">
        <img :src="brand.heroImg" :alt="brand.heroAlt" class="h-64 w-full object-cover object-top sm:h-72" />
        <div class="absolute inset-0 bg-gradient-to-t from-black/75 via-black/25 to-transparent"></div>
        <div class="absolute bottom-0 left-0 right-0 p-6 text-white">
          <h1 class="text-2xl font-bold tracking-wide sm:text-3xl">{{ brand.page.heading }}</h1>
          <p class="mt-1 text-sm opacity-90">{{ brand.page.subheading }}</p>
        </div>
      </section>

      <!-- 连接卡片 -->
      <section class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{{ brand.page.connectCardTitle }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ brand.page.connectCardDesc }}
            </p>
          </div>
          <button
            class="shrink-0 rounded-xl px-6 py-3 text-sm font-semibold text-white shadow transition disabled:cursor-not-allowed disabled:opacity-60"
            :style="{ backgroundColor: brand.primary }"
            :disabled="state === 'issuing' || state === 'opening'"
            @click="connect"
          >
            {{ state === 'issuing' ? '正在签发授权码…' : state === 'opening' ? brand.page.connectButtonOpening : brand.page.connectButtonIdle }}
          </button>
        </div>

        <div v-if="state === 'fallback'" class="mt-4 rounded-xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-700 dark:bg-amber-900/30 dark:text-amber-200">
          <p class="font-medium">{{ brand.page.fallbackTitle }}</p>
          <p class="mt-1">{{ brand.page.fallbackBody }}</p>
        </div>
        <p v-if="state === 'opening'" class="mt-4 text-sm text-gray-500 dark:text-gray-400">
          {{ brand.page.openingHint }}
        </p>
      </section>

      <!-- 下载卡片 -->
      <section class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{{ brand.page.downloadCardTitle }}</h2>
          <span class="rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-600 dark:bg-dark-800 dark:text-gray-300">
            检测到：{{ detectedLabel }}
          </span>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ brand.page.downloadCardDesc }}
        </p>

        <!-- Campus Harness: 最新版本区块（数据来自 /downloads/latest-mucode.json，缺失时隐藏）；
             与主分支 /muc 页同步引入，主色随品牌。 -->
        <div
          v-if="latest"
          class="mt-3 rounded-xl border p-4 text-sm"
          :style="{ borderColor: hexAlpha(brand.primary, 0.35), backgroundColor: hexAlpha(brand.primary, 0.06) }"
        >
          <p class="font-medium" :style="{ color: brand.primary }">
            最新版本：{{ latest.version }}
            <span v-if="latest.releasedAt"> · 发布于 {{ latest.releasedAt.slice(0, 10) }}</span>
          </p>
          <p v-if="latest.notes" class="mt-1 text-xs leading-5 opacity-90">{{ latest.notes }}</p>
          <p class="mt-1 text-xs opacity-75">
            已装旧版时重新下载覆盖安装即可升级；完整性可用同目录 SHA256SUMS 校验。
          </p>
        </div>

        <div class="mt-4 grid gap-3 sm:grid-cols-3">
          <a
            v-for="opt in brand.downloads"
            :key="opt.file"
            :href="downloadUrl(opt.file)"
            :aria-disabled="!downloadUrl(opt.file)"
            :tabindex="downloadUrl(opt.file) ? 0 : -1"
            class="rounded-xl border p-4 transition"
            :class="opt.key === platform.key
              ? 'ring-1'
              : 'border-gray-200 hover:border-gray-300 dark:border-dark-700 dark:hover:border-dark-600'"
            :style="opt.key === platform.key
              ? { borderColor: brand.primary, backgroundColor: hexAlpha(brand.primary, 0.05), '--tw-ring-color': brand.primary }
              : undefined"
          >
            <div class="flex items-center justify-between">
              <span class="text-sm font-medium text-gray-900 dark:text-gray-100">{{ opt.label }}</span>
              <span
                v-if="opt.key === platform.key && downloadUrl(opt.file)"
                class="rounded-full px-2 py-0.5 text-[10px] text-white"
                :style="{ backgroundColor: brand.primary }"
              >推荐</span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ downloadUrl(opt.file) ? `下载 ${latest?.version}` : '当前版本暂未提供此平台安装包' }}</p>
          </a>
        </div>
      </section>

      <!-- 使用步骤 -->
      <section class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">四步开始使用</h2>
        <ol class="mt-4 grid gap-4 text-sm text-gray-600 dark:text-gray-300 sm:grid-cols-4">
          <li v-for="(step, i) in brand.page.steps" :key="i" class="rounded-xl bg-gray-50 p-4 dark:bg-dark-800">
            <span
              class="mb-2 inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold text-white"
              :style="{ backgroundColor: brand.primary }"
            >{{ i + 1 }}</span>
            <p>{{ step }}</p>
          </li>
        </ol>
        <div class="mt-4 rounded-xl border border-amber-200 bg-amber-50 p-4 text-xs text-amber-800 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-200">
          <p class="font-medium">macOS 首次打开提示「已损坏」？</p>
          <p class="mt-1">安装包为开发测试签名（未公证），两种方式任选：
          ① 终端执行 <code class="rounded bg-black/10 px-1">xattr -dr com.apple.quarantine "{{ brand.page.xattrAppPath }}"</code>；
          ② 或右键 app → 打开，并在 系统设置 → 隐私与安全性 中点「仍要打开」。之后即可正常使用。</p>
        </div>
        <p class="mt-4 text-xs text-gray-400 dark:text-gray-500">
          凭据保存在本机系统钥匙串（macOS Keychain）；每个设备生成独立 Key，可在「API 密钥」页单独撤销。
        </p>
        <p class="mt-2 text-xs text-gray-400 dark:text-gray-500">
          <br />
          使用其他工具（Claude Code 等）？无需安装 {{ brand.page.desktopName }}——在「API 密钥」页自行创建 Key，
          网关地址：<code class="text-primary-700 dark:text-primary-300">{{ gatewayHint }}</code>
        </p>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
// 校园 Harness 共用下载与一键连接页（MUC / HUBU 品牌驱动；MUC 渲染与历史版本逐字节一致）
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { createCampusConnectCode } from '@/api/campus'
import { fetchLatestMucodeManifest, type MucodeManifest } from '@/utils/mucUpdate'
import type { BrandConfig } from '@/brand'
import { useAppStore } from '@/stores/app'
const appStore = useAppStore()

type PlatformKey = 'mac-arm' | 'mac-intel' | 'win' | 'other'

const props = defineProps<{ brand: BrandConfig }>()

const platform = ref<{ key: PlatformKey; label: string }>({ key: 'mac-arm', label: 'macOS Apple Silicon' })
const detectedLabel = computed(() => platform.value.label)
const gatewayHint = `${location.origin}/v1`
const state = ref<'idle' | 'issuing' | 'opening' | 'fallback'>('idle')
const latest = ref<MucodeManifest | null>(null)

function downloadUrl(file: string): string | undefined {
  // 与网关同源；部署时将 dist/ 下的安装包挂载到 /downloads/ 路径
  const option = props.brand.downloads.find((item) => item.file === file)
  const target = option?.key === 'mac-arm' ? 'mac-arm64' : option?.key === 'mac-intel' ? 'mac-x64' : 'win-x64'
  const artifact = latest.value?.downloads[target]?.file
  // Never fall back to a stale fixed-name installer under a newer version heading.
  return artifact ? `${props.brand.downloadBase}/${encodeURIComponent(artifact)}` : undefined
}

function hexAlpha(hex: string, alpha: number): string {
  const n = hex.replace('#', '')
  const r = parseInt(n.slice(0, 2), 16)
  const g = parseInt(n.slice(2, 4), 16)
  const b = parseInt(n.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function detectPlatform(): void {
  const ua = navigator.userAgent
  const isMac = /Macintosh|Mac OS X/i.test(ua)
  const isWin = /Windows/i.test(ua)
  if (isWin) {
    platform.value = { key: 'win', label: 'Windows x64' }
    return
  }
  if (!isMac) {
    platform.value = { key: 'other', label: '未识别的平台' }
    return
  }
  // Apple Silicon 探测：优先 User-Agent Client Hints，默认 arm64
  const uad = (navigator as unknown as { userAgentData?: { getHighEntropyValues?: (hints: string[]) => Promise<{ architecture?: string }> } }).userAgentData
  if (!uad?.getHighEntropyValues) {
    platform.value = /MacIntel|Intel/i.test(navigator.platform) ? { key: 'mac-intel', label: 'macOS Intel' } : { key: 'mac-arm', label: 'macOS Apple Silicon' }
  }
  if (uad?.getHighEntropyValues) {
    uad
      .getHighEntropyValues(['architecture'])
      .then(({ architecture }) => {
        if (architecture === 'x86') platform.value = { key: 'mac-intel', label: 'macOS Intel' }
        else platform.value = { key: 'mac-arm', label: 'macOS Apple Silicon' }
      })
      .catch(() => {
        platform.value = { key: 'mac-arm', label: 'macOS Apple Silicon' }
      })
  }
}

async function connect(): Promise<void> {
  if (state.value === 'issuing' || state.value === 'opening') return
  state.value = 'issuing'
  try {
    const { code } = await createCampusConnectCode(props.brand)
    state.value = 'opening'
    // 唤起品牌桌面端；若 2.5s 内页面未失焦，视为未安装
    let left = false
    const onBlur = (): void => {
      left = true
    }
    window.addEventListener('blur', onBlur)
    window.location.href = `${props.brand.protocolScheme}://connect?code=${encodeURIComponent(code)}`
    window.setTimeout(() => {
      window.removeEventListener('blur', onBlur)
      // 未失焦 → 未安装提示；已失焦（唤起成功/用户切走）→ 回到 idle，
      // 否则按钮会永远停在"正在唤起"且禁用，只能刷新页面恢复。
      if (!left) state.value = 'fallback'
      else state.value = 'idle'
    }, 2500)
  } catch (error) {
    state.value = 'idle'
    appStore.showError(error instanceof Error ? error.message : '授权码签发失败，请稍后重试')
  }
}

onMounted(() => {
  detectPlatform()
  // 已看过下载引导，后续登录直达控制台
  localStorage.setItem(props.brand.seenKey, '1')
  // 最新版本信息拉取失败时静默（版本区块整体隐藏）
  void fetchLatestMucodeManifest(fetch, props.brand.manifestPath).then((m) => {
    if (m) latest.value = m
  })
})
</script>
