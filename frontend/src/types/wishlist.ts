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
  sort_order: number
  created_at: number
  item?: WishlistItemBrief
}

export interface WishlistListResponse {
  entries: WishlistEntry[]
}
