import React from 'react'
import ItemInfoCard, { DASHBOARD_THEME, POPOUT_THEME } from './ItemInfoCard'
import type { ItemDisplayHook } from '../../hooks/useItemDisplay'
import type { ItemInfoSurface } from '../../hooks/useItemInfoSettings'

// Body of the Item Info overlay, shared by the dashboard panel and the popout
// window: the summary card for whatever item was last opened in game.
export default function ItemInfoView({
  shown,
  item,
  notFound,
  surface,
}: ItemDisplayHook & { surface: ItemInfoSurface }): React.ReactElement {
  const theme = surface === 'popout' ? POPOUT_THEME : DASHBOARD_THEME
  const note = (text: string) => (
    <p style={{ padding: '12px 10px', margin: 0, fontSize: 11, color: theme.faint }}>{text}</p>
  )
  if (!shown) return note('Click an item link or inspect an item in game to see its details here.')
  if (notFound) return note(`${shown.name || 'This item'} (#${shown.item_id}) isn't in the item database.`)
  if (!item) return note(`Loading ${shown.name}…`)
  // Keyed by id so expanded lists reset for each new item.
  return <ItemInfoCard key={item.id} item={item} surface={surface} theme={theme} />
}
