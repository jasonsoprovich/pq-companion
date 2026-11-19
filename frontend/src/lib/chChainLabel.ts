// Shared label helpers for the CH Chain overlay renderers (the dashboard
// card, the popped-out window, and the lanes view used by both).

// parseLabel splits the matcher's "#N  Target  ← Caster" label into a numeric
// chain position (for sorting + a badge) and the remaining display text.
export function parseLabel(label: string): { position: number; text: string } {
  const m = /^#(\d+)\s+(.*)$/.exec(label)
  if (!m) return { position: Number.MAX_SAFE_INTEGER, text: label }
  return { position: parseInt(m[1], 10), text: m[2] }
}

// positionLetter maps a ramp-chain position back to its letter marker for
// the badge (1 → A, 2 → B). Falls back to the number outside A–Z range.
export function positionLetter(position: number): string {
  if (position >= 1 && position <= 26) {
    return String.fromCharCode(64 + position)
  }
  return String(position)
}
