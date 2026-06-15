import { useEffect, useRef, useState } from 'react'
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
 * Hover is tracked with mouseenter/mouseleave on the document element — the
 * overlay fills its frameless window, so entering the page IS entering the
 * overlay. This sidesteps the rootInteractionProps/headerInteractionProps
 * split in useOverlayLock (which wires hover into only one region depending
 * on the locked mode), and it works while locked too: setIgnoreMouseEvents is
 * applied with forward:true, so enter/leave keep flowing during passthrough.
 */
export function useOverlayChromeFade(): boolean {
  const [enabled, setEnabled] = useState(false)
  const [delayMs, setDelayMs] = useState(DEFAULT_FADE_DELAY_MS)
  const [hovered, setHovered] = useState(false)
  const [chromeVisible, setChromeVisible] = useState(true)
  const fadeTimer = useRef<number | null>(null)
  // While positioning overlays the chrome must stay visible so a faded-out or
  // empty overlay is still visible and grabbable.
  const positionMode = useOverlayPositionMode()

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
    const el = document.documentElement
    const onEnter = (): void => setHovered(true)
    const onLeave = (): void => setHovered(false)
    el.addEventListener('mouseenter', onEnter)
    el.addEventListener('mouseleave', onLeave)
    return () => {
      el.removeEventListener('mouseenter', onEnter)
      el.removeEventListener('mouseleave', onLeave)
    }
  }, [])

  useEffect(() => {
    if (fadeTimer.current !== null) {
      window.clearTimeout(fadeTimer.current)
      fadeTimer.current = null
    }
    if (!enabled || hovered) {
      setChromeVisible(true)
      return
    }
    fadeTimer.current = window.setTimeout(() => {
      fadeTimer.current = null
      setChromeVisible(false)
    }, delayMs)
    return () => {
      if (fadeTimer.current !== null) {
        window.clearTimeout(fadeTimer.current)
        fadeTimer.current = null
      }
    }
  }, [enabled, hovered, delayMs])

  return chromeVisible || positionMode
}
