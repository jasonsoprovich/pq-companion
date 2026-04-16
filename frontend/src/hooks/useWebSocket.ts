import { useEffect, useRef, useState } from 'react'

const WS_URL = 'ws://localhost:8080/ws'
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

function setState(state: WsReadyState): void {
  currentState = state
  stateHandlers.forEach((h) => h(state))
}

function connect(): void {
  if (
    socket?.readyState === WebSocket.OPEN ||
    socket?.readyState === WebSocket.CONNECTING
  ) {
    return
  }
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }

  setState('connecting')
  const ws = new WebSocket(WS_URL)
  socket = ws

  ws.onopen = () => setState('open')

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
    socket = null
    setState('closed')
    // Reconnect automatically as long as consumers are mounted.
    if (stateHandlers.size > 0) {
      reconnectTimer = setTimeout(connect, RECONNECT_DELAY_MS)
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

    connect()

    return () => {
      stateHandlers.delete(stateHandler)
      messageHandlers.delete(msgHandler)
    }
  }, [])

  return readyState
}
