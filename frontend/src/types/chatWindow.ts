// Mirrors backend/internal/chatwindow/models.go.

export type ChatMatchMode = 'simple' | 'regex'
export type ChatSimpleKind = 'starts' | 'contains' | 'exact'

export interface ChatSources {
  channels: string[]
  all_named?: boolean
  other?: boolean
}

export interface ChatRule {
  id: string
  name?: string
  enabled: boolean
  channel?: string
  speakers?: string[]
  mode: ChatMatchMode
  kind?: ChatSimpleKind
  text?: string
  color?: string
  pin_minutes?: number
}

export interface ChatAppearance {
  font_scale: number
  opacity: number
  show_timestamps: boolean
  show_channel: boolean
  channel_colors?: Record<string, string>
  // Your own lines: omitted = white, 'none' = the channel color, or a hex.
  self_color?: string
  // Omitted = 'combined'.
  layout?: ChatLayout
  tell_tabs?: boolean
  max_tell_tabs?: number
}

export type ChatLayout = 'combined' | 'tabs'


export interface ChatWindowDef {
  id?: string
  name: string
  enabled: boolean
  characters?: string[]
  sources: ChatSources
  keep: ChatRule[]
  hide: ChatRule[]
  appearance: ChatAppearance
  max_lines: number
}

export interface ChatLine {
  seq: number
  ts: number
  character?: string
  channel: string
  direction?: string
  speaker?: string
  peer?: string
  message?: string
  text: string
  color?: string
}

export interface ChatPin {
  seq: number
  until: number
  line: ChatLine
}

export interface ChatWindowState {
  epoch: number
  seq: number
  lines: ChatLine[]
  pins: ChatPin[]
  // True while a Log Feed replay fills the window.
  replay?: boolean
}

export interface ChatWindow {
  def: ChatWindowDef & { id: string }
  state: ChatWindowState
}

export interface ChatWindowsState {
  windows: ChatWindow[]
  now: number
}

export interface ChatLinesBatch {
  id: string
  epoch: number
  lines: ChatLine[]
  pins: ChatPin[]
  replay?: boolean
}

export interface ChatLinesEvent {
  windows: ChatLinesBatch[]
  now: number
}

export type ChatVerdict = 'kept' | 'shown' | 'hidden' | 'filtered'

export interface ChatTestStep {
  line: string
  text: string
  verdict: ChatVerdict
  rule?: string
  color?: string
  pin?: boolean
}

export interface ChatTestResult {
  steps: ChatTestStep[]
  def?: ChatWindowDef
  error?: string
}

export interface ChatSuggestion {
  rule: ChatRule
  matches: number
}

export interface ChatWindowShare {
  format: string
  version: number
  windows: ChatWindowDef[]
}

export type ChatWindowAction =
  | { action: 'enable'; enabled: boolean }
  | { action: 'move'; value: number }
  | { action: 'clear' }
  | { action: 'appearance'; appearance: ChatAppearance }
