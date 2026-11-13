/**
 * FeatureDotsProvider / useFeatureDots — live state for the green "new
 * feature" dots (see lib/featureRegistry.ts for the rules). Mounted once in
 * the main window; overlay windows don't use it.
 *
 * Initialization (the first-run baseline) happens in App's startup effect,
 * not here, because it must read last_seen_changelog_version before the
 * What's New logic stamps it. This provider just reads the result, follows
 * config:updated, and records dots as seen.
 */
import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { getChangelog, getConfig, saveConfigChanges } from '../services/api'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { FeatureDotsState } from '../types/config'
import {
  keysSeenOnVisit,
  liveFeatures,
  navHasDot,
  type FeatureEntry,
} from '../lib/featureRegistry'

interface FeatureDotsValue {
  navDot: (route: string) => boolean
  settingsTabDot: (tab: string) => boolean
  overlayDot: (panelKey: string) => boolean
  markSeen: (keys: string[]) => void
  // Clear the deeper dot for an opened Settings tab / used overlay row.
  markSettingsTabSeen: (tab: string) => void
  markOverlaySeen: (panelKey: string) => void
}

const noop: FeatureDotsValue = {
  navDot: () => false,
  settingsTabDot: () => false,
  overlayDot: () => false,
  markSeen: () => undefined,
  markSettingsTabSeen: () => undefined,
  markOverlaySeen: () => undefined,
}

const FeatureDotsContext = createContext<FeatureDotsValue>(noop)

export function FeatureDotsProvider({ children }: { children: React.ReactNode }): React.ReactElement {
  const [state, setState] = useState<FeatureDotsState | undefined>()
  const [appVersion, setAppVersion] = useState('')
  const [changelog, setChangelog] = useState<string[]>([])
  // Keys marked seen this session but possibly not yet echoed back by a
  // config:updated — merged in so a dot doesn't flicker back meanwhile.
  const pendingSeen = useRef<Set<string>>(new Set())

  useEffect(() => {
    getConfig().then((c) => setState(c.feature_dots)).catch(() => undefined)
    window.electron?.app?.getVersion?.()
      .then((v) => setAppVersion(v ?? ''))
      .catch(() => undefined)
    getChangelog()
      .then(({ entries }) => setChangelog(entries.map((e) => e.version)))
      .catch(() => undefined)
  }, [])

  useWebSocket(
    useCallback((msg: { type: string; data?: unknown }) => {
      if (msg.type !== WSEvent.ConfigUpdated) return
      getConfig().then((c) => setState(c.feature_dots)).catch(() => undefined)
    }, []),
  )

  const seen = useMemo(
    () => [...(state?.seen ?? []), ...pendingSeen.current],
    [state],
  )
  const live: FeatureEntry[] = useMemo(
    () => liveFeatures(state && { ...state, seen }, appVersion, changelog),
    [state, seen, appVersion, changelog],
  )

  const markSeen = useCallback((keys: string[]) => {
    const fresh = keys.filter((k) => !pendingSeen.current.has(k))
    if (fresh.length === 0) return
    fresh.forEach((k) => pendingSeen.current.add(k))
    setState((s) => (s ? { ...s, seen: [...(s.seen ?? []), ...fresh] } : s))
    getConfig()
      .then((c) => {
        const cur = c.feature_dots
        if (!cur?.baseline) return
        const union = Array.from(new Set([...(cur.seen ?? []), ...pendingSeen.current]))
        return saveConfigChanges({ ...c, feature_dots: { ...cur, seen: union } }, c)
      })
      .catch(() => undefined)
  }, [])

  // Clear nav dots for the page the user lands on.
  const { pathname } = useLocation()
  useEffect(() => {
    const keys = keysSeenOnVisit(live, pathname).filter((k) => !seen.includes(k))
    if (keys.length > 0) markSeen(keys)
  }, [pathname, live, seen, markSeen])

  const value = useMemo<FeatureDotsValue>(
    () => ({
      navDot: (route) => navHasDot(live, seen, route),
      settingsTabDot: (tab) => live.some((e) => e.settingsTab === tab),
      overlayDot: (key) => live.some((e) => e.overlay === key),
      markSeen,
      markSettingsTabSeen: (tab) => {
        const ids = live.filter((e) => e.settingsTab === tab).map((e) => e.id)
        if (ids.length > 0) markSeen(ids)
      },
      markOverlaySeen: (key) => {
        const ids = live.filter((e) => e.overlay === key).map((e) => e.id)
        if (ids.length > 0) markSeen(ids)
      },
    }),
    [live, seen, markSeen],
  )

  return <FeatureDotsContext.Provider value={value}>{children}</FeatureDotsContext.Provider>
}

export function useFeatureDots(): FeatureDotsValue {
  return useContext(FeatureDotsContext)
}

// Small green dot used everywhere a feature is new. Orange stays reserved
// for warnings (e.g. the large-log dot on Settings).
export function NewDot({ className = '', title = 'New since your last update' }: { className?: string; title?: string }): React.ReactElement {
  return (
    <span
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${className}`}
      style={{ backgroundColor: 'var(--color-success)', boxShadow: '0 0 0 3px rgba(34,197,94,0.15)' }}
      title={title}
    />
  )
}
