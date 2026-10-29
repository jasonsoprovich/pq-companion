import React, { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { isHiddenOn, sectionKey, setKeyVisibility, type NPCSurface } from '../../lib/npcItemFilters'
import {
  ITEM_INFO_HEADER_FIELDS,
  ITEM_INFO_SECTIONS,
  emptyModeFor,
  listLimitFor,
  resolveItemInfoOrder,
  sectionDef,
} from '../../lib/itemInfoLayout'
import type { ItemInfoOverlaySettings } from '../../types/config'

interface Props {
  settings: ItemInfoOverlaySettings
  onChange: (next: ItemInfoOverlaySettings) => void
}

const SURFACES: Array<{ id: NPCSurface; label: string }> = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'popout', label: 'Popout' },
]

const GRID = 'grid grid-cols-[minmax(0,1fr)_5.5rem_5.5rem] items-center'

interface Row {
  id: string
  label: string
  hint: string
  // Null for the header pseudo-section, which can't be hidden as a whole.
  sectionKey: string | null
  fields: Array<{ key: string; label: string; hint?: string }>
}

/**
 * Item Info overlay visibility editor: untick a section to hide all of it,
 * or open it to hide single rows, separately for the dashboard panel and the
 * popout. Listed sections also take their own list limit and empty behavior.
 */
