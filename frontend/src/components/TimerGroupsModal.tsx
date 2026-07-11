import React, { useEffect, useState } from 'react'
import { X, Plus, Pencil, Trash2, ExternalLink, Check, Hourglass, Crosshair } from 'lucide-react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { ConfirmModal } from './ConfirmModal'
import { createTimerGroup, renameTimerGroup, deleteTimerGroup } from '../services/api'
import type { TimerGroup } from '../types/trigger'

// Same bounds/lock persistence key the main process uses for a named group's
// window (see electron/main/index.ts customTimerWindowKey) — shared here so
// reset-position/move-to-display target the right window.
function groupWindowKey(groupId: string): string {
  return `customTimer:${groupId}`
}

interface TimerGroupsModalProps {
  groups: TimerGroup[]
  onChanged: () => void
  onClose: () => void
}

// Compact icon button matching OverlaysDashboard's RowIconButton styling.
function RowIconButton({
  onClick,
  title,
  danger,
  children,
}: {
  onClick: () => void
  title: string
  danger?: boolean
  children: React.ReactNode
}): React.ReactElement {
  return (
    <button
      onClick={onClick}
      title={title}
      className="flex items-center justify-center rounded"
      style={{
        width: 26,
        height: 24,
        backgroundColor: 'var(--color-surface)',
        color: danger ? 'var(--color-destructive)' : 'var(--color-muted-foreground)',
        border: '1px solid var(--color-border)',
        cursor: 'pointer',
      }}
    >
      {children}
    </button>
  )
}

/**
 * TimerGroupsModal — dedicated management screen for named Custom Timers
 * windows (see backend trigger.TimerGroup). Create/rename/delete groups and
 * pop each one's Electron overlay window open/closed. Trigger assignment
 * happens in the trigger editor's "Custom Timers window" picker, not here.
 */
