// Master entry — one keyring-eligible item from quarm.db keyring_data,
// joined with items.Name + zone.long_name for display.
export interface KeyringMasterEntry {
  key_item: number
  key_name: string
  item_name: string
  zone_id: number
  zone_name: string
  stage: number
}

// Entry — one row of user.db keyring_entries.
export interface KeyringEntry {
  character: string
  key_item: number
  first_seen_at: number
  last_seen_at: number
}

export interface KeyringMasterResponse {
  keys: KeyringMasterEntry[]
}

export interface KeyringCharactersResponse {
  characters: string[]
}

export interface KeyringCharacterResponse {
  character: string
  entries: KeyringEntry[]
}
