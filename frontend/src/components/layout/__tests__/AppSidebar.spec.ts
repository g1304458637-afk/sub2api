import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar subscription feature flag', () => {
  it('gates the My Subscriptions entry behind the subscription public-settings flag', () => {
    expect(componentSource).toContain('const flagSubscription = makeSidebarFlag(FeatureFlags.subscription)')
    expect(componentSource).toMatch(/path: '\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('also hides the admin Subscription Management entry on recharge-only sites', () => {
    expect(componentSource).toMatch(/path: '\/admin\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('keeps /pricing as the plan entry behind the payment flag', () => {
    expect(componentSource).toMatch(/path: '\/pricing', label: t\('nav\.pricing'\)/)
  })
})

describe('AppSidebar Campus AI navigation convergence', () => {
  // 信息架构收敛：技术型仪表盘/旧使用记录/独立充值/兑换不再出现在用户一级导航，
  // 但路由保留兼容（/dashboard 直达、/usage → /spend 重定向、/purchase 由钱包进入）。
  const mineItemsMatch = componentSource.match(/const mineItems: NavItem\[\] = \[([\s\S]*?)\n  \]\n/)

  it('exposes the mine group declaration to contract tests', () => {
    expect(mineItemsMatch).not.toBeNull()
  })

  it('hides Dashboard, Recharge, Redeem and the old technical Usage entries from user nav', () => {
    const mineItems = mineItemsMatch?.[1] ?? ''
    expect(mineItems).not.toContain("path: '/dashboard'")
    expect(mineItems).not.toContain("path: '/purchase'")
    expect(mineItems).not.toContain("path: '/redeem'")
    expect(mineItems).not.toContain("path: '/usage'")
  })

  it('shows the new Model Spend entry at /spend', () => {
    const mineItems = mineItemsMatch?.[1] ?? ''
    expect(mineItems).toContain("path: '/spend'")
    expect(componentSource).toMatch(/path: '\/spend', label: t\('nav\.modelSpend'\)/)
  })

  it('links 模型与价格 to the embedded /models plaza alias behind its opt-in flag', () => {
    const mineItems = mineItemsMatch?.[1] ?? ''
    expect(componentSource).toContain('const flagModelPlaza = makeSidebarFlag(FeatureFlags.modelPlaza)')
    expect(mineItems).toContain("path: '/model-pricing?embedded=1'")
    expect(componentSource).toMatch(/path: '\/model-pricing\?embedded=1', label: t\('nav\.modelsPricing'\)/)
  })

  it('keeps the converged ordering: keys → models pricing → spend → subscriptions → wallet → pricing → orders → research → profile', () => {
    const mineItems = mineItemsMatch?.[1] ?? ''
    const order = [
      "path: '/keys'",
      "path: '/model-pricing?embedded=1'",
      "path: '/spend'",
      "path: '/subscriptions'",
      "path: '/wallet'",
      "path: '/pricing'",
      "path: '/orders'",
      "path: '/research-discount'",
      "path: '/profile'",
    ]
    let last = -1
    for (const marker of order) {
      const at = mineItems.indexOf(marker)
      expect(at).toBeGreaterThan(last)
      last = at
    }
  })

  it('sends the regular-user logo/home target to the chat portal instead of the hidden dashboard', () => {
    expect(componentSource).toContain("const homePath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/chat'))")
  })

  it('matches active state by path key so query-bearing nav items (e.g. /model-pricing?embedded=1) highlight', () => {
    expect(componentSource).toContain('function navPathKey(path: string): string')
    expect(componentSource).toContain('return item.children.some(child => isActive(child.path))')
  })
})
