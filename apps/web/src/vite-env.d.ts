/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
  readonly VITE_APP_BASE_PATH?: string
  readonly VITE_DEV_PROXY_TARGET?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}

interface Window {
  RUNTIME_CONFIG?: {
    APP_BASE_PATH?: string
    API_BASE_URL?: string
  }
}
