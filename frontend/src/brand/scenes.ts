/** Presentation only: billing, permissions and navigation stay in their existing routes. */
export function campusScene(path: string): 'apex' | 'globe' | 'orbit' | 'particles' | 'quiet' {
  if (path.startsWith('/admin') || path === '/setup') return 'quiet'
  if (/^\/(chat|research-discount)(\/|$)/.test(path)) return 'particles'
  if (/^\/(wallet|subscriptions|purchase|orders|redeem|tts|music)(\/|$)/.test(path)) return 'orbit'
  if (/^\/(pricing|dashboard|draw|model-plaza)(\/|$)/.test(path)) return 'apex'
  return 'globe'
}

const videoRoot = 'https://d8j0ntlcm91z4.cloudfront.net/user_38xzZboKViGWJOttwIXH07lWA1P/'
export const campusVideos = {
  apex: `${videoRoot}hf_20260309_042944_4a2205b7-b061-490a-852b-92d9e9955ce9.mp4`,
  globe: `${videoRoot}hf_20260912_104036_bd6924f6-3c8e-417e-8465-6d03c8c2e9e6.mp4`,
  orbit: `${videoRoot}hf_20260912_104303_0c6d60b2-9353-408e-9449-585108a22fb5.mp4`,
}

export function hasCampusScenes(brand: string): boolean {
  return brand === 'muc' || brand === 'hubu'
}

export function usesDarkTheme(brand: string, saved: string | null): boolean {
  return hasCampusScenes(brand) || saved !== 'light'
}
