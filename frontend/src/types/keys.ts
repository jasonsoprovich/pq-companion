export interface KeyComponent {
  item_id: number
  item_name: string
  notes?: string
}

export interface KeyDef {
  id: string
  name: string
  description: string
  components: KeyComponent[]
}

export interface KeysResponse {
  keys: KeyDef[]
}

export interface ComponentStatus {
  item_id: number
  item_name: string
  have: boolean
  shared_bank: boolean
}

export interface CharacterKeyProgress {
  character: string
  has_export: boolean
  components: ComponentStatus[]
}

export interface KeyProgress {
  key_id: string
  characters: CharacterKeyProgress[]
}

export interface KeysProgressResponse {
  configured: boolean
  keys: KeyProgress[]
}
