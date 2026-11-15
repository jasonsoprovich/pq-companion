/**
 * The DPS meter's HPS view: your healing in a fight, split by who you healed.
 * Only your own heals are in the log with a target, so the view is about you
 * rather than a ranking of healers.
 */
import type { FightState, HealerStats } from '../types/combat'
import { hpsForMode, type DPSMode } from '../hooks/useDPSMode'
import { EQ_CHAT_LINE_MAX } from './eqClipboard'
import { isYouRow } from './dpsRollup'

export const HPS_COLOR = '#4ade80'

export function hpsModeAbbrev(mode: DPSMode): string {
  switch (mode) {
    case 'personal':
      return 'pHPS'
    case 'raid':
      return 'rHPS'
    case 'encounter':
      return 'HPS'
  }
}

// The active character's healer row, if they healed in this fight.
export function youHealer(fight: Pick<FightState, 'healers'> | null | undefined): HealerStats | null {
  return fight?.healers?.find(isYouRow) ?? null
}

// Your HPS in the given mode. liveSecs, when set, replaces the fight's
// duration for Encounter so the live number ticks like the DPS one.
export function youHPS(fight: FightState | null | undefined, mode: DPSMode, liveSecs?: number): number {
  const you = youHealer(fight)
  if (!fight || !you) return 0
  if (mode === 'encounter' && liveSecs !== undefined) {
    return liveSecs > 0 ? you.total_heal / liveSecs : 0
  }
  return hpsForMode(you, mode)
}

export interface HealTargetRow {
  name: string
  total_heal: number
  heal_count: number
  max_heal: number
  is_you: boolean
  rate: number
}

// Your heals by target. Each row's rate uses the same time base as your
// overall HPS, so the rows add up to it.
export function healTargetRows(fight: FightState | null | undefined, mode: DPSMode, liveSecs?: number): HealTargetRow[] {
  const you = youHealer(fight)
  if (!you || you.total_heal <= 0) return []
  const perPoint = youHPS(fight, mode, liveSecs) / you.total_heal
  return (you.targets ?? []).map((t) => ({
    name: t.name,
    total_heal: t.total_heal,
    heal_count: t.heal_count,
    max_heal: t.max_heal,
    is_you: !!t.is_you,
    rate: t.total_heal * perPoint,
  }))
}

// One paste-into-EQ line, like the DPS summary:
// "Aten Ha Ra: Osui 412hps 98,765 healed | Tank 70% | Osui 20% | +2 more".
export function buildHpsFightSummary(fight: FightState | null | undefined, mode: DPSMode): string {
  const you = youHealer(fight)
  if (!fight || !you || you.total_heal <= 0) return ''
  const unit = hpsModeAbbrev(mode).toLowerCase()
  let out = `${fight.primary_target ? `${fight.primary_target}: ` : ''}${you.name} ${Math.round(youHPS(fight, mode))}${unit} ${you.total_heal.toLocaleString()} healed`
  const rows = healTargetRows(fight, mode)
  for (let i = 0; i < rows.length; i++) {
    const entry = ` | ${rows[i].name} ${Math.round((rows[i].total_heal / you.total_heal) * 100)}%`
    const left = rows.length - i - 1
    const suffix = left > 0 ? ` | +${left} more`.length : 0
    if ((out + entry).length + suffix > EQ_CHAT_LINE_MAX) {
      return `${out} | +${rows.length - i} more`
    }
    out += entry
  }
  return out
}
