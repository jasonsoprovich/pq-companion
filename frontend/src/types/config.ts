export interface Preferences {
  overlay_opacity: number
  minimize_to_tray: boolean
  parse_combat_log: boolean
}

export interface Config {
  eq_path: string
  character: string
  server_addr: string
  preferences: Preferences
}
