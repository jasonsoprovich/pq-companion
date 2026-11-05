import type { NPCOutlineRule } from '../types/config'
import type { ConsiderReading } from '../types/overlay'
import { BUCKET_COLOR, BUCKET_ORDER } from './factionBuckets'

// Id of the synthetic outline rule built from the target's /con reading.
export const CONSIDER_RULE_ID = 'consider'

// Scowls always aggros and threatening can, so those two draw a solid frame;
// dubious and apprehensive get a faint one. Everything kinder is a label only.
const FRAME_COLOR: Record<number, string> = {
  9: '#dc2626',
  8: '#f59e0b',
  7: 'rgba(245,158,11,0.45)',
  6: 'rgba(234,179,8,0.35)',
}

/**
 * The outline rule for a /con reading, or undefined for none. `first` is true
 * for the KOS standings, which go ahead of the user's own rules (a safety cue
 * beats a custom tag); the rest go after them.
 */
export function considerRule(
  c: ConsiderReading | undefined,
): { rule: NPCOutlineRule; first: boolean } | undefined {
  if (!c || c.faction < 1 || c.faction > 9) return undefined
  // BUCKET_ORDER runs worst (scowling) to best; faction runs 9 .. 1.
  const frame = FRAME_COLOR[c.faction]
  const rule: NPCOutlineRule = {
    id: CONSIDER_RULE_ID,
    name: c.while ? `${c.standing} · ${c.while}` : c.standing,
    enabled: true,
    match: 'any',
    tags: [],
    classes: [],
    color: frame ?? BUCKET_COLOR[BUCKET_ORDER[9 - c.faction]],
    flash: false,
    labelOnly: frame === undefined,
  }
  return { rule, first: c.faction >= 8 }
}
