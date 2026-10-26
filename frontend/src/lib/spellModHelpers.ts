// Shared display helpers for the Spell Modifiers panel and its what-if
// loadout editor.

export function spaLabel(spa: number): string {
  switch (spa) {
    case 124: return 'Spell Damage'
    case 125: return 'Healing'
    case 126: return 'Resist Reduction'
    case 127: return 'Cast Time'
    case 128: return 'Duration'
    case 129: return 'Range'
    case 130: return 'Spell Hate'
    case 131: return 'Reagent Conserve'
    case 132: return 'Mana Cost'
    default: return `SPA ${spa}`
  }
}

// Focus SPAs whose magnitude is a reduction (shown with a minus sign): cast
// time (127) and mana cost (132). Everything else is a bonus shown with a plus.
export function spaSign(spa: number): string {
  return spa === 127 || spa === 132 ? '−' : '+'
}

export function formatDuration(sec: number): string {
  if (sec <= 0) return '0s'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  const parts: string[] = []
  if (h) parts.push(`${h}h`)
  if (m) parts.push(`${m}m`)
  if (s || parts.length === 0) parts.push(`${s}s`)
  return parts.join(' ')
}
