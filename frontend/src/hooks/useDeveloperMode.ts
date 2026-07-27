import { useEffect, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket } from './useWebSocket'
import type { Config } from '../types/config'

// useDeveloperMode returns preferences.developer_mode. It reads the config
// once on mount and tracks live changes via the config:updated broadcast, so
// toggling Developer Mode (Settings, or Ctrl+Shift+D) immediately shows/hides
// dev-gated affordances elsewhere in the app — e.g. the inline Spell Emote
// editor on the Spells page. Defaults to false until the config loads.
export function useDeveloperMode(): boolean {
  const [enabled, setEnabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => {
        if (!cancelled) setEnabled(Boolean(c.preferences.developer_mode))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== 'config:updated') return
    const cfg = msg.data as Config
    setEnabled(Boolean(cfg?.preferences?.developer_mode))
  })

  return enabled
}
