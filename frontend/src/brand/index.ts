/**
 * 校园品牌注册表（前端）：MUC / HUBU 共用，构建期由 VITE_BRAND（来自 BRAND env，默认 muc）决定站点品牌。
 * 规则：
 * - 站点级品牌（登录页、导航、主题色）取当前品牌；两个品牌页 /muc /hubu 始终共存可访问。
 * - MUC 配置的文案/色值/图片与历史硬编码逐字节一致，保证 MUC 构建零视觉变化。
 * - HUBU 色值采样自湖北大学主视觉图 hubu-hero.png（深湖大绿 #135440 / 青铜金 #BC9D53），非官方 VI 标准色。
 */

export interface BrandDownloadOption {
  key: 'mac-arm' | 'mac-intel' | 'win' | 'other'
  label: string
  file: string
}

export interface BrandConfig {
  id: string
  /** API 路径段：/api/v1/{pathSegment}/connect-code */
  pathSegment: string
  name: string
  englishName: string
  shortName: string
  productName: string
  motto: string
  founded: string
  /** 深链协议：muc:// / hubu:// */
  protocolScheme: string
  /** 首次登录引导 localStorage key */
  seenKey: string
  /** 品牌页路径 */
  homePath: string
  /** 导航下载入口文案 */
  downloadLabel: string
  /** 登录页标语（AuthLayout） */
  authTagline: string
  authTaglineColor: string
  /** 登录页背景图 */
  authBg: string
  /** 品牌页 hero 图 */
  heroImg: string
  heroAlt: string
  /** 品牌页主色（按钮/边框，与 MUC 历史 hex 对齐） */
  primary: string
  primaryDark: string
  /** 品牌页文案 */
  page: {
    heading: string
    subheading: string
    connectCardTitle: string
    connectCardDesc: string
    connectButtonIdle: string
    connectButtonOpening: string
    fallbackTitle: string
    fallbackBody: string
    openingHint: string
    downloadCardTitle: string
    downloadCardDesc: string
    steps: string[]
    xattrAppPath: string
    desktopName: string
  }
  downloads: BrandDownloadOption[]
}

import mucCampusImg from '@/assets/muc/campus.png'
import hubuHeroImg from '@/assets/hubu/hubu-hero.png'

const muc: BrandConfig = {
  id: 'muc',
  pathSegment: 'muc',
  name: '中央民族大学',
  englishName: 'Minzu University of China',
  shortName: 'MUC',
  productName: 'MUC AI Harness',
  motto: '美美与共 知行合一',
  founded: '1941',
  protocolScheme: 'muc',
  seenKey: 'muc_seen',
  homePath: '/muc',
  downloadLabel: '下载 MUC',
  authTagline: '中央民族大学 · 美美与共 知行合一',
  authTaglineColor: '#8f6a3c',
  authBg: mucCampusImg,
  heroImg: mucCampusImg,
  heroAlt: '中央民族大学',
  primary: '#AC0E0F',
  primaryDark: '#8f0c0d',
  page: {
    heading: 'MUC AI Harness',
    subheading: '中央民族大学 · 美美与共 知行合一',
    connectCardTitle: '连接 MUC 桌面端',
    connectCardDesc: '无需手动配置 API，无需填写 Base URL；授权码 60 秒内有效，仅可使用一次。',
    connectButtonIdle: '一键连接 MUC',
    connectButtonOpening: '正在唤起 MUC…',
    fallbackTitle: '未检测到 MUC 应用',
    fallbackBody:
      '浏览器没有响应 muc:// 链接。请先下载并安装 MUC（见下方），安装后重新点击「一键连接 MUC」。授权码已失效，重新连接会自动签发新的授权码。',
    openingHint: '如果浏览器没有自动弹出 MUC，请确认已安装 MUC 桌面端。',
    downloadCardTitle: '下载 MUC',
    downloadCardDesc: '安装后回到本页点击「一键连接 MUC」。登录后自动同步你的可用模型。',
    steps: [
      '登录本站，下载对应平台的 MUC 安装包',
      '安装 MUC：macOS 拖入 Applications，Windows 双击安装',
      '回到本页，点击「一键连接 MUC」并允许浏览器打开应用',
      'MUC 自动验证授权并同步你的可用模型，开始使用',
    ],
    xattrAppPath: '/Applications/mucode.app',
    desktopName: 'MUC',
  },
  downloads: [
    { key: 'mac-arm', label: 'macOS Apple Silicon', file: 'mucode-mac-arm64.dmg' },
    { key: 'mac-intel', label: 'macOS Intel', file: 'mucode-mac-x64.dmg' },
    { key: 'win', label: 'Windows x64', file: 'mucode-win-x64.exe' },
  ],
}