export default function ItemInfoDisplayCard({ settings, onChange }: Props): React.ReactElement {
  const [open, setOpen] = useState<Set<string>>(new Set())
  const hidden = settings.hidden_items ?? []

  const rows: Row[] = [
    {
      id: 'header',
      label: 'Header',
      hint: 'Ownership badge, wishlist star, open-in-app link',
      sectionKey: null,
      fields: ITEM_INFO_HEADER_FIELDS,
    },
    ...resolveItemInfoOrder(settings.section_order).map((id) => {
      const def = sectionDef(id)
      return { id, label: def.label, hint: def.hint, sectionKey: sectionKey(id), fields: def.fields }
    }),
  ]

  const shown = (key: string, s: NPCSurface): boolean => !isHiddenOn(hidden, key, s)

  const setVisible = (keys: string[], surfaces: NPCSurface[], show: boolean): void => {
    let next = hidden
    for (const k of keys) next = setKeyVisibility(next, k, surfaces, show)
    onChange({ ...settings, hidden_items: next })
  }

  const toggleOpen = (id: string): void =>
    setOpen((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const summary = (row: Row): string => {
    const keys = row.fields.map((f) => f.key)
    if (keys.length === 0) return ''
    const counts = SURFACES.map((s) => keys.filter((k) => !shown(k, s.id)).length)
    if (counts[0] === 0 && counts[1] === 0) return 'all rows shown'
    if (counts[0] === counts[1]) return `${counts[0]} hidden`
    return `${counts[0]} hidden on dashboard, ${counts[1]} on popout`
  }

  const checkbox = (key: string, s: NPCSurface, disabled = false): React.ReactElement => (
    <span className="flex justify-center">
      <input
        type="checkbox"
        aria-label={`${key} on ${s}`}
        checked={shown(key, s)}
        disabled={disabled}
        onChange={(e) => setVisible([key], [s], e.target.checked)}
        style={{ opacity: disabled ? 0.35 : 1 }}
      />
    </span>
  )

  const muted = { color: 'var(--color-muted-foreground)' }
  const inputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface)',
    border: '1px solid var(--color-border)',
    color: 'var(--color-foreground)',
  }

  const setLimit = (id: string, raw: string): void => {
    const n = parseInt(raw, 10)
    const limits = { ...(settings.section_limits ?? {}) }
    // 0 = follow the global limit (the PUT can't reliably drop map keys).
    limits[id] = Number.isFinite(n) && n > 0 ? Math.min(n, 99) : 0
    onChange({ ...settings, section_limits: limits })
  }

  const setEmpty = (id: string, mode: string): void => {
    onChange({ ...settings, empty_modes: { ...(settings.empty_modes ?? {}), [id]: mode } })
  }

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        Item Info Display
      </h2>
      <p className="mb-4 text-xs" style={muted}>
        Choose what the Item Info overlay shows, separately for the dashboard panel and the popout
        window. Untick a section to hide all of it, or open it to hide single rows, set how many
        entries it lists before collapsing to a count, and whether it says &ldquo;no&rdquo; when
        there&rsquo;s nothing to show. The item name and icon always show.
      </p>

      <div className="flex flex-col gap-1.5">
        <div className={`${GRID} px-3 text-[11px] font-semibold uppercase tracking-wide`} style={muted}>
          <span />
          {SURFACES.map((s) => (
            <span key={s.id} className="text-center">
              {s.label}
            </span>
          ))}
        </div>

        {rows.map((row) => {
          const isOpen = open.has(row.id)
          const def = ITEM_INFO_SECTIONS.find((s) => s.id === row.id)
          const own = settings.section_limits?.[row.id] ?? 0
          const ownEmpty = settings.empty_modes?.[row.id] ?? ''
          return (
            <div
              key={row.id}
              className="rounded"
              style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
            >
              <div className={`${GRID} px-3 py-2`}>
                <button
                  onClick={() => toggleOpen(row.id)}
                  className="flex min-w-0 items-start gap-2 text-left"
                >
                  <span className="mt-0.5 w-[13px] shrink-0" style={{ color: 'var(--color-muted)' }}>
                    {isOpen ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
                  </span>
                  <span className="min-w-0">
                    <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                      {row.label}
                    </span>
                    {summary(row) && (
                      <span className="ml-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                        {summary(row)}
                      </span>
                    )}
                    <span className="block text-xs" style={muted}>
                      {row.hint}
                    </span>
                  </span>
                </button>
                {SURFACES.map((s) => (
                  <React.Fragment key={s.id}>
                    {row.sectionKey ? checkbox(row.sectionKey, s.id) : <span />}
                  </React.Fragment>
                ))}
              </div>

              {isOpen && (
                <div className="border-t px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
                  {row.fields.length > 0 && (
                    <div className="mb-1 flex gap-3 pl-[21px] text-xs">
                      <button
                        onClick={() => setVisible(row.fields.map((f) => f.key), ['dashboard', 'popout'], true)}
                        style={{ color: 'var(--color-primary)' }}
                      >
                        Show all rows
                      </button>
                    </div>
                  )}
                  {row.fields.map((f) => (
                    <div key={f.key} className={`${GRID} rounded py-0.5 hover:bg-black/10`} title={f.hint}>
                      <span className="truncate pl-[21px] text-sm" style={{ color: 'var(--color-foreground)' }}>
                        {f.label}
                      </span>
                      {SURFACES.map((s) => (
                        <React.Fragment key={s.id}>
                          {checkbox(f.key, s.id, row.sectionKey ? !shown(row.sectionKey, s.id) : false)}
                        </React.Fragment>
                      ))}
                    </div>
                  ))}

                  {def?.listed && (
                    <div className="mt-2 flex flex-wrap items-center gap-x-5 gap-y-2 pl-[21px] text-xs" style={muted}>
                      <label className="flex items-center gap-2">
                        List up to
                        <input
                          inputMode="numeric"
                          value={own > 0 ? String(own) : ''}
                          placeholder={String(listLimitFor({ list_limit: settings.list_limit }, def.id))}
                          onChange={(e) => setLimit(row.id, e.target.value.replace(/\D/g, ''))}
                          className="w-12 rounded px-1.5 py-0.5 text-sm"
                          style={inputStyle}
                        />
                        <span style={{ color: 'var(--color-muted)' }}>
                          {own > 0 ? 'entries' : 'entries (default)'}
                        </span>
                      </label>
                      {def.emptyDefault && (
                        <label className="flex items-center gap-2">
                          When empty
                          <select
                            value={ownEmpty === 'show' || ownEmpty === 'hide' ? ownEmpty : ''}
                            onChange={(e) => setEmpty(row.id, e.target.value)}
                            className="rounded px-1.5 py-0.5 text-sm"
                            style={inputStyle}
                          >
                            <option value="">
                              Default ({emptyModeFor(undefined, def.id) === 'show' ? 'say no' : 'hide'})
                            </option>
                            <option value="show">Say no</option>
                            <option value="hide">Hide section</option>
                          </select>
                        </label>
                      )}
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}
