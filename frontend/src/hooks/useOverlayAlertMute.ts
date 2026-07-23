/**
 * Bell-toggle state for an overlay header mute button. Persists to
 * localStorage under `key` so the alert-firing hook (mounted at the App
 * level, not inside the popout window) can read the same flag; a 'storage'
 * listener keeps it in sync if the same key is ever read from more than one
 * window (e.g. multiple Custom Timer group windows sharing one key).
 */
import { useCallback, useEffect, useState } from 'react'
import { loadAlertsEnabled, saveAlertsEnabled } from '../lib/overlayAlertMute'

export function useOverlayAlertMute(key: string): [boolean, () => void] {
  const [enabled, setEnabled] = useState(() => loadAlertsEnabled(key))

  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === key) setEnabled(loadAlertsEnabled(key))
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [key])

  const toggle = useCallback(() => {
    setEnabled((prev) => {
      const next = !prev
      saveAlertsEnabled(key, next)
      return next
    })
  }, [key])

  return [enabled, toggle]
}
