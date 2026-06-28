// Visual metadata for a PoP flag's step_kind (mirrors the backend
// popflag.PoPFlag.StepKind buckets). Drives the icon + accent-colour coding
// shared by the checklist rows and the dependency-graph nodes, so a player can
// tell at a glance whether a step is a raid kill, a must-act-now post-kill
// hail, or solo "homework" (an always-up hail / a loot-and-turn-in).
import { Swords, AlarmClock, Megaphone, Package, type LucideIcon } from 'lucide-react'

export type StepKind = 'kill' | 'timed_hail' | 'hail' | 'loot'

export interface StepKindMeta {
  kind: StepKind
  label: string
  icon: LucideIcon
  color: string // accent / icon colour (kept clear of the status palette)
  bg: string // faint fill for chips and the timed-hail row tint
  border: string // chip border
  tip: string // tooltip / legend explanation
}

export const STEP_KIND_META: Record<StepKind, StepKindMeta> = {
  kill: {
    kind: 'kill',
    label: 'Kill',
    icon: Swords,
    color: '#fb923c',
    bg: 'rgba(251,146,60,0.14)',
    border: 'rgba(251,146,60,0.5)',
    tip: 'Boss fight — be present at the raid.',
  },
  timed_hail: {
    kind: 'timed_hail',
    label: 'Timed hail',
    icon: AlarmClock,
    color: '#f472b6',
    bg: 'rgba(244,114,182,0.16)',
    border: 'rgba(244,114,182,0.6)',
    tip: 'Limited-window hail right after a boss — act fast, easy to miss.',
  },
  hail: {
    kind: 'hail',
    label: 'Hail',
    icon: Megaphone,
    color: '#a78bfa',
    bg: 'rgba(167,139,250,0.14)',
    border: 'rgba(167,139,250,0.5)',
    tip: 'Always-up NPC — homework you can do anytime.',
  },
  loot: {
    kind: 'loot',
    label: 'Loot',
    icon: Package,
    color: '#facc15',
    bg: 'rgba(250,204,21,0.14)',
    border: 'rgba(250,204,21,0.5)',
    tip: 'Loot or turn-in items — homework you can do anytime.',
  },
}

// Legend / display order: raid work first, then the solo homework kinds.
export const STEP_KIND_ORDER: StepKind[] = ['kill', 'timed_hail', 'hail', 'loot']

// stepKindMeta resolves a raw step_kind string to its metadata, or null when
// absent/unknown (so callers can simply skip the badge).
export function stepKindMeta(kind?: string): StepKindMeta | null {
  if (kind && kind in STEP_KIND_META) return STEP_KIND_META[kind as StepKind]
  return null
}
