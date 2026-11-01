/**
 * usePetState — live Pet overlay state shared by the dashboard panel and the
 * popout window. Seeds from the REST snapshot, then follows pet:state
 * broadcasts; refetches whenever the socket (re)opens so a backend restart
 * doesn't leave a stale pet on screen.
 *
 * usePetOverlayOffered gates the overlay behind the Developer-tab
 * pet_overlay_enabled flag, tracking config:updated so flipping it shows or
 * hides the panel without a reload.
 */
import { useEffect, useMemo, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { petOverlayOffered } from '../lib/overlays'
import { normalizeTargetName } from '../lib/targetName'
import { getConfig, getPetState, getTimerState } from '../services/api'
import type { Config, PetOverlaySettings } from '../types/config'
import type { PetState } from '../types/pet'
import type { ActiveTimer, TimerState } from '../types/timer'

const EMPTY: PetState = { mode: 'none', kind: '', zeal_limited: true, updated_at: '' }

export function usePetState(): PetState {
  const [state, setState] = useState<PetState>(EMPTY)
  const [loaded, setLoaded] = useState(false)

  const ready = useWebSocket((msg) => {
    if (msg.type === WSEvent.PetState) {
      setState(msg.data as PetState)
      setLoaded(true)
    }
  })

  useEffect(() => {
    if (ready !== 'open' && loaded) return
    getPetState()
      .then((s) => { setState(s); setLoaded(true) })
      .catch(() => {})
  }, [ready]) // eslint-disable-line react-hooks/exhaustive-deps

  return state
}

export function usePetOverlayOffered(): boolean {
  const [offered, setOffered] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => { if (!cancelled) setOffered(petOverlayOffered(c.preferences)) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    const cfg = msg.data as Config
    if (cfg?.preferences) setOffered(petOverlayOffered(cfg.preferences))
  })

  return offered
}

// usePetOverlaySettings returns preferences.pet_overlay, tracking
// config:updated so a Settings change (autosaved) reaches open panels live.
export function usePetOverlaySettings(): PetOverlaySettings {
  const [settings, setSettings] = useState<PetOverlaySettings>({})

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => { if (!cancelled) setSettings(c.preferences.pet_overlay ?? {}) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    const cfg = msg.data as Config
    if (cfg?.preferences) setSettings(cfg.preferences.pet_overlay ?? {})
  })

  return settings
}

/**
 * usePetTimers splits the live spell timers into the pet's charm timer and the
 * timers ticking on the pet itself (buffs you or others landed on it).
 *
 * Only one charm runs per character, and trigger-driven charm timers may carry
 * no target name, so the charm bar is simply the is_charm timer with the most time left. Like
 * useTargetTimers this ignores the display threshold: the pet card shows every
 * timer on the pet, including ones still hidden from the buff overlay.
 */
export function usePetTimers(petName: string | undefined): {
  charm: ActiveTimer | null
  timers: ActiveTimer[]
} {
  const [state, setState] = useState<TimerState | null>(null)

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  useWebSocket((msg) => {
    if (msg.type === WSEvent.OverlayTimers) setState(msg.data as TimerState)
  })

  return useMemo(() => {
    const all = state?.timers ?? []
    let charm: ActiveTimer | null = null
    for (const t of all) {
      if (t.is_charm && (!charm || t.remaining_seconds > charm.remaining_seconds)) charm = t
    }
    if (!petName) return { charm, timers: [] }
    const want = normalizeTargetName(petName)
    const timers = all
      .filter((t) => !t.is_charm && t.target_name && normalizeTargetName(t.target_name) === want)
      .sort((a, b) => a.remaining_seconds - b.remaining_seconds)
    return { charm, timers }
  }, [state, petName])
}
