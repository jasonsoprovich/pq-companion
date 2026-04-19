import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'

const POLL_INTERVAL = 3000

export function useOverlayOpacity(defaultOpacity = 0.25): number {
  const [opacity, setOpacity] = useState(defaultOpacity)

  useEffect(() => {
    let cancelled = false
    const fetch = (): void => {
      getConfig()
        .then((c) => { if (!cancelled) setOpacity(c.preferences.overlay_opacity) })
        .catch(() => {})
    }
    fetch()
    const id = setInterval(fetch, POLL_INTERVAL)
    return () => { cancelled = true; clearInterval(id) }
  }, [])

  return opacity
}
