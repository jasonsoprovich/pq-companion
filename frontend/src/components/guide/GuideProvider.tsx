/**
 * GuideProvider — owns guided-tour state: which tours the user has seen,
 * the tour currently running, and whether a setup modal (onboarding,
 * overlay setup, What's New) is open and tours must wait.
 *
 * - startTour(id) navigates to the tour's route, waits for its ready
 *   anchor, then runs it. Used for replays (Help → Tours, help drawer).
 * - useAutoTour(id) on a page starts that tour once, on first visit, when
 *   no setup modal is open.
 * - A tour is marked seen only when finished or explicitly skipped.
 */
import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { getConfig, saveConfigChanges } from '../../services/api'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { TOURS, tourById, type TourDef } from '../../lib/tours'
import Tour from './Tour'

const READY_TIMEOUT_MS = 3000

interface GuideValue {
  // True while a setup modal is open; auto tours wait for it to clear.
  busy: boolean
  seen: string[]
  loaded: boolean
  active: TourDef | null
  // auto: started by useAutoTour on the page itself. It doesn't navigate
  // (keeps the user's tab/panel) and fails silently.
  startTour: (id: string, opts?: { auto?: boolean }) => void
}

const GuideContext = createContext<GuideValue>({
  busy: false,
  seen: [],
  loaded: false,
  active: null,
  startTour: () => undefined,
})

export function useGuide(): GuideValue {
  return useContext(GuideContext)
}

// findAnchor returns the first element tagged data-tour=anchor, or null.
export function findAnchor(anchor: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-tour="${anchor}"]`)
}

// waitForAnchor resolves true once the anchor mounts, false on timeout.
function waitForAnchor(anchor: string, timeoutMs: number): Promise<boolean> {
  return new Promise((resolve) => {
    const deadline = Date.now() + timeoutMs
    const tick = (): void => {
      if (findAnchor(anchor)) return resolve(true)
      if (Date.now() > deadline) return resolve(false)
      requestAnimationFrame(tick)
    }
    tick()
  })
}

export function GuideProvider({
  busy,
  children,
}: {
  busy: boolean
  children: React.ReactNode
}): React.ReactElement {
  const navigate = useNavigate()
  const location = useLocation()
  const [seen, setSeen] = useState<string[]>([])
  const [loaded, setLoaded] = useState(false)
  const [active, setActive] = useState<TourDef | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  // Bumped on each start so a slow ready-wait from an older request
  // can't start a tour the user has since moved on from.
  const startSeq = useRef(0)

  const refresh = useCallback(() => {
    getConfig()
      .then((c) => {
        setSeen(c.preferences?.tours_seen ?? [])
        setLoaded(true)
      })
      .catch(() => undefined)
  }, [])
  useEffect(refresh, [refresh])
  useWebSocket(
    useCallback(
      (msg: { type: string }) => {
        if (msg.type === WSEvent.ConfigUpdated) refresh()
      },
      [refresh],
    ),
  )

  useEffect(() => {
    if (!notice) return
    const t = setTimeout(() => setNotice(null), 4000)
    return () => clearTimeout(t)
  }, [notice])

  const startTour = useCallback(
    (id: string, opts?: { auto?: boolean }) => {
      const def = tourById(id)
      if (!def) return
      const seq = ++startSeq.current
      setActive(null)
      const target = def.route
      if (!opts?.auto && location.pathname + location.search !== target) navigate(target)
      void waitForAnchor(def.ready, READY_TIMEOUT_MS).then((ok) => {
        if (seq !== startSeq.current) return
        if (ok) setActive(def)
        else if (!opts?.auto) setNotice(`Couldn't start the "${def.title}" tour on this page.`)
      })
    },
    [location.pathname, location.search, navigate],
  )

  const markSeen = useCallback((ids: string[]) => {
    setSeen((s) => Array.from(new Set([...s, ...ids])))
    getConfig()
      .then((c) => {
        const cur = c.preferences?.tours_seen ?? []
        if (ids.every((id) => cur.includes(id))) return
        return saveConfigChanges(
          {
            ...c,
            preferences: { ...c.preferences, tours_seen: Array.from(new Set([...cur, ...ids])) },
          },
          c,
        )
      })
      .catch(() => undefined)
  }, [])

  const end = useCallback(
    (def: TourDef) => {
      setActive(null)
      markSeen([def.id])
    },
    [markSeen],
  )

  const value = useMemo<GuideValue>(
    () => ({ busy, seen, loaded, active, startTour }),
    [busy, seen, loaded, active, startTour],
  )

  return (
    <GuideContext.Provider value={value}>
      {children}
      {active && (
        <Tour
          def={active}
          onEnd={() => end(active)}
          // Marks every tour seen so none auto-start again; Help → Tours
          // can still replay any of them.
          onSkipAll={() => {
            setActive(null)
            markSeen(TOURS.map((t) => t.id))
          }}
        />
      )}
      {notice && (
        <div
          className="fixed bottom-4 left-1/2 z-[60] -translate-x-1/2 rounded px-3 py-2 text-sm shadow-lg"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          role="status"
        >
          {notice}
        </div>
      )}
    </GuideContext.Provider>
  )
}

// useAutoTour starts tour `id` the first time its page is shown, once the
// seen list has loaded and no setup modal or other tour is running.
export function useAutoTour(id: string): void {
  const { busy, seen, loaded, active, startTour } = useGuide()
  const fired = useRef(false)
  useEffect(() => {
    if (fired.current || !loaded || busy || active) return
    if (seen.includes(id)) return
    fired.current = true
    startTour(id, { auto: true })
  }, [id, busy, seen, loaded, active, startTour])
}
