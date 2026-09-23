import React, { useEffect, useMemo, useRef, useState } from 'react'
import { X, ScrollText, CheckCircle2, AlertCircle, Lock, FileSearch, RefreshCw, Terminal } from 'lucide-react'
import {
  previewPopSeer, commitPopSeer, scanPopSeer,
  previewPopFlagsCmd, commitPopFlagsCmd, scanPopFlagsCmd,
} from '../services/api'
import type { PoPResolved, SeerDetected } from '../types/popflag'

interface ImportSeerModalProps {
  character: string
  onClose: () => void
  onCommitted: (resolved: PoPResolved) => void
}

type Mode = 'seer' | 'popflags'

// A source-agnostic preview: both the Seer reading and the '#popflags' report
// reduce to "which flags does this detect, and how many are new" — the two
// only differ in how they're scanned/pasted/committed, and popflags adds a
// list of pending checklist ('cl_*') names.
interface Preview {
  detected: SeerDetected[]
  newCount: number
  pending?: string[]
}

// ImportSeerModal turns an in-game progression reading into flag state. It
// supports two sources — the Seer Mal Nae`Shi "guided meditation" (the
// original path) and the '#popflags' command (EQMacEmu PR #382, added ahead
// of the PoP launch) — sharing the same scan/paste/preview/commit shell,
// since both ultimately answer "what does this reading detect."
//
// #popflags only ever covers the section the player ran (the overview, or one
// tier), so its commit MERGES onto the character's stored snapshot instead of
// replacing it — syncing '#popflags 1' through '#popflags 5' one at a time
// progressively fills in the full picture. It also has no per-flag conflict
// override (unlike the Seer path): a manual setting is always kept as-is.
export default function ImportSeerModal({
  character, onClose, onCommitted,
}: ImportSeerModalProps): React.ReactElement {
  const [mode, setMode] = useState<Mode>('seer')
  const [text, setText] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [scanMsg, setScanMsg] = useState<string | null>(null)
  // Flag IDs the user chose to accept FROM a Seer reading despite a prior
  // manual setting (resolving a conflict). Not used in popflags mode.
  const [accepted, setAccepted] = useState<Set<string>>(new Set())

  const resetForModeSwitch = (next: Mode): void => {
    setMode(next)
    setText('')
    setPreview(null)
    setError(null)
    setScanMsg(null)
    setAccepted(new Set())
  }

  const runPreview = (): void => {
    if (!text.trim()) return
    setBusy(true)
    setError(null)
    setAccepted(new Set())
    const req = mode === 'seer'
      ? previewPopSeer(character, text).then((p) => ({ detected: p.detected, newCount: p.new_count }))
      : previewPopFlagsCmd(character, text).then((p) => ({ detected: p.detected, newCount: p.new_count, pending: p.pending }))
    req.then(setPreview).catch((e: Error) => setError(e.message)).finally(() => setBusy(false))
  }

  const toggleAccept = (id: string): void => {
    setAccepted((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  // Read the character's EQ log for the most recent reading. On a hit we fill
  // the textarea and preview so the user can just commit; on a miss we point
  // them at the manual paste box below.
  const runScan = (): void => {
    setBusy(true)
    setError(null)
    setScanMsg(null)
    const notFoundMsg = mode === 'seer'
      ? `No Seer reading found in ${character}'s log. Do the in-game guided meditation (or paste it below).`
      : `No #popflags report found in ${character}'s log. Run #popflags (or #popflags 1-5) in game (or paste it below).`
    const req = mode === 'seer'
      ? scanPopSeer(character).then((resp) => {
        if (!resp.found || !resp.text) return null
        setText(resp.text)
        return { detected: resp.detected ?? [], newCount: resp.new_count ?? 0 }
      })
      : scanPopFlagsCmd(character).then((resp) => {
        if (!resp.found || !resp.text) return null
        setText(resp.text)
        return { detected: resp.detected ?? [], newCount: resp.new_count ?? 0, pending: resp.pending }
      })
    req
      .then((p) => {
        if (!p) { setScanMsg(notFoundMsg); return }
        setAccepted(new Set())
        setPreview(p)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  const runCommit = (): void => {
    setBusy(true)
    setError(null)
    const req = mode === 'seer'
      ? commitPopSeer(character, text, Array.from(accepted))
      : commitPopFlagsCmd(character, text)
    req
      .then((r) => { onCommitted(r); onClose() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  // Auto-scan once on open so the modal lands straight on the preview when a
  // reading is already in the log — a one-click "check the log" refresh. The
  // user still confirms before anything is written.
  const scannedOnce = useRef(false)
  useEffect(() => {
    if (scannedOnce.current) return
    scannedOnce.current = true
    runScan()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Split detections into new / already-have / manual-blocked buckets.
  const buckets = useMemo(() => {
    const fresh: SeerDetected[] = []
    const have: SeerDetected[] = []
    const blocked: SeerDetected[] = []
    for (const d of preview?.detected ?? []) {
      if (d.manual_blocked) blocked.push(d)
      else if (d.already_done) have.push(d)
      else fresh.push(d)
    }
    return { fresh, have, blocked }
  }, [preview])

  const sourceLabel = mode === 'seer' ? 'Seer reading' : '#popflags report'

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={() => !busy && onClose()}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-2xl flex-col rounded-lg overflow-hidden shadow-2xl"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="flex items-center gap-2 border-b px-4 py-3 shrink-0"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <ScrollText size={16} style={{ color: 'var(--color-primary)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Import {sourceLabel} — {character}
          </span>
          <button onClick={onClose} className="ml-auto" style={{ color: 'var(--color-muted)' }}>
            <X size={16} />
          </button>
        </div>

        {/* Mode switch */}
        <div className="flex gap-1 border-b px-4 pt-2 pb-2 shrink-0" style={{ borderColor: 'var(--color-border)' }}>
          <ModeTab
            active={mode === 'seer'}
            icon={<ScrollText size={12} />}
            label="Seer reading"
            onClick={() => busy || resetForModeSwitch('seer')}
          />
          <ModeTab
            active={mode === 'popflags'}
            icon={<Terminal size={12} />}
            label="#popflags command"
            onClick={() => busy || resetForModeSwitch('popflags')}
          />
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-3">
          {mode === 'seer' ? (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              In Plane of Knowledge, sit near Seer Mal Nae`Shi and say{' '}
              <code style={{ color: 'var(--color-primary)' }}>guided meditation</code>. The app can
              read the lines straight from {character}'s log — no copy-paste needed.
            </p>
          ) : (
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Type <code style={{ color: 'var(--color-primary)' }}>#popflags</code>,{' '}
              <code style={{ color: 'var(--color-primary)' }}>#popflags overview</code>, or{' '}
              <code style={{ color: 'var(--color-primary)' }}>#popflags 1</code> through{' '}
              <code style={{ color: 'var(--color-primary)' }}>#popflags 5</code> in game. One command
              only reports the section you ran — sync each tier separately to fill in the whole
              tracker.
            </p>
          )}

          {/* Primary path: scan the log file. */}
          <button
            onClick={runScan}
            disabled={busy}
            className="flex w-full items-center justify-center gap-2 rounded px-3 py-2 text-xs font-medium"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-background)',
              opacity: busy ? 0.6 : 1,
            }}
          >
            {busy ? <RefreshCw size={13} className="animate-spin" /> : <FileSearch size={13} />}
            Scan {character}'s log for the latest {mode === 'seer' ? 'reading' : 'report'}
          </button>
          {scanMsg && (
            <div className="flex items-start gap-2 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
              <AlertCircle size={12} className="mt-0.5 shrink-0" />
              {scanMsg}
            </div>
          )}

          {/* Fallback: manual paste (teammate's reading, a log from another PC). */}
          <div className="flex items-center gap-2 pt-1">
            <div className="h-px flex-1" style={{ backgroundColor: 'var(--color-border)' }} />
            <span className="text-[10px] uppercase tracking-wider" style={{ color: 'var(--color-muted)' }}>
              or paste manually
            </span>
            <div className="h-px flex-1" style={{ backgroundColor: 'var(--color-border)' }} />
          </div>
          <textarea
            value={text}
            onChange={(e) => { setText(e.target.value); setPreview(null); setScanMsg(null); setAccepted(new Set()) }}
            placeholder={mode === 'seer'
              ? "Paste the Seer's guided-meditation output here…"
              : 'Paste the #popflags output here…'}
            rows={8}
            className="w-full rounded px-2 py-1.5 text-xs font-mono"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              color: 'var(--color-foreground)',
            }}
          />

          {error && (
            <div className="flex items-center gap-2 text-xs" style={{ color: '#f87171' }}>
              <AlertCircle size={13} />
              {error}
            </div>
          )}

          {preview && (
            <div className="space-y-3">
              <p className="text-xs font-medium" style={{ color: 'var(--color-foreground)' }}>
                {preview.detected.length === 0
                  ? 'No flags detected — check the pasted text.'
                  : `Detected ${preview.detected.length} flag${preview.detected.length === 1 ? '' : 's'} · ${preview.newCount} new` +
                    (buckets.blocked.length > 0
                      ? ` · ${buckets.blocked.length} conflict${buckets.blocked.length === 1 ? '' : 's'}`
                      : '')}
              </p>
              <DetectGroup title="New" color="var(--color-success)" items={buckets.fresh} icon={<CheckCircle2 size={12} />} />
              <DetectGroup title="Already recorded" color="var(--color-muted)" items={buckets.have} icon={<CheckCircle2 size={12} />} />

              {preview.pending && preview.pending.length > 0 && (
                <div
                  className="rounded px-2 py-1.5 text-[11px]"
                  style={{ backgroundColor: 'rgba(245,158,11,0.10)', color: '#f59e0b' }}
                >
                  Pending checklist {preview.pending.length === 1 ? 'memory' : 'memories'} named:{' '}
                  {preview.pending.map((p) => p.replace(/^cl_/, '')).join(', ')}. Sit near Seer Mal
                  Nae`Shi and say 'unlock memories' in game, then re-sync.
                </div>
              )}

              {/* Seer-only: conflicts with a prior manual change, resolvable
                  per-flag. The popflags path has no override — a manual
                  setting always wins, shown here informationally. */}
              {buckets.blocked.length > 0 && (
                <div>
                  <p className="mb-1 flex items-center gap-1 text-[10px] font-semibold uppercase tracking-wider" style={{ color: '#f59e0b' }}>
                    <Lock size={11} />
                    Conflicts with your manual changes ({buckets.blocked.length})
                  </p>
                  <p className="mb-1.5 text-[10px]" style={{ color: 'var(--color-muted)' }}>
                    {mode === 'seer'
                      ? 'You set these by hand, so the reading is kept out by default. Tick one to let the reading override your manual setting.'
                      : "You set these by hand — a #popflags sync never overrides a manual change."}
                  </p>
                  <div className="space-y-0.5">
                    {buckets.blocked.map((d) => {
                      const acc = accepted.has(d.id)
                      return (
                        <label
                          key={d.id}
                          className="flex items-center gap-2 rounded px-1.5 py-1 text-xs"
                          style={{
                            color: 'var(--color-foreground)',
                            backgroundColor: acc ? 'rgba(52,211,153,0.10)' : 'var(--color-surface-2)',
                            border: `1px solid ${acc ? 'var(--color-success)' : 'var(--color-border)'}`,
                            cursor: mode === 'seer' ? 'pointer' : 'default',
                          }}
                        >
                          {mode === 'seer' && (
                            <input
                              type="checkbox"
                              checked={acc}
                              onChange={() => toggleAccept(d.id)}
                              className="shrink-0"
                              style={{ accentColor: 'var(--color-success)' }}
                            />
                          )}
                          <span className="shrink-0" style={{ color: acc ? 'var(--color-success)' : '#f59e0b' }}>
                            {acc ? <CheckCircle2 size={12} /> : <Lock size={12} />}
                          </span>
                          <span className="flex-1 truncate">{d.label}</span>
                          {mode === 'seer' && (
                            <span
                              className="shrink-0 text-[9px] uppercase tracking-wider"
                              style={{ color: acc ? 'var(--color-success)' : 'var(--color-muted)' }}
                            >
                              {acc ? 'accept reading' : 'kept as-is'}
                            </span>
                          )}
                          <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>{d.zone}</span>
                        </label>
                      )
                    })}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-end gap-2 border-t px-4 py-3 shrink-0"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <button
            onClick={onClose}
            className="rounded px-3 py-1.5 text-xs"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            Cancel
          </button>
          {!preview ? (
            <button
              onClick={runPreview}
              disabled={busy || !text.trim()}
              className="rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: 'var(--color-background)',
                opacity: busy || !text.trim() ? 0.6 : 1,
              }}
            >
              Preview
            </button>
          ) : (
            <button
              onClick={runCommit}
              disabled={busy || preview.detected.length === 0}
              className="rounded px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: 'var(--color-background)',
                opacity: busy || preview.detected.length === 0 ? 0.6 : 1,
              }}
            >
              Commit {mode === 'seer' ? 'reading' : 'report'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

interface ModeTabProps {
  active: boolean
  icon: React.ReactNode
  label: string
  onClick: () => void
}

function ModeTab({ active, icon, label, onClick }: ModeTabProps): React.ReactElement {
  return (
    <button
      onClick={onClick}
      className="flex items-center gap-1.5 rounded px-2.5 py-1 text-[11px] font-medium"
      style={{
        backgroundColor: active ? 'var(--color-surface-2)' : 'transparent',
        color: active ? 'var(--color-foreground)' : 'var(--color-muted-foreground)',
        border: `1px solid ${active ? 'var(--color-border)' : 'transparent'}`,
      }}
    >
      {icon}
      {label}
    </button>
  )
}

interface DetectGroupProps {
  title: string
  color: string
  items: SeerDetected[]
  icon: React.ReactNode
}

function DetectGroup({ title, color, items, icon }: DetectGroupProps): React.ReactElement | null {
  if (items.length === 0) return null
  return (
    <div>
      <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider" style={{ color }}>
        {title} ({items.length})
      </p>
      <div className="space-y-0.5">
        {items.map((d) => (
          <div key={d.id} className="flex items-center gap-2 text-xs" style={{ color: 'var(--color-foreground)' }}>
            <span style={{ color }}>{icon}</span>
            <span className="flex-1 truncate">{d.label}</span>
            <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted)' }}>{d.zone}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
