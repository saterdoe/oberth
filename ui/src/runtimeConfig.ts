type DesktopRuntimeConfig = {
  api_url: string
  api_token: string
  desktop: boolean
  state?: 'preparing' | 'ready' | 'error'
  message?: string
  error?: string
}

declare global {
  interface Window {
    go?: { main?: { App?: {
      RuntimeConfig?: () => Promise<DesktopRuntimeConfig>
      RetryStartup?: () => Promise<DesktopRuntimeConfig>
      PickRepository?: () => Promise<NativePickedRepository>
      PickDirectory?: (title:string) => Promise<NativePickedRepository>
    } } }
  }
}

export type NativePickedRepository = { canceled: boolean; path?: string; name?: string }

let config: DesktopRuntimeConfig = {
  api_url: import.meta.env.VITE_API_URL || '',
  api_token: import.meta.env.VITE_API_TOKEN || '',
  desktop: false,
}

export async function bootstrapRuntimeConfig() {
  const nativeConfig = window.go?.main?.App?.RuntimeConfig
  if (!nativeConfig) return config
  try { config = await nativeConfig() } catch { /* browser dev mode */ }
  return config
}

export async function retryRuntimeStartup() {
  const retry = window.go?.main?.App?.RetryStartup
  if (!retry) return config
  config = await retry()
  return config
}

export function apiBase() { return config.api_url || import.meta.env.VITE_API_URL || '' }
export function apiToken() { return config.api_token || import.meta.env.VITE_API_TOKEN || '' }
export function isDesktop() { return config.desktop }
export function runtimeError() { return config.error || '' }
export async function pickNativeRepository() {
  const picker = window.go?.main?.App?.PickRepository
  return picker ? picker() : undefined
}
export async function pickNativeDirectory(title:string) {
  const picker = window.go?.main?.App?.PickDirectory
  return picker ? picker(title) : undefined
}
