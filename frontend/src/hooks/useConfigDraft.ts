/**
 * useConfigDraft — an editable copy of the app config that saves itself, for
 * settings shown outside the Settings page (the Overlays hub's drawer).
 * Same behaviour as Settings: edits apply locally at once and are saved
 * after a short pause, only the fields changed here are written (so edits
 * made elsewhere aren't reverted), changes from elsewhere are merged in, and
 * a pending edit is flushed on unmount.
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { getConfig, saveConfigChanges } from '../services/api'
import { mergeConfigEdits } from '../lib/configMerge'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import type { Config } from '../types/config'

export type DraftSaveState = 'idle' | 'saving' | 'saved' | 'error'

const SAVE_DELAY_MS = 600

export function useConfigDraft(): {
  config: Config | null
  setConfig: (c: Config) => void
  saveState: DraftSaveState
} {
  const [config, setConfigState] = useState<Config | null>(null)
  const [saved, setSaved] = useState<Config | null>(null)
  const [saveState, setSaveState] = useState<DraftSaveState>('idle')
  const configRef = useRef<Config | null>(null)
  const savedRef = useRef<Config | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const clearState = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => { configRef.current = config }, [config])
  useEffect(() => { savedRef.current = saved }, [saved])

  useEffect(() => {
    getConfig()
      .then((c) => { setConfigState(c); setSaved(c) })
      .catch(() => {})
  }, [])

  // Rebase onto changes made elsewhere (overlay windows, Settings, backend);
  // an unsaved edit made here wins.
  useWebSocket((msg: { type: string }) => {
    if (msg.type !== WSEvent.ConfigUpdated) return
    getConfig()
      .then((srv) => {
        const base = savedRef.current
        if (!base) return
        setConfigState((c) => c && mergeConfigEdits(srv, base, c))
        setSaved(srv)
      })
      .catch(() => {})
  })

  const persist = useCallback((c: Config) => {
    const base = savedRef.current ?? c
    setSaveState('saving')
    saveConfigChanges(c, base)
      .then((fresh) => {
        setConfigState((cur) => cur && mergeConfigEdits(fresh, base, cur))
        setSaved(fresh)
        setSaveState('saved')
        if (clearState.current) clearTimeout(clearState.current)
        clearState.current = setTimeout(() => setSaveState('idle'), 2000)
      })
      .catch(() => setSaveState('error'))
  }, [])

  useEffect(() => {
    if (!config || !saved || JSON.stringify(config) === JSON.stringify(saved)) return
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => persist(config), SAVE_DELAY_MS)
    return () => { if (timer.current) clearTimeout(timer.current) }
  }, [config, saved, persist])

  useEffect(() => () => {
    if (timer.current) clearTimeout(timer.current)
    if (clearState.current) clearTimeout(clearState.current)
    const c = configRef.current
    const base = savedRef.current
    if (c && base && JSON.stringify(c) !== JSON.stringify(base)) void saveConfigChanges(c, base)
  }, [])

  const setConfig = useCallback((c: Config) => setConfigState(c), [])
  return { config, setConfig, saveState }
}
