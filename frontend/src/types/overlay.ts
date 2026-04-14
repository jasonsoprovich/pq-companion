import type { NPC } from './npc'

export interface SpecialAbility {
  code: number
  value: number
  name: string
}

export interface TargetState {
  has_target: boolean
  target_name?: string
  npc_data?: NPC
  special_abilities?: SpecialAbility[]
  current_zone?: string
  last_updated: string
}
