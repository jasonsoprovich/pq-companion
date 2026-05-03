/**
 * TriggerOverlayWindowPage — transparent always-on-top overlay that shows
 * trigger alert text when triggers fire. Each alert auto-dismisses after its
 * configured duration. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Zap, X as XIcon } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import type { TriggerFired } from '../types/trigger'
import { postTriggerTestPosition, endTriggerTestSession, getActiveTriggerTest } from '../services/api'

interface TestAlert {
  testId: string
  text: string
  color: string
  fontSize: number
  position: { x: number; y: number }
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

function AlertCard({ entry }: { entry: AlertEntry }): React.ReactElement {
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

  // Live trigger alerts render as text-only — no card background or border —
  // so they don't block the gameplay view. A dark text shadow gives enough
  // contrast against any background. The dashed border / hint text only
  // belong to the test card during a positioning session.
  return (
    <div
      style={{
        transition: 'opacity 0.5s ease',
        opacity,
        pointerEvents: 'none',
        ...(positioned
          ? { position: 'fixed', left: position.x, top: position.y, zIndex: 10 }
          : { padding: '4px 8px' }),
      }}
    >
      <div
        style={{
          fontSize,
          fontWeight: 800,
          letterSpacing: '0.04em',
          color,
          textShadow: `0 0 8px ${color}aa, 0 0 3px rgba(0,0,0,0.95), 0 1px 2px rgba(0,0,0,0.95)`,
          textAlign: 'center',
          userSelect: 'none',
          whiteSpace: 'nowrap',
        }}
      >
        {text}
      </div>
    </div>
  )
}

// ── Test alert card (draggable) ────────────────────────────────────────────────

interface TestAlertCardProps {
  alert: TestAlert
  onPositionCommit: (position: { x: number; y: number }) => void
}

function TestAlertCard({ alert, onPositionCommit }: TestAlertCardProps): React.ReactElement {
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
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
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
      <div style={{ fontSize: 9, color: 'rgba(255,255,255,0.6)', textAlign: 'center', marginTop: 4, letterSpacing: '0.04em' }}>
        Drag anywhere to position
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function TriggerOverlayWindowPage(): React.ReactElement {
  const overlayOpacity = useOverlayOpacity()
  const [alerts, setAlerts] = useState<AlertEntry[]>([])
  const [testAlert, setTestAlert] = useState<TestAlert | null>(null)
  const gcTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  // Garbage-collect expired alerts every 250 ms. Test alerts are sticky and
  // only cleared when the editor ends the positioning session, so they're
  // explicitly excluded from this gc loop.
  useEffect(() => {
    gcTimer.current = setInterval(() => {
      const now = Date.now()
      setAlerts((prev) => prev.filter((a) => now < a.expiresAt + 500))
    }, 250)
    return () => {
      if (gcTimer.current !== null) clearInterval(gcTimer.current)
    }
  }, [])

  // Window is fully click-through unless a positioning session is active.
  // setIgnoreMouseEvents(true) lets the OS pass clicks straight to the game
  // underneath; flipping it off lets the user drag the test card.
  useEffect(() => {
    if (testAlert) {
      window.electron?.overlay?.setIgnoreMouseEvents(false)
    } else {
      window.electron?.overlay?.setIgnoreMouseEvents(true)
    }
  }, [testAlert])

  // Hydrate from the backend on first mount so a positioning session that
  // was started before this overlay window finished loading still shows up.
  // Otherwise the editor's initial trigger:test broadcast is lost to the
  // window-startup race and the user has to click Set Position twice.
  useEffect(() => {
    let cancelled = false
    void getActiveTriggerTest()
      .then((active) => {
        if (cancelled || !active) return
        const fontSize = active.font_size && active.font_size > 0 ? active.font_size : 20
        const defaultPos = {
          x: Math.max(0, Math.round(window.innerWidth / 2 - 100)),
          y: Math.max(0, Math.round(window.innerHeight / 2 - 40)),
        }
        setTestAlert((prev) => prev ?? {
          testId: active.test_id,
          text: active.text || '',
          color: active.color || '#ffffff',
          fontSize,
          position: active.position ?? defaultPos,
        })
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

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
      const fontSize = data.font_size && data.font_size > 0 ? data.font_size : 20
      const defaultPos = {
        x: Math.max(0, Math.round(window.innerWidth / 2 - 100)),
        y: Math.max(0, Math.round(window.innerHeight / 2 - 40)),
      }
      setTestAlert({
        testId: data.test_id,
        text: data.text || '',
        color: data.color || '#ffffff',
        fontSize,
        position: data.position ?? defaultPos,
      })
      return
    }
    if (msg.type === 'trigger:test_session_ended') {
      const data = msg.data as { test_id: string }
      setTestAlert((prev) => (prev && prev.testId === data.test_id ? null : prev))
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

  const handleEndSession = useCallback(() => {
    if (!testAlert) return
    void endTriggerTestSession(testAlert.testId).catch(() => {})
  }, [testAlert])

  // The chrome (positioning banner + close) only shows during a Test/Position
  // session. The rest of the time the trigger overlay is invisible and
  // click-through — real triggers pop at their pinned positions only.
  const positioning = testAlert !== null

  return (
    <div
      style={{
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        // Tint the canvas while positioning so the user can see the area
        // they're working with. Transparent the rest of the time so the
        // window doesn't dim the game.
        backgroundColor: positioning ? 'rgba(10,10,12,0.28)' : 'transparent',
        transition: 'background-color 0.15s ease',
      }}
    >
      {/* Positioning banner — only rendered during a session. The whole banner
          is a drag handle (drag-region) so the user can move the entire
          positioning canvas around the screen; the Done button and label hint
          opt out of dragging via no-drag so clicks register normally. */}
      {positioning && (
        <div
          className="drag-region"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 12,
            padding: '8px 14px',
            backgroundColor: `rgba(10,10,12,${Math.max(0.85, overlayOpacity)})`,
            borderBottom: '1px solid rgba(167,139,250,0.5)',
            flexShrink: 0,
            userSelect: 'none',
            cursor: 'grab',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
            <Zap size={14} style={{ color: '#a78bfa', flexShrink: 0 }} />
            <span style={{ fontSize: 12, fontWeight: 700, color: 'rgba(255,255,255,0.92)', whiteSpace: 'nowrap' }}>
              Positioning trigger alert
            </span>
            <span style={{ fontSize: 11, color: 'rgba(255,255,255,0.55)', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              · drag this bar to move the canvas, drag the card to place the alert
            </span>
          </div>
          <button
            className="no-drag"
            onClick={handleEndSession}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              fontSize: 12,
              fontWeight: 700,
              lineHeight: 1,
              padding: '6px 14px',
              borderRadius: 4,
              border: '1px solid rgba(34,197,94,0.6)',
              backgroundColor: 'rgba(34,197,94,0.85)',
              color: '#ffffff',
              cursor: 'pointer',
              boxShadow: '0 0 12px rgba(34,197,94,0.4)',
              flexShrink: 0,
            }}
            title="End the positioning session and save the current placement"
          >
            <XIcon size={13} />
            Done — Save Position
          </button>
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
          <AlertCard key={entry.id} entry={entry} />
        ))}
      </div>
      {testAlert && (
        <TestAlertCard
          alert={testAlert}
          onPositionCommit={handleTestPositionCommit}
        />
      )}
    </div>
  )
}
