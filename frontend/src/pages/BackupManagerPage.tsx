import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Archive,
  RefreshCw,
  AlertCircle,
  Plus,
  Trash2,
  RotateCcw,
  CheckCircle2,
  X,
  HardDrive,
  Lock,
  LockOpen,
  Clock,
  Settings,
  Zap,
  Download,
  Upload,
  FolderOpen,
  AlertTriangle,
  FileText,
  History,
  Laptop,
} from 'lucide-react'
import {
  listBackups,
  createBackup,
  deleteBackup,
  restoreBackup,
  listBackupFiles,
  lockBackup,
  unlockBackup,
  pruneBackups,
  getConfig,
  saveConfigChanges,
  exportAppBackup,
  previewAppImport,
  stageAppImport,
  resetApp,
  type AppBackupManifest,
  type AppResetMode,
} from '../services/api'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { collectClientState } from '../services/clientState'
import type { Backup, BackupFile } from '../types/backup'
import FileHistoryPanel, { FILE_KIND_LABELS } from '../components/settings/FileHistoryPanel'
import type { Config, BackupSettings } from '../types/config'

// ── Helpers ────────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }) + ' at ' + d.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
  })
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

// ── Create form ────────────────────────────────────────────────────────────────

interface CreateFormProps {
  onCreated: (b: Backup) => void
  onCancel: () => void
}

function CreateForm({ onCreated, onCancel }: CreateFormProps): React.ReactElement {
  const [name, setName] = useState('')
  const [notes, setNotes] = useState('')
  const [includeLogs, setIncludeLogs] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => { nameRef.current?.focus() }, [])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    setSubmitting(true)
    setError(null)
    createBackup(name.trim(), notes.trim(), includeLogs)
      .then((b) => onCreated(b))
      .catch((err: Error) => {
        setError(err.message)
        setSubmitting(false)
      })
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-lg p-4 space-y-3"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-primary)',
      }}
    >
      <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
        New snapshot
      </p>

      <div className="space-y-2">
        <input
          ref={nameRef}
          type="text"
          placeholder="Snapshot name (required)"
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="w-full rounded px-3 py-1.5 text-sm outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          disabled={submitting}
        />
        <input
          type="text"
          placeholder="Notes (optional)"
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
          className="w-full rounded px-3 py-1.5 text-sm outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
          disabled={submitting}
        />
        <label className="flex items-start gap-2 text-xs cursor-pointer" style={{ color: 'var(--color-foreground)' }}>
          <input
            type="checkbox"
            checked={includeLogs}
            onChange={(e) => setIncludeLogs(e.target.checked)}
            disabled={submitting}
            className="w-4 h-4 mt-px accent-primary"
          />
          <span>
            Include log files
            <span className="block text-[11px]" style={{ color: 'var(--color-muted)' }}>
              Copies every <code className="font-mono">eqlog_*.txt</code> too. Logs can be large,
              so trim them in Settings → Logs first if they are.
            </span>
          </span>
        </label>
      </div>

      {error && (
        <p className="text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}

      <div className="flex items-center gap-2 justify-end">
        <button
          type="button"
          onClick={onCancel}
          disabled={submitting}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting || !name.trim()}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: name.trim() && !submitting ? 'var(--color-primary)' : 'var(--color-surface-2)',
            color: name.trim() && !submitting ? 'var(--color-background)' : 'var(--color-muted)',
            border: '1px solid transparent',
            cursor: name.trim() && !submitting ? 'pointer' : 'not-allowed',
          }}
        >
          {submitting ? (
            <RefreshCw size={11} className="animate-spin" />
          ) : (
            <Archive size={11} />
          )}
          {submitting ? 'Creating…' : 'Create snapshot'}
        </button>
      </div>
    </form>
  )
}

// ── Trigger badge ──────────────────────────────────────────────────────────────

function TriggerBadge({ reason }: { reason: string }): React.ReactElement | null {
  if (reason === 'manual' || !reason) return null
  const isAuto = reason === 'auto'
  const label = isAuto ? 'on change' : 'auto-saved'
  const title = isAuto
    ? 'Saved automatically when your EQ config files changed'
    : 'Saved automatically on your backup schedule'
  const Icon = isAuto ? Zap : Clock
  return (
    <span
      title={title}
      className="flex items-center gap-0.5 text-[10px] px-1.5 py-0.5 rounded"
      style={{
        backgroundColor: 'var(--color-surface-2)',
        color: 'var(--color-muted-foreground)',
        border: '1px solid var(--color-border)',
      }}
    >
      <Icon size={9} />
      {label}
    </span>
  )
}

// ── Backup card ────────────────────────────────────────────────────────────────

