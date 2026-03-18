const runtimeAppBasePath = window.RUNTIME_CONFIG?.APP_BASE_PATH?.trim()
const envAppBasePath = import.meta.env.VITE_APP_BASE_PATH?.trim()

export function normalizePath(raw: string | undefined): string {
  const value = (raw || '').trim()
  if (!value || value === '/') {
    return '/'
  }
  const segments = value.split('/').filter(Boolean)
  return segments.length ? `/${segments.join('/')}` : '/'
}

export function resolveAppBasePath(runtimePath: string | undefined, envPath: string | undefined): string {
  const normalizedRuntimePath = normalizePath(runtimePath)
  const normalizedEnvPath = normalizePath(envPath)

  if (normalizedRuntimePath === '/' && normalizedEnvPath !== '/') {
    return normalizedEnvPath
  }
  return normalizedRuntimePath !== '/' ? normalizedRuntimePath : normalizedEnvPath
}

export const APP_BASE_PATH = resolveAppBasePath(runtimeAppBasePath, envAppBasePath)

export function appUrl(path: string): string {
  const normalizedPath = normalizePath(path)
  if (APP_BASE_PATH === '/') {
    return normalizedPath
  }
  return normalizedPath === '/' ? APP_BASE_PATH : `${APP_BASE_PATH}${normalizedPath}`
}

export function stripAppBasePath(pathname: string): string {
  const normalizedPathname = normalizePath(pathname)
  if (APP_BASE_PATH === '/') {
    return normalizedPathname
  }
  if (normalizedPathname === APP_BASE_PATH) {
    return '/'
  }
  if (normalizedPathname.startsWith(`${APP_BASE_PATH}/`)) {
    return normalizedPathname.slice(APP_BASE_PATH.length)
  }
  return normalizedPathname
}

export function defaultApiBaseUrl(): string {
  return '/api'
}
