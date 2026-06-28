import React, { useMemo, useState } from 'react'
import { X, ScrollText, CheckCircle2, AlertCircle, Lock } from 'lucide-react'
import { previewPopSeer, commitPopSeer } from '../services/api'
import type { PoPResolved, SeerPreviewResponse, SeerDetected } from '../types/popflag'

interface ImportSeerModalProps {
  character: string
  onClose: () => void
  onCommitted: (resolved: PoPResolved) => void
}

// ImportSeerModal pastes the Seer Mal Nae`Shi "guided meditation" output and
// previews which PoP flags it detects before committing them as seer-sourced
// state (manual toggles are preserved by the backend).
export default function ImportSeerModal({
  character, onClose, onCommitted,
}: ImportSeerModalProps): React.ReactElement {
  const [text, setText] = useState('')
  const [preview, setPreview] = useState<SeerPreviewResponse | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const runPreview = (): void => {
    if (!text.trim()) return
    setBusy(true)
    setError(null)
    previewPopSeer(character, text)
      .then((p) => setPreview(p))
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  const runCommit = (): void => {
    setBusy(true)
    setError(null)
    commitPopSeer(character, text)
      .then((r) => { onCommitted(r); onClose() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

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

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      style={{ backgroundColor: 'rgba(0,0,0,0.5)' }}
      onClick={onClose}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-2xl flex-col rounded-lg overflow-hidden"
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
            Import Seer reading — {character}
          </span>
          <button onClick={onClose} className="ml-auto" style={{ color: 'var(--color-muted)' }}>
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-3">
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            In Plane of Knowledge, sit near Seer Mal Nae`Shi and say{' '}
            <code style={{ color: 'var(--color-primary)' }}>guided meditation</code>. Copy the
            lines she prints into your chat and paste them below.
          </p>
          <textarea
            value={text}
            onChange={(e) => { setText(e.target.value); setPreview(null) }}
            placeholder="Paste the Seer's guided-meditation output here…"
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
                  : `Detected ${preview.detected.length} flag${preview.detected.length === 1 ? '' : 's'} · ${preview.new_count} new`}
              </p>
              <DetectGroup title="New" color="var(--color-success)" items={buckets.fresh} icon={<CheckCircle2 size={12} />} />
              <DetectGroup title="Already recorded" color="var(--color-muted)" items={buckets.have} icon={<CheckCircle2 size={12} />} />
              <DetectGroup
                title="Blocked by manual override"
                color="#f59e0b"
                items={buckets.blocked}
                icon={<Lock size={12} />}
                note="You previously set these manually; the reading won't change them."
              />
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
              Commit reading
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

interface DetectGroupProps {
  title: string
  color: string
  items: SeerDetected[]
  icon: React.ReactNode
  note?: string
}

function DetectGroup({ title, color, items, icon, note }: DetectGroupProps): React.ReactElement | null {
  if (items.length === 0) return null
  return (
    <div>
      <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider" style={{ color }}>
        {title} ({items.length})
      </p>
      {note && <p className="mb-1 text-[10px]" style={{ color: 'var(--color-muted)' }}>{note}</p>}
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
