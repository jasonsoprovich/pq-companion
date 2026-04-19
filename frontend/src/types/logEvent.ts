export type LogEventType =
  | 'log:zone'
  | 'log:combat_hit'
  | 'log:combat_miss'
  | 'log:spell_cast'
  | 'log:spell_interrupt'
  | 'log:spell_resist'
  | 'log:spell_fade'
  | 'log:death'
  | 'log:heal'

export interface LogEvent {
  type: LogEventType
  timestamp: string
  message: string
  data?: unknown
}

export interface ZoneData {
  zone_name: string
}

export interface CombatHitData {
  actor: string
  skill: string
  target: string
  damage: number
}

export interface CombatMissData {
  actor: string
  target: string
  miss_type: string
}

export interface SpellCastData {
  spell_name: string
}

export interface SpellInterruptData {
  spell_name: string
}

export interface SpellResistData {
  spell_name: string
}

export interface SpellFadeData {
  spell_name: string
}

export interface DeathData {
  slain_by: string
}

export interface LogTailerStatus {
  enabled: boolean
  file_path: string
  file_exists: boolean
  tailing: boolean
  offset: number
  size_bytes: number
  large_file: boolean
}

export interface LogFileInfo {
  size_bytes: number
  oldest_entry: string
  newest_entry: string
  large_file: boolean
}
