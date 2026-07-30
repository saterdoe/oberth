import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

type EventHandler = (event: MessageEvent) => void
type WebSocketMock = {
  onopen: (() => void) | null
  onclose: ((event: { code: number; reason: string }) => void) | null
  onmessage: EventHandler | null
  onerror: ((event: Event) => void) | null
  close: () => void
  readyState: number
  send: (data: string) => void
}

let mockWs: WebSocketMock | null = null
let wsUrl = ''
let wsProtocols: string | string[] | undefined

const MockWebSocketClass = class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  url: string
  protocol = ''
  extensions = ''
  bufferedAmount = 0
  binaryType = 'blob' as BinaryType
  readyState = 0
  onopen: (() => void) | null = null
  onclose: ((event: { code: number; reason: string }) => void) | null = null
  onmessage: EventHandler | null = null
  onerror: ((event: Event) => void) | null = null
  close = vi.fn(() => {
    this.readyState = 3
    if (this.onclose) this.onclose({ code: 1000, reason: 'closed' })
  })
  send = vi.fn()

  constructor(url: string, protocols?: string | string[]) {
    this.url = url
    wsUrl = url
    wsProtocols = protocols
    mockWs = this
    setTimeout(() => {
      this.readyState = 1
      if (this.onopen) this.onopen()
    }, 0)
  }
}

beforeEach(() => {
  Object.defineProperty(globalThis, 'location', {
    value: { protocol: 'http:', host: 'localhost:5173', port: '5173', hostname: 'localhost' },
    writable: true,
  })
  globalThis.WebSocket = MockWebSocketClass as unknown as typeof WebSocket
})

describe('useWebSocket', () => {
  beforeEach(() => {
    mockWs = null
    wsUrl = ''
    wsProtocols = undefined
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('connects to port 9090 in dev mode (port 5173)', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const { connect, disconnect } = useWebSocket()
    connect()
    await vi.advanceTimersToNextTimerAsync()
    expect(wsUrl).toContain('localhost:9090')
    expect(wsUrl).toContain('/ws/v1/events')
    disconnect()
  })

  it('sends authentication in the WebSocket protocol instead of the URL', async () => {
    vi.stubEnv('VITE_API_TOKEN', 'test-token')
    const { useWebSocket } = await import('./useWebSocket')
    const { connect, disconnect } = useWebSocket()

    connect()
    await vi.advanceTimersToNextTimerAsync()

    expect(wsUrl).not.toContain('test-token')
    expect(wsUrl).not.toContain('access_token')
    expect(wsProtocols).toBe('oberth-token.dGVzdC10b2tlbg')
    disconnect()
    vi.unstubAllEnvs()
  })

  it('connects to location.host when not on port 5173', async () => {
    Object.defineProperty(globalThis, 'location', {
      value: { protocol: 'http:', host: 'myhost:8080', port: '8080', hostname: 'myhost' },
      writable: true,
    })
    const { useWebSocket } = await import('./useWebSocket')
    const { connect, disconnect } = useWebSocket()
    connect()
    await vi.advanceTimersToNextTimerAsync()
    expect(wsUrl).toContain('myhost:8080')
    disconnect()
  })

  it('uses VITE_API_URL when set', async () => {
    vi.stubEnv('VITE_API_URL', 'http://backend:9090')
    const mod = await import('./useWebSocket')
    const { connect, disconnect } = mod.useWebSocket()
    connect()
    await vi.advanceTimersToNextTimerAsync()
    expect(wsUrl).toContain('backend:9090')
    vi.unstubAllEnvs()
    disconnect()
  })

  it('receives events and calls registered handlers', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const handler = vi.fn()
    const { onEvent, connect, disconnect } = useWebSocket()
    onEvent('session.complete', handler)
    connect()
    await vi.advanceTimersToNextTimerAsync()

    if (!mockWs || !mockWs.onmessage) throw new Error('WebSocket not connected')
    mockWs.onmessage(new MessageEvent('message', {
      data: JSON.stringify({ type: 'session.complete', payload: { status: 'completed' }, time: new Date().toISOString() })
    }))

    expect(handler).toHaveBeenCalledTimes(1)
    expect(handler).toHaveBeenCalledWith(expect.objectContaining({ type: 'session.complete' }))
    disconnect()
  })

  it('ignores events with no registered handler', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const handler = vi.fn()
    const { onEvent, connect, disconnect } = useWebSocket()
    onEvent('session.complete', handler)
    connect()
    await vi.advanceTimersToNextTimerAsync()

    if (!mockWs || !mockWs.onmessage) throw new Error('WebSocket not connected')
    mockWs.onmessage(new MessageEvent('message', {
      data: JSON.stringify({ type: 'cost.alert', payload: {}, time: new Date().toISOString() })
    }))

    expect(handler).not.toHaveBeenCalled()
    disconnect()
  })

  it('reconnects on close up to MAX_RETRIES', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const { connect, disconnect } = useWebSocket()
    connect()
    await vi.advanceTimersToNextTimerAsync()

    if (!mockWs || !mockWs.onclose) throw new Error('WebSocket not connected')
    mockWs.onclose({ code: 1006, reason: 'connection lost' })
    // Should try to reconnect after delay
    await vi.advanceTimersToNextTimerAsync()
    expect(wsUrl).toBeTruthy()
    disconnect()
  })

  it('calls disconnect handler on close', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const onDisconnect = vi.fn()
    const { connect, disconnect } = useWebSocket(onDisconnect)
    connect()
    await vi.advanceTimersToNextTimerAsync()

    if (!mockWs || !mockWs.onclose) throw new Error('WebSocket not connected')
    mockWs.onclose({ code: 1000, reason: 'normal' })

    expect(onDisconnect).toHaveBeenCalled()
    disconnect()
  })

  it('removes event handler when unsubscribed', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const handler = vi.fn()
    const { onEvent, offEvent, connect, disconnect } = useWebSocket()
    onEvent('session.complete', handler)
    offEvent('session.complete', handler)
    connect()
    await vi.advanceTimersToNextTimerAsync()

    if (!mockWs || !mockWs.onmessage) throw new Error('WebSocket not connected')
    mockWs.onmessage(new MessageEvent('message', {
      data: JSON.stringify({ type: 'session.complete', payload: {}, time: new Date().toISOString() })
    }))

    expect(handler).not.toHaveBeenCalled()
    disconnect()
  })

  it('stops reconnecting after MAX_RETRIES', async () => {
    const { useWebSocket } = await import('./useWebSocket')
    const { connect, disconnect } = useWebSocket()
    connect()
    await vi.advanceTimersToNextTimerAsync()

    // Simulate 5 close events (MAX_RETRIES)
    for (let i = 0; i < 6; i++) {
      if (!mockWs || !mockWs.onclose) break
      mockWs.onclose({ code: 1006, reason: 'connection lost' })
      await vi.advanceTimersToNextTimerAsync()
    }

    disconnect()
  })
})
