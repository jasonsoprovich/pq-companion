/**
 * TriggerOverlayWindowPage — transparent always-on-top overlay that shows
 * trigger alert text when triggers fire. Each alert auto-dismisses after its
 * configured duration. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { TriggerFired } from '../types/trigger'
import { postTriggerTestPosition, getActiveTriggerTest } from '../services/api'

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
  onInteractiveChange: (interactive: boolean) => void
}

function TestAlertCard({ alert, onPositionCommit, onInteractiveChange }: TestAlertCardProps): React.ReactElement {
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
    onInteractiveChange(true)
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

  // Hover state drives per-region click-through: outside the card the trigger
  // overlay is fully click-through so the underlying app/game keeps receiving
  // input. While dragging, ignore mouseleave — pointer capture means the card
  // tracks the cursor but the cursor can briefly fall outside the box.
  function handleMouseEnter() {
    onInteractiveChange(true)
  }
  function handleMouseLeave() {
    if (dragOffset.current === null) onInteractiveChange(false)
  }

  const { color, fontSize, text } = alert

  return (
    <div
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerUp}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
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
        Drag to position · click Done in the editor to lock
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function TriggerOverlayWindowPage(): React.ReactElement {
  const [alerts, setAlerts] = useState<AlertEntry[]>([])
  const [testAlert, setTestAlert] = useState<TestAlert | null>(null)
  const [interactive, setInteractive] = useState(false)
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

  // Per-region click-through: keep the window click-through everywhere except
  // when the cursor is over the draggable test card. The main process loaded
  // the window with setIgnoreMouseEvents(true, { forward: true }) so DOM
  // mouseenter/leave still fire on elements while click-through is on.
  useEffect(() => {
    if (testAlert && interactive) {
      window.electron?.overlay?.setIgnoreMouseEvents(false)
    } else {
      window.electron?.overlay?.setIgnoreMouseEvents(true)
    }
  }, [testAlert, interactive])

  // Reset the interactive flag whenever a positioning session ends so the
  // window snaps back to click-through even if the card unmounted mid-hover.
  useEffect(() => {
    if (!testAlert) setInteractive(false)
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
    if (msg.type === WSEvent.TriggerFired) {
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
    if (msg.type === WSEvent.TriggerTest) {
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
    if (msg.type === WSEvent.TriggerTestSessionEnded) {
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

  // The trigger overlay is fully invisible and click-through. The only thing
  // it ever shows is real-fire alerts (text-only, pointer-events:none) and,
  // during a positioning session, a single draggable test card. The "Done"
  // confirmation lives in the editor that opened the session — there's no
  // outer chrome here.
  return (
    <div
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
      <div
        style={{
          flex: 1,
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'center',
          alignItems: 'center',
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
          onInteractiveChange={setInteractive}
        />
      )}
    </div>
  )
}
