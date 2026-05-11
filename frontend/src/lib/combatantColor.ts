// Stable per-combatant palette for DPS meter rows. Each name hashes to one of
// a handful of saturated hues so the same combatant gets the same bar color
// across views. The local player ("You") is pinned to the project's primary
// indigo so the player's own row is always instantly identifiable.

const PLAYER_HUE = 238 // indigo, matches --color-primary

const COMBATANT_HUES = [
  10,   // red
  30,   // orange
  50,   // amber
  90,   // lime
  140,  // green
  170,  // teal
  195,  // sky
  260,  // violet
  290,  // fuchsia
  330,  // pink
]

function hash(name: string): number {
  let h = 2166136261
  for (let i = 0; i < name.length; i++) {
    h ^= name.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

export function combatantHue(name: string): number {
  if (name === 'You') return PLAYER_HUE
  return COMBATANT_HUES[hash(name.toLowerCase()) % COMBATANT_HUES.length]
}

// Translucent fill suitable for a progress-bar background behind row text.
export function combatantBarColor(name: string, alpha = 0.22): string {
  const hue = combatantHue(name)
  const sat = name === 'You' ? 80 : 65
  const lum = name === 'You' ? 62 : 55
  return `hsla(${hue}, ${sat}%, ${lum}%, ${alpha})`
}
