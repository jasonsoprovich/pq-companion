// Project Quarm client-file status surfaced by /api/quarm/client-status. The
// shape mirrors backend/internal/quarm/status.go — keep the two in sync.

export type QuarmMatchStatus = 'match' | 'mismatch' | 'missing' | 'unknown'

export interface QuarmFileLocal {
  path: string
  size: number
  md5: string
  compiled_at: string // RFC3339
  file_version?: string
}

export interface QuarmManifestEntry {
  name: string
  md5: string
  date: string // YYYYMMDD
  size: number
  // FileVersion extracted from the reference DLL the manifest points to.
  // Empty when the entry isn't a DLL, the download failed, or the binary
  // has no VS_VERSION_INFO resource. Used to mark "MD5 differs but version
  // matches" cases as up-to-date instead of falsely warning.
  ref_file_version?: string
}

export interface QuarmFileStatus {
  name: string
  status: QuarmMatchStatus
  local?: QuarmFileLocal
  manifest?: QuarmManifestEntry
  reason?: string
}

export interface QuarmClientStatus {
  eq_path: string
  files: QuarmFileStatus[]
  manifest_version?: string
  manifest_error?: string
}
