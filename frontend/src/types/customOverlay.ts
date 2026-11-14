// Mirrors backend/internal/customoverlay/models.go — user-built overlays made
// of widgets whose contents are driven by regex rules over log lines.

export type CustomWidgetType = 'fields' | 'table' | 'feed' | 'counter' | 'countdown'

export type CustomOpType =
  | 'set_field'
  | 'clear_field'
  | 'upsert_row'
  | 'remove_row'
  | 'add_line'
  | 'counter_add'
  | 'counter_set'
  | 'countdown_start'
  | 'countdown_stop'
  | 'reset_widget'
  | 'reset_all'

export type CustomMergeMode = 'replace' | 'highest' | 'add'

export interface CustomColorRule {
  contains: string
  color: string
}

export interface CustomField {
  key: string
  label: string
  colors?: CustomColorRule[]
}

export interface CustomColumn {
  key: string
  label: string
}

export interface CustomWidget {
  id: string
  type: CustomWidgetType
  title?: string
  fields?: CustomField[]
  columns?: CustomColumn[]
  sort_by?: string
  sort_desc?: boolean
  merge?: CustomMergeMode
  merge_column?: string
  max_rows?: number
  max_lines?: number
  seconds?: number
}

export interface CustomOp {
  type: CustomOpType
  widget?: string
  field?: string
  value?: string
  key?: string
  cells?: Record<string, string>
  amount?: number
}

export type CustomSimpleKind = 'starts' | 'contains' | 'exact'

// Plain-text pattern with {placeholders}; the backend builds `pattern` from
// it on save (see backend SimpleMatch).
export interface CustomSimpleMatch {
  kind: CustomSimpleKind
  text: string
}

export interface CustomRule {
  id: string
  name?: string
  enabled: boolean
  channel?: string
  direction?: '' | 'in' | 'out'
  speakers?: string[]
  simple?: CustomSimpleMatch
  pattern?: string
  ops: CustomOp[]
}

export interface CustomOverlayDef {
  id: string
  name: string
  enabled: boolean
  characters?: string[]
  widgets: CustomWidget[]
  rules: CustomRule[]
}

export interface CustomRow {
  key: string
  cells: Record<string, string>
  seq: number
  updated_at: number
}

export interface CustomFeedLine {
  text: string
  ts: number
}

export interface CustomWidgetState {
  fields?: Record<string, string>
  rows?: CustomRow[] // already sorted for display
  lines?: CustomFeedLine[] // newest first
  count?: number
  ends_at?: number // countdown, unix ms; 0/absent = stopped
}

export interface CustomOverlayState {
  widgets: Record<string, CustomWidgetState | undefined>
  next_seq: number
}

export interface CustomOverlay {
  def: CustomOverlayDef
  state: CustomOverlayState
  updated_at: number // unix ms of the last change
}

export interface CustomOverlaysState {
  overlays: CustomOverlay[]
  now: number // server unix ms
}

export interface CustomOverlayTestStep {
  line: string
  matched: string[] // rule ids
}

export interface CustomOverlayTestResult {
  steps: CustomOverlayTestStep[]
  state: CustomOverlayState
  def?: CustomOverlayDef // normalized (generated ids and patterns)
  error?: string
}

export interface CustomOverlayTemplate {
  key: string
  name: string
  description: string
  def: CustomOverlayDef
}

export type CustomOverlayAction =
  | { action: 'reset' }
  | { action: 'reset_widget'; widget: string }
  | { action: 'remove_row'; widget: string; key: string }
  | { action: 'enable'; enabled: boolean }
  | { action: 'move'; value: -1 | 1 }

// Exported share file (backend Share): a definition without its ID or
// character filter, tagged so imports can check what they're reading.
export interface CustomOverlayShare {
  format: 'pq-companion-custom-overlay'
  version: number
  overlay: CustomOverlayDef
}
