/**
 * useMetronomeAlerts — fires an optional audio/TTS cue when the CH Metronome's
 * personal countdown starts and when it reaches "cast now". Independently
 * recomputes the same anchor math as the two visual metronomes
 * (CHMetronomePanel / CHMetronomeOverlayWindowPage) from the shared
 * lib/chMetronome helpers and the same localStorage config.
 *
 * Alerts only fire while the popped-out CH Metronome overlay window is open
 * (the countdown only means something to a healer actively watching it) and
 * while its header bell toggle isn't muted — both gates are read from
 * localStorage/IPC state the window itself owns, since this hook is mounted
 * at the App level rather than inside that window.
 *
 * Mount once at the App level (alongside useTimerAlerts/useRespawnAlerts) —
 * mounting equivalent logic inside the view components too would double-fire
 * whenever both the dashboard panel and the popped-out overlay are open at
 * once.
 */
import { useCallback, useEffect, useRef } from 'react'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getConfig, getTimerState } from '../services/api'
import { playSound, speakText } from '../services/audio'
import {
  ALERTS_ENABLED_KEY,
  CH_CAST,
  type AnchorResult,
  type ChainView,
  computeAnchorMs,
  loadAlertsEnabled,
  loadSeen,
  mergeSeen,
  saveSeen,
  seenStorageKey,
} from '../lib/chMetronome'
import type { ActiveTimer, TimerState } from '../types/timer'
import type { TimerAlertPref } from '../types/config'

// Mirrors CHMetronomePanel/CHMetronomeOverlayWindowPage's constants exactly
// so "countdown started" / "cast now" fire on the same edges the UI renders.
const ANCHOR_GRACE_SECS = 3
const PULSE_SECS = 1.5
const CHAIN_STORAGE_KEY = 'chMetronome:chain'

type Cfg = { position: number; chainSize: number; delay: number }
const DEFAULT_CFG: Cfg = { position: 2, chainSize: 3, delay: 4 }

function loadCfg(): Cfg {
  const read = (k: string, d: number): number => {
    const raw = localStorage.getItem(`chMetronome:${k}`)
    const n = raw == null ? NaN : parseInt(raw, 10)
    return Number.isFinite(n) ? n : d
  }
  return {
    position: read('position', DEFAULT_CFG.position),
    chainSize: read('chainSize', DEFAULT_CFG.chainSize),
    delay: read('delay', DEFAULT_CFG.delay),
  }
}

function loadChainSelection(): ChainView {
  return localStorage.getItem(CHAIN_STORAGE_KEY) === 'ramp' ? 'ramp' : 'main'
}

function fire(pref: TimerAlertPref | undefined): void {
  if (!pref?.enabled) return
  if (pref.type === 'play_sound' && pref.sound_path) {
    playSound(pref.sound_path, pref.volume / 100)
  } else if (pref.type === 'text_to_speech' && pref.tts_template) {
    speakText(pref.tts_template, pref.voice, pref.tts_volume / 100)
  }
}

