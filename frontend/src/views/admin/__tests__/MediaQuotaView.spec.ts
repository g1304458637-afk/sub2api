import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import MediaQuotaView from '../MediaQuotaView.vue'

const { getImageQuota, getMediaModelStats, getMediaPricing, listUsage } = vi.hoisted(() => ({
  getImageQuota: vi.fn(),
  getMediaModelStats: vi.fn(),
  getMediaPricing: vi.fn(),
  listUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    mediaQuota: { getImageQuota, getMediaModelStats, getMediaPricing },
    usage: { list: listUsage }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

// 图片额度端点三态的样例响应
const quotaReachable = {
  enabled: true,
  reachable: true,
  version: 'v1.2.0',
  uptime_s: 7200,
  budget: { limit: 100, used: 40, remaining: 60, window_ms: 10800000, reset_at: '2099-01-01T00:00:00Z' },
  busy: true,
  totals: { today: 12, last_24h: 30, success: 28, failed: 2 },
  recent: [],
  auth_ok: true,
  latency: { p50_ms: 120, p95_ms: 450 }
}
const quotaUnreachable = { enabled: true, reachable: false, error: 'bridge down' }
const quotaUnconfigured = { enabled: false }

// 全 null 定价（默认价兜底）
const nullPricing = {
  imagePrice1k: null,
  imagePrice2k: null,
  imagePrice4k: null,
  musicPricePerTrack: null,
  audioTtsPricePerMillionChars: null,
  audioRealtimePricePerMin: null,
  audioSttPricePerHour: null
}

// 显式分组定价
const explicitPricing = {
  imagePrice1k: 0.08,
  imagePrice2k: 0.2,
  imagePrice4k: 0.4,
  musicPricePerTrack: 0.6,
  audioTtsPricePerMillionChars: 15,
  audioRealtimePricePerMin: 0.1,
  audioSttPricePerHour: 0.36
}

// 按模型聚合统计样例：suno/audio 前缀多条 + 精确图片模型 + 无关模型
const modelStats = [
  { model: 'suno-v4', requests: 2, cost: 0.3, actual_cost: 0.3 },
  { model: 'suno-v4.5', requests: 1, cost: 0.2, actual_cost: 0.2 },
  { model: 'audio-tts-1', requests: 3, cost: 1.1, actual_cost: 1.1 },
  { model: 'audio-realtime', requests: 1, cost: 0.9, actual_cost: 0.9 },
  { model: 'gpt-image-2', requests: 4, cost: 0.8, actual_cost: 0.8 },
  { model: 'gpt-4o', requests: 10, cost: 5, actual_cost: 5 }
]

const usageRow = (overrides: Record<string, unknown>) => ({
  id: Math.floor(Math.random() * 100000),
  user_id: 1,
  model: 'gpt-image-2',
  created_at: '2026-09-22T08:00:00Z',
  duration_ms: 5200,
  image_count: 1,
  image_size: '1024x1024',
  total_cost: 0.08,
  ...overrides
})

// listUsage 双调用：带 model=gpt-image-2 的图片过滤请求 + 不带过滤的通用请求
const mockDefaultUsage = () => {
  listUsage.mockImplementation(async (params: { model?: string }) => {
    if (params?.model === 'gpt-image-2') {
      return { items: [usageRow({})], total: 1, page: 1, page_size: 10, pages: 1 }
    }
    return {
      items: [
        usageRow({ model: 'suno-v4', total_cost: 0.15, image_count: 0, image_size: null }),
        usageRow({ model: 'audio-tts-1', total_cost: 0.4, image_count: 0, image_size: null }),
        usageRow({ model: 'gpt-4o', total_cost: 0.99, image_count: 0, image_size: null })
      ],
      total: 3,
      page: 1,
      page_size: 100,
      pages: 1
    }
  })
}

const mountView = () => shallowMount(MediaQuotaView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' }
    }
  }
})

let wrapper: ReturnType<typeof mountView> | undefined

beforeEach(() => {
  vi.clearAllMocks()
  getImageQuota.mockResolvedValue(quotaReachable)
  getMediaModelStats.mockResolvedValue(modelStats)
  getMediaPricing.mockResolvedValue({ pricing: nullPricing, groupName: 'openai-main', found: true })
  mockDefaultUsage()
})

afterEach(() => wrapper?.unmount())

