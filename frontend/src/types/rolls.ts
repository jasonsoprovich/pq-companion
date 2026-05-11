export type WinnerRule = 'highest' | 'lowest'

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
  rolls: Roll[]
}

export interface RollsState {
  sessions: RollSession[]
  winner_rule: WinnerRule
}
