import { useEffect, useState } from 'react'
import { getPlayer } from '../services/api'
import type { PlayerSighting } from '../types/player'

/**
 * Looks up a targeted name in the player-sighting tracker. The NPC overlay uses
 * this to show class / level / guild when the target isn't an NPC in the game
 * database — i.e. an actual player character seen in a prior /who.
 *
 * `enabled` gates the fetch so the tracker isn't queried for every NPC target;
 * callers pass true only when the target has no quarm.db record. Resolves to
 * null on a 404 (untracked name) so the caller can fall back to a generic
 * "no record" message.
 */
export function useTargetPlayer(
  name: string | undefined,
  enabled: boolean,
): { player: PlayerSighting | null; loading: boolean } {
  const [player, setPlayer] = useState<PlayerSighting | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!name || !enabled) {
      setPlayer(null)
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    setPlayer(null)
    getPlayer(name)
      .then((p) => { if (!cancelled) setPlayer(p) })
      .catch(() => { if (!cancelled) setPlayer(null) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [name, enabled])

  return { player, loading }
}
