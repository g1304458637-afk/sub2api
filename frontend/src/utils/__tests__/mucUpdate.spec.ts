import { describe, expect, it, vi } from 'vitest'
import { fetchLatestMucodeManifest, parseMucodeManifest } from '../mucUpdate'

const validManifest = {
  version: '1.18.31-muc.1',
  releasedAt: '2026-09-20T03:00:00.000Z',
  notes: '修复深链连接',
  minSupported: '1.18.31-muc.1',
  downloads: {
    'mac-arm64': {
      file: 'mucode-mac-arm64.dmg',
      url: 'https://admin.wuxuexi.top/downloads/mucode-mac-arm64.dmg',
      sha256: 'abc123',
    },
    'win-x64': { file: 'mucode-win-x64.exe', url: 'https://admin.wuxuexi.top/downloads/mucode-win-x64.exe' },
  },
}

function fetchOk(body: unknown): typeof fetch {
  return vi.fn(async () => new Response(JSON.stringify(body), { status: 200 })) as unknown as typeof fetch
}

function fetchFails(status = 404): typeof fetch {
  return vi.fn(async () => new Response('not found', { status })) as unknown as typeof fetch
}

describe('parseMucodeManifest', () => {
  it('解析完整 manifest', () => {
    const m = parseMucodeManifest(validManifest)
    expect(m).not.toBeNull()
    expect(m!.version).toBe('1.18.31-muc.1')
    expect(m!.notes).toBe('修复深链连接')
    expect(m!.minSupported).toBe('1.18.31-muc.1')
    expect(m!.downloads['mac-arm64']!.sha256).toBe('abc123')
    expect(m!.downloads['win-x64']!.sha256).toBeUndefined()
  })

  it('接受缺省的可选字段（无 notes/downloads）', () => {
    const m = parseMucodeManifest({ version: 'v1.0.0' })
    expect(m).not.toBeNull()
    expect(m!.version).toBe('v1.0.0')
    expect(m!.releasedAt).toBe('')
    expect(m!.downloads).toEqual({})
  })

  it('拒绝缺失/非法版本号、非对象主体', () => {
    expect(parseMucodeManifest(null)).toBeNull()
    expect(parseMucodeManifest('x')).toBeNull()
    expect(parseMucodeManifest({})).toBeNull()
    expect(parseMucodeManifest({ version: 'muc-latest' })).toBeNull()
    expect(parseMucodeManifest({ version: 123 })).toBeNull()
  })

  it('跳过结构非法的下载项', () => {
    const m = parseMucodeManifest({
      version: '1.0.0',
      downloads: { bad: { file: 'x' }, alsonot: 'string', good: { file: 'a.dmg', url: 'https://x/a.dmg' } },
    })
    expect(m!.downloads).toEqual({ good: { file: 'a.dmg', url: 'https://x/a.dmg' } })
  })
})

describe('fetchLatestMucodeManifest', () => {
  it('正常返回解析结果', async () => {
    const f = fetchOk(validManifest)
    const m = await fetchLatestMucodeManifest(f)
    expect(m?.version).toBe('1.18.31-muc.1')
    expect(f).toHaveBeenCalledWith('/downloads/latest-mucode.json', expect.objectContaining({ cache: 'no-store' }))
  })

  it('HTTP 错误返回 null', async () => {
    expect(await fetchLatestMucodeManifest(fetchFails(404))).toBeNull()
    expect(await fetchLatestMucodeManifest(fetchFails(500))).toBeNull()
  })

  it('网络异常/非法 JSON 返回 null', async () => {
    const networkError = vi.fn(async () => {
      throw new Error('offline')
    }) as unknown as typeof fetch
    expect(await fetchLatestMucodeManifest(networkError)).toBeNull()

    const badJson = vi.fn(async () => new Response('<html>', { status: 200 })) as unknown as typeof fetch
    expect(await fetchLatestMucodeManifest(badJson)).toBeNull()
  })
})