const hubu: BrandConfig = {
  id: 'hubu',
  pathSegment: 'hubu',
  name: '湖北大学',
  englishName: 'Hubei University',
  shortName: 'HUBU',
  productName: 'HUBU AI',
  motto: '日思日睿 笃志笃行',
  founded: '1931',
  protocolScheme: 'hubu',
  seenKey: 'hubu_seen',
  homePath: '/hubu',
  downloadLabel: '下载 HUBU',
  authTagline: '湖北大学 · 日思日睿 笃志笃行',
  authTaglineColor: '#8a6d2f',
  authBg: hubuHeroImg,
  heroImg: hubuHeroImg,
  heroAlt: '湖北大学',
  primary: '#135440',
  primaryDark: '#0F4433',
  page: {
    heading: 'HUBU AI',
    subheading: '湖北大学 HUBEI UNIVERSITY · 日思日睿 笃志笃行 · 1931',
    connectCardTitle: '连接 HUBU AI 桌面端',
    connectCardDesc: '无需手动配置 API，无需填写 Base URL；授权码 60 秒内有效，仅可使用一次。',
    connectButtonIdle: '一键连接 HUBU',
    connectButtonOpening: '正在唤起 HUBU AI…',
    fallbackTitle: '未检测到 HUBU AI 应用',
    fallbackBody:
      '浏览器没有响应 hubu:// 链接。请先下载并安装 HUBU AI（见下方），安装后重新点击「一键连接 HUBU」。授权码已失效，重新连接会自动签发新的授权码。',
    openingHint: '如果浏览器没有自动弹出 HUBU AI，请确认已安装 HUBU AI 桌面端。',
    downloadCardTitle: '下载 HUBU AI',
    downloadCardDesc: '安装后回到本页点击「一键连接 HUBU」。登录后自动同步你的可用模型。',
    steps: [
      '登录本站，下载对应平台的 HUBU AI 安装包',
      '安装 HUBU AI：macOS 拖入 Applications，Windows 双击安装',
      '回到本页，点击「一键连接 HUBU」并允许浏览器打开应用',
      'HUBU AI 自动验证授权并同步你的可用模型，开始使用',
    ],
    xattrAppPath: '/Applications/HUBU AI.app',
    desktopName: 'HUBU AI',
  },
  downloads: [
    { key: 'mac-arm', label: 'macOS Apple Silicon', file: 'hubu-ai-mac-arm64.dmg' },
    { key: 'mac-intel', label: 'macOS Intel', file: 'hubu-ai-mac-x64.dmg' },
    { key: 'win', label: 'Windows x64', file: 'hubu-ai-win-x64.exe' },
  ],
}

const registry: Record<string, BrandConfig> = { muc, hubu }

/** 构建期品牌（Vite define 注入，默认 muc —— 保证未设 BRAND 时与历史构建完全一致） */
export const currentBrand: BrandConfig = registry[import.meta.env.VITE_BRAND as string] ?? muc

export function getBrand(id: string): BrandConfig {
  return registry[id] ?? muc
}

export const brands = { muc, hubu }
