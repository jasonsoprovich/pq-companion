/**
 * Display style for the CH Chain overlay: the original countdown list, or
 * the "lanes" view (one lane per heal target, callout blocks sliding toward
 * the target's name as the cast completes — the layout Quarm Tools users
 * are used to). Purely a display preference, so it lives in localStorage
 * like the Main/Secondary `chChain:view` switch. The dashboard card, the
 * popped-out window and the settings page share it via the `storage`
 * event (plus a same-window custom event, since `storage` only fires in
 * OTHER windows).
 */
import { useCallback, useEffect, useState } from 'react'

export type ChainStyle = 'list' | 'lanes'

const STORAGE_KEY = 'chChain:style'
const LOCAL_EVENT = 'chChain:style-changed'

function readStyle(): ChainStyle {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'lanes' ? 'lanes' : 'list'
  } catch {
    return 'list'
  }
}

export function useChainStyle(): [ChainStyle, (s: ChainStyle) => void] {
  const [style, setStyleState] = useState<ChainStyle>(readStyle)

  useEffect(() => {
    const sync = (): void => setStyleState(readStyle())
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) sync()
    }
    window.addEventListener('storage', onStorage)
    window.addEventListener(LOCAL_EVENT, sync)
    return () => {
      window.removeEventListener('storage', onStorage)
      window.removeEventListener(LOCAL_EVENT, sync)
    }
  }, [])

  const setStyle = useCallback((s: ChainStyle) => {
    setStyleState(s)
    try {
      localStorage.setItem(STORAGE_KEY, s)
    } catch {
      // Storage unavailable — keep the in-memory choice for this session.
    }
    window.dispatchEvent(new Event(LOCAL_EVENT))
  }, [])

  return [style, setStyle]
}
