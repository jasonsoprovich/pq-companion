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

// Running tally for one tracked faction, persisted per character across
// restarts and character switches. EQ never logs a faction's absolute value
// or point amount — better/worse are raw log-line counts, estimated_net is a
// best-effort sum of quarm.db per-kill point deltas for changes that
// correlated to a resolved kill, and unresolved counts changes that couldn't
// be tied to a kill (quest turn-ins, hails, unresolvable NPCs).
export interface FactionTally {
  faction_id: number
  faction_name: string
  better: number
  worse: number
  estimated_net: number
  unresolved: number
  // last_bucket is the most recent /con disposition bucket read for one of
  // this faction's primary-faction NPCs (see lib/factionBuckets.ts), or
  // absent if never considered. The one piece of ground truth EQ exposes —
  // everything else on this tally is an estimate.
  last_bucket?: string
  last_considered_at?: string
  // last_consider_suspect flags that last_bucket may be wrong because the
  // player had an illusion active at the time of the reading.
  last_consider_suspect?: boolean
}

export interface FactionSessionState {
  tallies: FactionTally[]
}
