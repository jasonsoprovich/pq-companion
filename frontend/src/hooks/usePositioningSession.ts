/**
 * usePositioningSession — drives an overlay-text positioning session against
 * the trigger overlay window: opens the overlay, fires the draggable test
 * card, applies drag updates to the caller's position field live, and tears
 * the session down on confirm / cancel / Escape / unmount.
 *
 * Extracted verbatim from NotificationActionEditor's OverlayTextFields so the
 * Settings page's "default overlay position" control shares the exact same
 * (multi-monitor-hardened) flow rather than reimplementing it.
 */
import { useEffect, useRef, useState } from 'react'
import { fireTriggerTestOverlay, endTriggerTestSession } from '../services/api'
import { useWebSocket } from './useWebSocket'
import { useEscapeToClose } from './useEscapeToClose'
import { WSEvent } from '../lib/wsEvents'

export interface PositioningSessionOptions {
  position: { x: number; y: number } | null | undefined
  onPositionChange?: (p: { x: number; y: number } | null) => void
  /** Text shown on the draggable test card. */
  testText: string
  testColor: string
  testDurationSecs: number
}

export interface PositioningSession {
  positioning: boolean
  start: () => void
  confirm: () => void
  cancel: () => void
  /** Start when idle, confirm when positioning — the Set Position button action. */
  toggle: () => void
}

export function usePositioningSession({
  position,
  onPositionChange,
  testText,
  testColor,
  testDurationSecs,
}: PositioningSessionOptions): PositioningSession {
  // A per-editor session id is round-tripped through the test endpoints so
  // simultaneous editors don't clobber each other's position updates.
  const [testId] = useState(() => `test-${Math.random().toString(36).slice(2)}-${Date.now()}`)
  const [positioning, setPositioning] = useState(false)
  // The position as it was when the session started. Drag updates are applied
  // to the field live during a session, so cancel/Escape needs this to revert.
  const startPosRef = useRef<{ x: number; y: number } | null>(null)
  // Whether a positioning session was ever opened by this editor. Drives the
  // unmount teardown independently of the `positioning` flag, so a desync can't
  // strand the overlay card.
  const everStartedRef = useRef(false)

  useWebSocket((msg) => {
    if (msg.type === WSEvent.TriggerTestPosition) {
      const data = msg.data as { test_id: string; position: { x: number; y: number } }
      if (data.test_id !== testId) return
      onPositionChange?.(data.position)
      return
    }
    if (msg.type === WSEvent.TriggerTestSessionEnded) {
      const data = msg.data as { test_id: string; cancelled?: boolean }
      // The session may have been ended from the overlay window (its Done /
      // Cancel button or Escape there). Reset our button state on any
      // session-ended while we're positioning — there's only ever one
      // positioner at a time, so we don't need an exact id match to clear the
      // stuck "Done" label. Revert the position only on an id match + cancel.
      if (data.test_id === testId && data.cancelled) {
        onPositionChange?.(startPosRef.current ?? null)
      }
      setPositioning(false)
      return
    }
  })

  // Always end the session when this editor unmounts (e.g. the trigger modal
  // closes / saves mid-session). This runs unconditionally — not gated on the
  // `positioning` flag — so even if that state desynced, the desktop-spanning
  // overlay can never be left with an orphaned input-capturing card (the cause
  // of the "app hung, nothing clickable" reports). The backend no-ops if the
  // id doesn't match an active session.
  useEffect(() => {
    return () => {
      if (everStartedRef.current) {
        // Force the overlay hidden from the main process so it can never be
        // left capturing input if this editor goes away mid-session.
        void window.electron?.overlay?.setTriggerMode?.('hidden')
        void endTriggerTestSession(testId).catch(() => {})
      }
    }
  }, [testId])

  function start() {
    startPosRef.current = position ?? null
    everStartedRef.current = true
    setPositioning(true)
    void window.electron?.overlay?.openTrigger?.()
    void fireTriggerTestOverlay({
      test_id: testId,
      text: testText || 'Test alert',
      color: testColor || '#ffffff',
      // duration_secs is informational only — sticky session, no auto-dismiss.
      duration_secs: Math.max(8, testDurationSecs || 5),
      position: position ?? null,
    }).catch(() => {
      // If we can't open the session, roll the toggle back so the button
      // doesn't get stuck in the "Done" state.
      setPositioning(false)
    })
  }

  // Confirm keeps the dragged position (already applied to the field live).
  function confirm() {
    setPositioning(false)
    // Force the overlay hidden via the main process so input is restored even
    // if the overlay renderer is slow to process the session-ended broadcast.
    void window.electron?.overlay?.setTriggerMode?.('hidden')
    void endTriggerTestSession(testId, false).catch(() => {})
  }

  // Cancel reverts the field to the position captured when the session began.
  function cancel() {
    setPositioning(false)
    onPositionChange?.(startPosRef.current ?? null)
    void window.electron?.overlay?.setTriggerMode?.('hidden')
    void endTriggerTestSession(testId, true).catch(() => {})
  }

  // The editor's Done button confirms; Escape cancels and reverts.
  useEscapeToClose(cancel, positioning)

  function toggle() {
    if (positioning) confirm()
    else start()
  }

  return { positioning, start, confirm, cancel, toggle }
}
