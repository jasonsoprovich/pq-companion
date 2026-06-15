import { useCallback, useEffect, useRef, useState } from 'react'
import type { MouseEventHandler } from 'react'
import { getConfig } from '../services/api'
import { resolveLockedMode } from '../lib/overlays'
import type { OverlayName, LockedMode } from '../lib/overlays'
import { useOverlayPositionMode } from './useOverlayPositionMode'

const MODE_POLL_INTERVAL = 3000

type InteractionProps = {
  onMouseEnter?: MouseEventHandler
  onMouseLeave?: MouseEventHandler
}

/**
 * Manages an overlay popout window's lock state and its user-selectable
 * locked behaviour.
 *
 * Unlocked (default): the window is fully interactive — drag the header to
 * move, drag edges to resize, all controls clickable.
 *
 * Locked: the window passes mouse events through to the game underneath via
 * Electron's setIgnoreMouseEvents. Click-through is a single window-global
 * flag, so a region can't be both passthrough AND interactive at once — it's
 * time-multiplexed by cursor position. forward:true keeps mouseenter/leave
 * flowing to the renderer even while passthrough is on, which is what drives
 * the auto-toggle. Move/resize stay disabled while locked (drag gated by the
 * no-drag class, resize turned off in the main process).
 *
 * Which region re-enables interaction on hover depends on the per-overlay
 * `overlay_locked_modes` preference (see lib/overlays.ts):
 *   "interactive"  — the WHOLE overlay goes interactive on hover, so the
 *                    scrollable list and per-row controls work (issue #127).
 *   "clickthrough" — only the HEADER goes interactive on hover; the body
 *                    stays click-through so clicks reach the game.
 *   "display-only" — nothing re-enables interaction; the title bar is hidden
 *                    (via the `overlay-hide-header` document class) so the
 *                    overlay never captures the mouse. A pure HUD.
 *
 * The hook hands each page two prop bundles to spread — `rootInteractionProps`
 * onto the root container and `headerInteractionProps` onto the title bar —
 * and wires the hover handlers into exactly one of them based on the mode.
 *
 * The global "Position overlays" mode (useOverlayPositionMode) overrides all of
 * this: the main process makes every overlay interactive, so the hover toggling
 * is suspended (it would fight the main process) and the hidden title bar is
 * revealed so the overlay can be dragged and unlocked.
 *
 * Lock state is persisted per overlay in the main process; the mode is polled
 * from the app config (same cadence as overlay opacity).
 */
export function useOverlayLock(name: OverlayName): {
  locked: boolean
  mode: LockedMode
  toggleLocked: () => void
  rootInteractionProps: InteractionProps
  headerInteractionProps: InteractionProps
} {
  const [locked, setLocked] = useState(false)
  const [mode, setMode] = useState<LockedMode>('interactive')
  const positionMode = useOverlayPositionMode()
  // Always-current lock value for the display-only restore effect below, so it
  // can read the lock state without re-running on every lock toggle.
  const lockedRef = useRef(locked)
  lockedRef.current = locked

  useEffect(() => {
    let cancelled = false
    window.electron?.overlay?.getLocked().then((value) => {
      if (!cancelled) setLocked(value)
    }).catch(() => {})
    // The main process can clear the lock from outside this window (a position
    // reset auto-unlocks the overlay so a stuck off-screen window is movable
    // again). Subscribe so the padlock button reflects that without a reload.
    const off = window.electron?.overlay?.onLockChanged?.((value) => {
      if (!cancelled) setLocked(value)
    })
    return () => {
      cancelled = true
      off?.()
    }
  }, [])

  // Poll the config for this overlay's mode so a settings change is picked up
  // live, mirroring useOverlayOpacity.
  useEffect(() => {
    let cancelled = false
    const fetch = (): void => {
      getConfig()
        .then((c) => {
          if (!cancelled) {
            setMode(resolveLockedMode(c.preferences.overlay_locked_modes, name))
          }
        })
        .catch(() => {})
    }
    fetch()
    const id = setInterval(fetch, MODE_POLL_INTERVAL)
    return () => { cancelled = true; clearInterval(id) }
  }, [name])

  const toggleLocked = useCallback(() => {
    setLocked((prev) => {
      const next = !prev
      // setLocked in the main process applies setIgnoreMouseEvents(next) to
      // sync the window state. When locking, the user just clicked the lock
      // button (which lives in the header), so the cursor is over the header
      // — re-assert interactive mode after the round-trip so the header stays
      // clickable until the cursor actually leaves it.
      const p = window.electron?.overlay?.setLocked(next)
      if (next && p) {
        p.then(() => window.electron?.overlay?.setIgnoreMouseEvents(false))
         .catch(() => {})
      }
      return next
    })
  }, [])

  // While locked, hovering the active region temporarily disables passthrough
  // so clicks and scroll register; leaving it restores click-through.
  const enableInteraction = useCallback(() => {
    if (!locked) return
    window.electron?.overlay?.setIgnoreMouseEvents(false)
  }, [locked])

  const enableClickThrough = useCallback(() => {
    if (!locked) return
    window.electron?.overlay?.setIgnoreMouseEvents(true)
  }, [locked])

  const handlers: InteractionProps = {
    onMouseEnter: enableInteraction,
    onMouseLeave: enableClickThrough,
  }

  // Hide the title bar entirely while a display-only overlay is "set" — but not
  // while positioning, when it must be visible to grab. The class lives on the
  // document so a single CSS rule can hide every overlay header (see index.css).
  useEffect(() => {
    const hide = mode === 'display-only' && !positionMode
    const root = document.documentElement
    root.classList.toggle('overlay-hide-header', hide)
    return () => root.classList.remove('overlay-hide-header')
  }, [mode, positionMode])

  // Display-only forces full click-through regardless of the lock flag (a pure
  // HUD must never capture), so it works even if the user never locked it. We
  // only act on entering/leaving display-only (not on every lock toggle, which
  // the main process and toggleLocked already handle) to avoid fighting the
  // "re-assert interactive after locking" logic for the other modes. While
  // positioning, the main process owns input state, so stay out of its way.
  const wasDisplayOnly = useRef(false)
  useEffect(() => {
    if (positionMode) return
    const isDisplayOnly = mode === 'display-only'
    if (isDisplayOnly) {
      window.electron?.overlay?.setIgnoreMouseEvents(true)
    } else if (wasDisplayOnly.current) {
      // Just left display-only — restore the lock-based base state.
      window.electron?.overlay?.setIgnoreMouseEvents(lockedRef.current)
    }
    wasDisplayOnly.current = isDisplayOnly
  }, [mode, positionMode])

  // In "interactive" mode the whole root drives the toggle; in "clickthrough"
  // mode only the header does. React's onMouseEnter/Leave are non-bubbling, so
  // putting the handlers on the header alone keeps the body click-through:
  // moving header → body fires the header's leave and restores passthrough.
  // "display-only" wires up neither, so the overlay stays fully click-through.
  // While positioning, the main process owns input state, so suspend the hover
  // toggling to avoid fighting it.
  const active = !positionMode
  const rootInteractionProps = active && mode === 'interactive' ? handlers : {}
  const headerInteractionProps = active && mode === 'clickthrough' ? handlers : {}

  return { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps }
}
