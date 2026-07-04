// Mirrors backend internal/api/skills.go skillView / skillsResponse and
// internal/skills.Update.

export interface SkillView {
  skill_id: number   // EQMac skill_id, or -1 when the name didn't map
  skill_name: string // in-game display name from the log
  value: number      // most recent observed rank
  cap: number        // class/level cap from skill_caps; 0 = unknown / none
  updated_at: number // unix seconds of the last improvement
}

export interface SkillsResponse {
  character: string
  class: number // 0-indexed EQ class, -1 if unknown
  level: number
  skills: SkillView[]
}

// Mirrors backend character.TradeskillEntry (enriched by the API layer). From
// the quarmy "SkillID\tValue" section added in Zeal 1.4.3.
export interface TradeskillView {
  skill_id: number
  value: number       // raw server value; 254/255 = untrained sentinels
  name?: string       // resolved tradeskill name, e.g. "Research"
  cap?: number        // class/level cap from skill_caps; 0/absent = unknown
  untrained: boolean  // true for the 254/255 sentinels (can never train)
}

export interface TradeskillsResponse {
  tradeskills: TradeskillView[]
}

// Broadcast payload for the WSEvent.SkillsUpdate event.
export interface SkillUpdate {
  character: string
  skill_id: number
  skill_name: string
  value: number
}
