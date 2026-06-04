import React, { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { runBackfill } from '../services/api'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'

export interface BackfillProgress {
  character: string
  done: number
  total: number
}

export interface BackfillRunResult {
  character: string
  results: Record<string, number>
  error?: string
}

interface BackfillContextValue {
  running: boolean
  /** Characters in the current/last run, in order. */
  runChars: string[]
  /** Live per-character byte progress from the backend scan. */
  prog: BackfillProgress | null
  /** Seconds elapsed on the current character. */
  elapsed: number
  /** Per-character results once each finishes (accumulates during the run). */
  results: BackfillRunResult[] | null
  /** Kick off a backfill. No-op if one is already running. */
  startBackfill: (chars: string[], sections: string[]) => void
  /** Clear the finished-run results (dismisses the completion bar). */
  dismissResults: () => void
}

const BackfillContext = createContext<BackfillContextValue | null>(null)

// BackfillProvider owns the backfill run so it survives navigation: the scan
// runs server-side and streams `backfill:progress` over WS, and this provider
// (mounted at the app root) holds the run state. That lets the Settings panel
// kick off a run and the user keep using the app while a persistent bottom bar
// shows progress — instead of a blocking modal that dies when Settings unmounts.
export function BackfillProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  const [running, setRunning] = useState(false)
  const [runChars, setRunChars] = useState<string[]>([])
  const [prog, setProg] = useState<BackfillProgress | null>(null)
  const [elapsed, setElapsed] = useState(0)
  const [results, setResults] = useState<BackfillRunResult[] | null>(null)
  const charStartRef = useRef<number>(0)
  const runningRef = useRef(false)

  const onWs = useCallback((msg: { type: string; data?: unknown }) => {
    if (msg.type !== WSEvent.BackfillProgress) return
    setProg(msg.data as BackfillProgress)
  }, [])
  useWebSocket(onWs)

  // Tick elapsed so the timer/ETA advance smoothly between progress events.
  useEffect(() => {
    if (!running) return
    const t = setInterval(() => setElapsed((Date.now() - charStartRef.current) / 1000), 500)
    return () => clearInterval(t)
  }, [running])

  const startBackfill = useCallback((chars: string[], sections: string[]) => {
    if (runningRef.current || chars.length === 0 || sections.length === 0) return
    runningRef.current = true
    setRunChars(chars)
    setResults(null)
    setRunning(true)

    void (async () => {
      const out: BackfillRunResult[] = []
      for (const c of chars) {
        charStartRef.current = Date.now()
        setElapsed(0)
        setProg({ character: c, done: 0, total: 0 })
        try {
          const r = await runBackfill(c, sections)
          out.push({ character: c, results: r.results })
        } catch (e) {
          out.push({ character: c, results: {}, error: (e as Error).message })
        }
        // Surface each character's result as it completes.
        setResults([...out])
      }
      setProg(null)
      setRunning(false)
      runningRef.current = false
    })()
  }, [])

  const dismissResults = useCallback(() => setResults(null), [])

  return (
    <BackfillContext.Provider
      value={{ running, runChars, prog, elapsed, results, startBackfill, dismissResults }}
    >
      {children}
    </BackfillContext.Provider>
  )
}

export function useBackfill(): BackfillContextValue {
  const ctx = useContext(BackfillContext)
  if (!ctx) throw new Error('useBackfill must be used within a BackfillProvider')
  return ctx
}
