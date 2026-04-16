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
