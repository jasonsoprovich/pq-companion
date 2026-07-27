// Mirrors backend/internal/emote's models — the developer-mode Spell Emote
// Customizer, which edits the client-visible chat emote text in
// <EQPath>/spells_en.txt.

export interface EmoteText {
  you_cast: string
  other_casts: string
  cast_on_you: string
  cast_on_other: string
  spell_fades: string
}

// Any subset of columns; only the fields present are changed server-side.
export type EmoteColumnsPatch = Partial<EmoteText>

export interface SpellEmote {
  spell_id: number
  name: string
  default: EmoteText
  current: EmoteText
  customized: boolean
  overridden_fields: string[] | null
}

export interface EmoteFieldDiff {
  field: string
  label: string
  old: string
  new: string
}

export interface SpellEmoteDiff {
  spell_id: number
  name: string
  fields: EmoteFieldDiff[]
}

export interface EmoteStatus {
  configured: boolean
  file_present: boolean
  has_default_backup: boolean
  override_count: number
  pending_external_change: boolean
  external_change_at?: number
}
