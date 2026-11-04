/**
 * useLatestDeath — the most recent death, recap included, for the Death
 * Recap dashboard panel and popout. Follows overlay:combat broadcasts and
 * falls back to the newest saved death, so the card still shows the last
 * death after an app restart.
 */
import { useEffect, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { getCombatState, listDeaths } from '../services/api'
import type { CombatState, DeathRecord } from '../types/combat'

function lastOf(state: CombatState): DeathRecord | null {
  const d = state.deaths ?? []
  return d.length > 0 ? d[d.length - 1] : null
}

export function useLatestDeath(): DeathRecord | null {
  const [death, setDeath] = useState<DeathRecord | null>(null)

  // Combat state broadcasts every second in a fight; only take a death that
  // is newer than the one on screen.
  const take = (d: DeathRecord | null): void => {
    if (!d) return
    setDeath((cur) =>
      cur && Date.parse(cur.timestamp) >= Date.parse(d.timestamp) ? cur : d)
  }

  const ready = useWebSocket((msg) => {
    if (msg.type === WSEvent.OverlayCombat) take(lastOf(msg.data as CombatState))
  })

  useEffect(() => {
    if (ready !== 'open') return
    getCombatState().then((s) => take(lastOf(s))).catch(() => {})
    listDeaths({ limit: 1 }).then((r) => take(r.deaths[0] ?? null)).catch(() => {})
  }, [ready])

  return death
}
