import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaGroupSection from '../PlazaGroupSection.vue'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null })
}))

function tokenModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'gpt-5.6-sol',
    platform: 'openai',
    pricing: {
      billing_mode: 'token',
      input_price: 5e-6,
      output_price: 3e-5,
      cache_write_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 5e-6,
      output_price: 3e-5,
      cache_write_price: null,
      cache_read_price: null
    },
    display_pricing: {
      billing_mode: 'token',
      input_price: 1e-4,
      output_price: 8e-4,
      cache_write_price: null,
      cache_write_1h_price: null,
      cache_read_price: null,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    presentation_source: 'manual',
    ...overrides
  }
}

function group(overrides: Partial<ModelPlazaGroup> = {}): ModelPlazaGroup {
  return {
    id: 1,
    name: 'g',
    description: '',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    long_context_pricing_enabled: true,
    models: [tokenModel()],
    ...overrides
  }
}

function mountSection(g: ModelPlazaGroup) {
  return mount(PlazaGroupSection, {
    props: { group: g },
    global: {
      stubs: {
        GroupBadge: true,
        Icon: true,
        PlazaModelPricingTable: true
      }
    }
  })
}

describe('PlazaGroupSection（标准价语义）', () => {
  it('把模型与分组平台传给价格表', () => {
    const wrapper = mountSection(group())
    const table = wrapper.findComponent(PlazaModelPricingTable)
    expect(table.props('models')).toHaveLength(1)
    expect(table.props('models')[0].name).toBe('gpt-5.6-sol')
    expect(table.props('platform')).toBe('openai')
    // 旧计费口径 props 不再传递:价格表只吃标准价语义
    expect(table.props('rateMultiplier')).toBeUndefined()
    expect(table.props('peakWindow')).toBeUndefined()
  })

  it('专属 / 订阅徽章按字段渲染,描述文本透传', () => {
    const wrapper = mountSection(
      group({
        description: 'g-desc',
        is_exclusive: true,
        subscription_type: 'subscription'
      })
    )
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.badges.exclusive')
    expect(text).toContain('modelPlaza.badges.subscription')
    expect(text).toContain('g-desc')
    // 旧高峰披露文案已随计费语义移除
    expect(text).not.toContain('modelPlaza.detail.peakNote')
  })

  it('无模型时显示空态而非价格表', () => {
    const wrapper = mountSection(group({ models: [] }))
    expect(wrapper.findComponent(PlazaModelPricingTable).exists()).toBe(false)
    expect(wrapper.text()).toContain('modelPlaza.detail.noModels')
  })
})
