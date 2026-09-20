import { beforeEach, describe, expect, it } from 'vitest'
import { updateFavicon, resolveSiteName, resolveSiteSubtitle, DEFAULT_SITE_NAME } from '@/utils/branding'

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})

describe('school site name', () => {
  it('normalizes only legacy names and blank settings', () => {
    for (const name of [undefined, '', '  ', 'Sub2API', ' sub2api ', 'MUAPI']) {
      expect(resolveSiteName(name)).toBe(DEFAULT_SITE_NAME)
    }
  })
  it('preserves a configured school name', () => {
    expect(resolveSiteName(' 校园智能服务 ')).toBe('校园智能服务')
  })
})

describe('school subtitle', () => {
  it('replaces upstream defaults and preserves configured campus text', () => {
    expect(resolveSiteSubtitle('Subscription to API Conversion Platform')).toBe('面向教学与科研的一站式 AI 服务')
    expect(resolveSiteSubtitle(' AI API Gateway Platform ')).toBe('面向教学与科研的一站式 AI 服务')
    expect(resolveSiteSubtitle('校园教学助手')).toBe('校园教学助手')
  })
})
