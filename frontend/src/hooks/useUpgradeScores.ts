import { useCallback, useEffect, useState } from 'react'
import {
  getCharacterItemUpgradeScores,
  listCharacters,
  type ItemUpgradeScore,
} from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

/**
 * Returns the Gear Upgrade Finder score for each of itemIds that would be an
 * upgrade for the active character (score > 0), keyed by item id. Items that
 * aren't upgrades — or can't be worn — are simply absent. Like
 * useActiveCharacterLevel it resolves the active character itself, so it
 * works in standalone overlay windows, and refetches on character switch or
 * config updates (a fresh Zeal export changes what's equipped).
 */
export function useUpgradeScores(itemIds: number[]): Map<number, ItemUpgradeScore> {
  const [scores, setScores] = useState<Map<number, ItemUpgradeScore>>(new Map())
  const [rev, setRev] = useState(0)
  const key = [...new Set(itemIds)].sort((a, b) => a - b).join(',')

  useEffect(() => {
    setScores(new Map())
    if (!key) return
    let cancelled = false
    listCharacters()
      .then((resp) => {
        const active = resp.active?.toLowerCase()
        const char = resp.characters.find((c) => c.name.toLowerCase() === active)
        if (!char) return null
        return getCharacterItemUpgradeScores(char.id, key.split(',').map(Number))
      })
      .then((r) => {
        if (cancelled || !r) return
        setScores(new Map(Object.entries(r.scores).map(([id, s]) => [Number(id), s])))
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [key, rev])

  const handle = useCallback((msg: WsMessage) => {
    if (msg.type === WSEvent.ConfigCharacterDetected || msg.type === WSEvent.ConfigUpdated) {
      setRev((n) => n + 1)
    }
  }, [])
  useWebSocket(handle)

  return scores
}