type CardAction = 'none' | 'confirm-delete' | 'confirm-restore' | 'restoring' | 'deleting' | 'restored'

interface BackupCardProps {
  backup: Backup
  onDeleted: (id: string) => void
  onRestored: (id: string) => void
  onLockToggled: (id: string, locked: boolean) => void
}

function BackupCard({ backup, onDeleted, onRestored, onLockToggled }: BackupCardProps): React.ReactElement {
  const [action, setAction] = useState<CardAction>('none')
  const [error, setError] = useState<string | null>(null)
  const [lockBusy, setLockBusy] = useState(false)
  const [files, setFiles] = useState<BackupFile[] | null>(null)
  const [picked, setPicked] = useState<Set<string>>(new Set())

  // Restoring asks which files: everything but the logs is ticked, since a
  // restored log would replace the one EverQuest is writing.
  const openRestore = () => {
    setAction('confirm-restore')
    setError(null)
    listBackupFiles(backup.id)
      .then((r) => {
        setFiles(r.files)
        setPicked(new Set(r.files.filter((f) => f.kind !== 'log').map((f) => f.name)))
      })
      .catch((err: Error) => setError(err.message))
  }

  const handleDelete = () => {
    setAction('deleting')
    setError(null)
    deleteBackup(backup.id)
      .then(() => onDeleted(backup.id))
      .catch((err: Error) => {
        setError(err.message)
        setAction('none')
      })
  }

  const handleRestore = () => {
    setAction('restoring')
    setError(null)
    restoreBackup(backup.id, [...picked])
      .then(() => {
        setAction('restored')
        onRestored(backup.id)
        setTimeout(() => setAction('none'), 3000)
      })
      .catch((err: Error) => {
        setError(err.message)
        setAction('none')
      })
  }

  const handleLockToggle = () => {
    setLockBusy(true)
    const fn = backup.locked ? unlockBackup : lockBackup
    fn(backup.id)
      .then(() => onLockToggled(backup.id, !backup.locked))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLockBusy(false))
  }

  return (
    <div
      className="rounded-lg p-4"
      style={{
        backgroundColor: 'var(--color-surface)',
        border: `1px solid ${action === 'restored' ? 'var(--color-success)' : backup.locked ? 'var(--color-primary)' : 'var(--color-border)'}`,
      }}
    >
      {/* Top row: name + actions */}
      <div className="flex items-start gap-3">
        <Archive
          size={16}
          className="shrink-0 mt-0.5"
          style={{ color: 'var(--color-primary)' }}
        />
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate" style={{ color: 'var(--color-foreground)' }}>
            {backup.trigger_reason && backup.trigger_reason !== 'manual'
              ? 'Automatic snapshot'
              : backup.name}
          </p>
          {backup.notes && (
            <p className="text-xs mt-0.5 truncate" style={{ color: 'var(--color-muted-foreground)' }}>
              {backup.notes}
            </p>
          )}
        </div>

        {/* Action buttons — right side */}
        {action === 'none' && (
          <div className="flex items-center gap-1.5 shrink-0">
            <button
              title={backup.locked ? 'Unlock this backup' : 'Lock this backup (protect from auto-cleanup)'}
              onClick={handleLockToggle}
              disabled={lockBusy}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: backup.locked ? 'var(--color-primary)' : 'var(--color-surface-2)',
                color: backup.locked ? 'var(--color-background)' : 'var(--color-muted-foreground)',
                border: `1px solid ${backup.locked ? 'transparent' : 'var(--color-border)'}`,
              }}
            >
              {lockBusy ? <RefreshCw size={11} className="animate-spin" /> : backup.locked ? <Lock size={11} /> : <LockOpen size={11} />}
            </button>
            <button
              title="Restore files from this snapshot"
              onClick={openRestore}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <RotateCcw size={11} />
              Restore
            </button>
            <button
              title="Delete this backup"
              onClick={() => setAction('confirm-delete')}
              className="flex items-center gap-1 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <Trash2 size={11} />
            </button>
          </div>
        )}

        {action === 'restored' && (
          <div className="flex items-center gap-1 text-xs shrink-0" style={{ color: 'var(--color-success)' }}>
            <CheckCircle2 size={13} />
            Restored
          </div>
        )}

        {(action === 'restoring' || action === 'deleting') && (
          <RefreshCw
            size={14}
            className="animate-spin shrink-0"
            style={{ color: 'var(--color-muted)' }}
          />
        )}
      </div>

      {/* Meta row */}
      <div className="flex items-center gap-3 mt-2 ml-7 flex-wrap">
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {formatDate(backup.created_at)}
        </span>
        <span
          className="text-[10px] px-1.5 py-0.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          {backup.file_count} file{backup.file_count !== 1 ? 's' : ''}
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {formatBytes(backup.size_bytes)}
        </span>
        <TriggerBadge reason={backup.trigger_reason} />
        {backup.locked && (
          <span className="flex items-center gap-0.5 text-[10px]" style={{ color: 'var(--color-primary)' }}>
            <Lock size={9} /> locked
          </span>
        )}
      </div>

      {/* Inline confirmation — delete */}
      {action === 'confirm-delete' && (
        <div
          className="mt-3 ml-7 flex items-center gap-2 rounded px-3 py-2"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          <AlertCircle size={13} style={{ color: 'var(--color-danger)' }} />
          <span className="flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
            Delete this snapshot permanently?
          </span>
          <button
            onClick={handleDelete}
            className="text-xs px-2 py-0.5 rounded font-medium"
            style={{
              backgroundColor: 'var(--color-danger)',
              color: '#fff',
              border: '1px solid transparent',
            }}
          >
            Delete
          </button>
          <button
            onClick={() => setAction('none')}
            className="text-xs px-1.5 py-0.5 rounded"
            style={{
              color: 'var(--color-muted-foreground)',
            }}
          >
            <X size={13} />
          </button>
        </div>
      )}

      {/* Inline confirmation — restore, with the files to restore */}
      {action === 'confirm-restore' && (
        <div
          className="mt-3 ml-7 rounded px-3 py-2 space-y-2"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
          }}
        >
          <div className="flex items-center gap-2">
            <RotateCcw size={13} style={{ color: 'var(--color-primary)' }} />
            <span className="flex-1 text-xs" style={{ color: 'var(--color-foreground)' }}>
              Restore these files to your EQ folder? Camp first: EverQuest rewrites its
              files when you log out.
            </span>
            <button
              onClick={() => setAction('none')}
              className="text-xs px-1.5 py-0.5 rounded"
              style={{ color: 'var(--color-muted-foreground)' }}
            >
              <X size={13} />
            </button>
          </div>
          {files === null ? (
            <RefreshCw size={12} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          ) : (
            <RestoreFilePicker files={files} picked={picked} onChange={setPicked} />
          )}
          <div className="flex items-center gap-2">
            <span className="flex-1 text-[11px]" style={{ color: 'var(--color-muted)' }}>
              The files being replaced are saved to Character files history first.
            </span>
            <button
              onClick={handleRestore}
              disabled={picked.size === 0}
              className="text-xs px-2 py-0.5 rounded font-medium disabled:opacity-50"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: 'var(--color-background)',
                border: '1px solid transparent',
              }}
            >
              Restore {picked.size} file{picked.size === 1 ? '' : 's'}
            </button>
          </div>
        </div>
      )}

      {/* Error */}
      {error && (
        <p className="mt-2 ml-7 text-xs" style={{ color: 'var(--color-danger)' }}>
          {error}
        </p>
      )}
    </div>
  )
}

