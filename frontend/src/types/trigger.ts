export type ActionType = 'overlay_text'

export interface Action {
  type: ActionType
  text: string
  duration_secs: number
  color: string
}

export interface Trigger {
  id: string
  name: string
  enabled: boolean
  pattern: string
  actions: Action[]
  pack_name: string
  created_at: string
}

export interface TriggerFired {
  trigger_id: string
  trigger_name: string
  matched_line: string
  actions: Action[]
  fired_at: string
}

export interface TriggerPack {
  pack_name: string
  description: string
  triggers: Trigger[]
}
