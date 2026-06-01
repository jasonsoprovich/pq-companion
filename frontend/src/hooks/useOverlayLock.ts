import { useCallback, useEffect, useState } from 'react'
import type { MouseEventHandler } from 'react'
import { getConfig } from '../services/api'
import { resolveLockedMode } from '../lib/overlays'
import type { OverlayName, LockedMode } from '../lib/overlays'

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
 *
 * The hook hands each page two prop bundles to spread — `rootInteractionProps`
 * onto the root container and `headerInteractionProps` onto the title bar —
 * and wires the hover handlers into exactly one of them based on the mode.
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

  useEffect(() => {
    let cancelled = false
    window.electron?.overlay?.getLocked().then((value) => {
      if (!cancelled) setLocked(value)
    }).catch(() => {})
    return () => {
      cancelled = true
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

  // In "interactive" mode the whole root drives the toggle; in "clickthrough"
  // mode only the header does. React's onMouseEnter/Leave are non-bubbling, so
  // putting the handlers on the header alone keeps the body click-through:
  // moving header → body fires the header's leave and restores passthrough.
  const rootInteractionProps = mode === 'interactive' ? handlers : {}
  const headerInteractionProps = mode === 'clickthrough' ? handlers : {}

  return { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps }
}
