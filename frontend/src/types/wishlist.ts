// Lightweight item slice bundled with each wishlist entry — just enough to
// render the wishlist row. Use getItem(id) when full item details are needed.
export interface WishlistItemBrief {
  id: number
  name: string
  icon: number
  slots: number
  item_class: number
  item_type: number
}

export interface WishlistEntry {
  id: number
  item_id: number
  slot_bucket: string
  // Global per-character ordering — the same field drives within-card order
  // (filtered to a bucket) and the flat "All items" view.
  sort_order: number
  created_at: number
  item?: WishlistItemBrief
}

// Per-character layout for one slot card: where it sits relative to other
// cards in category view and whether it's collapsed. Missing buckets fall
// back to canonical order / expanded on the client.
export interface WishlistSlotLayout {
  slot_bucket: string
  position: number
  collapsed: boolean
}

export interface WishlistListResponse {
  entries: WishlistEntry[]
  slot_layout: WishlistSlotLayout[]
}
