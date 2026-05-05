import { useCallback, useSyncExternalStore } from 'react'
import { useWebSocket, type WsMessage } from './useWebSocket'
import type { LogEvent } from '../types/logEvent'

/**
 * Module-level event store so the Log Feed survives navigation between tabs.
 *
 * Why module-level rather than React state:
 *   The page component unmounts on every route change. Keeping events in its
 *   local state means the feed wipes any time the user clicks another tab.
 *   Lifting to context would also work, but the only real consumer is one
 *   page plus a top-level subscriber — a tiny external store + the
 *   useSyncExternalStore hook is simpler and doesn't force a re-render of
 *   every other consumer when an event lands.
 *
 * Cleared only via clearLogFeed() (the page's Trash button) or by reloading
 * the renderer. The MAX_EVENTS cap prevents unbounded growth over a long
 * play session.
 */

const MAX_EVENTS = 200

let store: LogEvent[] = []
const listeners = new Set<() => void>()

function emit(): void {
  for (const fn of listeners) fn()
}

function append(ev: LogEvent): void {
  const next = [ev, ...store]
  store = next.length > MAX_EVENTS ? next.slice(0, MAX_EVENTS) : next
  emit()
}

export function clearLogFeed(): void {
  if (store.length === 0) return
  store = []
  emit()
}

/**
 * Mount once at the top of the main window (e.g. inside MainWindowLayout) so
 * the shared feed keeps filling regardless of which tab the user is on.
 */
export function useLogFeedSubscriber(): void {
  const handle = useCallback((msg: WsMessage) => {
    if (!msg.type.startsWith('log:')) return
    append(msg.data as LogEvent)
  }, [])
  useWebSocket(handle)
}

/**
 * Read-only consumer hook for pages that want to render the live feed.
 * Returns the same array reference between events, so memoised filters /
 * derived state remain stable across renders.
 */
export function useLogFeed(): LogEvent[] {
  const subscribe = useCallback((cb: () => void) => {
    listeners.add(cb)
    return () => { listeners.delete(cb) }
  }, [])
  const getSnapshot = (): LogEvent[] => store
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export const LOG_FEED_MAX = MAX_EVENTS