export default function TimerGroupsModal({ groups, onChanged, onClose }: TimerGroupsModalProps): React.ReactElement {
  useEscapeToClose(onClose)
  const [newName, setNewName] = useState('')
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [renamingID, setRenamingID] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [deletingGroup, setDeletingGroup] = useState<TimerGroup | null>(null)
  const [busyID, setBusyID] = useState<string | null>(null)

  // Which monitor each group's window currently lives on (open or closed —
  // see the main process's overlay:display-ids), mirroring Settings' Overlay
  // Lock Behaviour card. Only shown when more than one display is present.
  const [displays, setDisplays] = useState<Array<{ id: number; label: string; isPrimary: boolean }>>([])
  const [windowDisplays, setWindowDisplays] = useState<Record<string, number>>({})
  useEffect(() => {
    let cancelled = false
    window.electron?.screen?.listDisplays?.()
      .then((list) => { if (!cancelled) setDisplays(list ?? []) })
      .catch(() => {})
    const poll = (): void => {
      window.electron?.overlay?.displayIds?.()
        .then((m) => { if (!cancelled && m) setWindowDisplays(m) })
        .catch(() => {})
    }
    poll()
    const id = setInterval(poll, 1500)
    return () => { cancelled = true; clearInterval(id) }
  }, [])
  const multiMonitor = displays.length > 1

  const handleAdd = (): void => {
    const trimmed = newName.trim()
    if (!trimmed) return
    setAdding(true)
    setError(null)
    createTimerGroup(trimmed)
      .then(() => {
        setNewName('')
        onChanged()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setAdding(false))
  }

  const startRename = (g: TimerGroup): void => {
    setRenamingID(g.id)
    setRenameValue(g.name)
    setError(null)
  }

  const commitRename = (g: TimerGroup): void => {
    const trimmed = renameValue.trim()
    if (!trimmed || trimmed === g.name) {
      setRenamingID(null)
      return
    }
    setBusyID(g.id)
    renameTimerGroup(g.id, trimmed)
      .then(() => {
        setRenamingID(null)
        onChanged()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusyID(null))
  }

  const handleDelete = (g: TimerGroup): void => {
    setBusyID(g.id)
    deleteTimerGroup(g.id)
      .then(() => {
        setDeletingGroup(null)
        onChanged()
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setBusyID(null))
  }

  const popOutDefault = (): void => { window.electron?.overlay?.toggleCustomTimer() }
  const popOutGroup = (g: TimerGroup): void => { window.electron?.overlay?.toggleCustomTimerGroup(g.id, g.name) }
  const resetGroupPosition = (g: TimerGroup): void => { window.electron?.overlay?.resetPosition?.(groupWindowKey(g.id)) }
  const moveGroupToDisplay = (g: TimerGroup, displayId: number): void => {
    window.electron?.overlay?.moveToDisplay?.(groupWindowKey(g.id), displayId)
    setWindowDisplays((m) => ({ ...m, [groupWindowKey(g.id)]: displayId })) // optimistic; poll reconciles
  }

  return (
    <>
      <div
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          backgroundColor: 'rgba(0,0,0,0.6)',
          zIndex: 1100,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 16,
        }}
      >
        <div
          onClick={(e) => e.stopPropagation()}
          className="rounded-lg"
          style={{
            backgroundColor: 'var(--color-surface)',
            border: '1px solid var(--color-primary)',
            width: '100%',
            maxWidth: 480,
            maxHeight: '80vh',
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <div className="flex items-center gap-2 px-4 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
            <Hourglass size={16} style={{ color: 'var(--color-primary)' }} />
            <span className="text-sm font-semibold flex-1" style={{ color: 'var(--color-foreground)' }}>
              Timer Groups
            </span>
            <button onClick={onClose} style={{ color: 'var(--color-muted-foreground)' }}>
              <X size={16} />
            </button>
          </div>

          <div className="px-4 py-3 overflow-y-auto space-y-2">
            <p className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
              Split raid/boss triggers into their own popout Custom Timers window, separate
              from the chaos of everything else. Assign a trigger to a group from its editor's
              "Custom Timers window" field.
            </p>

            {/* Default window — always exists, not renamable/deletable here. */}
            <div
              className="flex items-center gap-2 rounded px-2.5 py-2"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
            >
              <span className="flex-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
                Custom Timers <span style={{ color: 'var(--color-muted)' }}>(default window)</span>
              </span>
              <button
                onClick={popOutDefault}
                title="Open/close this window"
                className="flex items-center gap-1 text-[11px] px-2 py-1 rounded"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  color: 'var(--color-muted-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <ExternalLink size={11} /> Pop Out
              </button>
            </div>

            {groups.map((g) => (
              <div
                key={g.id}
                className="flex items-center gap-2 rounded px-2.5 py-2"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
              >
                {renamingID === g.id ? (
                  <>
                    <input
                      autoFocus
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') { e.preventDefault(); commitRename(g) }
                        else if (e.key === 'Escape') { e.preventDefault(); setRenamingID(null) }
                      }}
                      className="flex-1 rounded px-2 py-1 text-sm outline-none"
                      style={{
                        backgroundColor: 'var(--color-surface)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                      disabled={busyID === g.id}
                    />
                    <RowIconButton onClick={() => commitRename(g)} title="Save">
                      <Check size={12} />
                    </RowIconButton>
                    <RowIconButton onClick={() => setRenamingID(null)} title="Cancel">
                      <X size={12} />
                    </RowIconButton>
                  </>
                ) : (
                  <>
                    <span className="flex-1 text-sm truncate" style={{ color: 'var(--color-foreground)' }}>
                      {g.name}
                    </span>
                    <span className="text-[10px] shrink-0" style={{ color: 'var(--color-muted)' }}>
                      {g.count} trigger{g.count === 1 ? '' : 's'}
                    </span>
                    {multiMonitor && (
                      <select
                        value={windowDisplays[groupWindowKey(g.id)] ?? ''}
                        onChange={(e) => moveGroupToDisplay(g, Number(e.target.value))}
                        className="rounded px-1 py-1 text-[11px] outline-none"
                        style={{
                          maxWidth: 88,
                          backgroundColor: 'var(--color-surface)',
                          border: '1px solid var(--color-border)',
                          color: 'var(--color-foreground)',
                          cursor: 'pointer',
                        }}
                        title={`Send ${g.name} to a specific monitor`}
                      >
                        {displays.map((d, i) => (
                          <option key={d.id} value={d.id}>
                            {d.label || `Display ${i + 1}`}{d.isPrimary ? ' (primary)' : ''}
                          </option>
                        ))}
                      </select>
                    )}
                    <RowIconButton onClick={() => resetGroupPosition(g)} title="Reset position — recenter on the primary monitor and unlock">
                      <Crosshair size={12} />
                    </RowIconButton>
                    <RowIconButton onClick={() => popOutGroup(g)} title="Open/close this window">
                      <ExternalLink size={12} />
                    </RowIconButton>
                    <RowIconButton onClick={() => startRename(g)} title="Rename">
                      <Pencil size={12} />
                    </RowIconButton>
                    <RowIconButton onClick={() => setDeletingGroup(g)} title="Delete" danger>
                      <Trash2 size={12} />
                    </RowIconButton>
                  </>
                )}
              </div>
            ))}

            <div className="flex gap-2 pt-1">
              <input
                type="text"
                placeholder="New window name (e.g. Raid Timers)"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); handleAdd() } }}
                className="flex-1 rounded px-3 py-1.5 text-sm outline-none"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
                disabled={adding}
              />
              <button
                onClick={handleAdd}
                disabled={adding || !newName.trim()}
                className="flex items-center gap-1 rounded px-3 py-1.5 text-xs font-semibold"
                style={{
                  backgroundColor: 'var(--color-primary)',
                  color: 'var(--color-background)',
                  border: '1px solid transparent',
                  cursor: 'pointer',
                }}
              >
                <Plus size={12} /> Add
              </button>
            </div>
            {error && <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{error}</p>}
          </div>

          <div className="flex justify-end gap-2 px-4 py-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
            <button
              onClick={onClose}
              className="px-3 py-1.5 text-sm rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              Close
            </button>
          </div>
        </div>
      </div>

      {deletingGroup && (
        <ConfirmModal
          title="Delete timer group"
          message={
            <>
              Delete <strong>{deletingGroup.name}</strong>? Its {deletingGroup.count} trigger
              {deletingGroup.count === 1 ? '' : 's'} will move back to the default Custom Timers
              window — nothing is deleted.
            </>
          }
          confirmLabel="Delete"
          tone="danger"
          onConfirm={() => handleDelete(deletingGroup)}
          onCancel={() => setDeletingGroup(null)}
        />
      )}
    </>
  )
}
