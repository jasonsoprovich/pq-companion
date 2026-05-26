// Types mirror backend/internal/sandbox.{Result,Table,Column}.
//
// Rows are unknown[][] because each value can be any of the SQLite native
// types — number, string, null, or (rarely) a Buffer-like for BLOBs. The
// UI renders them via String() and a null sentinel.

export interface SandboxResult {
  columns: string[]
  rows: unknown[][]
  row_count: number
  duration_ms: number
  truncated: boolean
}

export interface SandboxColumn {
  name: string
  type: string
  notnull: boolean
  pk: boolean
}

export interface SandboxTable {
  name: string
  kind: 'table' | 'view'
  columns: SandboxColumn[]
}

export interface SandboxSchemaResponse {
  tables: SandboxTable[]
}

// Mirrors backend/internal/savedquery.SavedQuery. created_at / updated_at
// arrive as ISO-8601 strings (Go time.Time JSON default).
export interface SavedQuery {
  id: string
  name: string
  description: string
  sql: string
  created_at: string
  updated_at: string
}

export interface SavedQueryListResponse {
  queries: SavedQuery[]
}

// Mirrors savedquery.Pack — the import/export envelope. The kind literal
// pins it to this feature so other JSON shapes get rejected at the type
// level (and at the server's import handler).
export interface SavedQueryPack {
  kind: 'pq-companion.query-pack'
  version: number
  exported_at: number
  queries: Array<{ name: string; description: string; sql: string }>
}

export interface SavedQueryImportResponse {
  status: string
  inserted: number
}
