// Expansion-era constants, mirroring backend internal/era. Project Quarm is
// pre-Planes-of-Power until the expansion launches (October 2026); the
// pop_enabled preference (Developer tab preview toggle, default off) swaps
// the whole app over. Read the flag via hooks/usePoPEnabled.

export const PRE_POP_MAX_LEVEL = 60
export const POP_MAX_LEVEL = 65

// maxLevel returns the server level cap for the given era state.
export function maxLevel(popEnabled: boolean): number {
  return popEnabled ? POP_MAX_LEVEL : PRE_POP_MAX_LEVEL
}
