import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import type { PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/stores/currencyDisplay', () => ({
  useCurrencyDisplayStore: () => ({
    displayCurrency: 'USD',
    usdToCnyRate: 0,
    canDisplayCNY: false,
    toggleCurrency: vi.fn(),
    formatUSD: (amount: number | null | undefined) => `$${Number(amount ?? 0).toFixed(2)}`
  })
}))

/** 计费口径价（pricing）：¥3/1M —— 展示价解耦后仅作参考对照,不进表格。 */
function tokenModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'claude-sonnet',
    platform: 'anthropic',
    pricing: {
      billing_mode: 'token',
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_read_price: 3e-7,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    official_pricing: {
      input_price: 3e-6,
      output_price: 1.5e-5,
      cache_write_price: 3.75e-6,
      cache_write_1h_price: 6e-6,
      cache_read_price: 3e-7
    },
    display_pricing: {
      billing_mode: 'token',
      input_price: 1e-4,
      output_price: 8e-4,
      cache_write_price: 1.25e-5,
      cache_write_1h_price: null,
      cache_read_price: 1e-6,
      image_input_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    },
    presentation_source: 'manual',
    ...overrides
  }
}

function mountTable(models: PlazaModel[], platform?: string) {
  return mount(PlazaModelPricingTable, { props: { models, platform } })
}

describe('PlazaModelPricingTable（标准价语义）', () => {
  it('主展示列为标准价格(display_pricing 绝对值,$/1M),不乘任何倍率', () => {
    const wrapper = mountTable([tokenModel()])
    const text = wrapper.text()
    // 标准价:input $100 / output $800 / cache write $12.50 / cache read $1.00
    expect(text).toContain('$100')
    expect(text).toContain('$800')
    expect(text).toContain('$12.50')
    expect(text).toContain('$1.00')
    // 旧语义字段不应出现:无倍率列、无「实付(折后)」标题
    expect(text).not.toContain('1x')
    expect(text).not.toContain('modelPlaza.table.paidPrice')
    expect(text).toContain('modelPlaza.table.standardPrice')
  })

  it('计费口径价(pricing)与分组倍率不影响标准价列 —— 展示价与计费解耦', () => {
    // pricing 原价 $3/$15(计费口径),display $100/$800:页面只显示 display
    const m = tokenModel()
    m.pricing!.input_price = 3e-6
    m.pricing!.output_price = 1.5e-5
    const wrapper = mountTable([m])
    const firstRowCells = wrapper.findAll('tbody td')
    // 标准价输入列显示 $100,绝无 $3.00(计费价)
    expect(firstRowCells[1].text()).toContain('$100')
    expect(firstRowCells[1].text()).not.toContain('$3.00')
  })

  it('两级表头:标准价区与官方区各拆输入/输出/缓存列,token 行共 7 列', () => {
    const wrapper = mountTable([tokenModel()])
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.standardPrice')
    expect(text).toContain('modelPlaza.table.officialPrice')
    expect(wrapper.findAll('tbody td')).toHaveLength(7)
  })

  it('官方参考价列展示官方原值(含 1h 缓存写入)', () => {
    const wrapper = mountTable([tokenModel()])
    const text = wrapper.text()
    expect(text).toContain('$3.00')
    expect(text).toContain('$15.00')
    expect(text).toContain('(1h')
  })

  it('official_pricing 为 null 时官方三列显示 -', () => {
    const wrapper = mountTable([tokenModel({ official_pricing: null })])
    const cells = wrapper.findAll('tbody td')
    expect(cells[4].text().trim()).toBe('-')
    expect(cells[5].text().trim()).toBe('-')
    expect(cells[6].text().trim()).toBe('-')
  })

  it('display_pricing 为 none 时标准价区显示「价格暂未公布」,不猜测数字', () => {
    const wrapper = mountTable([
      tokenModel({ display_pricing: null, presentation_source: 'none' })
    ])
    const text = wrapper.text()
    expect(text).toContain('modelPlaza.table.priceNotPublished')
    expect(text).not.toContain('$100')
  })

  it('标准价分别展示 5m 与 1h 缓存写入价', () => {
    const model = tokenModel()
    model.display_pricing!.cache_write_1h_price = 2e-5
    const text = mountTable([model]).text()
    expect(text).toContain('$12.50')
    expect(text).toContain('$20.00')
    expect(text).toContain('(1h')
  })

  it('按次/按图计费模型展示单次标准价与单位后缀,沉到 token 模型之后', () => {
    const image = tokenModel({
      name: 'gpt-image-2',
      display_pricing: {
        billing_mode: 'image',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: 0.04,
        intervals: []
      }
    })
    const token = tokenModel({ name: 'gpt-5.6' })
    const wrapper = mountTable([image, token])
    const names = wrapper.findAll('tbody tr').map((tr) => tr.find('td').text())
    expect(names[0]).toContain('gpt-5.6')
    expect(names[1]).toContain('gpt-image-2')
    const text = wrapper.text()
    expect(text).toContain('$0.04')
    expect(text).toContain('modelPlaza.table.perImage')
    expect(text).toContain('modelPlaza.table.perUnitImage')
  })

  it('模型按标准价输出价从高到低排序,无标准价的排最后,同价按名称降序', () => {
    const expensive = tokenModel({ name: 'model-expensive' })
    const cheap = tokenModel({
      name: 'model-cheap',
      display_pricing: {
        billing_mode: 'token',
        input_price: 1e-5,
        output_price: 2e-4,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: []
      }
    })
    const none = tokenModel({ name: 'model-none', display_pricing: null, presentation_source: 'none' })
    const wrapper = mountTable([cheap, none, expensive])
    const names = wrapper.findAll('tbody tr').map((tr) => tr.find('td').text())
    expect(names).toEqual(['model-expensive', 'model-cheap', 'model-none'])

    const newer = tokenModel({ name: 'gpt-5.6-sol' })
    const older = tokenModel({ name: 'gpt-5.5' })
    const wrapper2 = mountTable([older, newer])
    expect(wrapper2.findAll('tbody tr').map((tr) => tr.find('td').text())).toEqual([
      'gpt-5.6-sol',
      'gpt-5.5'
    ])
  })

  it('Composite 分组中相同模型名按具体平台分别展示徽章', () => {
    const anthropic = tokenModel({ name: 'shared-model', platform: 'anthropic' })
    const openai = tokenModel({ name: 'shared-model', platform: 'openai' })
    const wrapper = mountTable([anthropic, openai], 'composite')

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows.map((row) => row.find('td').text())).toEqual([
      'shared-modelAnthropic',
      'shared-modelOpenAI'
    ])
    expect(wrapper.text()).toContain('Anthropic')
    expect(wrapper.text()).toContain('OpenAI')
  })
})

