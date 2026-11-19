import { useCallback, useEffect, useState } from 'react'
import {
  getCharacterItemUpgradeScores,
  listCharacters,
  type ItemPriorityFocus,
  type ItemUpgradeScore,
} from '../services/api'
import { useWebSocket, type WsMessage } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export interface UpgradeInfo {
  // Upgrades only (score > 0).
  scores: Map<number, ItemUpgradeScore>
  // Items carrying one of the character's wanted foci, whatever their score.
  priority: Map<number, ItemPriorityFocus>
}

/**
 * Returns the Gear Upgrade Finder score for each of itemIds that would be an
 * upgrade for the active character (score > 0), keyed by item id. Items that
 * aren't upgrades — or can't be worn — are simply absent. Like
 * useActiveCharacterLevel it resolves the active character itself, so it
 * works in standalone overlay windows, and refetches on character switch or
 * config updates (a fresh Zeal export changes what's equipped).
 */
export function useUpgradeScores(itemIds: number[]): Map<number, ItemUpgradeScore> {
  return useUpgradeInfo(itemIds).scores
}

/**
 * useUpgradeScores plus the wanted-focus flags: priority lists items carrying
 * a focus the character picked in the finder, even when they're a stat loss.
 */
export function useUpgradeInfo(itemIds: number[]): UpgradeInfo {
  const [info, setInfo] = useState<UpgradeInfo>(EMPTY)
  const [rev, setRev] = useState(0)
  const key = [...new Set(itemIds)].sort((a, b) => a - b).join(',')

  useEffect(() => {
    setInfo(EMPTY)
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
        const byId = <T,>(rec: Record<string, T> | undefined): Map<number, T> =>
          new Map(Object.entries(rec ?? {}).map(([id, v]) => [Number(id), v]))
        setInfo({ scores: byId(r.scores), priority: byId(r.priority) })
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

  return info
}

const EMPTY: UpgradeInfo = { scores: new Map(), priority: new Map() }
