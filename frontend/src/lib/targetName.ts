// Mirrors the backend's normalizeNPCName (spelltimer/engine.go): lowercase,
// trim, and strip a leading English article. The spell-timer engine keys each
// timer by a raw, article-prefixed target name captured from the log ("a gnoll
// pup"), and the NPC overlay's target name is captured the same way — so
// normalizing both sides identically lets the overlay match the timers ticking
// on its current target. Player names pass through unchanged apart from casing.
export function normalizeTargetName(s: string): string {
  const t = s.toLowerCase().trim()
  if (t.startsWith('a ')) return t.slice(2).trim()
  if (t.startsWith('an ')) return t.slice(3).trim()
  if (t.startsWith('the ')) return t.slice(4).trim()
  return t
}
