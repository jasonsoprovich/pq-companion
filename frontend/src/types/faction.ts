// One row from quarm.db's faction_list — a faction NPCs can be tied to.
export interface Faction {
  id: number
  name: string
}

// One faction the active character has starred for session tracking.
export interface FactionWishlistEntry {
  id: number
  character_id: number
  faction_id: number
  faction_name: string
  sort_order: number
  created_at: number
}

// Running session tally for one tracked faction. EQ never logs a faction's
// absolute value or point amount — better/worse are raw log-line counts,
// estimated_net is a best-effort sum of quarm.db per-kill point deltas for
// changes that correlated to a resolved kill, and unresolved counts changes
// that couldn't be tied to a kill (quest turn-ins, hails, unresolvable NPCs).
export interface FactionTally {
  faction_id: number
  faction_name: string
  better: number
  worse: number
  estimated_net: number
  unresolved: number
}

export interface FactionSessionState {
  tallies: FactionTally[]
}
