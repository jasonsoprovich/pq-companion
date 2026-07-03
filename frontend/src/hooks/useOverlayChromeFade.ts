import { useCallback, useEffect, useRef, useState } from 'react'
import { getConfig } from '../services/api'
import { useOverlayPositionMode } from './useOverlayPositionMode'

const POLL_INTERVAL = 3000

// Delay between the cursor leaving the overlay and the chrome fading out,
// used when preferences.overlay_fade_delay_secs is unset (0/missing).
const DEFAULT_FADE_DELAY_MS = 2500

/**
 * Drives the optional "fade when inactive" overlay behaviour
 * (preferences.overlay_fade_enabled): a few seconds after the cursor leaves
 * the overlay window the chrome — root background, border, title bar — fades
 * to fully transparent, leaving only content (timer bars, NPC stats) visible.
 * Re-entering the window restores the chrome instantly.
 *
 * Returns whether the chrome should currently be shown. Pages render the
 * chrome at the configured overlay opacity when true and fully transparent
 * when false, with a CSS transition between the two. Always returns true
 * while the preference is off.
 *
 * Activity is tracked by mouse *movement* over the window rather than paired
 * mouseenter/mouseleave. The overlay fills its frameless window, so any move
 * within the page is a move over the overlay; each move shows the chrome and
 * restarts a fade countdown, and the chrome fades once movement stops for the
 * delay. This works while locked too — setIgnoreMouseEvents is applied with
 * forward:true, so move events keep flowing during passthrough.
 *
 * Why movement and not mouseenter/mouseleave: in an Electron overlay the
 * leave event is frequently dropped. Toggling passthrough
 * (setIgnoreMouseEvents) under a moving cursor confuses Chromium's boundary
 * tracking, so after a few rapid hovers a mouseleave never arrives, the old
 * `hovered` flag stuck true, and the chrome stayed visible forever (never
 * responding to hover again). Absence of movement can't be "dropped", so the
 * countdown always eventually fires and the fade self-heals.
 */
export function useOverlayChromeFade(): boolean {
  const [enabled, setEnabled] = useState(false)
  const [delayMs, setDelayMs] = useState(DEFAULT_FADE_DELAY_MS)
  const [chromeVisible, setChromeVisible] = useState(true)
  const fadeTimer = useRef<number | null>(null)
  // Read inside the movement handler (a stable listener) so live config
  // changes take effect without re-subscribing.
  const enabledRef = useRef(enabled)
  enabledRef.current = enabled
  const delayRef = useRef(delayMs)
  delayRef.current = delayMs
  // While positioning overlays the chrome must stay visible so a faded-out or
  // empty overlay is still visible and grabbable.
  const positionMode = useOverlayPositionMode()

  const clearFade = useCallback(() => {
    if (fadeTimer.current !== null) {
      window.clearTimeout(fadeTimer.current)
      fadeTimer.current = null
    }
  }, [])

  const scheduleFade = useCallback(() => {
    clearFade()
    fadeTimer.current = window.setTimeout(() => {
      fadeTimer.current = null
      setChromeVisible(false)
    }, delayRef.current)
  }, [clearFade])

  // Poll the config so a settings change is picked up live, mirroring
  // useOverlayOpacity.
  useEffect(() => {
    let cancelled = false
    const fetch = (): void => {
      getConfig()
        .then((c) => {
          if (cancelled) return
          setEnabled(Boolean(c.preferences.overlay_fade_enabled))
          const secs = c.preferences.overlay_fade_delay_secs
          setDelayMs(secs && secs > 0 ? secs * 1000 : DEFAULT_FADE_DELAY_MS)
        })
        .catch(() => {})
    }
    fetch()
    const id = setInterval(fetch, POLL_INTERVAL)
    return () => { cancelled = true; clearInterval(id) }
  }, [])

  useEffect(() => {
    // Any movement over the overlay = active: show chrome, restart countdown.
    const onMove = (): void => {
      if (!enabledRef.current) return
      setChromeVisible(true)
      scheduleFade()
    }
    // A leave, when it does arrive, is a fast path to start the countdown; the
    // movement handler is the reliable backstop when it doesn't.
    const onLeave = (): void => {
      if (!enabledRef.current) return
      scheduleFade()
    }
    window.addEventListener('mousemove', onMove)
    document.documentElement.addEventListener('mouseleave', onLeave)
    return () => {
      window.removeEventListener('mousemove', onMove)
      document.documentElement.removeEventListener('mouseleave', onLeave)
      clearFade()
    }
  }, [scheduleFade, clearFade])

  // React to the preference toggling: off → chrome pinned on; on → reveal the
  // chrome and start the countdown so an untouched overlay fades shortly after
  // it opens (matching the prior behaviour), without needing a first hover.
  useEffect(() => {
    if (!enabled) {
      clearFade()
      setChromeVisible(true)
    } else {
      setChromeVisible(true)
      scheduleFade()
    }
  }, [enabled, delayMs, scheduleFade, clearFade])

  return chromeVisible || positionMode
}