// ── Restore file picker ────────────────────────────────────────────────────────

// fileLabel describes a snapshot file: "Osui · Spellsets", "Log file", ...
function fileLabel(f: BackupFile): string {
  const kind =
    f.kind === 'log' ? 'Log file' : f.kind ? FILE_KIND_LABELS[f.kind].label : 'Other settings file'
  return f.character ? `${f.character} · ${kind}` : kind
}

function RestoreFilePicker({
  files,
  picked,
  onChange,
}: {
  files: BackupFile[]
  picked: Set<string>
  onChange: (next: Set<string>) => void
}): React.ReactElement {
  const toggle = (name: string) => {
    const next = new Set(picked)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    onChange(next)
  }
  // Characters A–Z, then shared files, then unrecognised ones (hand-made
  // copies and the like).
  const rank = (f: BackupFile) => (f.character ? 0 : f.kind ? 1 : 2)
  const sorted = [...files].sort(
    (a, b) =>
      rank(a) - rank(b) ||
      a.character.localeCompare(b.character) ||
      a.name.localeCompare(b.name),
  )
  return (
    <div>
      <div className="flex gap-2 mb-1 text-[11px]">
        <button className="underline" style={{ color: 'var(--color-primary)' }}
          onClick={() => onChange(new Set(files.filter((f) => f.kind !== 'log').map((f) => f.name)))}>
          All settings files
        </button>
        <button className="underline" style={{ color: 'var(--color-primary)' }}
          onClick={() => onChange(new Set())}>
          None
        </button>
      </div>
      <div className="max-h-56 overflow-y-auto space-y-0.5">
        {sorted.map((f) => (
          <label key={f.name} className="flex items-center gap-2 text-[11px] cursor-pointer">
            <input
              type="checkbox"
              checked={picked.has(f.name)}
              onChange={() => toggle(f.name)}
              className="accent-primary"
            />
            <span className="w-60 shrink-0 truncate" style={{ color: 'var(--color-foreground)' }}>
              {fileLabel(f)}
            </span>
            <code className="font-mono truncate" style={{ color: 'var(--color-muted)' }}>{f.name}</code>
            <span className="ml-auto shrink-0" style={{ color: 'var(--color-muted)' }}>
              {formatBytes(f.size_bytes)}
            </span>
          </label>
        ))}
      </div>
    </div>
  )
}

