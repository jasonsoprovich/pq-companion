/**
 * LayoutChips — one chip per overlay layout. Clicking a chip switches to it
 * (its windows open and move into place, the rest close). The active chip
 * can be renamed or deleted, or linked to characters (logging in on a linked
 * character switches to its layout; see useLayoutAutoSwitch). "New" copies
 * the active layout under a new name. Changes to the active layout save
 * themselves, so there's no Save button.
 */
import React, { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, CheckSquare, Pencil, Plus, Square, Trash2, User, X } from 'lucide-react'
import { useOverlayLayouts } from '../hooks/useOverlayLayouts'
import { listCharacters } from '../services/api'

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

const LINKS_WIDTH = 240

// The checklist of characters linked to a layout. Portaled to document.body
// at a fixed spot under its button, since the chip row scrolls sideways.
function CharacterLinks({
  anchor,
  layoutName,
  linked,
  linkedElsewhere,
  onChange,
  onClose,
}: {
  anchor: HTMLElement
  layoutName: string
  linked: string[]
  // Lower-cased name → the other layout it's linked to.
  linkedElsewhere: Map<string, string>
  onChange: (names: string[]) => void
  onClose: () => void
}): React.ReactElement {
  const panelRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ top: 0, left: 0 })
  const [known, setKnown] = useState<string[] | null>(null)

  useLayoutEffect(() => {
    const r = anchor.getBoundingClientRect()
    setPos({
      top: r.bottom + 6,
      left: Math.max(8, Math.min(r.left, window.innerWidth - LINKS_WIDTH - 8)),
    })
  }, [anchor])

  useEffect(() => {
    listCharacters()
      .then((d) => setKnown(d.characters.map((c) => c.name)))
      .catch(() => setKnown([]))
  }, [])

  useEffect(() => {
    const onDown = (e: MouseEvent): void => {
      const t = e.target as Node
      if (panelRef.current?.contains(t) || anchor.contains(t)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [anchor, onClose])

  const isLinked = (name: string): boolean => linked.some((c) => c.toLowerCase() === name.toLowerCase())
  // Linked names stay listed even if the character list no longer has them
  // (hidden, or removed), so they can be unlinked.
  const names = [...(known ?? [])]
  for (const c of linked) if (!names.some((n) => n.toLowerCase() === c.toLowerCase())) names.push(c)

  const toggle = (name: string): void => {
    onChange(isLinked(name) ? linked.filter((c) => c.toLowerCase() !== name.toLowerCase()) : [...linked, name])
  }

  return createPortal(
    <div
      ref={panelRef}
      className="fixed z-50 flex flex-col rounded-lg p-1.5"
      style={{
        top: pos.top,
        left: pos.left,
        width: LINKS_WIDTH,
        backgroundColor: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        boxShadow: '0 8px 24px rgba(0,0,0,0.4)',
      }}
    >
      <p className="px-2 pb-1.5 pt-0.5 text-[11px] leading-snug" style={{ color: 'var(--color-muted-foreground)' }}>
        Logging in on a ticked character switches to <strong>{layoutName}</strong>.
      </p>
      {known === null && (
        <p className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</p>
      )}
      {known !== null && names.length === 0 && (
        <p className="px-2 py-1 text-xs" style={{ color: 'var(--color-muted)' }}>
          No characters yet. They appear once the app has seen their logs.
        </p>
      )}
      <div className="flex max-h-[50vh] flex-col overflow-y-auto">
        {names.map((name) => {
          const on = isLinked(name)
          const other = on ? undefined : linkedElsewhere.get(name.toLowerCase())
          return (
            <button
              key={name}
              onClick={() => toggle(name)}
              className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs hover:bg-(--color-surface-2)"
              style={{ color: 'var(--color-foreground)' }}
              title={other ? `Linked to ${other}; ticking moves it here` : undefined}
            >
              {on
                ? <CheckSquare size={12} style={{ color: 'var(--color-primary)' }} />
                : <Square size={12} style={{ color: 'var(--color-muted-foreground)' }} />}
              <span className="flex-1 truncate">{name}</span>
              {other && (
                <span className="max-w-[45%] shrink-0 truncate text-[10px]" style={{ color: 'var(--color-muted)' }}>
                  {other}
                </span>
              )}
            </button>
          )
        })}
      </div>
    </div>,
    document.body,
  )
}

export default function LayoutChips(): React.ReactElement | null {
  const { state, supported, switchTo, create, rename, remove, setCharacters } = useOverlayLayouts()
  const [editing, setEditing] = useState<'new' | 'rename' | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [linksOpen, setLinksOpen] = useState(false)
  const linksRef = useRef<HTMLButtonElement>(null)

  if (!supported || !state) return null
  const active = state.layouts.find((l) => l.id === state.activeId)
  const linkedElsewhere = new Map<string, string>()
  for (const l of state.layouts) {
    if (l.id === active?.id) continue
    for (const c of l.characters) linkedElsewhere.set(c.toLowerCase(), l.name)
  }
  const linkedNote = (chars: string[]): string =>
    chars.length > 0 ? ` · switches here when ${chars.join(', ')} logs in` : ''

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
              (isActive
                ? `${l.name} (active) · ${l.windows.length} window${l.windows.length === 1 ? '' : 's'} · changes save automatically`
                : `Switch to ${l.name} · ${l.windows.length} window${l.windows.length === 1 ? '' : 's'}`)
              + linkedNote(l.characters)
            }
          >
            <span className="flex items-center gap-1">
              {l.characters.length > 0 && <User size={10} />}
              {l.name}
            </span>
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
          <button
            ref={linksRef}
            onClick={() => { setLinksOpen((o) => !o); setConfirmDelete(false) }}
            title={
              active.characters.length > 0
                ? `Characters linked to ${active.name}: ${active.characters.join(', ')}`
                : `Link characters to ${active.name}: logging in on one switches here`
            }
            className="shrink-0"
            style={{ color: active.characters.length > 0 || linksOpen ? 'var(--color-primary)' : 'var(--color-muted)' }}
          >
            <User size={11} />
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
      {linksOpen && active && linksRef.current && (
        <CharacterLinks
          anchor={linksRef.current}
          layoutName={active.name}
          linked={active.characters}
          linkedElsewhere={linkedElsewhere}
          onChange={(names) => setCharacters(active.id, names)}
          onClose={() => setLinksOpen(false)}
        />
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
