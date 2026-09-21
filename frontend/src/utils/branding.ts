import { sanitizeUrl } from '@/utils/url'

export const DEFAULT_SITE_NAME = '中央民族大学 AI 服务平台'

export function resolveSiteName(value?: string | null): string {
  const name = value?.trim() || ''
  return !name || /^(sub2api|muapi)$/i.test(name) ? DEFAULT_SITE_NAME : name
}

export function resolveSiteSubtitle(value?: string | null): string {
  const subtitle = value?.trim() || ''
  return !subtitle || /^(Subscription to API Conversion Platform|AI API Gateway Platform)$/i.test(subtitle)
    ? '面向教学与科研的一站式 AI 服务'
    : subtitle
}

export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  })
  if (!sanitizedLogoUrl) {
    return
  }

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  link.type = sanitizedLogoUrl.endsWith('.svg') ? 'image/svg+xml' : 'image/x-icon'
  link.href = sanitizedLogoUrl
}
