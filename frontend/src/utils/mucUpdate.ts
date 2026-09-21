// MUC Harness: mucode 桌面端最新版本信息（/downloads/latest-mucode.json）。
// 发布流程把安装包与 manifest 一起上传到 /downloads/，本页与 mucode 客户端
// 都以该 manifest 为唯一版本源。任何失败都返回 null，页面静默隐藏版本区块。

export type MucodeDownload = {
  file: string
  url: string
  sha256?: string
}

export type MucodeManifest = {
  version: string
  releasedAt: string
  notes?: string
  minSupported?: string
  downloads: Record<string, MucodeDownload>
}

const VERSION_LIKE = /^v?\d+\.\d+\.\d+/

export function parseMucodeManifest(body: unknown): MucodeManifest | null {
  if (!body || typeof body !== 'object') return null
  const raw = body as Record<string, unknown>
  if (typeof raw.version !== 'string' || !VERSION_LIKE.test(raw.version)) return null

  const manifest: MucodeManifest = {
    version: raw.version,
    releasedAt: typeof raw.releasedAt === 'string' ? raw.releasedAt : '',
    downloads: {},
  }
  if (typeof raw.notes === 'string' && raw.notes) manifest.notes = raw.notes
  if (typeof raw.minSupported === 'string' && raw.minSupported) manifest.minSupported = raw.minSupported

  if (raw.downloads && typeof raw.downloads === 'object') {
    for (const [key, value] of Object.entries(raw.downloads as Record<string, unknown>)) {
      if (!value || typeof value !== 'object') continue
      const d = value as Record<string, unknown>
      if (typeof d.file !== 'string' || typeof d.url !== 'string') continue
      const entry: MucodeDownload = { file: d.file, url: d.url }
      if (typeof d.sha256 === 'string' && d.sha256) entry.sha256 = d.sha256
      manifest.downloads[key] = entry
    }
  }
  return manifest
}

export async function fetchLatestMucodeManifest(
  fetchImpl: typeof fetch = fetch,
  manifestURL = '/downloads/latest-mucode.json',
): Promise<MucodeManifest | null> {
  try {
    const res = await fetchImpl(manifestURL, {
      cache: 'no-store',
      signal: AbortSignal.timeout(5000),
    })
    if (!res.ok) return null
    return parseMucodeManifest(await res.json())
  } catch {
    return null
  }
}