// ── Settings panel ─────────────────────────────────────────────────────────────

interface SettingsPanelProps {
  onClose: () => void
  onPruned: () => void
}

function SettingsPanel({ onClose, onPruned }: SettingsPanelProps): React.ReactElement {
  const [cfg, setCfg] = useState<Config | null>(null)
  // The config as last loaded/saved, so Save writes only this page's edits.
  const [baseCfg, setBaseCfg] = useState<Config | null>(null)
  const [saving, setSaving] = useState(false)
  const [pruning, setPruning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    getConfig()
      .then((c) => {
        setCfg(c)
        setBaseCfg(c)
      })
      .catch((err: Error) => setError(err.message))
  }, [])

  const bs = cfg?.backup ?? { auto_backup: false, schedule: 'off' as const, max_backups: 10 }

  const patch = (update: Partial<BackupSettings>) => {
    if (!cfg) return
    setCfg({ ...cfg, backup: { ...bs, ...update } })
  }

  const handleSave = () => {
    if (!cfg) return
    setSaving(true)
    setError(null)
    saveConfigChanges(cfg, baseCfg ?? cfg)
      .then((saved) => {
        setBaseCfg(saved)
        setSaved(true)
        setTimeout(() => setSaved(false), 2000)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setSaving(false))
  }

  const handlePrune = () => {
    const max = bs.max_backups
    if (!max || max <= 0) return
    setPruning(true)
    pruneBackups(max)
      .then(() => { onPruned(); onClose() })
      .catch((err: Error) => setError(err.message))
      .finally(() => setPruning(false))
  }

  if (!cfg) {
    return (
      <div
        className="rounded-lg p-4 space-y-4"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
      >
        <RefreshCw size={14} className="animate-spin mx-auto" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  return (
    <div
      className="rounded-lg p-4 space-y-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-primary)' }}
    >
      <div className="flex items-center justify-between">
        <p className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Snapshot settings
        </p>
        <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }}>
          <X size={13} />
        </button>
      </div>

      {/* Auto-backup toggle */}
      <label className="flex items-center justify-between gap-3 cursor-pointer">
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
          Snapshot when a file changes
        </span>
        <input
          type="checkbox"
          checked={bs.auto_backup}
          onChange={(e) => patch({ auto_backup: e.target.checked })}
          className="w-4 h-4 accent-primary"
        />
      </label>

      {/* Schedule */}
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>Schedule</span>
        <select
          value={bs.schedule}
          onChange={(e) => patch({ schedule: e.target.value as BackupSettings['schedule'] })}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        >
          <option value="off">Off</option>
          <option value="hourly">Hourly</option>
          <option value="daily">Daily</option>
        </select>
      </div>

      {/* Max backups */}
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs" style={{ color: 'var(--color-foreground)' }}>
          Snapshots to keep <span style={{ color: 'var(--color-muted)' }}>(0 = unlimited; locked ones are never deleted)</span>
        </span>
        <input
          type="number"
          min={0}
          value={bs.max_backups}
          onChange={(e) => patch({ max_backups: Math.max(0, parseInt(e.target.value) || 0) })}
          className="w-16 text-xs rounded px-2 py-1 outline-none text-right"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
          }}
        />
      </div>

      {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}

      <div className="flex items-center gap-2 justify-between pt-1">
        <button
          onClick={handlePrune}
          disabled={pruning || !bs.max_backups}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
            opacity: !bs.max_backups ? 0.5 : 1,
          }}
        >
          {pruning ? <RefreshCw size={11} className="animate-spin" /> : <Trash2 size={11} />}
          Prune now
        </button>
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: saved ? 'var(--color-success)' : 'var(--color-primary)',
            color: 'var(--color-background)',
            border: '1px solid transparent',
          }}
        >
          {saving ? <RefreshCw size={11} className="animate-spin" /> : saved ? <CheckCircle2 size={11} /> : null}
          {saved ? 'Saved' : 'Save'}
        </button>
      </div>
    </div>
  )
}

// ── App Data Transfer panel ────────────────────────────────────────────────────
//
// Exports user.db + every backup zip into a single .pqcb bundle for moving
// the entire app setup to another device. Import stages the bundle and
// schedules a restart — the actual file swap happens at next backend startup
// before any user.db connections open.

