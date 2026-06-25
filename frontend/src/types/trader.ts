// Types for the Bazaar Trader Tracker (developer-tab feature). They mirror the
// Go structs in backend/internal/trader and backend/internal/api/trader.go.

// One trader character in the picker. `has_bzr` means a BZR price file exists on
// disk; `snapshot_count` is how many inventory snapshots have been captured.
export interface TraderCharacter {
  name: string
  has_bzr: boolean
  listing_count: number
  for_sale_count: number
  snapshot_count: number
  last_captured?: number // unix seconds, 0/absent if never captured
}

// One BZR-priced item, annotated with how many are currently in a Trader's
// Satchel (from the latest snapshot). Price is in copper; 0 = not for sale.
export interface TraderListing {
  name: string
  price: number
  in_satchel: number
  item_id?: number
  icon?: number
}

// One item whose satchel count changed between two snapshots. For sold items
// qty is the amount that left; for restocked items it's the amount added.
export interface TraderSoldItem {
  item_id: number
  name: string
  qty: number
  unit_price: number // copper, from the BZR file (0 if unlisted)
  line_total: number // copper
  listed: boolean // had a BZR price > 0
  icon?: number
}

// One inferred sale session: the diff between two consecutive snapshots.
// from_time/to_time are RFC3339 timestamps; coin values are in copper.
export interface TraderSession {
  character: string
  from_time: string
  to_time: string
  sold: TraderSoldItem[]
  restocked: TraderSoldItem[]
  estimated_revenue: number
  on_person_delta: number
  total_coin_delta: number
  reconciles: boolean
  caveats: string[]
}

// One captured snapshot summary for the history list.
export interface TraderSnapshotInfo {
  taken_at: number // unix seconds
  item_count: number
  total_qty: number
  on_person: number // copper
  bank_copper: number // copper
}

// Result of a manual "Capture now" request.
export interface TraderCaptureResponse {
  captured: boolean // true if a NEW snapshot was stored
  reason?: string
  taken_at?: number
  item_count?: number
}
