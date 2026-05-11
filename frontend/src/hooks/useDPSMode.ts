import { useCallback, useEffect, useState } from 'react'
import type { EntityStats, HealerStats } from '../types/combat'

/**
 * Which DPS / HPS metric the meter shows. Mirrors the three EQLogParser
 * variants, with Personal as the default (matches EQLP's headline column).
 *
 * 'personal'  — total damage divided by THIS player's first-to-last span.
 *               Fair to the individual: a late-joiner or OOM caster isn't
 *               punished for time they weren't engaged. EQLP's "Dps".
 * 'raid'      — total damage divided by the RAID's first-to-last span
 *               (the same denominator for every combatant in the fight).
 *               EQLP's "Sdps". The right metric for ranking players within
 *               one fight.
 * 'encounter' — total damage divided by the fight's wall-clock duration.
 *               Useful for comparing whole fights to each other ("did we
 *               kill this faster than last week?"). The metric the tracker
 *               originally emitted as `dps`.
 */
export type DPSMode = 'personal' | 'raid' | 'encounter'

const STORAGE_KEY = 'pq-dps-mode'

// Cycle order shown in the UI; matches the toggle button rotation.
const CYCLE: DPSMode[] = ['personal', 'raid', 'encounter']

// readStored migrates the prior binary 'active'/'duration' preference into
// the new three-mode space. 'active' → 'personal' is a true rename (same
// field, new semantics under the EQLP-style first-to-last span).
function readStored(): DPSMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'personal' || v === 'raid' || v === 'encounter') return v
    if (v === 'active') return 'personal'
    if (v === 'duration') return 'encounter'
  } catch {
    /* noop */
  }
  return 'personal'
}

export function useDPSMode(): {
  mode: DPSMode
  toggle: () => void
  setMode: (m: DPSMode) => void
} {
  const [mode, setModeState] = useState<DPSMode>(() => readStored())

  // Pick up changes from sibling renderer processes (popout overlay vs
  // in-app panel). localStorage 'storage' events fire on other windows only.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setModeState(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setMode = useCallback((next: DPSMode) => {
    setModeState(next)
    try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
  }, [])

  const toggle = useCallback(() => {
    setModeState((prev) => {
      const idx = CYCLE.indexOf(prev)
      const next = CYCLE[(idx + 1) % CYCLE.length]
      try { localStorage.setItem(STORAGE_KEY, next) } catch { /* noop */ }
      return next
    })
  }, [])

  return { mode, toggle, setMode }
}

/** DPS value to display for a combatant under the current mode. */
export function dpsForMode(
  c: Pick<EntityStats, 'dps' | 'active_dps' | 'raid_dps'>,
  mode: DPSMode,
): number {
  switch (mode) {
    case 'personal':
      return c.active_dps
    case 'raid':
      return c.raid_dps
    case 'encounter':
      return c.dps
  }
}

/** HPS value to display for a healer under the current mode. */
export function hpsForMode(
  h: Pick<HealerStats, 'hps' | 'active_hps' | 'raid_hps'>,
  mode: DPSMode,
): number {
  switch (mode) {
    case 'personal':
      return h.active_hps
    case 'raid':
      return h.raid_hps
    case 'encounter':
      return h.hps
  }
}

// fightAggregateDPS returns the fight-level total DPS appropriate for the
// current mode. Combatants is needed because raid_seconds lives per-row
// (it's constant across the fight) and Personal at the fight level falls
// back to Raid (Personal is a per-player concept; there's no "personal"
// for a multi-player total).
//
//   personal  → total / raid_seconds (same as raid; per-player makes
//                no sense at the aggregate level)
//   raid      → total / raid_seconds
//   encounter → total / fight_duration
//
// Callers that want the active player's Personal DPS specifically should
// use playerAggregateDPS instead.
export function fightAggregateDPS(
  totalDamage: number,
  fightDuration: number,
  combatants: Pick<EntityStats, 'raid_seconds'>[],
  mode: DPSMode,
): number {
  if (mode === 'encounter') {
    return fightDuration > 0 ? totalDamage / fightDuration : 0
  }
  const raidSecs = combatants[0]?.raid_seconds ?? 0
  return raidSecs > 0 ? totalDamage / raidSecs : 0
}

// playerAggregateDPS returns the active player's fight-level DPS in the
// current mode. Personal mode reads the "You" combatant's active_dps
// (which is total / per-player-span). Raid and Encounter use the same
// fightAggregateDPS denominator since they don't differentiate per-
// player; the player-vs-raid distinction shows up at the row level.
export function playerAggregateDPS(
  youDamage: number,
  fightDuration: number,
  combatants: Pick<EntityStats, 'name' | 'active_dps' | 'raid_seconds'>[],
  playerLabel: string,
  mode: DPSMode,
): number {
  if (mode === 'personal') {
    for (const c of combatants) {
      if (c.name === playerLabel || c.name === 'You') {
        return c.active_dps
      }
    }
    // Fall through to the encounter denominator when "You" isn't in the
    // combatant list (rare — fight where the player only healed).
  }
  return fightAggregateDPS(youDamage, fightDuration, combatants, mode === 'personal' ? 'encounter' : mode)
}

/** Short label shown in the meter header / tooltips. */
export function dpsModeLabel(mode: DPSMode): string {
  switch (mode) {
    case 'personal':
      return 'Personal'
    case 'raid':
      return 'Raid'
    case 'encounter':
      return 'Encounter'
  }
}

/** Short metric abbreviation for clipboard exports / column headers. */
export function dpsModeAbbrev(mode: DPSMode): string {
  switch (mode) {
    case 'personal':
      return 'pDPS'
    case 'raid':
      return 'rDPS'
    case 'encounter':
      return 'DPS'
  }
}
