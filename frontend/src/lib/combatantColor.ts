// Per-combatant DPS bar palette. Each row colours by the combatant's class
// using the user's configured per-class hex palette (Settings → DPS Class
// Colors). Pets inherit their owner's class from the backend, so a magician's
// fire pet renders in the magician colour without any extra resolver here.
//
// Unresolved class (combatant not in the /who tracker, anonymous /who row,
// etc.) falls back to the palette's `unknown` colour.

import type { DPSClassColors } from '../types/config'
import { DEFAULT_DPS_CLASS_COLORS } from '../types/config'

// classKey maps a canonical EQ class name to the key on DPSClassColors.
// Accepts trimmed / lower-cased input so a future Zeal raid payload that
// reports "shadowknight" without the space still matches.
function classKey(name: string | undefined): keyof DPSClassColors {
  if (!name) return 'unknown'
  const norm = name.trim().toLowerCase()
  switch (norm) {
    case 'warrior': return 'warrior'
    case 'cleric': return 'cleric'
    case 'paladin': return 'paladin'
    case 'ranger': return 'ranger'
    case 'shadow knight':
    case 'shadowknight':
      return 'shadow_knight'
    case 'druid': return 'druid'
    case 'monk': return 'monk'
    case 'bard': return 'bard'
    case 'rogue': return 'rogue'
    case 'shaman': return 'shaman'
    case 'necromancer': return 'necromancer'
    case 'wizard': return 'wizard'
    case 'magician':
    case 'mage':
      return 'magician'
    case 'enchanter': return 'enchanter'
    case 'beastlord': return 'beastlord'
    default: return 'unknown'
  }
}

// hexToRgba converts "#RRGGBB" (or "#RGB") to "rgba(r, g, b, alpha)". Falls
// back to the unknown default when the input doesn't parse — keeps the bar
// visible even if the user types something invalid in Settings.
export function hexToRgba(hex: string, alpha: number): string {
  let h = (hex || '').trim()
  if (h.startsWith('#')) h = h.slice(1)
  if (h.length === 3) {
    h = h.split('').map((c) => c + c).join('')
  }
  if (h.length !== 6 || !/^[0-9a-fA-F]{6}$/.test(h)) {
    h = DEFAULT_DPS_CLASS_COLORS.unknown.slice(1)
  }
  const r = parseInt(h.slice(0, 2), 16)
  const g = parseInt(h.slice(2, 4), 16)
  const b = parseInt(h.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

// combatantBarColor returns the bar background colour for one combatant
// row. Pass the canonical class string from EntityStats.class. When the
// palette is not yet loaded the function uses the seeded defaults so first
// paint still shows class colour.
export function combatantBarColor(
  className: string | undefined,
  palette: DPSClassColors | null | undefined,
  alpha = 0.22,
): string {
  const pal = palette ?? DEFAULT_DPS_CLASS_COLORS
  const hex = pal[classKey(className)] || pal.unknown
  return hexToRgba(hex, alpha)
}
