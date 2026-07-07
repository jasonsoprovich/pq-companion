export interface KeyComponent {
  item_id: number
  item_name: string
  notes?: string
  /** Additional item IDs that also satisfy this component ("any one of"). */
  alt_item_ids?: number[]
}

export interface KeyDef {
  id: string
  name: string
  description: string
  components: KeyComponent[]
  final_item?: KeyComponent
  intermediate_item?: KeyComponent
  intermediate_cover_count?: number
}

export interface KeysResponse {
  keys: KeyDef[]
}

export interface ComponentStatus {
  item_id: number
  item_name: string
  have: boolean
  shared_bank: boolean
  /**
   * Present on the character's /keys key ring. Assembled keys are consumed
   * out of inventory onto the key ring, so a fully-keyed character shows
   * on_key_ring rather than have.
   */
  on_key_ring?: boolean
  /** Raw Zeal locations (e.g. "General1:Slot3", "Bank2") of the held item. */
  locations?: string[]
}

export interface CharacterKeyProgress {
  character: string
  has_export: boolean
  components: ComponentStatus[]
  final_item?: ComponentStatus
  intermediate_item?: ComponentStatus
}

export interface KeyProgress {
  key_id: string
  characters: CharacterKeyProgress[]
}

export interface KeysProgressResponse {
  configured: boolean
  keys: KeyProgress[]
}
