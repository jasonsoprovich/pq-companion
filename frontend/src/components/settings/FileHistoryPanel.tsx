import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Eye,
  History,
  RefreshCw,
  RotateCcw,
  X,
} from 'lucide-react'
import {
  captureFileHistory,
  getConfig,
  getFileHistory,
  getFileVersion,
  restoreFileVersion,
  saveConfigChanges,
} from '../../services/api'
import { useEscapeToClose } from '../../hooks/useEscapeToClose'
import type { FileVersion, SettingsFileKind, TrackedFile } from '../../types/backup'
import type { Config } from '../../types/config'

export const FILE_KIND_LABELS: Record<SettingsFileKind, { label: string; hint: string }> = {
  spellsets: { label: 'Spellsets', hint: 'Zeal spell sets' },
  bandolier: { label: 'Bandolier', hint: 'Zeal bandolier sets' },
  character: {
    label: 'Hotkeys, macros & settings',
    hint: 'Social macros, hotbuttons, chat and window options',
  },
  ui: { label: 'UI layout', hint: 'Window positions and sizes' },
  bazaar: { label: 'Bazaar prices', hint: 'Trader prices' },
  client: { label: 'EQ client settings', hint: 'eqclient.ini: video, sound, keys' },
  zeal: { label: 'Zeal settings', hint: 'zeal.ini, including bandolier storage' },
  quarm: { label: 'Quarm client settings', hint: 'quarmclient.ini' },
}

const KIND_ORDER: SettingsFileKind[] = [
  'spellsets', 'bandolier', 'character', 'ui', 'bazaar', 'client', 'zeal', 'quarm',
]

const SHARED = '__shared__'
const KEEP_CHOICES = [10, 30, 100]
const CHAR_KEY = 'backups.historyCharacter'

