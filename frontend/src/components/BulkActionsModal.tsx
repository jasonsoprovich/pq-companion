import React, { useEffect, useMemo, useState } from 'react'
import {
  X,
  RefreshCw,
  FolderOpen,
  Play,
  Square as SquareIcon,
  Volume2,
  CheckSquare,
  Square,
  MinusSquare,
  ChevronRight,
  ChevronDown,
  Search,
} from 'lucide-react'
import {
  listActionTemplates,
  bulkApplyActions,
  bulkConvertTTSToSound,
} from '../services/api'
import { playSoundForTest, stopTestPlayback } from '../services/audio'
import type { Action, ActionTemplate, BulkResult, Trigger } from '../types/trigger'

type BulkOperation = 'tts_to_sound' | 'apply_template'

interface BulkActionsModalProps {
  triggers: Trigger[]
  onClose: () => void
  onApplied: (result: BulkResult) => void
}

type CatState = 'none' | 'some' | 'all'

/**
 * Bulk action editor: hand-pick the triggers to edit — grouped by category
 * with per-trigger checkboxes and tri-state category toggles — then either
 * convert every text-to-speech alert to a sound file (the "packs ship TTS
 * but I use sounds" fix) or stamp a saved action template onto every trigger
 * selected.
 */
export default function BulkActionsModal({
  triggers,
  onClose,
  onApplied,
}: BulkActionsModalProps): React.ReactElement {
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(triggers.map((t) => t.id)),
  )
  const [search, setSearch] = useState('')
  const [collapsed, setCollapsed] = useState<Set<string>>(
    () => new Set(triggers.map((t) => t.pack_name ?? '')),
  )
  const [operation, setOperation] = useState<BulkOperation>('tts_to_sound')
  const [templates, setTemplates] = useState<ActionTemplate[]>([])
  const [templateId, setTemplateId] = useState<string>('')
  const [soundPath, setSoundPath] = useState('')
  const [volume, setVolume] = useState(100)
  const [includeTimerAlerts, setIncludeTimerAlerts] = useState(true)
  const [playing, setPlaying] = useState(false)
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !applying) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, applying])

  useEffect(() => {
    listActionTemplates()
      .then((list) => {
        setTemplates(list)
        if (list.length > 0) setTemplateId(list[0].id)
      })
      .catch(() => {})
    return () => stopTestPlayback()
  }, [])

  // Triggers grouped by category (pack_name), Uncategorized last, sorted by
  // name within each group.
  const grouped = useMemo(() => {
    const map = new Map<string, Trigger[]>()
    for (const t of triggers) {
      const key = t.pack_name ?? ''
      const arr = map.get(key)
      if (arr) arr.push(t)
      else map.set(key, [t])
    }
    const entries = Array.from(map.entries())
    entries.sort((a, b) => {
      if (a[0] === '') return 1
      if (b[0] === '') return -1
      return a[0].localeCompare(b[0])
    })
    for (const [, arr] of entries) arr.sort((x, y) => x.name.localeCompare(y.name))
    return entries
  }, [triggers])

  const query = search.trim().toLowerCase()

  // Groups filtered by the search box. When searching, empty groups drop out.
  const visibleGroups = useMemo(() => {
    if (!query) return grouped
    return grouped
      .map(
        ([key, arr]) =>
          [key, arr.filter((t) => t.name.toLowerCase().includes(query))] as const,
      )
      .filter(([, arr]) => arr.length > 0)
  }, [grouped, query])

  const selectedTriggers = useMemo(
    () => triggers.filter((t) => selected.has(t.id)),
    [triggers, selected],
  )

  const hasTTS = (t: Trigger) =>
    (t.actions ?? []).some((a: Action) => a.type === 'text_to_speech') ||
    (includeTimerAlerts &&
      (t.timer_alerts ?? []).some((a) => a.type === 'text_to_speech'))

  const affected = useMemo(
    () =>
      operation === 'tts_to_sound'
        ? selectedTriggers.filter(hasTTS)
        : selectedTriggers,
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [selectedTriggers, operation, includeTimerAlerts],
  )

  const toggleOne = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const setMany = (ids: string[], on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      for (const id of ids) {
        if (on) next.add(id)
        else next.delete(id)
      }
      return next
    })

  const catState = (arr: Trigger[]): CatState => {
    let sel = 0
    for (const t of arr) if (selected.has(t.id)) sel++
    if (sel === 0) return 'none'
    if (sel === arr.length) return 'all'
    return 'some'
  }

  const toggleCollapse = (key: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })

  const template = templates.find((t) => t.id === templateId)
  const canBrowse =
    typeof window !== 'undefined' && !!window.electron?.dialog?.selectSoundFile
  const canApply =
    affected.length > 0 &&
    !applying &&
    (operation === 'tts_to_sound' ? soundPath.trim().length > 0 : !!template)

  const handleBrowse = async () => {
    const picked = await window.electron?.dialog?.selectSoundFile()
    if (picked) setSoundPath(picked)
  }

  const handleTestSound = () => {
    if (playing) {
      stopTestPlayback()
      setPlaying(false)
      return
    }
    if (!soundPath.trim()) return
    setPlaying(true)
    playSoundForTest(soundPath, volume / 100, () => setPlaying(false))
  }

  const handleApply = () => {
    if (!canApply) return
    setApplying(true)
    setError(null)
    const ids = affected.map((t) => t.id)
    const req =
      operation === 'tts_to_sound'
        ? bulkConvertTTSToSound(ids, soundPath.trim(), volume / 100, includeTimerAlerts)
        : bulkApplyActions(ids, template?.actions ?? [])
    req
      .then((res) => onApplied(res))
      .catch((err: Error) => setError(err.message))
      .finally(() => setApplying(false))
  }

  const selectStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
  }

  const catIcon = (state: CatState) =>
    state === 'all' ? (
      <CheckSquare size={13} style={{ color: 'var(--color-accent)' }} />
    ) : state === 'some' ? (
      <MinusSquare size={13} style={{ color: 'var(--color-accent)' }} />
    ) : (
      <Square size={13} style={{ color: 'var(--color-muted)' }} />
    )

  return (
    <div
      onClick={() => !applying && onClose()}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1000,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          width: '100%',
          maxWidth: 520,
        }}
      >
        <div className="flex items-center justify-between">
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Bulk edit actions
          </p>
          <button onClick={() => !applying && onClose()} aria-label="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>

        {/* Trigger picker */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label
              className="text-[11px] font-medium"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              Triggers to edit
            </label>
            <div className="flex items-center gap-2 text-[11px]">
              <button
                onClick={() => setSelected(new Set(triggers.map((t) => t.id)))}
                className="hover:underline"
                style={{ color: 'var(--color-accent)' }}
              >
                Select all
              </button>
              <span style={{ color: 'var(--color-border)' }}>|</span>
              <button
                onClick={() => setSelected(new Set())}
                className="hover:underline"
                style={{ color: 'var(--color-accent)' }}
              >
                None
              </button>
            </div>
          </div>

          <div className="relative">
            <Search
              size={12}
              className="absolute left-2 top-1/2 -translate-y-1/2"
              style={{ color: 'var(--color-muted)' }}
            />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Filter by name…"
              className="w-full text-xs pl-7 pr-2 py-1.5 rounded"
              style={selectStyle}
            />
          </div>

          <div
            className="rounded overflow-auto"
            style={{
              border: '1px solid var(--color-border)',
              maxHeight: 240,
            }}
          >
            {visibleGroups.length === 0 ? (
              <p className="text-[11px] px-2.5 py-3" style={{ color: 'var(--color-muted)' }}>
                No triggers match “{search.trim()}”.
              </p>
            ) : (
              visibleGroups.map(([key, arr]) => {
                const state = catState(arr)
                const isCollapsed = !query && collapsed.has(key)
                const selCount = arr.filter((t) => selected.has(t.id)).length
                return (
                  <div key={key || '__uncat__'}>
                    {/* Category header */}
                    <div
                      className="flex items-center gap-1.5 px-2 py-1.5 sticky top-0"
                      style={{ backgroundColor: 'var(--color-surface-2)' }}
                    >
                      <button
                        onClick={() => toggleCollapse(key)}
                        aria-label={isCollapsed ? 'Expand' : 'Collapse'}
                        className="shrink-0"
                        style={{ color: 'var(--color-muted)' }}
                      >
                        {isCollapsed ? (
                          <ChevronRight size={13} />
                        ) : (
                          <ChevronDown size={13} />
                        )}
                      </button>
                      <button
                        onClick={() =>
                          setMany(
                            arr.map((t) => t.id),
                            state !== 'all',
                          )
                        }
                        aria-label="Toggle category"
                        className="shrink-0"
                      >
                        {catIcon(state)}
                      </button>
                      <button
                        onClick={() => toggleCollapse(key)}
                        className="flex-1 min-w-0 flex items-center justify-between text-left"
                      >
                        <span
                          className="truncate text-[11px] font-semibold"
                          style={{ color: 'var(--color-foreground)' }}
                        >
                          {key || 'Uncategorized'}
                        </span>
                        <span
                          className="shrink-0 text-[10px] ml-2"
                          style={{ color: 'var(--color-muted-foreground)' }}
                        >
                          {selCount}/{arr.length}
                        </span>
                      </button>
                    </div>

                    {/* Trigger rows */}
                    {!isCollapsed &&
                      arr.map((t) => {
                        const on = selected.has(t.id)
                        return (
                          <button
                            key={t.id}
                            onClick={() => toggleOne(t.id)}
                            className="flex w-full items-center gap-2 pl-8 pr-2 py-1 text-left"
                            style={{
                              backgroundColor: on
                                ? 'var(--color-surface-2)'
                                : 'transparent',
                            }}
                          >
                            <span className="shrink-0">
                              {on ? (
                                <CheckSquare
                                  size={13}
                                  style={{ color: 'var(--color-accent)' }}
                                />
                              ) : (
                                <Square
                                  size={13}
                                  style={{ color: 'var(--color-muted)' }}
                                />
                              )}
                            </span>
                            <span
                              className="truncate text-xs"
                              style={{ color: 'var(--color-foreground)' }}
                            >
                              {t.name}
                            </span>
                          </button>
                        )
                      })}
                  </div>
                )
              })
            )}
          </div>
        </div>

        {/* Operation */}
        <div className="space-y-1.5">
          <label className="flex items-start gap-2 cursor-pointer">
            <input
              type="radio"
              name="bulk-op"
              checked={operation === 'tts_to_sound'}
              onChange={() => setOperation('tts_to_sound')}
              className="mt-0.5"
            />
            <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
              Replace text-to-speech with a sound file
              <span className="block text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                Only the TTS parts change — overlay text and other actions
                stay as they are.
              </span>
            </span>
          </label>
          <label className="flex items-start gap-2 cursor-pointer">
            <input
              type="radio"
              name="bulk-op"
              checked={operation === 'apply_template'}
              onChange={() => setOperation('apply_template')}
              className="mt-0.5"
            />
            <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
              Apply an action template
              <span className="block text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                Replaces ALL actions on every selected trigger with the
                template's actions.
              </span>
            </span>
          </label>
        </div>

        {/* Operation config */}
        {operation === 'tts_to_sound' ? (
          <div className="space-y-2">
            <div className="flex items-center gap-1.5">
              <input
                type="text"
                value={soundPath}
                onChange={(e) => setSoundPath(e.target.value)}
                placeholder={'Sound file path (e.g. C:\\sounds\\alert.wav)'}
                className="flex-1 min-w-0 text-xs px-2 py-1.5 rounded font-mono"
                style={selectStyle}
              />
              {canBrowse && (
                <button
                  type="button"
                  onClick={handleBrowse}
                  className="p-1.5 rounded shrink-0"
                  style={selectStyle}
                  title="Browse for sound file"
                >
                  <FolderOpen size={12} />
                </button>
              )}
              <button
                type="button"
                onClick={handleTestSound}
                disabled={!soundPath.trim()}
                className="p-1.5 rounded shrink-0"
                style={{ ...selectStyle, opacity: soundPath.trim() ? 1 : 0.5 }}
                title={playing ? 'Stop' : 'Test sound'}
              >
                {playing ? <SquareIcon size={12} /> : <Play size={12} />}
              </button>
            </div>
            <div className="flex items-center gap-2">
              <Volume2 size={12} style={{ color: 'var(--color-muted)' }} />
              <input
                type="range"
                min={0}
                max={100}
                value={volume}
                onChange={(e) => setVolume(Number(e.target.value))}
                className="flex-1"
              />
              <span className="text-[11px] w-8 text-right" style={{ color: 'var(--color-muted-foreground)' }}>
                {volume}%
              </span>
            </div>
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={includeTimerAlerts}
                onChange={(e) => setIncludeTimerAlerts(e.target.checked)}
              />
              <span className="text-[11px]" style={{ color: 'var(--color-foreground)' }}>
                Also convert "fading soon" timer alerts
              </span>
            </label>
          </div>
        ) : (
          <div className="space-y-1">
            {templates.length === 0 ? (
              <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                No templates saved yet. To make one: open any trigger, add the
                actions you want (overlay text, sound, TTS…) in its Actions
                section, click the <strong>Templates</strong> button there, type
                a name, and Save. That same Templates menu is where you rename,
                update, or delete templates.
              </p>
            ) : (
              <select
                value={templateId}
                onChange={(e) => setTemplateId(e.target.value)}
                className="w-full text-xs px-2 py-1.5 rounded"
                style={selectStyle}
              >
                {templates.map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name}
                    {t.is_default ? ' (default)' : ''}
                  </option>
                ))}
              </select>
            )}
            {templates.length > 0 && (
              <p className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
                Create, rename, update, or delete templates from the{' '}
                <strong>Templates</strong> button in a trigger's Actions section.
              </p>
            )}
          </div>
        )}

        {/* Preview + apply */}
        <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          {operation === 'tts_to_sound' ? (
            <>
              {affected.length} of {selectedTriggers.length} selected trigger
              {selectedTriggers.length !== 1 ? 's' : ''} use
              {affected.length === 1 ? 's' : ''} text-to-speech and will be converted.
            </>
          ) : (
            <>
              The actions of {affected.length} selected trigger
              {affected.length !== 1 ? 's' : ''} will be overwritten.
            </>
          )}
        </p>
        {error && (
          <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <button
            onClick={() => !applying && onClose()}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={selectStyle}
          >
            Cancel
          </button>
          <button
            onClick={handleApply}
            disabled={!canApply}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-background)',
              opacity: canApply ? 1 : 0.5,
            }}
          >
            {applying && <RefreshCw size={11} className="animate-spin" />}
            {operation === 'tts_to_sound'
              ? `Convert ${affected.length} trigger${affected.length !== 1 ? 's' : ''}`
              : `Overwrite ${affected.length} trigger${affected.length !== 1 ? 's' : ''}`}
          </button>
        </div>
      </div>
    </div>
  )
}
