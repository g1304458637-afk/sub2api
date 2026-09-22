import { describe, expect, it } from 'vitest'
import { campusScene, usesDarkTheme } from '../scenes'

describe('Campus dark-only presentation', () => {
  it('ignores legacy light preferences for both campus brands', () => {
    expect(usesDarkTheme('muc', 'light')).toBe(true)
    expect(usesDarkTheme('muc', null)).toBe(true)
    expect(usesDarkTheme('hubu', 'light')).toBe(true)
    expect(usesDarkTheme('hubu', 'dark')).toBe(true)
  })
  it('keeps busy admin screens quiet and assigns distinct backgrounds to user flows', () => {
    expect(campusScene('/admin/users')).toBe('quiet')
    expect(campusScene('/pricing')).toBe('apex')
    expect(campusScene('/wallet')).toBe('orbit')
    expect(campusScene('/login')).toBe('globe')
    expect(campusScene('/research-discount')).toBe('particles')
    expect(campusScene('/chat/123')).toBe('particles')
  })
})