function AppTransferPanel(): React.ReactElement {
  const [busy, setBusy] = useState(false)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [importPreview, setImportPreview] = useState<{ path: string; manifest: AppBackupManifest } | null>(null)
  const [importStaged, setImportStaged] = useState(false)

  function reset() {
    setStatusMessage(null)
    setErrorMessage(null)
    setImportPreview(null)
    setImportStaged(false)
  }

  async function handleExport() {
    reset()
    if (!window.electron?.dialog?.saveExportBundle) {
      setErrorMessage('Export requires the desktop app — not available in browser preview.')
      return
    }
    const dest = await window.electron.dialog.saveExportBundle()
    if (!dest) return
    setBusy(true)
    try {
      const clientState = await collectClientState()
      const res = await exportAppBackup(dest, clientState)
      const soundNote = res.manifest.stats.sound_count > 0 ? `, ${res.manifest.stats.sound_count} custom sound(s)` : ''
      setStatusMessage(`Exported to ${res.bundle_path} (${formatBytes(res.manifest.stats.total_size_bytes)}, ${res.manifest.stats.backup_count} snapshot(s)${soundNote} included).`)
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function handleChooseImport() {
    reset()
    if (!window.electron?.dialog?.openImportBundle) {
      setErrorMessage('Import requires the desktop app — not available in browser preview.')
      return
    }
    const path = await window.electron.dialog.openImportBundle()
    if (!path) return
    setBusy(true)
    try {
      const res = await previewAppImport(path)
      setImportPreview({ path, manifest: res.manifest })
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  async function handleConfirmImport() {
    if (!importPreview) return
    setBusy(true)
    setErrorMessage(null)
    try {
      await stageAppImport(importPreview.path)
      setImportStaged(true)
      // The import is prepared; it only takes effect on the next launch, when
      // the backend swaps the files in before opening the database. Relaunch
      // immediately so the user gets one obvious outcome instead of being left
      // in a "staged — now what?" limbo (the original source of confusion). In
      // the browser preview there's no relaunch bridge, so the staged panel
      // stays up with a manual restart button as a fallback.
      if (window.electron?.app?.relaunch) {
        await window.electron.app.relaunch()
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : String(err))
      setImportStaged(false)
    } finally {
      setBusy(false)
    }
  }

  async function handleRestart() {
    if (!window.electron?.app?.relaunch) {
      setErrorMessage('Restart requires the desktop app.')
      return
    }
    await window.electron.app.relaunch()
  }

  return (
    <div
      className="rounded-lg border-2 p-4"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-primary)' }}
    >
      <div className="flex items-center gap-2 mb-1">
        <Download size={16} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          App Backup &amp; Restore
        </span>
        <span
          className="text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase tracking-wider"
          style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
        >
          Move to new device
        </span>
      </div>
      <p className="text-[11px] mb-3" style={{ color: 'var(--color-muted-foreground)' }}>
        Export puts your whole PQ Companion setup in one <code className="font-mono">.pqcb</code> file: characters, triggers, blocked buffs, combat history, AAs, tasks, key tracker, every setting, the overlay layout and window positions, custom trigger sounds, plus every snapshot and character-file version. Import it on the other computer after installing the app to get everything back as it was. It doesn't include your EverQuest folder itself; copy that separately, or restore your characters' files from the snapshots and history it carries.
      </p>

      {/* Export */}
      <div className="flex items-center gap-2 mb-3">
        <button
          onClick={handleExport}
          disabled={busy || importStaged}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium disabled:opacity-50"
          style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
        >
          <Download size={11} />
          Export app data
        </button>
        <button
          onClick={handleChooseImport}
          disabled={busy || importStaged}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded disabled:opacity-50"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          <Upload size={11} />
          Import from bundle…
        </button>
      </div>

      {/* Preview / confirm */}
      {importPreview && !importStaged && (
        <div
          className="rounded border p-3 mb-2 text-[11px]"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
        >
          <p className="font-semibold mb-1" style={{ color: 'var(--color-foreground)' }}>
            Confirm import
          </p>
          <p>
            Bundle exported {importPreview.manifest.exported_at} by app version {importPreview.manifest.app_version}.
          </p>
          <p>
            Contains {importPreview.manifest.stats.backup_count} snapshot(s)
            {importPreview.manifest.stats.sound_count > 0 ? `, ${importPreview.manifest.stats.sound_count} custom sound(s)` : ''}
            {importPreview.manifest.stats.config_included ? ', settings' : ''}
            {importPreview.manifest.stats.client_state_included ? ', overlay/window layout' : ''}
            , {formatBytes(importPreview.manifest.stats.total_size_bytes)} total.
          </p>
          {!importPreview.manifest.stats.config_included && (
            <p className="mt-1" style={{ color: 'var(--color-muted-foreground)' }}>
              This bundle predates settings/layout transfer — only characters, triggers, and history will be restored.
            </p>
          )}
          <p className="mt-2" style={{ color: 'var(--color-danger)' }}>
            This replaces your current app data and restarts the app to apply it. Your existing data is renamed aside (with a <code className="font-mono">.preimport</code> suffix) for recovery, not deleted.
          </p>
          <div className="flex items-center gap-2 mt-2">
            <button
              onClick={handleConfirmImport}
              disabled={busy}
              className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium disabled:opacity-50"
              style={{ backgroundColor: 'var(--color-danger)', color: 'var(--color-background)' }}
            >
              {busy ? <RefreshCw size={11} className="animate-spin" /> : <Upload size={11} />}
              {busy ? 'Importing…' : 'Import & restart'}
            </button>
            <button
              onClick={() => setImportPreview(null)}
              disabled={busy}
              className="text-xs px-2 py-1 rounded"
              style={{ backgroundColor: 'transparent', color: 'var(--color-muted-foreground)' }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Staged — needs restart */}
      {importStaged && (
        <div
          className="rounded border p-3 text-[11px]"
          style={{ borderColor: 'var(--color-primary)', backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
        >
          <p className="font-semibold mb-1 flex items-center gap-1.5" style={{ color: 'var(--color-foreground)' }}>
            <CheckCircle2 size={13} style={{ color: 'var(--color-success)' }} />
            Import ready — restart to finish
          </p>
          <p className="mb-2">
            Your bundle is prepared. Restart the app to apply it — the swap happens before any database connection opens, so it's safe. (The app normally restarts on its own; use this if it hasn't.)
          </p>
          <button
            onClick={handleRestart}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
          >
            Restart now
          </button>
        </div>
      )}

      {/* Status / error */}
      {statusMessage && (
        <p className="text-[11px] mt-2" style={{ color: 'var(--color-success)' }}>
          {statusMessage}
        </p>
      )}
      {errorMessage && (
        <p className="text-[11px] mt-2" style={{ color: 'var(--color-danger)' }}>
          {errorMessage}
        </p>
      )}
    </div>
  )
}

// ── Reset panel ────────────────────────────────────────────────────────────────
//
// Two destructive "start over" actions. Both stage a reset and restart — the
// wipe runs at next startup before the database is opened, and the old data is
// moved aside with a .prereset suffix for recovery (never hard-deleted here).
//
//  • Reset app data — clears characters, chat, loot, triggers, tasks, etc. but
//    keeps settings and your EQ folder path (no re-onboarding).
//  • Factory reset — also clears settings, reopening the onboarding wizard.

const RESET_MODES: Record<AppResetMode, {
  title: string
  blurb: string
  confirmWord: string
}> = {
  data: {
    title: 'Reset app data',
    blurb: 'Erases all tracked data — characters, chat history, loot, triggers, tasks, wishlists, notes, flags, and combat history — but keeps your settings and EQ folder path. Default trigger packs are restored fresh.',
    confirmWord: 'RESET',
  },
  factory: {
    title: 'Factory reset',
    blurb: 'Erases everything, including your settings and EQ folder path. The app reopens to the first-time setup wizard, exactly like a fresh install.',
    confirmWord: 'RESET',
  },
}

function ResetPanel(): React.ReactElement {
  const [mode, setMode] = useState<AppResetMode | null>(null)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  async function handleConfirm(m: AppResetMode) {
    setErrorMessage(null)
    try {
      await resetApp(m)
      // The wipe applies at next launch. Relaunch straight away so the user
      // sees the blank slate immediately rather than a "pending" limbo.
      if (window.electron?.app?.relaunch) {
        await window.electron.app.relaunch()
      } else {
        setErrorMessage('Reset prepared — restart the app to apply it.')
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : String(err))
    } finally {
      setMode(null)
    }
  }

  return (
    <div
      className="rounded-lg border p-4"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-danger)' }}
    >
      <div className="flex items-center gap-2 mb-1">
        <AlertTriangle size={16} style={{ color: 'var(--color-danger)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Reset
        </span>
        <span
          className="text-[10px] px-1.5 py-0.5 rounded font-semibold uppercase tracking-wider"
          style={{ backgroundColor: 'var(--color-danger)', color: 'var(--color-background)' }}
        >
          Danger zone
        </span>
      </div>
      <p className="text-[11px] mb-3" style={{ color: 'var(--color-muted-foreground)' }}>
        Start over with a clean slate. Your existing data is moved aside (with a <code className="font-mono">.prereset</code> suffix in <code className="font-mono">~/.pq-companion</code>) for recovery — not permanently deleted — but the app treats it as gone. Consider exporting a backup above first.
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <button
          onClick={() => { setErrorMessage(null); setMode('data') }}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-danger)', border: '1px solid var(--color-danger)' }}
        >
          <RotateCcw size={11} />
          Reset app data
        </button>
        <button
          onClick={() => { setErrorMessage(null); setMode('factory') }}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{ backgroundColor: 'var(--color-danger)', color: 'var(--color-background)' }}
        >
          <Trash2 size={11} />
          Factory reset
        </button>
      </div>

      {errorMessage && (
        <p className="text-[11px] mt-2" style={{ color: 'var(--color-danger)' }}>
          {errorMessage}
        </p>
      )}

      {mode && (
        <ResetConfirmModal
          mode={mode}
          onCancel={() => setMode(null)}
          onConfirm={() => handleConfirm(mode)}
        />
      )}
    </div>
  )
}

// Type-to-confirm modal: the destructive button stays disabled until the user
// types the confirmation word, so a reset can't be triggered by a stray click.
function ResetConfirmModal({
  mode, onCancel, onConfirm,
}: {
  mode: AppResetMode
  onCancel: () => void
  onConfirm: () => void
}): React.ReactElement {
  const [typed, setTyped] = useState('')
  const info = RESET_MODES[mode]
  const armed = typed.trim().toUpperCase() === info.confirmWord
  useEscapeToClose(onCancel)

  return (
    <div
      onClick={onCancel}
      style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-danger)', width: '100%', maxWidth: 460 }}
      >
        <div className="flex items-center gap-2">
          <AlertTriangle size={16} style={{ color: 'var(--color-danger)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{info.title}?</p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          {info.blurb}
        </p>
        <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          Type <code className="font-mono font-semibold" style={{ color: 'var(--color-danger)' }}>{info.confirmWord}</code> to confirm. The app restarts immediately.
        </p>
        <input
          autoFocus
          type="text"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && armed) onConfirm() }}
          placeholder={info.confirmWord}
          className="w-full text-sm rounded px-2 py-1.5 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
        />
        <div className="flex justify-end gap-2 pt-1">
          <button
            onClick={onCancel}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={!armed}
            className="text-xs px-3 py-1.5 rounded font-medium disabled:opacity-40 disabled:cursor-not-allowed"
            style={{ backgroundColor: 'var(--color-danger)', color: '#fff', border: '1px solid transparent' }}
          >
            {info.title}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

type BackupTab = 'files' | 'snapshots' | 'transfer' | 'reset'
const TAB_KEY = 'backups.tab'

const TABS: Array<{ id: BackupTab; label: string; icon: React.ReactNode; blurb: string }> = [
  {
    id: 'files',
    label: 'Character files',
    icon: <History size={14} />,
    blurb: 'Spellsets, bandolier, hotkeys and macros, UI layout. A version is saved automatically whenever one changes; restore any file to an earlier one.',
  },
  {
    id: 'snapshots',
    label: 'Snapshots',
    icon: <Archive size={14} />,
    blurb: 'A copy of every EQ settings file at one moment, and your logs if you choose. Make one before big changes; restore all or some files.',
  },
  {
    id: 'transfer',
    label: 'Move to another PC',
    icon: <Laptop size={14} />,
    blurb: 'Everything in PQ Companion, including blocked buffs, triggers, settings and all of the backups here, in one file.',
  },
  {
    id: 'reset',
    label: 'Reset',
    icon: <AlertTriangle size={14} />,
    blurb: 'Start over: clear app data, or everything including settings.',
  },
]

function readTab(): BackupTab {
  try {
    const t = localStorage.getItem(TAB_KEY)
    if (t && TABS.some((x) => x.id === t)) return t as BackupTab
  } catch {
    // Fall through to the default.
  }
  return 'files'
}

export default function BackupManagerPage(): React.ReactElement {
  const [tab, setTabState] = useState<BackupTab>(readTab)
  const navigate = useNavigate()
  const setTab = (t: BackupTab) => {
    setTabState(t)
    try {
      localStorage.setItem(TAB_KEY, t)
    } catch {
      // Remembering the tab is a convenience only.
    }
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <HardDrive size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Backup &amp; Restore
        </span>
        <button
          onClick={() => window.electron?.shell?.openBackupsFolder().catch(() => null)}
          title="Open the snapshots folder in your file manager"
          className="ml-auto flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-muted-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          <FolderOpen size={11} />
          Open folder
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-3">
        {/* What's protected: one tile per tab */}
        <div className="grid grid-cols-2 gap-2 xl:grid-cols-4">
          {TABS.map((t) => {
            const on = t.id === tab
            return (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="rounded-lg p-3 text-left"
                style={{
                  backgroundColor: on ? 'var(--color-surface-2)' : 'var(--color-surface)',
                  border: `1px solid ${on ? 'var(--color-primary)' : 'var(--color-border)'}`,
                }}
              >
                <span
                  className="flex items-center gap-1.5 text-xs font-semibold"
                  style={{ color: on ? 'var(--color-primary)' : 'var(--color-foreground)' }}
                >
                  {t.icon}
                  {t.label}
                </span>
                <span className="mt-1 block text-[11px] leading-snug" style={{ color: 'var(--color-muted-foreground)' }}>
                  {t.blurb}
                </span>
              </button>
            )
          })}
        </div>

        <p className="flex items-start gap-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
          <FileText size={12} className="shrink-0 mt-0.5" />
          <span>
            Log files: archive and trim them in{' '}
            <button
              className="underline"
              style={{ color: 'var(--color-primary)' }}
              onClick={() => navigate('/settings?tab=logs')}
            >
              Settings → Logs
            </button>
            . Archives stay next to the log, and Log Backfill reads them. To keep a copy of the
            logs themselves, make a snapshot with “Include log files”.
          </span>
        </p>

        {tab === 'files' && <FileHistoryPanel />}
        {tab === 'snapshots' && <SnapshotsPanel />}
        {tab === 'transfer' && <AppTransferPanel />}
        {tab === 'reset' && <ResetPanel />}
      </div>
    </div>
  )
}

function SnapshotsPanel(): React.ReactElement {
  const [backups, setBackups] = useState<Backup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [showSettings, setShowSettings] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listBackups()
      .then((resp) => setBackups(resp.backups))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleCreated = (b: Backup) => {
    setBackups((prev) => [b, ...prev])
    setShowCreate(false)
  }

  const handleDeleted = (id: string) => {
    setBackups((prev) => prev.filter((b) => b.id !== id))
  }

  const handleLockToggled = (id: string, locked: boolean) => {
    setBackups((prev) => prev.map((b) => b.id === id ? { ...b, locked } : b))
  }

  const toolBtn = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-muted-foreground)',
    border: '1px solid var(--color-border)',
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <p className="flex-1 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          Lock a snapshot to keep it from automatic cleanup. PQ Companion also takes one
          before it changes an EQ file for you.
        </p>
        <button onClick={load} className="flex items-center gap-1.5 text-xs px-2 py-1 rounded" style={toolBtn}>
          <RefreshCw size={11} />
          Refresh
        </button>
        <button
          onClick={() => { setShowSettings((v) => !v); setShowCreate(false) }}
          title="Automatic snapshots and how many to keep"
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={showSettings
            ? { backgroundColor: 'var(--color-primary)', color: 'var(--color-background)', border: '1px solid transparent' }
            : toolBtn}
        >
          <Settings size={11} />
          Schedule
        </button>
        <button
          onClick={() => { setShowCreate((v) => !v); setShowSettings(false) }}
          className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium"
          style={{
            backgroundColor: showCreate ? 'var(--color-surface-2)' : 'var(--color-primary)',
            color: showCreate ? 'var(--color-muted-foreground)' : 'var(--color-background)',
            border: `1px solid ${showCreate ? 'var(--color-border)' : 'transparent'}`,
          }}
        >
          <Plus size={11} />
          New snapshot
        </button>
      </div>

      {showSettings && (
        <SettingsPanel
          onClose={() => setShowSettings(false)}
          onPruned={load}
        />
      )}

      {showCreate && (
        <CreateForm
          onCreated={handleCreated}
          onCancel={() => setShowCreate(false)}
        />
      )}

      {loading && (
        <RefreshCw size={16} className="animate-spin mx-auto" style={{ color: 'var(--color-muted)' }} />
      )}

      {error && (
        <div className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-danger)' }}>
          <AlertCircle size={13} />
          {error}
        </div>
      )}

      {!loading && !error && backups.length === 0 && !showCreate && (
        <div className="flex flex-col items-center gap-3 py-8">
          <Archive size={28} style={{ color: 'var(--color-muted)' }} />
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            No snapshots yet.
          </p>
          <button
            onClick={() => setShowCreate(true)}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded"
            style={{ ...toolBtn, color: 'var(--color-foreground)' }}
          >
            <Plus size={11} />
            Make your first snapshot
          </button>
        </div>
      )}

      {backups.map((b) => (
        <BackupCard
          key={b.id}
          backup={b}
          onDeleted={handleDeleted}
          onRestored={() => {/* success shown inline in card */}}
          onLockToggled={handleLockToggled}
        />
      ))}
    </div>
  )
}
