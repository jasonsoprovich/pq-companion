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

// EqwStatus is the eqw.dll (EQW-TAKP) version check surfaced by
// /api/eqw/status. eqw.dll ships alongside eqgame.dll but has no PE version
// resource and isn't in the Quarm manifest, so it's checked by scanning the
// binary's build stamp and comparing against the newest GitHub release tag —
// mirrors backend/internal/eqw/status.go; keep the two in sync.
export interface EqwStatus {
  eq_path: string
  installed: boolean
  dll_path?: string
  version?: string
  latest_version?: string
  update_available: boolean
}
