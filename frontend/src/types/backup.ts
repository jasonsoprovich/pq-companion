export interface Backup {
  id: string
  name: string
  notes: string
  created_at: string // ISO 8601
  size_bytes: number
  file_count: number
}

export interface BackupsResponse {
  backups: Backup[]
}