function fmtWhen(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

function fmtSize(n: number): string {
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`
}

const btn: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  color: 'var(--color-muted-foreground)',
  border: '1px solid var(--color-border)',
}

/**
 * FileHistoryPanel lists each character's EQ settings files with the
 * versions saved automatically whenever one changes, and restores any file
 * to an earlier version.
 */
export default function FileHistoryPanel(): React.ReactElement {
  const [files, setFiles] = useState<TrackedFile[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<string | null>(null)
  const [cfg, setCfg] = useState<Config | null>(null)
  const [character, setCharacter] = useState<string>(() => {
    try {
      return localStorage.getItem(CHAR_KEY) ?? ''
    } catch {
      return ''
    }
  })
  const [viewing, setViewing] = useState<FileVersion | null>(null)

  const load = useCallback(() => {
    setError(null)
    getFileHistory()
      .then((r) => setFiles(r.files))
      .catch((e: Error) => setError(e.message))
  }, [])

  useEffect(() => {
    load()
    getConfig().then(setCfg).catch(() => {})
  }, [load])

  const characters = useMemo(() => {
    const set = new Set<string>()
    let shared = false
    for (const f of files ?? []) {
      if (f.character) set.add(f.character)
      else shared = true
    }
    const out = [...set].sort()
    if (shared) out.push(SHARED)
    return out
  }, [files])

  const selected = characters.includes(character) ? character : (characters[0] ?? '')
  const pick = (c: string) => {
    setCharacter(c)
    try {
      localStorage.setItem(CHAR_KEY, c)
    } catch {
      // Remembering the pick is a convenience only.
    }
  }

  const shown = useMemo(
    () =>
      (files ?? [])
        .filter((f) => (selected === SHARED ? !f.character : f.character === selected))
        .sort((a, b) => KIND_ORDER.indexOf(a.kind) - KIND_ORDER.indexOf(b.kind)),
    [files, selected],
  )

  const enabled = !cfg?.backup?.file_history_off
  const keep = cfg?.backup?.file_history_keep || 30

  const saveSetting = (patch: { file_history_off?: boolean; file_history_keep?: number }) => {
    if (!cfg) return
    const next = { ...cfg, backup: { ...cfg.backup, ...patch } }
    setCfg(next)
    saveConfigChanges(next, cfg).then(setCfg).catch((e: Error) => setError(e.message))
  }

  const captureNow = () => {
    setBusy(true)
    setNote(null)
    captureFileHistory()
      .then((r) => {
        setNote(r.saved > 0 ? `Saved ${r.saved} changed file${r.saved === 1 ? '' : 's'}.` : 'No changes since the last save.')
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  return (
    <div className="space-y-3">
      <div
        className="rounded-lg p-3 space-y-2"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <div className="flex flex-wrap items-center gap-3">
          <label className="flex items-center gap-2 text-xs cursor-pointer" style={{ color: 'var(--color-foreground)' }}>
            <input
              type="checkbox"
              checked={enabled}
              disabled={!cfg}
              onChange={(e) => saveSetting({ file_history_off: !e.target.checked })}
              className="w-4 h-4 accent-primary"
            />
            Save a version whenever a file changes
          </label>
          <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Keep
            <select
              value={keep}
              disabled={!cfg || !enabled}
              onChange={(e) => saveSetting({ file_history_keep: Number(e.target.value) })}
              className="text-xs rounded px-1.5 py-0.5 outline-none"
              style={{ ...btn, color: 'var(--color-foreground)' }}
            >
              {[...new Set([...KEEP_CHOICES, keep])].sort((a, b) => a - b).map((n) => (
                <option key={n} value={n}>{n}</option>
              ))}
            </select>
            versions per file
          </label>
          <button
            onClick={captureNow}
            disabled={busy || !enabled}
            className="ml-auto flex items-center gap-1.5 text-xs px-2 py-1 rounded disabled:opacity-50"
            style={btn}
            title="Check the EQ folder for changed files now instead of waiting up to a minute"
          >
            <RefreshCw size={11} className={busy ? 'animate-spin' : undefined} />
            Check now
          </button>
        </div>
        <p className="text-[11px] flex items-start gap-1.5" style={{ color: 'var(--color-muted-foreground)' }}>
          <AlertTriangle size={12} className="shrink-0 mt-0.5" style={{ color: 'var(--color-warning, #eab308)' }} />
          Camp the character before restoring its files. EverQuest rewrites them when you
          log out, which would undo the restore. Restoring saves the current file as a
          version first, so you can always go back.
        </p>
        {note && <p className="text-[11px]" style={{ color: 'var(--color-success)' }}>{note}</p>}
        {error && <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{error}</p>}
      </div>

      {files === null && !error && (
        <RefreshCw size={16} className="animate-spin mx-auto" style={{ color: 'var(--color-muted)' }} />
      )}

      {files !== null && files.length === 0 && (
        <p className="text-xs text-center py-6" style={{ color: 'var(--color-muted-foreground)' }}>
          No EverQuest settings files found yet. Check that your EQ folder is set in
          Settings → General.
        </p>
      )}

      {characters.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {characters.map((c) => (
            <button
              key={c}
              onClick={() => pick(c)}
              className="text-xs px-2.5 py-1 rounded"
              style={
                c === selected
                  ? { backgroundColor: 'var(--color-primary)', color: 'var(--color-background)', border: '1px solid transparent' }
                  : btn
              }
            >
              {c === SHARED ? 'All characters' : c}
            </button>
          ))}
        </div>
      )}

      {shown.map((f) => (
        <FileRow key={f.file_name} file={f} onView={setViewing} onRestored={load} />
      ))}

      {viewing && <VersionViewer version={viewing} onClose={() => setViewing(null)} />}
    </div>
  )
}

function FileRow({
  file,
  onView,
  onRestored,
}: {
  file: TrackedFile
  onView: (v: FileVersion) => void
  onRestored: () => void
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [confirm, setConfirm] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const meta = FILE_KIND_LABELS[file.kind]
  const latest = file.versions[0]

  const restore = (id: number) => {
    setBusy(true)
    setError(null)
    restoreFileVersion(id)
      .then(() => {
        setDone(id)
        setConfirm(null)
        onRestored()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  return (
    <div
      className="rounded-lg"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <button
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left"
      >
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            {meta?.label ?? file.file_name}
          </p>
          <p className="text-[11px] truncate" style={{ color: 'var(--color-muted)' }}>
            <code className="font-mono">{file.file_name}</code>
            {meta ? ` · ${meta.hint}` : ''}
            {!file.exists ? ' · missing from the EQ folder' : ''}
          </p>
        </div>
        <span className="text-[11px] shrink-0 text-right" style={{ color: 'var(--color-muted-foreground)' }}>
          {file.versions.length} version{file.versions.length === 1 ? '' : 's'}
          {latest && (
            <>
              <br />
              changed {fmtWhen(latest.modified_at)}
            </>
          )}
        </span>
      </button>

      {open && (
        <div className="border-t px-3 py-2 space-y-1" style={{ borderColor: 'var(--color-border)' }}>
          {file.versions.length === 0 && (
            <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
              Not saved yet. The first version is saved within a minute.
            </p>
          )}
          {file.versions.map((v, i) => (
            <div key={v.id}>
              <div className="flex items-center gap-2 text-xs">
                <History size={11} style={{ color: 'var(--color-muted)' }} />
                <span style={{ color: 'var(--color-foreground)' }}>{fmtWhen(v.modified_at)}</span>
                {i === 0 && (
                  <span
                    className="text-[10px] px-1.5 rounded"
                    style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
                    title="The newest saved version; normally the file as it is now"
                  >
                    latest
                  </span>
                )}
                <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>{fmtSize(v.size_bytes)}</span>
                {done === v.id && (
                  <span className="text-[11px]" style={{ color: 'var(--color-success)' }}>Restored</span>
                )}
                <div className="ml-auto flex items-center gap-1">
                  <button onClick={() => onView(v)} className="flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded" style={btn}>
                    <Eye size={10} /> View
                  </button>
                  {i > 0 && (
                    <button
                      onClick={() => setConfirm(v.id)}
                      className="flex items-center gap-1 text-[11px] px-1.5 py-0.5 rounded"
                      style={btn}
                    >
                      <RotateCcw size={10} /> Restore
                    </button>
                  )}
                </div>
              </div>
              {confirm === v.id && (
                <div
                  className="mt-1 mb-1 ml-5 flex items-center gap-2 rounded px-2 py-1.5"
                  style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
                >
                  <span className="flex-1 text-[11px]" style={{ color: 'var(--color-foreground)' }}>
                    Replace {file.file_name} with this version? Camp {file.character || 'your characters'} first.
                  </span>
                  <button
                    onClick={() => restore(v.id)}
                    disabled={busy}
                    className="text-[11px] px-2 py-0.5 rounded font-medium"
                    style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
                  >
                    {busy ? 'Restoring…' : 'Restore'}
                  </button>
                  <button onClick={() => setConfirm(null)} style={{ color: 'var(--color-muted-foreground)' }}>
                    <X size={12} />
                  </button>
                </div>
              )}
            </div>
          ))}
          {error && <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{error}</p>}
        </div>
      )}
    </div>
  )
}

function VersionViewer({ version, onClose }: { version: FileVersion; onClose: () => void }): React.ReactElement {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEscapeToClose(onClose)
  useEffect(() => {
    getFileVersion(version.id)
      .then((r) => setText(r.content))
      .catch((e: Error) => setError(e.message))
  }, [version.id])
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-6"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="flex max-h-full w-full max-w-3xl flex-col rounded-lg"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <code className="font-mono text-xs" style={{ color: 'var(--color-foreground)' }}>{version.file_name}</code>
          <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>{fmtWhen(version.modified_at)}</span>
          <button onClick={onClose} className="ml-auto" style={{ color: 'var(--color-muted-foreground)' }}>
            <X size={14} />
          </button>
        </div>
        <pre
          className="flex-1 overflow-auto p-3 text-[11px] font-mono whitespace-pre"
          style={{ color: 'var(--color-foreground)', minHeight: 200 }}
        >
          {error ?? text ?? 'Loading…'}
        </pre>
      </div>
    </div>
  )
}
