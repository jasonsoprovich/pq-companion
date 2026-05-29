import { useCallback, useEffect, useState } from 'react'
import { listCharacters, listWishlist } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

/**
 * Returns the set of item IDs on the active character's wishlist, used by the
 * NPC loot overlay to highlight drops the player is watching for.
 *
 * Like useActivePlayerName, this is deliberately independent of
 * ActiveCharacterContext so it works in standalone overlay windows (which
 * aren't wrapped in that provider) as well as the main app. It resolves the
 * active character via /api/characters, then loads that character's wishlist.
 *
 * The set is a pure function of (active character, that character's wishlist
 * contents) — nothing else changes it. So it only refetches on the two events
 * that can change those inputs:
 *   - config:character_detected / config:updated → active character changed
 *   - wishlist:changed → the active character's wishlist was edited in-game
 * Membership is deduped automatically by the Set (a multi-slot item appears
 * once per slot bucket in the wishlist but collapses to one id here).
 */
export function useWishlistItemIds(): Set<number> {
  const [ids, setIds] = useState<Set<number>>(() => new Set())

  const refresh = useCallback(() => {
    let cancelled = false
    listCharacters()
      .then(async (resp) => {
        const activeName = resp.active
        if (!activeName) return new Set<number>()
        const char = resp.characters.find(
          (c) => c.name.toLowerCase() === activeName.toLowerCase(),
        )
        if (!char) return new Set<number>()
        const wl = await listWishlist(char.id)
        return new Set(wl.entries.map((e) => e.item_id))
      })
      .then((next) => {
        if (!cancelled) setIds(next)
      })
      .catch(() => {
        if (!cancelled) setIds(new Set())
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => refresh(), [refresh])

  const handle = useCallback(
    (msg: WsMessage) => {
      if (
        msg.type === WSEvent.ConfigCharacterDetected ||
        msg.type === WSEvent.ConfigUpdated ||
        msg.type === WSEvent.WishlistChanged
      ) {
        refresh()
      }
    },
    [refresh],
  )
  useWebSocket(handle)

  return ids
}
