import { useCallback, useEffect, useState } from 'react'

/**
 * Whether the DPS meter shows damage or healing. Saved in localStorage and
 * shared between the popped-out meter and the dashboard panel, like the DPS
 * mode (useDPSMode). HPS covers only your own heals: the log names the target
 * of heals you cast but not other healers' heals.
 */
export type MeterKind = 'dps' | 'hps'

const STORAGE_KEY = 'pq-meter-kind'

function readStored(): MeterKind {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'hps' ? 'hps' : 'dps'
  } catch {
    return 'dps'
  }
}

export function useMeterKind(): { kind: MeterKind; setKind: (k: MeterKind) => void } {
  const [kind, setKindState] = useState<MeterKind>(() => readStored())

  // The other window's change arrives as a storage event.
  useEffect(() => {
    const onStorage = (e: StorageEvent): void => {
      if (e.key === STORAGE_KEY) setKindState(readStored())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setKind = useCallback((k: MeterKind) => {
    setKindState(k)
    try { localStorage.setItem(STORAGE_KEY, k) } catch { /* noop */ }
  }, [])

  return { kind, setKind }
}
