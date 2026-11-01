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
import { getCombatState, getConfig, getPetState, getTimerState } from '../services/api'
import { dpsForMode, useDPSMode, type DPSMode } from './useDPSMode'
import type { Config, PetOverlaySettings } from '../types/config'
import type { PetState } from '../types/pet'
import type { ActiveTimer, TimerState } from '../types/timer'
import type { CombatState, EntityStats } from '../types/combat'

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

export interface PetDPS {
  // DPS under the DPS meter's current mode (so the two agree).
  dps: number
  mode: DPSMode
  // Pet damage as a share of you + pet, 0–1.
  share: number
  // False when the numbers come from the last finished fight.
  live: boolean
}

/**
 * usePetDPS finds the pet's own row in the combat tracker's current fight
 * (or, between fights, the most recent one). Pets are separate combatants
 * whose owner_name is the player's name — or "You" before the name is
 * known — so the row is matched on owner plus pet name, falling back to the
 * player's only pet row when the name differs (article casing, a renamed
 * pet). Null when the pet dealt no damage in that fight.
 */
export function usePetDPS(petName: string | undefined): PetDPS | null {
  const [combat, setCombat] = useState<CombatState | null>(null)
  const { mode } = useDPSMode()

  useEffect(() => {
    getCombatState().then(setCombat).catch(() => {})
  }, [])

  useWebSocket((msg) => {
    if (msg.type === WSEvent.OverlayCombat) setCombat(msg.data as CombatState)
  })

  return useMemo(() => {
    if (!combat || !petName) return null
    const live = Boolean(combat.current_fight)
    const fight = combat.current_fight ?? combat.recent_fights?.[0]
    const rows: EntityStats[] = fight?.combatants ?? []
    const you = rows.find((c) => c.is_you)
    const owners = new Set(['You', you?.name].filter(Boolean) as string[])
    const mine = rows.filter((c) => c.owner_name && owners.has(c.owner_name))
    const want = normalizeTargetName(petName)
    const pet = mine.find((c) => normalizeTargetName(c.name) === want) ?? (mine.length === 1 ? mine[0] : undefined)
    if (!pet || pet.total_damage <= 0) return null
    const total = pet.total_damage + (you?.total_damage ?? 0)
    return {
      dps: dpsForMode(pet, mode),
      mode,
      share: total > 0 ? pet.total_damage / total : 0,
      live,
    }
  }, [combat, petName, mode])
}
