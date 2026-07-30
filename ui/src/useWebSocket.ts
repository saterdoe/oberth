import { apiBase, apiToken, isDesktop } from './runtimeConfig'
type EventType = string
type EventPayload = Record<string, unknown>

export interface WsEvent {
  version?: string
  id?: string
  sequence?: number
  type: EventType
  aggregate_id?: string
  payload: EventPayload
  time: string
}

type EventHandler = (event: WsEvent) => void
type DisconnectHandler = () => void

const RECONNECT_DELAY = 5000
const MAX_RETRIES = 5

function websocketTokenProtocol(token: string): string {
  const bytes = new TextEncoder().encode(token)
  let binary = ''
  bytes.forEach(byte => {
    binary += String.fromCharCode(byte)
  })
  return `oberth-token.${btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')}`
}

function wsUrl(): string {
  const base = apiBase()
  if (base) {
    const host = base.replace(/^https?:\/\//, '')
    return `ws://${host}/ws/v1/events`
  }
  if (isDesktop()) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${protocol}//${window.location.host}/ws/v1/events`
  }
  // Dev mode: Vite proxy doesn't forward WebSocket reliably, connect direct
  if (location.port === '5173') {
    return `ws://localhost:9090/ws/v1/events`
  }
  return `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws/v1/events`
}

export function useWebSocket(onDisconnect?: DisconnectHandler) {
  let ws: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let connected = false
  let retries = 0
  const handlers = new Map<EventType, Set<EventHandler>>()

  function connect() {
    if (connected || retries >= MAX_RETRIES) return
    const token = apiToken()
    ws = token
      ? new WebSocket(wsUrl(), websocketTokenProtocol(token))
      : new WebSocket(wsUrl())

    ws.onopen = () => {
      connected = true
      retries = 0
    }

    ws.onmessage = (event: MessageEvent) => {
      try {
        const evt: WsEvent = JSON.parse(event.data)
        const eventHandlers = handlers.get(evt.type)
        if (eventHandlers) {
          eventHandlers.forEach(fn => fn(evt))
        }
      } catch {
        // ignore malformed messages
      }
    }

    ws.onclose = () => {
      connected = false
      if (onDisconnect) onDisconnect()
      scheduleReconnect()
    }

    ws.onerror = () => {
      ws?.close()
    }
  }

  function scheduleReconnect() {
    if (reconnectTimer || retries >= MAX_RETRIES) return
    retries++
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      connect()
    }, RECONNECT_DELAY)
  }

  function disconnect() {
    retries = MAX_RETRIES  // stop reconnect
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (ws) {
      ws.onclose = null
      ws.close()
      ws = null
    }
    connected = false
  }

  function onEvent(type: EventType, handler: EventHandler) {
    if (!handlers.has(type)) {
      handlers.set(type, new Set())
    }
    handlers.get(type)!.add(handler)
  }

  function offEvent(type: EventType, handler: EventHandler) {
    const eventHandlers = handlers.get(type)
    if (eventHandlers) {
      eventHandlers.delete(handler)
      if (eventHandlers.size === 0) {
        handlers.delete(type)
      }
    }
  }

  return { connect, disconnect, onEvent, offEvent }
}
