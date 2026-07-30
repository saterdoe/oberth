import {afterEach, describe, expect, it, vi} from 'vitest'
import * as runtime from './runtimeConfig'

describe('desktop runtime configuration', () => {
  afterEach(() => {
    delete window.go
    vi.restoreAllMocks()
  })

  it('uses browser defaults when no desktop bridge is available', async () => {
    expect(await runtime.bootstrapRuntimeConfig()).toMatchObject({desktop: false})
    expect(runtime.isDesktop()).toBe(false)
    expect(runtime.runtimeError()).toBe('')
    expect(await runtime.pickNativeRepository()).toBeUndefined()
  })

  it('keeps browser configuration when the desktop bridge fails', async () => {
    window.go = {main: {App: {
      RuntimeConfig: vi.fn().mockRejectedValue(new Error('bridge unavailable')),
    }}}

    await expect(runtime.bootstrapRuntimeConfig()).resolves.toMatchObject({desktop: false})
  })

  it('loads configuration and repository selection from the desktop bridge', async () => {
    const picked = {canceled: false, path: 'C:\\repo', name: 'repo'}
    window.go = {main: {App: {
      RuntimeConfig: vi.fn().mockResolvedValue({
        api_url: 'http://127.0.0.1:9090',
        api_token: 'secret',
        desktop: true,
        state: 'ready',
        error: 'warning',
      }),
      PickRepository: vi.fn().mockResolvedValue(picked),
    }}}

    await runtime.bootstrapRuntimeConfig()
    expect(runtime.apiBase()).toBe('http://127.0.0.1:9090')
    expect(runtime.apiToken()).toBe('secret')
    expect(runtime.isDesktop()).toBe(true)
    expect(runtime.runtimeError()).toBe('warning')
    expect(await runtime.pickNativeRepository()).toEqual(picked)
  })

  it('exposes native startup retries and refreshes readiness', async () => {
    const retry = vi.fn().mockResolvedValue({
      api_url: 'http://127.0.0.1:9091',
      api_token: 'new-secret',
      desktop: true,
      state: 'ready',
    })
    window.go = {main: {App: {RetryStartup: retry}}}

    await expect(runtime.retryRuntimeStartup()).resolves.toMatchObject({state: 'ready'})
    expect(retry).toHaveBeenCalledOnce()
    expect(runtime.apiBase()).toBe('http://127.0.0.1:9091')
  })
})
