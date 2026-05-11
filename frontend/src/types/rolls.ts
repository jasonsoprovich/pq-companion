export type WinnerRule = 'highest' | 'lowest'
export type RollMode = 'manual' | 'timer'

export interface Roll {
  roller: string
  value: number
  timestamp: string
  duplicate: boolean
}

export interface RollSession {
  id: number
  max: number
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
}

export interface RollsSettingsPatch {
  winner_rule?: WinnerRule
  mode?: RollMode
  auto_stop_seconds?: number
}
