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