describe('PlazaModelPricingTable 标准价长上下文阶梯', () => {
  function ladderIntervals() {
    return [
      {
        min_tokens: 0,
        max_tokens: 272000,
        tier_label: '≤272K',
        input_price: 1e-4,
        output_price: 8e-4,
        cache_write_price: 1.25e-5,
        cache_read_price: 1e-6,
        per_request_price: null
      },
      {
        min_tokens: 272000,
        max_tokens: null,
        tier_label: '>272K',
        input_price: 2e-4,
        output_price: 1.2e-3,
        cache_write_price: 2.5e-5,
        cache_read_price: 2e-6,
        per_request_price: null
      }
    ]
  }

  function ladderModel(overrides: Partial<PlazaModel> = {}): PlazaModel {
    return tokenModel({
      name: 'gpt-5.6-sol',
      platform: 'openai',
      display_pricing: {
        billing_mode: 'token',
        input_price: 1e-4,
        output_price: 8e-4,
        cache_write_price: 1.25e-5,
        cache_write_1h_price: null,
        cache_read_price: 1e-6,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: ladderIntervals()
      },
      long_context_basis: 'whole_request',
      ...overrides
    })
  }

  it('标准价阶梯每档一行,档位标签只在输入列,缓存列按档对齐', () => {
    const wrapper = mountTable([ladderModel()])
    const cells = wrapper.findAll('tbody td')
    // 输入列带两档标签与价格
    expect(cells[1].text()).toContain('≤272K')
    expect(cells[1].text()).toContain('$100')
    expect(cells[1].text()).toContain('>272K')
    expect(cells[1].text()).toContain('$200')
    // 输出列只按行对齐,不重复标签
    expect(cells[2].text()).toContain('$800')
    expect(cells[2].text()).toContain('$1200.00')
    expect(cells[2].text()).not.toContain('272K')
    // 缓存列:写/读按档两行
    expect(cells[3].text()).toContain('modelPlaza.table.cacheWriteShort')
    expect(cells[3].text()).toContain('$12.50')
    expect(cells[3].text()).toContain('$1.00')
    expect(cells[3].text()).toContain('$25.00')
    expect(cells[3].text()).toContain('$2.00')
  })

  it('无标签的多档按区间生成统一形态(≤上限 / >下限),并按下限升序展示', () => {
    const model = ladderModel({
      display_pricing: {
        ...ladderModel().display_pricing!,
        intervals: [
          { ...ladderIntervals()[1], min_tokens: 1000000, tier_label: '' },
          { ...ladderIntervals()[0], min_tokens: 100000, max_tokens: 200000, tier_label: '' },
          { ...ladderIntervals()[0], max_tokens: 100000, tier_label: '' },
          { ...ladderIntervals()[1], min_tokens: 200000, max_tokens: 1000000, tier_label: '' }
        ]
      }
    })
    const rows = mountTable([model]).findAll('tbody td')[1].findAll('.leading-5')
    expect(rows.map((r) => r.text().split(/\s+/)[0])).toEqual(['≤100K', '≤200K', '≤1M', '>1M'])
  })

  it('低价值精度不被格式化为 0:cache read $0.10/1M 正常显示', () => {
    const model = tokenModel({
      display_pricing: {
        billing_mode: 'token',
        input_price: 1e-4,
        output_price: 8e-4,
        cache_write_price: null,
        cache_write_1h_price: null,
        cache_read_price: 1e-7,
        image_input_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: []
      }
    })
    const text = mountTable([model]).text()
    expect(text).toContain('$0.10')
  })
})
