// Colours for raiders on maps: by class, from the user's DPS class colours,
// or by raid group.

import type { DPSClassColors } from '../types/config'

// Zeal class ids are 1-indexed in this order.
const CLASS_KEYS: (keyof DPSClassColors)[] = [
  'warrior', 'cleric', 'paladin', 'ranger', 'shadow_knight', 'druid', 'monk',
  'bard', 'rogue', 'shaman', 'necromancer', 'wizard', 'magician', 'enchanter', 'beastlord',
]

export function classColor(palette: DPSClassColors, cls: number): string {
  const key = CLASS_KEYS[cls - 1]
  return key ? palette[key] : palette.unknown
}

// RAID_GROUP_COLORS are the twelve raid groups' colours, far enough apart
// that neighbouring groups read as different on a dark map.
const RAID_GROUP_COLORS = [
  '#f87171', '#fb923c', '#facc15', '#a3e635', '#34d399', '#22d3ee',
  '#60a5fa', '#a78bfa', '#f472b6', '#e879f9', '#fda4af', '#d6d3d1',
]
const UNGROUPED_COLOR = '#94a3b8'

// groupColor is a raid group's colour; 0 (ungrouped) is grey.
export function groupColor(group: number): string {
  return group >= 1 && group <= RAID_GROUP_COLORS.length ? RAID_GROUP_COLORS[group - 1] : UNGROUPED_COLOR
}
