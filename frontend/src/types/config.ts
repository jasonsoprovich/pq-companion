export interface Preferences {
  overlay_opacity: number
  minimize_to_tray: boolean
  parse_combat_log: boolean
  overlay_dps_enabled: boolean
  overlay_hps_enabled: boolean
}

export interface Config {
  eq_path: string
  character: string
  server_addr: string
  preferences: Preferences
}
