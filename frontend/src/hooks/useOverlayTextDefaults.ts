/**
 * useOverlayTextDefaults — fetches the global overlay-text style defaults
 * (Settings → Preferences) once on mount, so editors can show the resolved
 * "App default" swatches/values next to their per-trigger override fields.
 *
 * A one-shot fetch is enough here: the editor only needs the defaults for
 * display, and a stale value self-corrects the next time the editor opens.
 * (The overlay renderer, which must track Settings saves live, polls the
 * config itself — see TriggerOverlayWindowPage.)
 */
import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import type { OverlayTextStyleDefaults } from '../lib/overlayTextStyle'

export function useOverlayTextDefaults(): OverlayTextStyleDefaults | null {
  const [defaults, setDefaults] = useState<OverlayTextStyleDefaults | null>(null)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (cancelled) return
        setDefaults({
          default_overlay_text_color: c.preferences?.default_overlay_text_color,
          default_overlay_glow_color: c.preferences?.default_overlay_glow_color,
          default_overlay_font_family: c.preferences?.default_overlay_font_family,
          default_overlay_font_size: c.preferences?.default_overlay_font_size,
        })
      })
      .catch(() => {
        /* editors fall back to the built-in defaults */
      })
    return () => {
      cancelled = true
    }
  }, [])

  return defaults
}
