// Per-combatant DPS bar palette. Each row colours by the combatant's class
// using the user's configured per-class hex palette (Settings → DPS Class
// Colors). Pets inherit their owner's class from the backend, so a magician's
// fire pet renders in the magician colour without any extra resolver here.
//
// Unresolved class (combatant not in the /who tracker, anonymous /who row,
// etc.) falls back to the palette's `unknown` colour.

import type { DPSClassColors } from '../types/config'
import { DEFAULT_DPS_CLASS_COLORS } from '../types/config'

// CLASS_TITLE_TO_KEY maps every EQ class name — base AND level-progression
// titles (51 / 55 / 60) — to its DPSClassColors key, so a /who row that reports
// e.g. "Phantasmist" or "Hierophant" still colours as Enchanter / Druid. Mirrors
// the backend's ClassTitles table (internal/players/classes.go). Keep the two in
// sync. Titles are unique across player classes, so a flat lowercase map works.
const CLASS_TITLE_TO_KEY: Record<string, keyof DPSClassColors> = {}
{
  const titles: Record<keyof DPSClassColors, string[]> = {
    warrior: ['warrior', 'champion', 'myrmidon', 'warlord', 'overlord'],
    cleric: ['cleric', 'vicar', 'templar', 'high priest', 'archon'],
    paladin: ['paladin', 'cavalier', 'knight', 'crusader', 'lord protector'],
    ranger: ['ranger', 'pathfinder', 'outrider', 'warder', 'forest stalker'],
    shadow_knight: ['shadow knight', 'shadowknight', 'reaver', 'revenant', 'grave lord', 'dread lord'],
    druid: ['druid', 'wanderer', 'preserver', 'hierophant', 'storm warden'],
    monk: ['monk', 'disciple', 'master', 'grandmaster', 'transcendant'],
    bard: ['bard', 'minstrel', 'troubador', 'virtuoso', 'maestro'],
    rogue: ['rogue', 'rake', 'blackguard', 'assassin', 'deceiver'],
    shaman: ['shaman', 'mystic', 'luminary', 'oracle', 'prophet'],
    necromancer: ['necromancer', 'heretic', 'defiler', 'warlock', 'arch lich'],
    wizard: ['wizard', 'channeler', 'evoker', 'sorcerer', 'arcanist'],
    magician: ['magician', 'mage', 'elementalist', 'conjurer', 'arch mage', 'arch convoker'],
    enchanter: ['enchanter', 'illusionist', 'beguiler', 'phantasmist', 'coercer'],
    beastlord: ['beastlord', 'primalist', 'animist', 'savage lord', 'feral lord'],
    unknown: [],
  }
  for (const key of Object.keys(titles) as Array<keyof DPSClassColors>) {
    for (const t of titles[key]) CLASS_TITLE_TO_KEY[t] = key
  }
}

// classKey maps a canonical EQ class name OR a level-progression title to the
// key on DPSClassColors. Accepts trimmed / lower-cased input so a Zeal raid
// payload that reports "shadowknight" without the space still matches.
function classKey(name: string | undefined): keyof DPSClassColors {
  if (!name) return 'unknown'
  return CLASS_TITLE_TO_KEY[name.trim().toLowerCase()] ?? 'unknown'
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

// combatantClassHex returns the solid palette hex for a class — used where a
// full-opacity swatch is wanted (e.g. the class accent line in the Player
// Tracker) rather than the translucent bar fill. Unknown class → the palette's
// unknown grey.
export function combatantClassHex(
  className: string | undefined,
  palette: DPSClassColors | null | undefined,
): string {
  const pal = palette ?? DEFAULT_DPS_CLASS_COLORS
  return pal[classKey(className)] || pal.unknown
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
