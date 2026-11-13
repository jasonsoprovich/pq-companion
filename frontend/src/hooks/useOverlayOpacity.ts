import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { resolveOverlayOpacity } from '../lib/overlays'
import type { OverlayName } from '../lib/overlays'
import { useOverlayStack } from './overlayStackContext'

const POLL_INTERVAL = 3000

// useOverlayOpacity returns the background opacity for one overlay: its own
// override from preferences.overlay_opacities when set, otherwise the global
// overlay_opacity.
// Inside a stack, the stack's opacity.
export function useOverlayOpacity(name: OverlayName, defaultOpacity = 0.25): number {
  const stack = useOverlayStack()
  if (stack) return stack.opacity
  return useWindowOverlayOpacity(name, defaultOpacity)
}

function useWindowOverlayOpacity(name: OverlayName, defaultOpacity: number): number {
  const [opacity, setOpacity] = useState(defaultOpacity)

  useEffect(() => {
    let cancelled = false
    const fetch = (): void => {
      getConfig()
        .then((c) => {
          if (cancelled) return
          setOpacity(
            resolveOverlayOpacity(
              c.preferences.overlay_opacities,
              name,
              c.preferences.overlay_opacity,
            ),
          )
        })
        .catch(() => {})
    }
    fetch()
    const id = setInterval(fetch, POLL_INTERVAL)
    return () => { cancelled = true; clearInterval(id) }
  }, [name])

  return opacity
}
