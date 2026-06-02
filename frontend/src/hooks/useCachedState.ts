import { useCallback, useState } from 'react'

// Module-level cache of per-page UI state (search queries, filters, scroll).
// It survives a component unmount/remount within the session, so navigating
// from a search list into a detail view and pressing Back restores the search
// instead of resetting to an empty page (issue #9). It intentionally resets on
// app restart — this is a desktop app, so URL-bookmarkable search state isn't
// needed, and keeping it out of the URL avoids per-filter serialization.
const cache = new Map<string, unknown>()

// useCachedState is a drop-in replacement for useState whose value persists
// under `key` across mounts. Pass a stable, page-unique key, e.g. "zones.query".
export function useCachedState<T>(
  key: string,
  initial: T,
): [T, (value: T | ((prev: T) => T)) => void] {
  const [state, setState] = useState<T>(() => (cache.has(key) ? (cache.get(key) as T) : initial))

  const set = useCallback(
    (value: T | ((prev: T) => T)) => {
      setState((prev) => {
        const next = typeof value === 'function' ? (value as (p: T) => T)(prev) : value
        cache.set(key, next)
        return next
      })
    },
    [key],
  )

  return [state, set]
}

// clearCachedState drops a single cached key (e.g. when a reset button should
// forget the saved value too).
export function clearCachedState(key: string): void {
  cache.delete(key)
}
