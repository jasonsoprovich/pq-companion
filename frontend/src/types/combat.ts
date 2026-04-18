export interface EntityStats {
  name: string
  total_damage: number
  hit_count: number
  max_hit: number
  dps: number
}

export interface HealerStats {
  name: string
  total_heal: number
  heal_count: number
  max_heal: number
  hps: number
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
