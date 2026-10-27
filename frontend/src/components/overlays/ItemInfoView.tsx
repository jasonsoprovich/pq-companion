import React from 'react'
import ItemDetailView from '../ItemDetailView'
import type { ItemDisplayHook } from '../../hooks/useItemDisplay'

// Body of the Item Info overlay, shared by the dashboard panel and the popout
// window: the full item detail tabs for whatever item was last opened in game.
export default function ItemInfoView({ shown, item, notFound }: ItemDisplayHook): React.ReactElement {
  if (!shown) {
    return (
      <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-muted)' }}>
        Click an item link or inspect an item in game to see its details here.
      </p>
    )
  }
  if (notFound) {
    return (
      <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-muted)' }}>
        {shown.name || 'This item'} (#{shown.item_id}) isn&apos;t in the item database.
      </p>
    )
  }
  if (!item) {
    return (
      <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-muted)' }}>
        Loading {shown.name}…
      </p>
    )
  }
  // Keyed by id so the tabs reset to Overview for each new item.
  return <ItemDetailView key={item.id} item={item} compact />
}
