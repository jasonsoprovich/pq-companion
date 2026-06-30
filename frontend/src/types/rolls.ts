export type WinnerRule = 'highest' | 'lowest'
export type RollMode = 'manual' | 'timer'
export type ProfileMode = 'simple' | 'tiered'
export type ProfileScheme = 'suffix' | 'exact'

export interface ProfileTier {
  // suffix scheme: compared against max % divisor; exact scheme: the max.
  match: number
  label: string
}

// RollProfile mirrors the Go rolltracker.RollProfile. The zero/absent value
// (mode 'simple') means no grouping — today's flat per-range sessions.
export interface RollProfile {
  mode: ProfileMode
  scheme?: ProfileScheme
  divisor?: number
  tiers?: ProfileTier[]
}

export interface Roll {
  roller: string
  value: number
  timestamp: string
  duplicate: boolean
}

export interface RollSession {
  id: number
  min: number
  max: number
  item_name: string
  started_at: string
  last_roll_at: string
  active: boolean
  auto_stop_at?: string
  rolls: Roll[]
}

export interface RollsState {
  sessions: RollSession[]
  winner_rule: WinnerRule
  mode: RollMode
  auto_stop_seconds: number
  profile: RollProfile
}

export interface RollsSettingsPatch {
  winner_rule?: WinnerRule
  mode?: RollMode
  auto_stop_seconds?: number
  profile?: RollProfile
}
