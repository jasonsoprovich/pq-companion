export interface EntityStats {
  name: string
  total_damage: number
  hit_count: number
  max_hit: number
  dps: number
}

export interface FightState {
  start_time: string
  duration_seconds: number
  combatants: EntityStats[]  // outgoing damage dealers sorted by DPS desc
  total_damage: number       // all outgoing damage (all players)
  total_dps: number          // all outgoing DPS
  you_damage: number         // player personal damage
  you_dps: number            // player personal DPS
}

export interface FightSummary {
  start_time: string
  end_time: string
  duration_seconds: number
  combatants: EntityStats[]
  total_damage: number
  total_dps: number
  you_damage: number
  you_dps: number
}

export interface CombatState {
  in_combat: boolean
  current_fight?: FightState
  recent_fights: FightSummary[]
  session_damage: number  // player personal only
  session_dps: number     // player personal only
  last_updated: string
}
