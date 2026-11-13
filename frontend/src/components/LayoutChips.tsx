/**
 * LayoutChips — one chip per overlay layout. Clicking a chip switches to it
 * (its windows open and move into place, the rest close). The active chip
 * can be renamed or deleted; "New" copies the active layout under a new name.
 * Changes to the active layout save themselves, so there's no Save button.
 */
import React, { useState } from 'react'
import { Check, Pencil, Plus, Trash2, X } from 'lucide-react'
import { useOverlayLayouts } from '../hooks/useOverlayLayouts'

const MAX_NAME = 40

function NameInput({
  initial,
  placeholder,
  onSubmit,
  onCancel,
}: {
  initial: string
  placeholder: string
  onSubmit: (name: string) => void
  onCancel: () => void
}): React.ReactElement {
  const [value, setValue] = useState(initial)
  const submit = (): void => {
    const name = value.trim()
    if (name) onSubmit(name)
    else onCancel()
  }
  return (
    <span className="flex items-center gap-1">
      <input
        autoFocus
        value={value}
        maxLength={MAX_NAME}
        placeholder={placeholder}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') submit()
          else if (e.key === 'Escape') onCancel()
        }}
        className="text-xs px-1.5 py-0.5 rounded w-28 outline-none"
        style={{
          backgroundColor: 'var(--color-background)',
          color: 'var(--color-foreground)',
          border: '1px solid var(--color-primary)',
        }}
      />
      <button onClick={submit} title="Save" style={{ color: 'var(--color-primary)' }}>
        <Check size={12} />
      </button>
      <button onClick={onCancel} title="Cancel" style={{ color: 'var(--color-muted)' }}>
        <X size={12} />
      </button>
    </span>
  )
}

export default function LayoutChips(): React.ReactElement | null {
  const { state, supported, switchTo, create, rename, remove } = useOverlayLayouts()
  const [editing, setEditing] = useState<'new' | 'rename' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  if (!supported || !state) return null
  const active = state.layouts.find((l) => l.id === state.activeId)

  return (
    <div className="flex items-center gap-1.5 min-w-0 overflow-x-auto">
      <span className="text-[10px] uppercase tracking-wide shrink-0" style={{ color: 'var(--color-muted)' }}>
        Layout
      </span>
      {state.layouts.map((l) => {
        const isActive = l.id === state.activeId
        if (isActive && editing === 'rename') {
          return (
            <NameInput
              key={l.id}
              initial={l.name}
              placeholder="Layout name"
              onSubmit={(name) => { rename(l.id, name); setEditing(null) }}
              onCancel={() => setEditing(null)}
            />
          )
        }
        return (
          <button
            key={l.id}
            onClick={() => { if (!isActive) { switchTo(l.id); setConfirmDelete(false) } }}
            className="text-xs px-2 py-0.5 rounded-full shrink-0 whitespace-nowrap"
            style={{
              backgroundColor: isActive ? 'var(--color-primary)' : 'var(--color-surface)',
              color: isActive ? 'var(--color-background)' : 'var(--color-foreground)',
              border: `1px solid ${isActive ? 'var(--color-primary)' : 'var(--color-border)'}`,
              cursor: isActive ? 'default' : 'pointer',
              fontWeight: isActive ? 600 : 400,
            }}
            title={
              isActive
                ? `${l.name} (active) · ${l.windows.length} window${l.windows.length === 1 ? '' : 's'} · changes save automatically`
                : `Switch to ${l.name} · ${l.windows.length} window${l.windows.length === 1 ? '' : 's'}`
            }
          >
            {l.name}
          </button>
        )
      })}
      {active && editing === null && (
        <>
          <button
            onClick={() => setEditing('rename')}
            title={`Rename ${active.name}`}
            className="shrink-0"
            style={{ color: 'var(--color-muted)' }}
          >
            <Pencil size={11} />
          </button>
          {state.layouts.length > 1 &&
            (confirmDelete ? (
              <button
                onClick={() => { remove(active.id); setConfirmDelete(false) }}
                onBlur={() => setConfirmDelete(false)}
                className="text-[10px] px-1.5 py-0.5 rounded shrink-0"
                style={{ color: 'var(--color-danger)', border: '1px solid var(--color-danger)' }}
                title="Click again to delete this layout"
              >
                Delete {active.name}?
              </button>
            ) : (
              <button
                onClick={() => setConfirmDelete(true)}
                title={`Delete ${active.name}`}
                className="shrink-0"
                style={{ color: 'var(--color-muted)' }}
              >
                <Trash2 size={11} />
              </button>
            ))}
        </>
      )}
      {editing === 'new' ? (
        <NameInput
          initial=""
          placeholder="New layout"
          onSubmit={(name) => { create(name); setEditing(null) }}
          onCancel={() => setEditing(null)}
        />
      ) : (
        <button
          onClick={() => { setEditing('new'); setConfirmDelete(false) }}
          className="flex items-center gap-0.5 text-xs px-1.5 py-0.5 rounded-full shrink-0"
          style={{ color: 'var(--color-muted)', border: '1px dashed var(--color-border)' }}
          title="Copy the active layout under a new name, then make it active"
        >
          <Plus size={11} /> New
        </button>
      )}
    </div>
  )
}
