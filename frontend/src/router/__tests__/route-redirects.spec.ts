import { describe, expect, it, vi } from 'vitest'

/**
 * Campus AI 信息架构收敛后的路由兼容性契约：
 * - 旧「使用记录」/usage 由新的 /spend（模型消费）取代，旧链接重定向，不 404；
 * - 技术型 /dashboard 从导航隐藏但路由保留可访问（兼容旧书签/外部链接）；
 * - /purchase（充值）、/redeem（兑换）路由保留，仅导航入口收敛。
 */
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    checkAuth: vi.fn(),
    isAuthenticated: false,
    isAdmin: false,
    isSimpleMode: false,
    hasPendingAuthSession: false,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    siteName: 'Campus AI',
    backendModeEnabled: false,
    publicSettingsLoaded: false,
    cachedPublicSettings: null,
    fetchPublicSettings: vi.fn(),
  }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
    fetch: vi.fn(),
  }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
  }),
}))

vi.mock('@/api/setup', () => ({
  getSetupStatus: vi.fn(),
}))

import router from '@/router'

describe('navigation convergence route compatibility', () => {
  it('redirects the legacy /usage record to the new /spend model-spend view', () => {
    const record = router.getRoutes().find((r) => r.path === '/usage')
    expect(record).toBeDefined()
    expect(record?.redirect).toBe('/spend')
  })

  it('mounts the new ModelSpend view at /spend behind auth', () => {
    const record = router.getRoutes().find((r) => r.path === '/spend')
    expect(record).toBeDefined()
    expect(record?.name).toBe('ModelSpend')
    expect(record?.meta.requiresAuth).toBe(true)
    expect(record?.meta.requiresAdmin).toBe(false)
  })

  it('keeps /dashboard reachable for compatibility while no longer being a nav entry', () => {
    const record = router.getRoutes().find((r) => r.path === '/dashboard')
    expect(record).toBeDefined()
    expect(record?.components?.default).toBeDefined()
  })

  it('keeps /purchase (recharge flow) and /redeem (campaign deep links) routes alive', () => {
    expect(router.getRoutes().some((r) => r.path === '/purchase')).toBe(true)
    expect(router.getRoutes().some((r) => r.path === '/redeem')).toBe(true)
  })
})
