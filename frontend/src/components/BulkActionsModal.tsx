import React, { useEffect, useMemo, useState } from 'react'
import { X, RefreshCw, FolderOpen, Play, Square as SquareIcon, Volume2 } from 'lucide-react'
import {
  listActionTemplates,
  bulkApplyActions,
  bulkConvertTTSToSound,
} from '../services/api'
import { playSoundForTest, stopTestPlayback } from '../services/audio'
import type { Action, ActionTemplate, BulkResult, Trigger } from '../types/trigger'

const ALL_SCOPE = '__all__'

type BulkOperation = 'tts_to_sound' | 'apply_template'

interface BulkActionsModalProps {
  triggers: Trigger[]
  onClose: () => void
  onApplied: (result: BulkResult) => void
}

/**
 * Bulk action editor: pick a scope (one category or all triggers) and either
 * convert every text-to-speech alert to a sound file — the "packs ship TTS
 * but I use sounds" fix — or stamp a saved action template onto every
 * trigger in scope.
 */
export default function BulkActionsModal({
  triggers,
  onClose,
  onApplied,
}: BulkActionsModalProps): React.ReactElement {
  const [scope, setScope] = useState<string>(ALL_SCOPE)
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

  // Categories with counts, Uncategorized last.
  const categories = useMemo(() => {
    const counts = new Map<string, number>()
    for (const t of triggers) {
      const key = t.pack_name ?? ''
      counts.set(key, (counts.get(key) ?? 0) + 1)
    }
    return Array.from(counts.entries()).sort((a, b) => {
      if (a[0] === '') return 1
      if (b[0] === '') return -1
      return a[0].localeCompare(b[0])
    })
  }, [triggers])

  const scoped = useMemo(
    () =>
      scope === ALL_SCOPE
        ? triggers
        : triggers.filter((t) => (t.pack_name ?? '') === scope),
    [triggers, scope],
  )

  const hasTTS = (t: Trigger) =>
    (t.actions ?? []).some((a: Action) => a.type === 'text_to_speech') ||
    (includeTimerAlerts &&
      (t.timer_alerts ?? []).some((a) => a.type === 'text_to_speech'))

  const affected = useMemo(
    () => (operation === 'tts_to_sound' ? scoped.filter(hasTTS) : scoped),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [scoped, operation, includeTimerAlerts],
  )

  const template = templates.find((t) => t.id === templateId)
  const canBrowse = typeof window !== 'undefined' && !!window.electron?.dialog?.selectSoundFile
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
          maxWidth: 480,
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

        {/* Scope */}
        <div className="space-y-1">
          <label className="text-[11px] font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Triggers to edit
          </label>
          <select
            value={scope}
            onChange={(e) => setScope(e.target.value)}
            className="w-full text-xs px-2 py-1.5 rounded"
            style={selectStyle}
          >
            <option value={ALL_SCOPE}>All triggers ({triggers.length})</option>
            {categories.map(([name, count]) => (
              <option key={name || '__uncat__'} value={name}>
                {name || 'Uncategorized'} ({count})
              </option>
            ))}
          </select>
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
                Replaces ALL actions on every trigger in scope with the
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
                No templates saved yet. Open any trigger, set up its actions,
                and use the Templates button to save them first.
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
          </div>
        )}

        {/* Preview + apply */}
        <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          {operation === 'tts_to_sound' ? (
            <>
              {affected.length} of {scoped.length} trigger{scoped.length !== 1 ? 's' : ''} in
              scope use{affected.length === 1 ? 's' : ''} text-to-speech and will be converted.
            </>
          ) : (
            <>
              The actions of {affected.length} trigger{affected.length !== 1 ? 's' : ''} will be
              overwritten.
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
