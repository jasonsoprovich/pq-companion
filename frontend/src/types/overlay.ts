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
  // hp_percent is 0-100 when fed by the Zeal pipe, or -1 when unknown
  // (Zeal not running or no value yet for the current target).
  hp_percent: number
  pet_owner?: string
  last_updated: string
}
