/**
 * TriggerOverlayWindowPage — transparent always-on-top overlay that shows
 * trigger alert text when triggers fire. Each alert auto-dismisses after its
 * configured duration. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Check, X } from 'lucide-react'
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

  // Live trigger alerts render as text-only — no card background or border —
  // so they don't block the gameplay view. A dark text shadow gives enough
  // contrast against any background. The dashed border / hint text only
  // belong to the test card during a positioning session.
  //
  // Pinned alerts are clamped onto the current overlay window (one monitor). A
  // position saved under the old desktop-spanning overlay — or on a monitor
  // since deselected/unplugged — could otherwise render the text off-screen.
  // The margins keep at least the start of the text reachable.
  return (
    <div
      style={{
        transition: 'opacity 0.5s ease',
        opacity,
        pointerEvents: 'none',
        ...(position
          ? {
              position: 'fixed',
              left: Math.min(Math.max(0, position.x), Math.max(0, window.innerWidth - 40)),
              top: Math.min(Math.max(0, position.y), Math.max(0, window.innerHeight - 24)),
              zIndex: 10,
            }
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
  onCancel: () => void
}

function TestAlertCard({ alert, onPositionCommit, onDone, onCancel }: TestAlertCardProps): React.ReactElement {
  const [pos, setPos] = useState(alert.position)
  const dragOffset = useRef<{ dx: number; dy: number } | null>(null)
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    setPos(alert.position)
  }, [alert.testId, alert.position.x, alert.position.y])

  function handlePointerDown(e: React.PointerEvent<HTMLDivElement>) {
    // Don't start a drag when the user clicks one of the card's buttons.
    if ((e.target as HTMLElement).closest('button')) return
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
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, marginTop: 6 }}>
        <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.6)', letterSpacing: '0.04em' }}>
          Drag to position
        </span>
        <button
          type="button"
          onClick={onCancel}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 3,
            fontSize: 10,
            padding: '2px 6px',
            borderRadius: 3,
            backgroundColor: 'rgba(255,255,255,0.12)',
            color: '#fff',
            border: '1px solid rgba(255,255,255,0.25)',
            cursor: 'pointer',
            letterSpacing: '0.04em',
          }}
          title="Discard and revert to the previous position (Esc)"
        >
          <X size={10} />
          Cancel
        </button>
        <button
          type="button"
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
  // Each connected display's bounds in window-local coords, fetched from the
  // main process. Used to clamp the card onto a REAL monitor rather than the
  // virtual-desktop bounding rectangle, which can include dead gaps on
  // non-rectangular multi-monitor layouts.
  const displays = useRef<Array<{ x: number; y: number; width: number; height: number }> | null>(null)
  // Mirror of testAlert so end handlers can read it without putting impure side
  // effects inside a state updater (those are double-invoked in dev and have
  // proven fragile here).
  const testAlertRef = useRef<TestAlert | null>(null)
  useEffect(() => {
    testAlertRef.current = testAlert
  }, [testAlert])

  // Clamp a position into the current overlay viewport (the whole virtual
  // desktop) so a position saved on a previous monitor layout — e.g. on a
  // display that's since been unplugged — can't render the card off-screen.
  // Leaves a margin so the card body and its Done button stay reachable.
  const clampToViewport = useCallback((p: { x: number; y: number }): { x: number; y: number } => {
    const CARD_W = 120
    const CARD_H = 60
    const list = displays.current
    if (list && list.length > 0) {
      // Clamp within the display the point already falls on; otherwise snap to
      // the nearest display by center distance. This guarantees the card lands
      // on a real monitor even if the saved position came from a layout (e.g. a
      // since-unplugged display) that maps to a dead gap in the virtual desktop.
      let target = list.find(
        (d) => p.x >= d.x && p.x < d.x + d.width && p.y >= d.y && p.y < d.y + d.height,
      )
      if (!target) {
        let best = Infinity
        for (const d of list) {
          const cx = d.x + d.width / 2
          const cy = d.y + d.height / 2
          const dist = (p.x - cx) ** 2 + (p.y - cy) ** 2
          if (dist < best) {
            best = dist
            target = d
          }
        }
      }
      if (target) {
        const maxX = Math.max(target.x, target.x + target.width - CARD_W)
        const maxY = Math.max(target.y, target.y + target.height - CARD_H)
        return {
          x: Math.min(Math.max(target.x, p.x), maxX),
          y: Math.min(Math.max(target.y, p.y), maxY),
        }
      }
    }
    // Fallback before the display list resolves: clamp to the whole viewport.
    const maxX = Math.max(0, window.innerWidth - CARD_W)
    const maxY = Math.max(0, window.innerHeight - CARD_H)
    return {
      x: Math.min(Math.max(0, p.x), maxX),
      y: Math.min(Math.max(0, p.y), maxY),
    }
  }, [])

  const makeDefaultPos = useCallback((): { x: number; y: number } => {
    const c = defaultCenter.current
    if (c) return { x: Math.max(0, Math.round(c.x - 100)), y: Math.max(0, Math.round(c.y - 40)) }
    // Before the primary-center IPC resolves, fall back to the top-left of the
    // virtual desktop — always on a real monitor. The window *center* maps to
    // the seam between monitors on a multi-display setup, where the card would
    // appear to vanish entirely.
    return { x: 80, y: 80 }
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

  // Drive the overlay window's visibility + input mode from what it currently
  // needs to show:
  //   - positioning session active → 'interactive' (visible, captures the mouse
  //     so the card is draggable; the card carries Done/Cancel since the editor
  //     is behind this always-on-top window)
  //   - live alert(s) only → 'passthrough' (visible but click-through, text-only)
  //   - nothing → 'hidden' (the window is hidden entirely)
  // Hiding when idle is what guarantees the desktop-spanning overlay can never
  // capture input and lock the app out — click-through alone (setIgnoreMouse-
  // Events) proved unreliable on some multi-monitor Windows setups.
  useEffect(() => {
    const mode = testAlert ? 'interactive' : alerts.length > 0 ? 'passthrough' : 'hidden'
    void window.electron?.overlay?.setTriggerMode?.(mode)
  }, [testAlert, alerts.length])

  // Hydrate from the backend on first mount so a positioning session that
  // was started before this overlay window finished loading still shows up.
  // Otherwise the editor's initial trigger:test broadcast is lost to the
  // window-startup race and the user has to click Set Position twice.
  //
  // The primary-monitor center is fetched FIRST so that if we have to fall
  // back to a default position, it lands on the primary display rather than
  // at the virtual-desktop seam.
  useEffect(() => {
    let cancelled = false
    async function init() {
      try {
        const c = await window.electron?.screen?.triggerDefaultCenter()
        if (!cancelled && c) defaultCenter.current = c
      } catch {
        /* fall back to makeDefaultPos's top-left default */
      }
      try {
        const list = await window.electron?.screen?.triggerDisplays()
        if (!cancelled && list && list.length > 0) displays.current = list
      } catch {
        /* fall back to clampToViewport's whole-viewport clamp */
      }
      try {
        const active = await getActiveTriggerTest()
        if (cancelled || !active) return
        const fontSize = active.font_size && active.font_size > 0 ? active.font_size : 20
        setTestAlert((prev) => prev ?? {
          testId: active.test_id,
          text: active.text || '',
          color: active.color || '#ffffff',
          fontSize,
          position: active.position ? clampToViewport(active.position) : makeDefaultPos(),
        })
      } catch {
        /* no active session */
      }
    }
    void init()
    return () => { cancelled = true }
  }, [makeDefaultPos, clampToViewport])

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
      setTestAlert({
        testId: data.test_id,
        text: data.text || '',
        color: data.color || '#ffffff',
        fontSize,
        position: data.position ? clampToViewport(data.position) : makeDefaultPos(),
      })
      return
    }
    if (msg.type === WSEvent.TriggerTestSessionEnded) {
      const data = msg.data as { test_id: string }
      const cur = testAlertRef.current
      if (cur && cur.testId === data.test_id) {
        // Session ended from the editor (Done / Escape / unmount). Clearing the
        // card lets the mode effect hide the overlay and restore input/focus.
        setTestAlert(null)
      }
      return
    }
  }, [makeDefaultPos, clampToViewport])

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

  const endSession = useCallback((cancelled: boolean) => {
    const cur = testAlertRef.current
    if (!cur) return
    // Clear the card locally — the mode effect then hides the overlay (or drops
    // it to passthrough if a live alert is up), which restores input and focus.
    // Also tell the backend to end the session; `cancelled` decides whether the
    // editor reverts the position vs keeps the dragged one.
    setTestAlert(null)
    void endTriggerTestSession(cur.testId, cancelled).catch(() => {})
  }, [])

  // The card's Done button confirms (keeps the dragged position).
  const handleTestDone = useCallback(() => endSession(false), [endSession])

  // The main process fires this when the global Escape bail-out is hit — it
  // works even when focus is on a fullscreen game or the card spawned on a
  // monitor the user can't see, which is exactly when the renderer-local Escape
  // handlers can't. Treated as a CANCEL (revert), matching in-window Escape.
  useEffect(() => {
    const off = window.electron?.overlay?.onTriggerEscape?.(() => endSession(true))
    return () => off?.()
  }, [endSession])

  // Escape ends the positioning session from the overlay side too, as a CANCEL
  // (revert). Once the user drags the test card, keyboard focus is on this
  // overlay window rather than the editor, so the editor's own Escape handler
  // can't see the keypress — this guarantees Escape always works as a bail-out.
  useEffect(() => {
    if (!testAlert) return
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') endSession(true)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [testAlert, endSession])

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
          onCancel={() => endSession(true)}
        />
      )}
    </div>
  )
}
