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

// Broadcast payload for the WSEvent.SkillsUpdate event.
export interface SkillUpdate {
  character: string
  skill_id: number
  skill_name: string
  value: number
}
