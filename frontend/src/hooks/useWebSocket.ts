import { useEffect, useRef, useState } from 'react'
import { getBackendWsUrl } from '../services/backendUrl'

const RECONNECT_DELAY_MS = 2000

export type WsReadyState = 'connecting' | 'open' | 'closed'

export interface WsMessage {
  type: string
  data: unknown
}

// Singleton state — one WebSocket connection shared across all hook consumers.
let socket: WebSocket | null = null
const messageHandlers = new Set<(msg: WsMessage) => void>()
const stateHandlers = new Set<(state: WsReadyState) => void>()
let currentState: WsReadyState = 'closed'
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
// In-flight connect promise. Without this, parallel consumer mounts each call
// connect(), and the OPEN/CONNECTING guard above passes for every one of them
// because `socket` is still null until after the async getBackendWsUrl() await.
// Each call then constructs its own WebSocket, leaving multiple sockets alive
// — every onmessage fires across the shared handler set, so each event lands
// N times in the Log Feed where N is the number of leaked sockets (issue #124).
let connectPromise: Promise<void> | null = null

function setState(state: WsReadyState): void {
  currentState = state
  stateHandlers.forEach((h) => h(state))
}

function connect(): Promise<void> {
  if (
    socket?.readyState === WebSocket.OPEN ||
    socket?.readyState === WebSocket.CONNECTING ||
    // CLOSING counts as "connection pending": onerror calls ws.close(), which
    // moves the socket to CLOSING but leaves `socket` set until onclose fires
    // and schedules the reconnect. Creating a fresh socket during this window
    // leaks a second live connection into the shared handler set (issue #124).
    socket?.readyState === WebSocket.CLOSING
  ) {
    return Promise.resolve()
  }
  if (connectPromise) {
    return connectPromise
  }
  connectPromise = doConnect().finally(() => {
    connectPromise = null
  })
  return connectPromise
}

async function doConnect(): Promise<void> {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  setState('connecting')
  // Resolve the backend port lazily — it's discovered at app startup via IPC
  // from the Electron main process (and may differ between launches if the
  // preferred port was busy and the server fell back to an OS-assigned port).
  const wsUrl = await getBackendWsUrl()
  // If consumers unmounted before the port resolved, abort the connect.
  if (stateHandlers.size === 0) {
    setState('closed')
    return
  }
  const ws = new WebSocket(wsUrl)
  socket = ws

  ws.onopen = () => {
    // Ignore a stale socket that opened after `socket` was already replaced.
    if (socket !== ws) return
    setState('open')
  }

  ws.onmessage = (e) => {
    // The backend may batch multiple JSON objects into one frame, separated by
    // newlines. Split and parse each line individually.
    for (const line of (e.data as string).split('\n')) {
      const trimmed = line.trim()
      if (!trimmed) continue
      try {
        const msg = JSON.parse(trimmed) as WsMessage
        messageHandlers.forEach((h) => h(msg))
      } catch {
        // ignore non-JSON frames
      }
    }
  }

  ws.onclose = () => {
    // A stale socket closing must not null out the current `socket` or
    // schedule a duplicate reconnect — only the live socket owns that state.
    if (socket !== ws) return
    socket = null
    setState('closed')
    // Reconnect automatically as long as consumers are mounted.
    if (stateHandlers.size > 0) {
      reconnectTimer = setTimeout(() => {
        void connect()
      }, RECONNECT_DELAY_MS)
    }
  }

  ws.onerror = () => ws.close()
}

/**
 * useWebSocket returns the current connection state and optionally subscribes
 * to incoming messages. The underlying WebSocket is a singleton — all hook
 * consumers share one connection.
 *
 * @param onMessage - optional callback fired for every inbound message. The
 *   reference is stable via a ref so callers do not need to memoize it.
 */
export function useWebSocket(
  onMessage?: (msg: WsMessage) => void,
): WsReadyState {
  const [readyState, setReadyState] = useState<WsReadyState>(currentState)
  const callbackRef = useRef(onMessage)
  callbackRef.current = onMessage

  useEffect(() => {
    const stateHandler = (s: WsReadyState): void => setReadyState(s)
    stateHandlers.add(stateHandler)

    const msgHandler = (msg: WsMessage): void => callbackRef.current?.(msg)
    messageHandlers.add(msgHandler)

    void connect()

    return () => {
      stateHandlers.delete(stateHandler)
      messageHandlers.delete(msgHandler)
    }
  }, [])

  return readyState
}
