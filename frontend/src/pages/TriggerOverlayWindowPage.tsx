/**
 * TriggerOverlayWindowPage — transparent always-on-top overlay that shows
 * trigger alert text when triggers fire. Each alert auto-dismisses after its
 * configured duration. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Zap } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayClickThrough } from '../hooks/useOverlayClickThrough'
import type { TriggerFired } from '../types/trigger'

interface AlertEntry {
  id: number
  event: TriggerFired
  expiresAt: number
}

let nextId = 1

// ── Alert card ─────────────────────────────────────────────────────────────────

function AlertCard({ entry, bgOpacity }: { entry: AlertEntry; bgOpacity: number }): React.ReactElement {
  const [opacity, setOpacity] = useState(1)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Fade out in the last 500 ms of the alert's life.
  useEffect(() => {
    const remaining = entry.expiresAt - Date.now()
    if (remaining <= 0) {
      setOpacity(0)
      return
    }
    const fadeDelay = Math.max(0, remaining - 500)
    timerRef.current = setTimeout(() => setOpacity(0), fadeDelay)
    return () => {
      if (timerRef.current !== null) clearTimeout(timerRef.current)
    }
  }, [entry.expiresAt])

  // Show the first overlay_text action's text and color, or fall back to trigger name.
  const overlayAction = entry.event.actions.find((a) => a.type === 'overlay_text')
  const text = overlayAction?.text || entry.event.trigger_name
  const color = overlayAction?.color || '#ffffff'

  return (
    <div
      style={{
        padding: '10px 14px',
        borderRadius: 6,
        backgroundColor: `rgba(10,10,12,${bgOpacity})`,
        border: `1px solid ${color}44`,
        boxShadow: `0 0 12px ${color}22`,
        transition: 'opacity 0.5s ease',
        opacity,
        pointerEvents: 'none', // individual cards don't capture mouse
      }}
    >
      <div
        style={{
          fontSize: 20,
          fontWeight: 800,
          letterSpacing: '0.04em',
          color,
          textShadow: `0 0 8px ${color}88`,
          textAlign: 'center',
          userSelect: 'none',
        }}
      >
        {text}
      </div>
      <div
        style={{
          fontSize: 10,
          color: 'rgba(255,255,255,0.35)',
          textAlign: 'center',
          marginTop: 4,
          fontFamily: 'monospace',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          userSelect: 'none',
        }}
      >
        {entry.event.matched_line}
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function TriggerOverlayWindowPage(): React.ReactElement {
  const overlayOpacity = useOverlayOpacity()
  const { enableInteraction, enableClickThrough } = useOverlayClickThrough()
  const [alerts, setAlerts] = useState<AlertEntry[]>([])
  const gcTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  // Garbage-collect expired alerts every 250 ms.
  useEffect(() => {
    gcTimer.current = setInterval(() => {
      const now = Date.now()
      setAlerts((prev) => prev.filter((a) => now < a.expiresAt + 500))
    }, 250)
    return () => {
      if (gcTimer.current !== null) clearInterval(gcTimer.current)
    }
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== 'trigger:fired') return
    const event = msg.data as TriggerFired

    // Only show alerts that have an overlay_text action.
    const overlayAction = event.actions.find((a) => a.type === 'overlay_text')
    if (!overlayAction) return

    const durationMs = (overlayAction.duration_secs || 5) * 1000
    const entry: AlertEntry = {
      id: nextId++,
      event,
      expiresAt: Date.now() + durationMs,
    }

    setAlerts((prev) => [entry, ...prev].slice(0, 8)) // cap at 8 visible alerts
  }, [])

  useWebSocket(handleMessage)

  const isEmpty = alerts.length === 0

  return (
    <div
      onMouseLeave={enableClickThrough}
      style={{
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        // Fully transparent background when empty so it doesn't block the game
        backgroundColor: isEmpty ? 'rgba(0,0,0,0.01)' : 'transparent',
      }}
    >
      {/* Drag handle — always present so the window can be repositioned */}
      <div
        className="drag-region"
        onMouseEnter={enableInteraction}
        onMouseLeave={enableClickThrough}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '4px 8px',
          backgroundColor: isEmpty ? `rgba(10,10,12,${overlayOpacity * 0.6})` : `rgba(10,10,12,${overlayOpacity * 0.82})`,
          borderBottom: '1px solid rgba(255,255,255,0.08)',
          flexShrink: 0,
          userSelect: 'none',
          transition: 'background-color 0.3s',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Zap size={11} style={{ color: '#a78bfa' }} />
          <span style={{ fontSize: 10, fontWeight: 700, color: 'rgba(255,255,255,0.6)' }}>
            Triggers
          </span>
          {alerts.length > 0 && (
            <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.3)', marginLeft: 2 }}>
              {alerts.length}
            </span>
          )}
        </div>
        <div className="no-drag">
          <button
            onClick={() => window.electron?.overlay?.closeTrigger()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {/* Alert stack */}
      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          gap: 6,
          padding: alerts.length > 0 ? '8px 8px' : 0,
          overflow: 'hidden',
        }}
      >
        {alerts.map((entry) => (
          <AlertCard key={entry.id} entry={entry} bgOpacity={overlayOpacity} />
        ))}
      </div>
    </div>
  )
}
