import { useCallback, useEffect, useState } from 'react'
import { listCharacters } from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

/**
 * Returns the active character's level, or null while unknown/unresolved.
 *
 * Like useWishlistItemIds, this is deliberately independent of
 * ActiveCharacterContext so it works in standalone overlay windows as well as
 * the main app. It resolves the active character via /api/characters and
 * only refetches when that could have changed (character switch or a config
 * update, e.g. a fresh quarmy.txt import bumping the stored level).
 */
export function useActiveCharacterLevel(): number | null {
  const [level, setLevel] = useState<number | null>(null)

  const refresh = useCallback(() => {
    let cancelled = false
    listCharacters()
      .then((resp) => {
        const activeName = resp.active
        if (!activeName) return null
        const char = resp.characters.find(
          (c) => c.name.toLowerCase() === activeName.toLowerCase(),
        )
        return char?.level ?? null
      })
      .then((next) => {
        if (!cancelled) setLevel(next)
      })
      .catch(() => {
        if (!cancelled) setLevel(null)
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => refresh(), [refresh])

  const handle = useCallback(
    (msg: WsMessage) => {
      if (msg.type === WSEvent.ConfigCharacterDetected || msg.type === WSEvent.ConfigUpdated) {
        refresh()
      }
    },
    [refresh],
  )
  useWebSocket(handle)

  return level
}
