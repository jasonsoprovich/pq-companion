/**
 * useRespawnAlerts — watches overlay:respawns WebSocket events and fires an
 * audio cue as each NPC respawn timer crosses the user-configured remaining
 * threshold (Preferences.respawn_alert). The respawn engine keeps a timer
 * visible at 0:00 for a grace window, so a threshold of 0 ("at POP") fires
 * reliably.
 *
 * Mount once at the App level (alongside useTimerAlerts) so alerts fire
 * regardless of which page the user is on. The respawn timers don't carry
 * their own alert config — unlike spell timers — so this hook reads the single
 * global preference and applies it to every respawn.
 *
 * Algorithm mirrors useTimerAlerts: track previous remaining_seconds per timer
 * ID and fire on the downward crossing. The {npc} token in the TTS template is
 * substituted with the mob's name.
 *
 * The Respawn overlay window has a bell mute toggle in its header (see
 * OverlayMuteButton) that layers on top of this preference without changing
 * it. Read from localStorage since this hook is mounted at the App level,
 * not inside that window.
 */
import { useCallback, useEffect, useRef } from 'react'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getConfig } from '../services/api'
import { playSound, speakText } from '../services/audio'
import { RESPAWN_ALERTS_KEY, loadAlertsEnabled } from '../lib/overlayAlertMute'
import type { RespawnState } from '../types/respawn'
import type { TimerAlertPref } from '../types/config'

export function useRespawnAlerts(): void {
  const prevRemaining = useRef<Map<string, number>>(new Map())
  // Latest respawn alert preference, read live inside the WS handler so a
  // Settings change takes effect without remounting the hook.
  const prefRef = useRef<TimerAlertPref | undefined>(undefined)
  const muteRef = useRef(loadAlertsEnabled(RESPAWN_ALERTS_KEY))

  useEffect(() => {
    getConfig()
      .then((c) => { prefRef.current = c.preferences?.respawn_alert })
      .catch(() => {})
  }, [])

  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === RESPAWN_ALERTS_KEY) muteRef.current = loadAlertsEnabled(RESPAWN_ALERTS_KEY)
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const handleMessage = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ConfigUpdated) {
      getConfig()
        .then((c) => { prefRef.current = c.preferences?.respawn_alert })
        .catch(() => {})
      return
    }
    if (msg.type !== WSEvent.OverlayRespawns) return

    const state = msg.data as RespawnState
    if (!state?.timers) return

    const pref = prefRef.current
    const enabled = Boolean(pref?.enabled)
    const threshold = Math.max(0, pref?.seconds ?? 0)
    const activeIds = new Set(state.timers.map((t) => t.id))

    for (const timer of state.timers) {
      const prev = prevRemaining.current.get(timer.id) ?? timer.remaining_seconds + 1

      // Only announce respawns in the player's current zone — a pop in a zone
      // they've left is noise. When the zone is unknown, don't suppress.
      const inCurrentZone = !state.current_zone || timer.zone === state.current_zone

      if (enabled && pref && muteRef.current && inCurrentZone && prev > threshold && timer.remaining_seconds <= threshold) {
        if (pref.type === 'play_sound' && pref.sound_path) {
          playSound(pref.sound_path, pref.volume / 100)
        } else if (pref.type === 'text_to_speech' && pref.tts_template) {
          const text = pref.tts_template.replace('{npc}', timer.npc_name)
          speakText(text, pref.voice, pref.tts_volume / 100)
        }
      }

      prevRemaining.current.set(timer.id, timer.remaining_seconds)
    }

    for (const id of prevRemaining.current.keys()) {
      if (!activeIds.has(id)) {
        prevRemaining.current.delete(id)
      }
    }
  }, [])

  useWebSocket(handleMessage)
}
