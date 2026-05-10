export interface EntityStats {
  name: string
  total_damage: number
  hit_count: number
  max_hit: number
  // Three DPS variants — see backend combat.EntityStats for the full
  // explanation. Short form:
  //   dps          — Encounter:  total / fight wall-clock duration
  //   active_dps   — Personal:   total / per-player first-to-last span
  //   raid_dps     — Raid-wide:  total / raid first-to-last span
  // active_seconds and raid_seconds expose the latter two denominators
  // for tooltips ("engaged 30s of 60s").
  dps: number
  active_dps: number
  active_seconds: number
  raid_dps: number
  raid_seconds: number
  // crit_count   — number of "Scores a critical hit!" lines matched to a
  //                damage event from this actor in the fight.
  // crit_damage  — sum of damage from those matched crit hits.
  crit_count: number
  crit_damage: number
  // Controlling player's name when this entity is a pet (charmed NPC or
  // summoned pet). Empty/undefined for player damage dealers and for pets
  // whose owner could not be identified.
  owner_name?: string
}

export interface HealerStats {
  name: string
  total_heal: number
  heal_count: number
  max_heal: number
  hps: number
  active_hps: number
  active_seconds: number
  raid_hps: number
  raid_seconds: number
}

export interface FightState {
  start_time: string
  duration_seconds: number
  primary_target?: string    // most-hit NPC target
  combatants: EntityStats[]  // outgoing damage dealers sorted by DPS desc (NPCs excluded)
  total_damage: number       // all outgoing damage (all players)
  total_dps: number          // all outgoing DPS
  you_damage: number         // player personal damage
  you_dps: number            // player personal DPS
  healers: HealerStats[]     // healers sorted by total heal desc
  total_heal: number         // all healing done (all healers)
  total_hps: number          // all HPS
  you_heal: number           // player personal healing done
  you_hps: number            // player personal HPS
}

export interface FightSummary {
  start_time: string
  end_time: string
  duration_seconds: number
  primary_target?: string    // most-hit NPC target
  combatants: EntityStats[]
  total_damage: number
  total_dps: number
  you_damage: number
  you_dps: number
  healers: HealerStats[]
  total_heal: number
  total_hps: number
  you_heal: number
  you_hps: number
}

export interface DeathRecord {
  timestamp: string
  zone: string
  slain_by: string
}

export interface CombatState {
  in_combat: boolean
  current_fight?: FightState
  recent_fights: FightSummary[]
  session_damage: number  // player personal only
  session_dps: number     // player personal only
  session_heal: number    // player personal healing only
  session_hps: number     // player personal HPS only
  deaths: DeathRecord[]
  death_count: number
  last_updated: string
}

// StoredFight is one fight as it lives in user.db. Mirrors the backend's
// combat.StoredFight — a FightSummary plus identity / context fields the
// history page needs for filtering and detail rendering.
export interface StoredFight {
  id: number
  npc_name: string
  zone: string
  character_name: string
  start_time: string
  end_time: string
  duration_seconds: number
  total_damage: number
  you_damage: number
  total_heal: number
  you_heal: number
  combatants: EntityStats[]
  healers: HealerStats[]
}

// HistoryListResponse is the shape returned by GET /api/combat/history.
export interface HistoryListResponse {
  fights: StoredFight[]
  total: number
  limit: number
  offset: number
}

// HistoryFacets is the shape returned by GET /api/combat/history/facets —
// distinct character names and zones present in the saved fights, used to
// populate filter dropdowns instead of free-text inputs.
export interface HistoryFacets {
  characters: string[]
  zones: string[]
}

// HistoryFilter mirrors the backend's FightFilter; every field is optional
// and zero-values mean "no filter" for that field.
export interface HistoryFilter {
  start?: string  // RFC3339 lower bound on fight start_time
  end?: string    // RFC3339 upper bound on fight start_time
  npc?: string    // case-insensitive substring on NPC name
  character?: string
  zone?: string
  limit?: number
  offset?: number
}
