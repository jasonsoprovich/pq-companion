/**
 * useItemDisplay — the item the player last opened an in-game item window
 * for, shared by the Item Info dashboard panel and popout. Seeds from
 * GET /api/item-display/last, then follows item:display broadcasts and loads
 * the full item row for each.
 *
 * useItemInfoOffered — whether the Item Info overlay should be offered at
 * all (see itemInfoOffered): tracks the config live, so the overlay appears
 * the moment the backend sees the first Zeal item event.
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { itemInfoOffered } from '../lib/overlays'
import { getConfig, getItem, getLastItemDisplay } from '../services/api'
import type { Config } from '../types/config'
import type { Item, ItemDisplayShown } from '../types/item'

export interface ItemDisplayHook {
  // The latest event, even when its item isn't in the database.
  shown: ItemDisplayShown | null
  // The item row for shown, or null while loading / when not found.
  item: Item | null
  notFound: boolean
}

export function useItemDisplay(): ItemDisplayHook {
  const [shown, setShown] = useState<ItemDisplayShown | null>(null)
  const [item, setItem] = useState<Item | null>(null)
  const [notFound, setNotFound] = useState(false)
  // Drops a slow lookup that finishes after a newer item arrived.
  const latestId = useRef(0)

  const show = useCallback((s: ItemDisplayShown) => {
    setShown(s)
    if (s.item_id === latestId.current) return
    latestId.current = s.item_id
    setNotFound(false)
    getItem(s.item_id)
      .then((it) => { if (latestId.current === s.item_id) setItem(it) })
      .catch(() => {
        if (latestId.current !== s.item_id) return
        setItem(null)
        setNotFound(true)
      })
  }, [])

  const ready = useWebSocket((msg) => {
    if (msg.type === WSEvent.ItemDisplay) show(msg.data as ItemDisplayShown)
  })

  useEffect(() => {
    if (ready !== 'open') return
    getLastItemDisplay()
      .then((s) => { if (s) show(s) })
      .catch(() => {})
  }, [ready, show])

  return { shown, item, notFound }
}

export function useItemInfoOffered(): boolean {
  const [offered, setOffered] = useState(false)

  useEffect(() => {
    let cancelled = false
    getConfig()
      .then((c) => { if (!cancelled) setOffered(itemInfoOffered(c.preferences)) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  useWebSocket((msg) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    const cfg = msg.data as Config
    if (cfg?.preferences) setOffered(itemInfoOffered(cfg.preferences))
  })

  return offered
}

// useItemInfoAutoShow opens the Item Info popout (without taking focus) on
// each in-game item click when the user turned on auto-show in Settings.
// Mounted once, in the main window.
export function useItemInfoAutoShow(): void {
  const enabled = useRef(false)

  useEffect(() => {
    getConfig()
      .then((c) => {
        enabled.current = itemInfoOffered(c.preferences) && Boolean(c.preferences.item_info_overlay?.auto_show)
      })
      .catch(() => {})
  }, [])

  useWebSocket((msg) => {
    if (msg.type === WSEvent.ConfigUpdated) {
      const p = (msg.data as Config)?.preferences
      if (p) enabled.current = itemInfoOffered(p) && Boolean(p.item_info_overlay?.auto_show)
      return
    }
    if (msg.type === WSEvent.ItemDisplay && enabled.current) {
      window.electron?.overlay?.showItemInfo?.().catch(() => {})
    }
  })
}
