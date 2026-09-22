/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_BRAND: string
  readonly VITE_API_BASE_URL: string
  readonly BASE_URL: string
  /** 校园品牌（构建期由 BRAND env 注入，默认 muc） */
  readonly VITE_BRAND: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}

declare module '*.md?raw' {
  const content: string
  export default content
}
