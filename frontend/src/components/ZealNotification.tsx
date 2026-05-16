import React, { useEffect, useRef, useState } from 'react'
import { CheckCircle2, X, Zap } from 'lucide-react'
import { useWebSocket, type WsMessage } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

// Auto-dismiss window. Long enough to read the line, short enough not to
// linger after the player has already started doing something else.
const DISMISS_MS = 4000

// Suppress repeat toasts within this window so a flappy pipe (Zeal restart,
// app reconnect) doesn't blast the user with notifications.
const DEDUPE_MS = 30_000

type Phase =
  | { kind: 'idle' }
  | { kind: 'connected' }

// ZealNotification renders a transient bottom-right toast when the backend
// reports the Zeal pipe has connected. Listens directly to the WebSocket so
// no parent wiring is needed — mount it once at the app root.
export default function ZealNotification(): React.ReactElement | null {
  const [phase, setPhase] = useState<Phase>({ kind: 'idle' })
  const dismissTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastShownAt = useRef<number>(0)

  useEffect(() => {
    return () => {
      if (dismissTimer.current) clearTimeout(dismissTimer.current)
    }
  }, [])

  useWebSocket((msg: WsMessage) => {
    if (msg.type !== WSEvent.ZealConnected) return
    const now = Date.now()
    if (now - lastShownAt.current < DEDUPE_MS) return
    lastShownAt.current = now
    setPhase({ kind: 'connected' })
    if (dismissTimer.current) clearTimeout(dismissTimer.current)
    dismissTimer.current = setTimeout(() => setPhase({ kind: 'idle' }), DISMISS_MS)
  })

  if (phase.kind === 'idle') return null

  return (
    <div
      role="status"
      className="pointer-events-auto fixed bottom-4 right-4 z-50 flex items-center gap-3 rounded-lg px-4 py-3 shadow-lg"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        maxWidth: 360,
        animation: 'pq-zeal-toast-in 180ms ease-out',
      }}
    >
      <CheckCircle2 size={18} style={{ color: '#22c55e', flexShrink: 0 }} />
      <div className="flex-1">
        <p
          className="flex items-center gap-1.5 text-sm font-semibold"
          style={{ color: 'var(--color-foreground)' }}
        >
          <Zap size={12} style={{ color: 'var(--color-primary)' }} />
          Zeal connected
        </p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Live targeting, HP bar, and pipe triggers are active.
        </p>
      </div>
      <button
        onClick={() => setPhase({ kind: 'idle' })}
        className="rounded p-1"
        style={{ color: 'var(--color-muted-foreground)', cursor: 'pointer' }}
        aria-label="Dismiss"
      >
        <X size={14} />
      </button>
      <style>{`
        @keyframes pq-zeal-toast-in {
          from { transform: translateY(8px); opacity: 0; }
          to   { transform: translateY(0);   opacity: 1; }
        }
      `}</style>
    </div>
  )
}
