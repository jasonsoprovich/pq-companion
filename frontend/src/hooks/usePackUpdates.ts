/**
 * usePackUpdates — the installed trigger packs with a pending update (this
 * build's definitions differ from what the user installed). One shared fetch
 * for every consumer: the sidebar's Triggers dot, the Packs tab dot and the
 * Packs tab itself. Packs only change with an app update or when the user
 * installs, removes or updates one, so it loads once and again whenever
 * refreshPackUpdates is called (the Packs tab does after each change).
 */
import { useEffect, useState } from 'react'
import { getPackUpdates } from '../services/api'
import type { PackUpdateSummary } from '../types/trigger'

let current: PackUpdateSummary[] = []
let loaded = false
const listeners = new Set<(u: PackUpdateSummary[]) => void>()

/** Re-fetch pending pack updates and tell every consumer. */
export function refreshPackUpdates(): Promise<PackUpdateSummary[]> {
  loaded = true
  return getPackUpdates()
    .catch(() => [] as PackUpdateSummary[])
    .then((list) => {
      current = list ?? []
      listeners.forEach((fn) => fn(current))
      return current
    })
}

export function usePackUpdates(): PackUpdateSummary[] {
  const [updates, setUpdates] = useState<PackUpdateSummary[]>(current)
  useEffect(() => {
    listeners.add(setUpdates)
    if (!loaded) refreshPackUpdates()
    else setUpdates(current)
    return () => {
      listeners.delete(setUpdates)
    }
  }, [])
  return updates
}
