// Mirrors backend/internal/lockout Entry + handler responses.

export type LockoutSection = 'loot' | 'legacy'

// One persisted lockout row for a character. expires_at is unix seconds; 0
// means the target was "Available" (no active lockout) at snapshot time. The
// UI derives a live countdown from this absolute instant, so it keeps ticking
// even while the game and app are closed.
export interface LockoutEntry {
  character: string
  section: LockoutSection
  position: number
  target_name: string
  expires_at: number
  observed_at: number
}

export interface LockoutCharactersResponse {
  characters: string[]
}

export interface LockoutCharacterResponse {
  character: string
  entries: LockoutEntry[]
}
