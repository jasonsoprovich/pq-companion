import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { getConfig } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { hiddenKeysFor, type NPCSurface } from '../lib/npcItemFilters'
import {
  emptyModeFor,
  listLimitFor,
  resolveItemInfoOrder,
  type EmptyMode,
  type ItemInfoSectionId,
} from '../lib/itemInfoLayout'
import type { Config, ItemInfoOverlaySettings } from '../types/config'

export type ItemInfoSurface = NPCSurface

// The Settings live preview wraps the card in this so it reads the unsaved,
// in-memory settings instead of the saved config. Null everywhere else.
export const ItemInfoPreviewContext = createContext<ItemInfoOverlaySettings | null>(null)

export interface ItemInfoSettings {
  raw: ItemInfoOverlaySettings | undefined
  order: ItemInfoSectionId[]
  // Hidden keys on this surface, prefixes stripped.
  hidden: Set<string>
  limitFor: (id: ItemInfoSectionId) => number
  emptyMode: (id: ItemInfoSectionId) => EmptyMode
}

// Drops From and Other Sources start hidden (mirrors the backend seed), so
// the card matches the defaults before config loads.
const FALLBACK: ItemInfoOverlaySettings = {
  hidden_items: ['section.drops', 'section.sources', 'facts.races'],
}

/**
 * Returns the Item Info overlay's display setup for one surface. Reads on
 * mount and follows `config:updated`, so Settings changes reach both the
 * dashboard panel and the popout live.
 */
export function useItemInfoSettings(surface: ItemInfoSurface): ItemInfoSettings {
  const preview = useContext(ItemInfoPreviewContext)
  const [saved, setSaved] = useState<ItemInfoOverlaySettings | undefined>(undefined)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => { if (!cancelled) setSaved(c.preferences?.item_info_overlay) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    const cfg = msg.data as Config
    if (cfg?.preferences) setSaved(cfg.preferences.item_info_overlay)
  }, [])
  useWebSocket(handle)

  const raw = preview ?? saved ?? FALLBACK
  return useMemo(
    () => ({
      raw,
      order: resolveItemInfoOrder(raw.section_order),
      hidden: hiddenKeysFor(raw.hidden_items ?? [], surface),
      limitFor: (id: ItemInfoSectionId) => listLimitFor(raw, id),
      emptyMode: (id: ItemInfoSectionId) => emptyModeFor(raw, id),
    }),
    [raw, surface],
  )
}