export function useMetronomeAlerts(): void {
  const startPrefRef = useRef<TimerAlertPref | undefined>(undefined)
  const castPrefRef = useRef<TimerAlertPref | undefined>(undefined)
  const secondaryEnabledRef = useRef(false)
  const chainRef = useRef<ChainView>(loadChainSelection())
  const seenRef = useRef<Map<number, number>>(loadSeen(chainRef.current))
  const anchorRef = useRef<AnchorResult | null>(null)
  const prevActiveRef = useRef(false)
  const prevFlashingRef = useRef(false)
  // Mute toggle from the popped-out overlay's bell button (localStorage-backed,
  // synced via 'storage' events since this hook runs at the App level, not
  // inside that window).
  const alertsEnabledRef = useRef(loadAlertsEnabled())
  // Alerts are scoped to the popped-out CH Metronome overlay: the countdown
  // math only means something to a healer actively watching it, so muting
  // isn't enough on its own — closing the window should stop the alerts too.
  // Electron doesn't push window open/close events to the renderer, so this
  // is polled like OverlaysDashboard's own popoutStates check.
  const overlayOpenRef = useRef(false)

  const loadPrefs = useCallback(() => {
    getConfig()
      .then((c) => {
        startPrefRef.current = c.preferences?.metronome_start_alert
        castPrefRef.current = c.preferences?.metronome_cast_alert
        secondaryEnabledRef.current = c.ch_chain?.secondary_enabled ?? false
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    loadPrefs()
  }, [loadPrefs])

  // Follow the same active-chain selection (main vs secondary) the visual
  // metronomes use, shared via the same localStorage key, and reload the
  // learned chain-number map on a real switch.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === CHAIN_STORAGE_KEY) {
        const next = loadChainSelection()
        if (next !== chainRef.current) {
          chainRef.current = next
          seenRef.current = loadSeen(next)
          anchorRef.current = null
        }
      } else if (e.key === seenStorageKey(chainRef.current)) {
        mergeSeen(seenRef.current, loadSeen(chainRef.current))
      } else if (e.key === ALERTS_ENABLED_KEY) {
        alertsEnabledRef.current = loadAlertsEnabled()
      }
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  // Electron doesn't push window open/close state, so poll the same way
  // OverlaysDashboard tracks popoutStates. 1.5s is plenty responsive for a
  // mute gate and matches the existing poll cadence elsewhere in the app.
  useEffect(() => {
    if (!window.electron?.overlay?.popoutStates) return
    let cancelled = false
    const check = (): void => {
      window.electron.overlay
        .popoutStates()
        .then((s) => {
          if (!cancelled) overlayOpenRef.current = !!s.chMetronome
        })
        .catch(() => {})
    }
    check()
    const id = setInterval(check, 1500)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])

  // With the secondary chain off in settings, always follow the main chain —
  // mirrors CHMetronomePanel's activeChain fallback.
  const activeChain = useCallback((): ChainView => (secondaryEnabledRef.current ? chainRef.current : 'main'), [])

  const recomputeAnchor = useCallback(
    (timers: ActiveTimer[]) => {
      const chain = activeChain()
      const anchor = computeAnchorMs(timers, loadCfg(), chain, seenRef.current, Date.now())
      saveSeen(chain, seenRef.current)
      if (anchor != null) anchorRef.current = anchor
    },
    [activeChain],
  )

  useEffect(() => {
    getTimerState()
      .then((s) => recomputeAnchor(s.timers))
      .catch(() => {})
  }, [recomputeAnchor])

  const handleMessage = useCallback(
    (msg: WsMessage) => {
      if (msg.type === WSEvent.ConfigUpdated) {
        loadPrefs()
        return
      }
      if (msg.type !== WSEvent.OverlayTimers) return
      const state = msg.data as TimerState
      if (!state?.timers) return
      recomputeAnchor(state.timers)
    },
    [loadPrefs, recomputeAnchor],
  )
  useWebSocket(handleMessage)

  // Poll at the same 100ms cadence the visual metronomes render at, so the
  // "cast now" edge (a ~1.65s flash window) and the countdown-start edge are
  // caught reliably rather than only at the ~1s WebSocket pulse rate.
  useEffect(() => {
    const id = setInterval(() => {
      if (!startPrefRef.current?.enabled && !castPrefRef.current?.enabled) return

      const a = anchorRef.current
      const now = Date.now()
      const active = a != null && now - a.anchorMs <= (CH_CAST + ANCHOR_GRACE_SECS) * 1000
      const cfg = loadCfg()
      const elapsed = active ? (now - (a as AnchorResult).anchorMs) / 1000 : 0
      const castIn = active ? cfg.delay - elapsed : 0
      const flashing = active && castIn <= 0.15 && elapsed <= cfg.delay + PULSE_SECS

      // Gated (muted, or the overlay isn't popped out/open): keep the edge
      // trackers in sync with reality but don't fire, so that when the gate
      // reopens only a genuinely NEW active/flashing edge fires — not a stale
      // transition that happened while gated.
      if (!alertsEnabledRef.current || !overlayOpenRef.current) {
        prevActiveRef.current = active
        prevFlashingRef.current = flashing
        return
      }

      if (active && !prevActiveRef.current) fire(startPrefRef.current)
      if (flashing && !prevFlashingRef.current) fire(castPrefRef.current)

      prevActiveRef.current = active
      prevFlashingRef.current = flashing
    }, 100)
    return () => clearInterval(id)
  }, [])
}
