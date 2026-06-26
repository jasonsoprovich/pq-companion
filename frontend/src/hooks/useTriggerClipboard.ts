import { useCallback, useRef } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { TriggerFired } from '../types/trigger'

/**
 * useTriggerClipboard subscribes to WebSocket trigger:fired events and writes
 * any `clipboard` actions to the system clipboard. The action text arrives
 * already capture-substituted from the backend (e.g. "/tar Soandso"), so the
 * user can paste it straight into EverQuest.
 *
 * Mount this hook once at the App level (alongside useAudioEngine) so clipboard
 * actions fire regardless of which page is open. It must NOT run in overlay
 * windows — they don't hold focus and would race the main window's write.
 */

// Kept in sync with useAudioEngine / TriggerOverlayWindowPage: a single trigger
// should never legitimately fire twice for the same matched line inside this
// window, so collapse duplicates to one clipboard write. Deliberately NOT gated
// by the per-trigger repeat-audio cooldown — that's an audio-only preference.
const DEDUP_WINDOW_MS = 750

export function useTriggerClipboard(): void {
  const lastFired = useRef<Map<string, number>>(new Map())

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== WSEvent.TriggerFired) return

    const fired = msg.data as TriggerFired
    if (!fired?.actions) return

    const clipboardActions = fired.actions.filter((a) => a.type === 'clipboard')
    if (clipboardActions.length === 0) return

    const now = Date.now()
    const key = `${fired.trigger_id}|${fired.matched_line}`
    const prev = lastFired.current.get(key)
    if (prev !== undefined && now - prev < DEDUP_WINDOW_MS) return
    lastFired.current.set(key, now)
    if (lastFired.current.size > 256) {
      for (const [k, t] of lastFired.current) {
        if (now - t > DEDUP_WINDOW_MS) lastFired.current.delete(k)
      }
    }

    // Multiple clipboard actions on one trigger overwrite each other (the
    // clipboard holds a single value) — last one wins, which matches the
    // visual order in the editor. Empty strings are skipped so a half-filled
    // action doesn't wipe the clipboard.
    for (const action of clipboardActions) {
      const text = action.text?.trim()
      if (!text) continue
      navigator.clipboard.writeText(action.text).catch(() => {
        // Some restricted/older Electron contexts reject clipboard writes;
        // there's nothing actionable to surface from a background handler.
      })
    }
  }, [])

  useWebSocket(handleMessage)
}
