// Types mirror the Go popflag package (backend/internal/popflag). The dataset
// itself is served from the API (GET /api/popflags/dataset) — there is no TS
// copy of the data, only these shapes.

export interface QualifyCond {
  qglobal: string
  value: string
}

export interface PoPFlag {
  id: string
  tier: number
  zone: string
  zone_short: string
  label: string
  detail: string
  prereqs: string[]
  level?: number
  // Action bucket for icon/colour coding (see backend popflag.PoPFlag.StepKind):
  // 'kill' | 'timed_hail' | 'hail' | 'loot'. Optional for forward-compat.
  step_kind?: string
  // Phase-2 completion-detection fields (present in the dataset, unused by the
  // Phase 1 checklist UI).
  qglobal?: string
  qglobal_value?: string
  counter?: boolean
  bit_position?: number
  satisfied_by?: QualifyCond[]
  seer_phrases?: string[]
}

export interface PoPFlagStatus extends PoPFlag {
  done: boolean
  source?: string // 'manual' | 'seer' | 'auto'
  locked: boolean
  missing?: string[] // prereq IDs not yet done
}

export interface PoPProgress {
  tier?: number
  key: string
  label: string
  done: number
  total: number
}

export interface PoPResolved {
  flags: PoPFlagStatus[]
  tiers: PoPProgress[]
  zones: PoPProgress[]
  done: number
  total: number
}

export interface PoPFlagDatasetResponse {
  flags: PoPFlag[]
}

export interface SeerDetected {
  id: string
  label: string
  zone: string
  tier: number
  already_done: boolean
  manual_blocked: boolean
}

export interface SeerPreviewResponse {
  qglobals: Record<string, string>
  detected: SeerDetected[]
  new_count: number
}
