/**
 * TriggerOverlayWindowPage — transparent always-on-top overlay that shows
 * trigger alert text when triggers fire. Each alert auto-dismisses after its
 * configured duration. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Zap } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import OverlayLockButton from '../components/OverlayLockButton'
import type { TriggerFired } from '../types/trigger'
import { postTriggerTestPosition } from '../services/api'

interface TestAlert {
  testId: string
  text: string
  color: string
  fontSize: number
  position: { x: number; y: number }
  expiresAt: number
}

interface TriggerTestPayload {
  test_id: string
  text: string
  color: string
  duration_secs: number
  font_size?: number
  position?: { x: number; y: number } | null
}

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
  const fontSize = overlayAction?.font_size && overlayAction.font_size > 0 ? overlayAction.font_size : 20
  const position = overlayAction?.position
  const positioned = !!position

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
        ...(positioned ? { position: 'fixed', left: position.x, top: position.y, zIndex: 10 } : {}),
      }}
    >
      <div
        style={{
          fontSize,
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

// ── Test alert card (draggable) ────────────────────────────────────────────────

interface TestAlertCardProps {
  alert: TestAlert
  onPositionCommit: (position: { x: number; y: number }) => void
  onMouseEnter?: () => void
  onMouseLeave?: () => void
}

function TestAlertCard({ alert, onPositionCommit, onMouseEnter, onMouseLeave }: TestAlertCardProps): React.ReactElement {
  const [pos, setPos] = useState(alert.position)
  const dragOffset = useRef<{ dx: number; dy: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    setPos(alert.position)
  }, [alert.testId, alert.position.x, alert.position.y])

  function handlePointerDown(e: React.PointerEvent<HTMLDivElement>) {
    e.currentTarget.setPointerCapture(e.pointerId)
    dragOffset.current = { dx: e.clientX - pos.x, dy: e.clientY - pos.y }
    setDragging(true)
  }

  function handlePointerMove(e: React.PointerEvent<HTMLDivElement>) {
    if (!dragOffset.current) return
    const x = Math.max(0, Math.round(e.clientX - dragOffset.current.dx))
    const y = Math.max(0, Math.round(e.clientY - dragOffset.current.dy))
    setPos({ x, y })
  }

  function handlePointerUp(e: React.PointerEvent<HTMLDivElement>) {
    if (!dragOffset.current) return
    try { e.currentTarget.releasePointerCapture(e.pointerId) } catch { /* ignore */ }
    dragOffset.current = null
    setDragging(false)
    onPositionCommit(pos)
  }

  const { color, fontSize, text } = alert

  return (
    <div
      className="no-drag"
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
      style={{
        position: 'fixed',
        left: pos.x,
        top: pos.y,
        padding: '10px 14px',
        borderRadius: 6,
        backgroundColor: 'rgba(10,10,12,0.85)',
        border: `2px dashed ${color}`,
        boxShadow: `0 0 12px ${color}55`,
        cursor: dragging ? 'grabbing' : 'grab',
        userSelect: 'none',
        pointerEvents: 'auto',
        zIndex: 50,
      }}
    >
      <div
        style={{
          fontSize,
          fontWeight: 800,
          letterSpacing: '0.04em',
          color,
          textShadow: `0 0 8px ${color}88`,
          textAlign: 'center',
        }}
      >
        {text || 'Test Overlay'}
      </div>
      <div style={{ fontSize: 9, color: 'rgba(255,255,255,0.5)', textAlign: 'center', marginTop: 4 }}>
        Drag to position · auto-dismiss in a moment
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function TriggerOverlayWindowPage(): React.ReactElement {
  const overlayOpacity = useOverlayOpacity()
  const { locked, toggleLocked, enableInteraction, enableClickThrough } = useOverlayLock()
  const [alerts, setAlerts] = useState<AlertEntry[]>([])
  const [testAlert, setTestAlert] = useState<TestAlert | null>(null)
  const gcTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const testTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

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

  // Clear stale test alert when its timer expires.
  useEffect(() => {
    if (!testAlert) return
    if (testTimerRef.current !== null) clearTimeout(testTimerRef.current)
    const remaining = testAlert.expiresAt - Date.now()
    testTimerRef.current = setTimeout(() => setTestAlert(null), Math.max(0, remaining))
    return () => {
      if (testTimerRef.current !== null) clearTimeout(testTimerRef.current)
    }
  }, [testAlert])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'trigger:fired') {
      const event = msg.data as TriggerFired
      const overlayAction = event.actions.find((a) => a.type === 'overlay_text')
      if (!overlayAction) return
      const durationMs = (overlayAction.duration_secs || 5) * 1000
      const entry: AlertEntry = {
        id: nextId++,
        event,
        expiresAt: Date.now() + durationMs,
      }
      setAlerts((prev) => [entry, ...prev].slice(0, 8))
      return
    }
    if (msg.type === 'trigger:test') {
      const data = msg.data as TriggerTestPayload
      const durationMs = Math.max(2, data.duration_secs || 5) * 1000
      const fontSize = data.font_size && data.font_size > 0 ? data.font_size : 20
      const defaultPos = {
        x: Math.max(0, Math.round(window.innerWidth / 2 - 100)),
        y: 60,
      }
      setTestAlert({
        testId: data.test_id,
        text: data.text || '',
        color: data.color || '#ffffff',
        fontSize,
        position: data.position ?? defaultPos,
        expiresAt: Date.now() + durationMs,
      })
      return
    }
  }, [])

  useWebSocket(handleMessage)

  const handleTestPositionCommit = useCallback(
    (position: { x: number; y: number }) => {
      if (!testAlert) return
      // Echo locally so the card stays put even if the round-trip is slow.
      setTestAlert((prev) => (prev ? { ...prev, position } : prev))
      void postTriggerTestPosition(testAlert.testId, position).catch(() => {
        // Best-effort; the editor may have closed. Card stays where dropped.
      })
    },
    [testAlert],
  )

  // The chrome (drag handle, lock, close) only shows during a Test/Position
  // session. The rest of the time the trigger overlay is invisible — alerts
  // pop at their pinned positions and disappear.
  const positioning = testAlert !== null

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
        backgroundColor: 'transparent',
      }}
    >
      {/* Drag handle / chrome — only rendered while positioning so a player who
          isn't actively configuring sees no header in the middle of the game. */}
      {positioning && (
        <div
          className={locked ? 'no-drag' : 'drag-region'}
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '4px 8px',
            backgroundColor: `rgba(10,10,12,${overlayOpacity * 0.82})`,
            borderBottom: '1px solid rgba(255,255,255,0.08)',
            flexShrink: 0,
            userSelect: 'none',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
            <Zap size={11} style={{ color: '#a78bfa' }} />
            <span style={{ fontSize: 10, fontWeight: 700, color: 'rgba(255,255,255,0.6)' }}>
              Positioning
            </span>
          </div>
          <div
            className="no-drag"
            onMouseEnter={enableInteraction}
            onMouseLeave={enableClickThrough}
            style={{ display: 'flex', alignItems: 'center', gap: 6 }}
          >
            <OverlayLockButton locked={locked} onToggle={toggleLocked} />
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
      )}

      {/* Alert stack — unpinned alerts flow here; pinned alerts and the test
          card render position:fixed so they ignore the header's presence. */}
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
      {testAlert && (
        <TestAlertCard
          alert={testAlert}
          onPositionCommit={handleTestPositionCommit}
          onMouseEnter={enableInteraction}
          onMouseLeave={enableClickThrough}
        />
      )}
    </div>
  )
}
