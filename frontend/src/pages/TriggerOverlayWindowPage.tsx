/**
 * TriggerOverlayWindowPage — transparent always-on-top overlay that shows
 * trigger alert text when triggers fire. Each alert auto-dismisses after its
 * configured duration. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Check } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { TriggerFired } from '../types/trigger'
import { postTriggerTestPosition, getActiveTriggerTest, endTriggerTestSession } from '../services/api'

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
  onDone: () => void
}

function TestAlertCard({ alert, onPositionCommit, onDone }: TestAlertCardProps): React.ReactElement {
  const [pos, setPos] = useState(alert.position)
  const dragOffset = useRef<{ dx: number; dy: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    setPos(alert.position)
  }, [alert.testId, alert.position.x, alert.position.y])

  function handlePointerDown(e: React.PointerEvent<HTMLDivElement>) {
    // Don't start a drag when the user clicks the Done button.
    if ((e.target as HTMLElement).closest('[data-trigger-test-done]')) return
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
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, marginTop: 6 }}>
        <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.6)', letterSpacing: '0.04em' }}>
          Drag to position
        </span>
        <button
          type="button"
          data-trigger-test-done
          onClick={onDone}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 3,
            fontSize: 10,
            padding: '2px 6px',
            borderRadius: 3,
            backgroundColor: '#16a34a',
            color: '#fff',
            border: 'none',
            cursor: 'pointer',
            letterSpacing: '0.04em',
          }}
          title="Lock in this position and close the positioner"
        >
          <Check size={10} />
          Done
        </button>
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

// DEDUP_WINDOW_MS: if the same trigger_id fires again within this window with
// the same matched_line, treat it as a duplicate of the first event and skip.
// Defends against any double-broadcast path we haven't pinned down yet —
// users were seeing stacked "MEZ BROKE!" / "ROOT BROKE!" overlay text on a
// single in-game event. A real trigger never legitimately fires twice for
// the exact same log line at the same instant.
const DEDUP_WINDOW_MS = 750

export default function TriggerOverlayWindowPage(): React.ReactElement {
  const [alerts, setAlerts] = useState<AlertEntry[]>([])
  const [testAlert, setTestAlert] = useState<TestAlert | null>(null)
  const gcTimer = useRef<ReturnType<typeof setInterval> | null>(null)
  const lastFired = useRef<Map<string, number>>(new Map())
  // Window-local center of the primary monitor, fetched from the main process.
  // The overlay spans the whole virtual desktop, so without this the test card
  // would default to the seam between monitors. Falls back to window center.
  const defaultCenter = useRef<{ x: number; y: number } | null>(null)

  useEffect(() => {
    void window.electron?.screen?.triggerDefaultCenter()
      .then((c) => { defaultCenter.current = c })
      .catch(() => {})
  }, [])

  const makeDefaultPos = useCallback((): { x: number; y: number } => {
    const cx = defaultCenter.current?.x ?? window.innerWidth / 2
    const cy = defaultCenter.current?.y ?? window.innerHeight / 2
    return { x: Math.max(0, Math.round(cx - 100)), y: Math.max(0, Math.round(cy - 40)) }
  }, [])

  // Garbage-collect expired alerts every 250 ms. Test alerts are sticky and
  // only cleared when the editor ends the positioning session, so they're
  // explicitly excluded from this gc loop.
  useEffect(() => {
    gcTimer.current = setInterval(() => {
      const now = Date.now()
      setAlerts((prev) => prev.filter((a) => now < a.expiresAt + 500))
      for (const [k, t] of lastFired.current) {
        if (now - t > DEDUP_WINDOW_MS) lastFired.current.delete(k)
      }
    }, 250)
    return () => {
      if (gcTimer.current !== null) clearInterval(gcTimer.current)
    }
  }, [])

  // Click-through is toggled wholesale by whether a positioning session is
  // active. Earlier versions tried a per-region (mouseenter/leave) toggle so
  // the underlying app/game stayed clickable while positioning, but Electron's
  // forwarded mousemove signal was unreliable on Windows for this screen-
  // spanning transparent window — the first pointerdown never reached the
  // card, so dragging silently failed. Instead, while testAlert is set the
  // whole overlay accepts input (so the card is always draggable) and the
  // card itself carries a Done button (since the editor that opened the
  // session is behind this always-on-top window).
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
        const defaultPos = makeDefaultPos()
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
  }, [makeDefaultPos])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.TriggerFired) {
      const event = msg.data as TriggerFired
      const overlayAction = event.actions.find((a) => a.type === 'overlay_text')
      if (!overlayAction) return
      const now = Date.now()
      const key = `${event.trigger_id}|${event.matched_line}`
      const prev = lastFired.current.get(key)
      if (prev !== undefined && now - prev < DEDUP_WINDOW_MS) return
      lastFired.current.set(key, now)
      const durationMs = (overlayAction.duration_secs || 5) * 1000
      const entry: AlertEntry = {
        id: nextId++,
        event,
        expiresAt: now + durationMs,
      }
      setAlerts((prev) => [entry, ...prev].slice(0, 8))
      return
    }
    if (msg.type === WSEvent.TriggerTest) {
      const data = msg.data as TriggerTestPayload
      const fontSize = data.font_size && data.font_size > 0 ? data.font_size : 20
      const defaultPos = makeDefaultPos()
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
  }, [makeDefaultPos])

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

  const handleTestDone = useCallback(() => {
    if (!testAlert) return
    const id = testAlert.testId
    // Clear locally first so click-through is restored immediately, even if
    // the backend round-trip is slow or the editor has already closed.
    setTestAlert(null)
    void endTriggerTestSession(id).catch(() => {})
  }, [testAlert])

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
          onDone={handleTestDone}
        />
      )}
    </div>
  )
}
