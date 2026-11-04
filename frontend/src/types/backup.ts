export interface Backup {
  id: string
  name: string
  notes: string
  created_at: string // ISO 8601
  size_bytes: number
  file_count: number
  locked: boolean
  trigger_reason: string // "manual" | "auto" | "scheduled"
}

export interface BackupsResponse {
  backups: Backup[]
}

export interface BackupSettings {
  auto_backup: boolean
  schedule: 'off' | 'hourly' | 'daily'
  max_backups: number
}

// Kind of EQ settings file, mirroring backup.ClassifyFile. Snapshot file
// listings also use 'log' for log files and '' for anything else.
export type SettingsFileKind =
  | 'spellsets'
  | 'bandolier'
  | 'character'
  | 'ui'
  | 'bazaar'
  | 'client'
  | 'zeal'
  | 'quarm'

export interface BackupFile {
  name: string
  size_bytes: number
  modified: string
  kind: SettingsFileKind | 'log' | ''
  character: string
}

export interface FileVersion {
  id: number
  file_name: string
  captured_at: string
  // The file's modification time when it was saved.
  modified_at: string
  size_bytes: number
  sha256: string
}

export interface TrackedFile {
  file_name: string
  kind: SettingsFileKind
  // '' for files shared by every character (eqclient.ini, zeal.ini).
  character: string
  // Whether the file is in the EQ folder now.
  exists: boolean
  // Newest first.
  versions: FileVersion[]
}
