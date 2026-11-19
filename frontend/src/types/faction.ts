// One row from quarm.db's faction_list — a faction NPCs can be tied to.
export interface Faction {
  id: number
  name: string
}

// A faction search result, optionally carrying the requested character's
// persisted tally for it — so the picker can show "already have data" for a
// faction before the user pins it, the same way searching a name in the
// Player Tracker surfaces sightings whether or not that player is starred.
export interface FactionSearchResult extends Faction {
  tally?: FactionTally
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
  // inferred counts changes that matched no kill but took their size from
  // recent attributed changes to the same faction (included in
  // estimated_net, not in unresolved).
  inferred?: number
  // Standing estimate: the range the character's personal faction value
  // must lie in, once a "could not possibly get any worse/better" line
  // (range_source "cap") or a clean /con ("con") has anchored it.
  range_known?: boolean
  range_lo: number
  range_hi: number
  range_source?: 'cap' | 'con'
  range_at?: string
  info?: FactionInfo
}

// The faction's personal-value caps and this character's race/class/deity
// modifier: /con shows personal + modifier. modifier_exact is false when an
// unknown race/class/deity could change the modifier.
export interface FactionInfo {
  min_cap: number
  max_cap: number
  modifier: number
  modifier_exact: boolean
}

export interface FactionSessionState {
  tallies: FactionTally[]
}