describe('media quota view', () => {
  it('renders the configured-and-reachable quota card with auth/busy/progress', async () => {
    wrapper = mountView()
    await flushPromises()

    expect(getImageQuota).toHaveBeenCalled()
    expect(wrapper.find('[data-test="image-quota-card"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="image-quota-unconfigured"]').exists()).toBe(false)

    // auth_ok=true → 绿点；busy=true → 徽标
    expect(wrapper.get('[data-test="auth-dot"]').classes()).toContain('bg-emerald-500')
    expect(wrapper.find('[data-test="busy-badge"]').exists()).toBe(true)

    // 进度条与已用/上限文本
    expect(wrapper.find('[data-test="quota-progress"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="quota-progress"]').text()).toContain('40')

    // 图片最近表来自 usage.list(model=gpt-image-2)
    expect(listUsage).toHaveBeenCalledWith(expect.objectContaining({ model: 'gpt-image-2' }))
    expect(wrapper.find('[data-test="image-recent"]').exists()).toBe(true)
  })

  it('renders the unreachable error banner when the proxy is enabled but unreachable', async () => {
    getImageQuota.mockResolvedValue(quotaUnreachable)
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="image-quota-card"]').exists()).toBe(false)
    const banner = wrapper.get('[data-test="image-quota-unreachable"]')
    expect(banner.text()).toContain('admin.mediaQuota.quota.unreachable')
    expect(banner.text()).toContain('bridge down')
  })

  it('renders the unconfigured guide when the proxy endpoint is disabled', async () => {
    getImageQuota.mockResolvedValue(quotaUnconfigured)
    wrapper = mountView()
    await flushPromises()

    const guide = wrapper.get('[data-test="image-quota-unconfigured"]')
    expect(guide.text()).toContain('admin.mediaQuota.quota.unconfigured')
    expect(guide.text()).toContain('admin.mediaQuota.quota.unconfiguredHint')
    expect(wrapper.find('[data-test="image-quota-card"]').exists()).toBe(false)
  })

  it('sums suno/audio prefixed models and matches gpt-image-2 exactly', async () => {
    wrapper = mountView()
    await flushPromises()

    // 音乐 = suno 前缀求和：requests 2+1=3，cost 0.3+0.2=0.5
    expect(wrapper.get('[data-test="music-today-requests"]').text()).toBe('3')
    expect(wrapper.get('[data-test="music-today-cost"]').text()).toContain('0.50')

    // 语音 = audio 前缀求和：requests 3+1=4，cost 1.1+0.9=2.0
    expect(wrapper.get('[data-test="audio-today-requests"]').text()).toBe('4')
    expect(wrapper.get('[data-test="audio-today-cost-summary"]').text()).toContain('2.00')

    // 图片 = gpt-image-2 精确：requests 4，cost 0.8
    expect(wrapper.get('[data-test="image-today-cost"]').text()).toContain('0.80')

    // 合计 0.5 + 2.0 + 0.8 = 3.3，gpt-4o 的 5.0 不计入
    expect(wrapper.get('[data-test="today-total"]').text()).toContain('3.30')
  })

  it('shows empty states alongside the unconfigured guide when there is no data', async () => {
    getImageQuota.mockResolvedValue(quotaUnconfigured)
    getMediaModelStats.mockResolvedValue([])
    listUsage.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 100, pages: 0 })
    wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="today-total"]').text()).toContain('0.00')
    expect(wrapper.find('[data-test="image-recent-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="music-recent-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="audio-recent-empty"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="image-quota-unconfigured"]').text()).toContain(
      'admin.mediaQuota.quota.unconfigured'
    )
  })

  it('falls back to default price labels when group pricing is null', async () => {
    wrapper = mountView()
    await flushPromises()

    const imagePricing = wrapper.get('[data-test="image-pricing"]').text()
    expect(imagePricing).toContain('admin.mediaQuota.pricing.defaultPrice')
    expect(imagePricing).not.toContain('$')

    expect(wrapper.get('[data-test="music-pricing"]').text()).toContain(
      'admin.mediaQuota.pricing.defaultMusicPrice'
    )

    const audioPricing = wrapper.get('[data-test="audio-pricing"]').text()
    expect(audioPricing).toContain('admin.mediaQuota.pricing.defaultPrice')
  })

  it('shows explicit group prices when pricing fields are set', async () => {
    getMediaPricing.mockResolvedValue({ pricing: explicitPricing, groupName: 'openai-paid', found: true })
    wrapper = mountView()
    await flushPromises()

    const imagePricing = wrapper.get('[data-test="image-pricing"]').text()
    expect(imagePricing).toContain('0.08')
    expect(imagePricing).toContain('0.40')
    expect(imagePricing).not.toContain('admin.mediaQuota.pricing.defaultPrice')

    expect(wrapper.get('[data-test="music-pricing"]').text()).toContain('0.60')

    const audioPricing = wrapper.get('[data-test="audio-pricing"]').text()
    expect(audioPricing).toContain('15.00')
    expect(audioPricing).toContain('0.36')
  })

  it('filters music/audio recent tables by model prefix on the client', async () => {
    wrapper = mountView()
    await flushPromises()

    // 通用拉取不带 model 过滤，前端按前缀拆分：suno-v4 → 音乐，audio-tts-1 → 语音，gpt-4o 两边都不进
    expect(listUsage).toHaveBeenCalledWith(expect.objectContaining({ page: 1, page_size: 100 }))
    const musicTable = wrapper.get('[data-test="music-recent"]').text()
    expect(musicTable).toContain('suno-v4')
    expect(musicTable).not.toContain('audio-tts-1')

    const audioTable = wrapper.get('[data-test="audio-recent"]').text()
    expect(audioTable).toContain('audio-tts-1')
    expect(audioTable).not.toContain('suno-v4')
    expect(audioTable).not.toContain('gpt-4o')
  })

  it('reloads all data when the refresh button is clicked', async () => {
    wrapper = mountView()
    await flushPromises()

    expect(getImageQuota).toHaveBeenCalledTimes(1)
    await wrapper.get('[data-test="refresh"]').trigger('click')
    await flushPromises()

    expect(getImageQuota).toHaveBeenCalledTimes(2)
    expect(getMediaModelStats).toHaveBeenCalledTimes(2)
  })
})
