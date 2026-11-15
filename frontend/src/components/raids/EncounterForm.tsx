import React, { useEffect, useMemo, useRef, useState } from 'react'
import { Save, X, ChevronDown } from 'lucide-react'
import type { RaidEncounter, RaidStatus, RaidTaxonomy } from '../../types/raid'
import type { Zone } from '../../types/zone'
import type { NPC } from '../../types/npc'
import { roleLabel, subLabel } from '../../lib/raidLabels'
import { getNPC, searchNPCs } from '../../services/api'
import { npcLevelLabel } from '../../lib/npcHelpers'
import { matchesSearch } from '../../lib/textSearch'

interface Leaf {
  key: string
  role: string
  sub?: string
  label: string
  classes: string[]
}

// CompDraft keeps the comp grid editable in string form; numbers are parsed
// (falling back to 0) on save so typing is forgiving.
interface CompDraft {
  key: string
  leaf: Leaf
  include: boolean
  min: string
  rec: string
}

interface Props {
  taxonomy: RaidTaxonomy
  zones: Zone[]
  encounter: RaidEncounter | null
  onSubmit: (enc: RaidEncounter) => Promise<void>
  onCancel: () => void
  saving: boolean
}

function taxonomyLeaves(tax: RaidTaxonomy): Leaf[] {
  const leaves: Leaf[] = []
  for (const role of tax.role_order) {
    const spec = tax.roles[role]
    if (spec.sub_roles) {
      for (const sub of Object.keys(spec.sub_roles).sort()) {
        const s = spec.sub_roles[sub]
        leaves.push({ key: `${role}.${sub}`, role, sub, label: s.label || `${role} / ${sub}`, classes: s.classes ?? [] })
      }
    } else {
      leaves.push({ key: role, role, label: spec.label || role, classes: spec.classes ?? [] })
    }
  }
  return leaves
}

const inputCls =
  'w-full rounded px-2 py-1.5 text-sm outline-none border focus:ring-1 focus:ring-(--color-primary)'
const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  borderColor: 'var(--color-border)',
  color: 'var(--color-foreground)',
}

const numCls = 'w-16 rounded px-1.5 py-1 text-sm text-center tabular-nums outline-none border'
const numStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  borderColor: 'var(--color-border)',
  color: 'var(--color-foreground)',
}

// ZoneField — type-ahead combobox over the game's zone catalog (same source
// the Zones browser uses). Picking a zone sets its zoneidnumber (what Zeal
// reports), so encounter detection matches by id, not by name spelling.
function ZoneField({ zones, value, onPick }: {
  zones: Zone[]
  value: { text: string; id: number }
  onPick: (text: string, id: number) => void
}): React.ReactElement {
  const [open, setOpen] = useState(false)
  const [focus, setFocus] = useState(false)

  const q = value.text.trim().toLowerCase()
  const matches = useMemo(() => {
    const list = zones.filter((z) => !q || matchesSearch(z.long_name, q) || matchesSearch(z.short_name, q))
    return list.slice(0, 12)
  }, [zones, q])

  function choose(z: Zone): void {
    onPick(z.long_name, z.zone_id_number)
    setOpen(false)
  }

  return (
    <div className="relative" onBlur={() => setTimeout(() => setOpen(false), 150)}>
      <div className="relative">
        <input
          className={inputCls}
          style={inputStyle}
          value={value.text}
          placeholder="type a zone name…"
          onFocus={() => { setOpen(true); setFocus(true) }}
          onChange={(e) => {
            onPick(e.target.value, 0)
            setOpen(true)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && matches.length > 0) { e.preventDefault(); choose(matches[0]) }
            if (e.key === 'Escape') setOpen(false)
          }}
        />
        <ChevronDown
          size={14}
          className="absolute right-2 top-1/2 -translate-y-1/2 pointer-events-none"
          style={{ color: 'var(--color-muted-foreground)' }}
        />
      </div>
      <div className="absolute left-0 right-0 z-30">
        {/* (combobox dropdown renders below in normal flow) */}
      </div>
      {open && matches.length > 0 && (
        <div
          className="absolute left-0 right-0 z-30 mt-1 rounded overflow-hidden shadow-lg"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', maxHeight: 260, overflowY: 'auto' }}
        >
          {matches.map((z) => (
            <button
              key={z.id}
              type="button"
              className="w-full text-left px-2.5 py-1.5 text-sm hover:bg-(--color-surface-3)"
              style={{ color: 'var(--color-foreground)' }}
              onClick={() => choose(z)}
              onMouseDown={(e) => e.preventDefault()}
            >
              <span>{z.long_name}</span>{' '}
              <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                ({z.short_name} · {z.zone_id_number})
              </span>
            </button>
          ))}
        </div>
      )}
      {value.id > 0 && (
        <span className="text-[11px]" style={{ color: 'var(--color-success)' }}>
          ✓ zone id {value.id}
        </span>
      )}
      {focus && value.text && value.id === 0 && (
        <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          freeform name (no matching zone id)
        </span>
      )}
    </div>
  )
}

// NPCField — type-ahead combobox over the NPC database (same search the NPCs
// browser uses). Picking an NPC links the encounter to its npc_types row so
// the checker page can pull resists / HP / special abilities / signature
// spells straight from the game database instead of duplicating them here.
function NPCField({ value, onPick }: {
  value: { id: number; name: string }
  onPick: (id: number, name: string) => void
}): React.ReactElement {
  const [text, setText] = useState(value.name)
  const [open, setOpen] = useState(false)
  const [matches, setMatches] = useState<NPC[]>([])
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Keep the field's text in sync when a different encounter is loaded (or
  // the linked NPC's name resolves after an async lookup on mount).
  useEffect(() => {
    setText(value.name)
  }, [value.id, value.name])

  useEffect(() => {
    const q = text.trim()
    if (!q || q === value.name) {
      setMatches([])
      return
    }
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      searchNPCs(q, 12)
        .then((res) => setMatches(res.items))
        .catch(() => setMatches([]))
    }, 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [text])

  function choose(npc: NPC): void {
    onPick(npc.id, npc.name)
    setText(npc.name)
    setOpen(false)
  }

  function clear(): void {
    onPick(0, '')
    setText('')
    setOpen(false)
  }

  return (
    <div className="relative" onBlur={() => setTimeout(() => setOpen(false), 150)}>
      <div className="relative">
        <input
          className={inputCls}
          style={inputStyle}
          value={text}
          placeholder="search NPC name to link stats (optional)…"
          onFocus={() => setOpen(true)}
          onChange={(e) => {
            setText(e.target.value)
            onPick(0, e.target.value)
            setOpen(true)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && matches.length > 0) { e.preventDefault(); choose(matches[0]) }
            if (e.key === 'Escape') setOpen(false)
          }}
        />
        {value.id > 0 ? (
          <button
            type="button"
            onClick={clear}
            title="Unlink NPC"
            className="absolute right-2 top-1/2 -translate-y-1/2"
            style={{ color: 'var(--color-muted-foreground)' }}
          >
            <X size={14} />
          </button>
        ) : (
          <ChevronDown
            size={14}
            className="absolute right-2 top-1/2 -translate-y-1/2 pointer-events-none"
            style={{ color: 'var(--color-muted-foreground)' }}
          />
        )}
      </div>
      {open && matches.length > 0 && (
        <div
          className="absolute left-0 right-0 z-30 mt-1 rounded overflow-hidden shadow-lg"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', maxHeight: 260, overflowY: 'auto' }}
        >
          {matches.map((npc) => (
            <button
              key={npc.id}
              type="button"
              className="w-full text-left px-2.5 py-1.5 text-sm hover:bg-(--color-surface-3)"
              style={{ color: 'var(--color-foreground)' }}
              onClick={() => choose(npc)}
              onMouseDown={(e) => e.preventDefault()}
            >
              <span>{npc.name.replace(/_/g, ' ')}</span>{' '}
              <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                (L{npcLevelLabel(npc)} · {npc.hp.toLocaleString()} HP · id {npc.id})
              </span>
            </button>
          ))}
        </div>
      )}
      {value.id > 0 && (
        <span className="text-[11px]" style={{ color: 'var(--color-success)' }}>
          ✓ linked to npc id {value.id}
        </span>
      )}
    </div>
  )
}

export default function EncounterForm({ taxonomy, zones, encounter, onSubmit, onCancel, saving }: Props): React.ReactElement {
  const leaves = useMemo(() => taxonomyLeaves(taxonomy), [taxonomy])

  const [id, setId] = useState(encounter?.id ?? '')
  const [name, setName] = useState(encounter?.name ?? '')
  const [zone, setZone] = useState<{ text: string; id: number }>({
    text: encounter?.zone ?? '',
    id: encounter?.zone_id ?? 0,
  })
  const [npc, setNpc] = useState<{ id: number; name: string }>({
    id: encounter?.npc_id ?? 0,
    name: '',
  })
  // The encounter only carries npc_id, not a name — resolve it once on load
  // so the combobox shows something other than a bare id.
  useEffect(() => {
    if (!encounter?.npc_id) return
    let cancelled = false
    getNPC(encounter.npc_id)
      .then((n) => { if (!cancelled) setNpc({ id: n.id, name: n.name.replace(/_/g, ' ') }) })
      .catch(() => {})
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [encounter?.npc_id])
  const [status, setStatus] = useState<RaidStatus>(encounter?.status ?? 'active')
  const [trigger, setTrigger] = useState(encounter?.trigger ?? '')
  const [source, setSource] = useState(encounter?.source ?? '')
  const [notes, setNotes] = useState(encounter?.notes ?? '')
  const [reqsText, setReqsText] = useState((encounter?.reqs ?? []).join('\n'))
  const [strategy, setStrategy] = useState<Record<string, string>>(encounter?.strategy ?? {})

  const [drafts, setDrafts] = useState<CompDraft[]>(() => {
    const byPath = new Map<string, { min: number; rec: number }>()
    for (const c of encounter?.comps ?? []) {
      byPath.set(c.role + (c.sub_role ? '.' + c.sub_role : ''), { min: c.min, rec: c.rec })
    }
    return leaves.map((leaf) => {
      const existing = byPath.get(leaf.key)
      return {
        key: leaf.key,
        leaf,
        include: existing !== undefined,
        min: existing !== undefined ? String(existing.min) : '0',
        rec: existing !== undefined ? String(existing.rec) : '0',
      }
    })
  })

  const [error, setError] = useState('')

  // Focus target for the Min input when a row is toggled on; the input only
  // becomes enabled after the include=true commits, so focus happens in an
  // effect rather than synchronously.
  const [pendingFocusKey, setPendingFocusKey] = useState<string | null>(null)
  const minRefs = useRef<Record<string, HTMLInputElement | null>>({})

  useEffect(() => {
    if (!pendingFocusKey) return
    const el = minRefs.current[pendingFocusKey]
    if (el && !el.disabled) {
      el.focus()
      el.select()
    }
    setPendingFocusKey(null)
  }, [pendingFocusKey, drafts])

  // toggleRow handles the include checkbox (clicking the checkbox itself or
  // anywhere on the row). Toggling on auto-focuses the Min input and defaults
  // both counts to 1 when they were both 0; toggling off only blurs.
  function toggleRow(key: string, on: boolean): void {
    setDrafts((d) =>
      d.map((row) => {
        if (row.key !== key) return row
        if (!on) return { ...row, include: false }
        const zero = row.min === '0' && row.rec === '0'
        return { ...row, include: true, min: zero ? '1' : row.min, rec: zero ? '1' : row.rec }
      }),
    )
    setPendingFocusKey(on ? key : null)
    if (!on) {
      ;(document.activeElement as HTMLElement | null)?.blur?.()
    }
  }

  function setDraft(key: string, patch: Partial<CompDraft>): void {
    setDrafts((d) => d.map((row) => (row.key === key ? { ...row, ...patch } : row)))
  }

  async function handleSubmit(e: React.FormEvent): Promise<void> {
    e.preventDefault()
    setError('')
    if (!name.trim()) {
      setError('Name is required')
      return
    }
    if (!zone.text.trim()) {
      setError('Zone is required — pick one from the list')
      return
    }
    const comps = drafts
      .filter((d) => d.include)
      .map((d) => ({
        role: d.leaf.role,
        sub_role: d.leaf.sub,
        min: Math.max(0, parseInt(d.min, 10) || 0),
        rec: Math.max(0, parseInt(d.rec, 10) || 0),
      }))
    for (const c of comps) {
      if (c.rec < c.min) {
        setError(`Rec is below min for ${c.sub_role ? `${c.role} / ${c.sub_role}` : c.role}`)
        return
      }
    }
    const enc: RaidEncounter = {
      id: id.trim() || name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-'),
      name: name.trim(),
      zone: zone.text.trim(),
      zone_id: zone.id,
      npc_id: npc.id || undefined,
      status,
      trigger: trigger.trim(),
      source: source.trim(),
      notes: notes.trim(),
      reqs: reqsText.split('\n').map((l) => l.trim()).filter(Boolean),
      strategy: Object.fromEntries(
        taxonomy.strategy_order.map((s) => [s, (strategy[s] ?? '').trim()]).filter(([, v]) => v),
      ),
      comps,
      created_at: encounter?.created_at ?? 0,
      updated_at: encounter?.updated_at ?? 0,
    }
    await onSubmit(enc)
  }

  const field = (label: string, node: React.ReactNode): React.ReactElement => (
    <label className="flex flex-col gap-1">
      <span className="text-xs font-medium" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      {node}
    </label>
  )

  const classNameHint = (classes: string[]): string =>
    classes.map((c) => taxonomy.class_names?.[c] ?? c).join(', ')

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4 px-6 py-4 overflow-auto" style={{ height: '100%' }}>
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {encounter ? `Edit: ${encounter.name}` : 'New Raid Encounter'}
        </h2>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
          >
            <X size={14} /> Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded font-medium"
            style={{
              backgroundColor: saving ? 'var(--color-muted)' : 'var(--color-primary)',
              color: 'var(--color-primary-foreground, #fff)',
            }}
          >
            <Save size={14} /> {saving ? 'Saving…' : 'Save Encounter'}
          </button>
        </div>
      </div>

      {error ? (
        <div className="px-3 py-2 text-sm rounded" style={{ backgroundColor: 'var(--color-danger)', color: '#fff' }}>
          {error}
        </div>
      ) : null}

      <div className="grid grid-cols-2 gap-3">
        {field('ID (slug)', (
          <input
            className={inputCls}
            style={inputStyle}
            value={id}
            onChange={(e) => setId(e.target.value)}
            placeholder="aow"
            disabled={!!encounter}
          />
        ))}
        {field('Status', (
          <select
            className={inputCls}
            style={inputStyle}
            value={status}
            onChange={(e) => setStatus(e.target.value as RaidStatus)}
          >
            <option value="active">active</option>
            <option value="placeholder">placeholder</option>
          </select>
        ))}
        {field('Name', (
          <input
            className={inputCls}
            style={inputStyle}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Avatar of War"
          />
        ))}
        {field('Zone (type-ahead)', (
          <ZoneField zones={zones} value={zone} onPick={(text, id) => setZone({ text, id })} />
        ))}
        {field('Linked NPC (type-ahead, optional)', (
          <NPCField value={npc} onPick={(id, name) => setNpc({ id, name })} />
        ))}
        {field('Trigger / spawn mechanics', (
          <input
            className={inputCls}
            style={inputStyle}
            value={trigger}
            onChange={(e) => setTrigger(e.target.value)}
            placeholder="optional"
          />
        ))}
        {field('Source', (
          <input
            className={inputCls}
            style={inputStyle}
            value={source}
            onChange={(e) => setSource(e.target.value)}
            placeholder="optional raid guide name"
          />
        ))}
      </div>

      {field('Prerequisites (one per line)', (
        <textarea
          className={inputCls}
          style={{ ...inputStyle, minHeight: 52 }}
          value={reqsText}
          onChange={(e) => setReqsText(e.target.value)}
        />
      ))}

      {taxonomy.strategy_order.length > 0 && (
        <div className="grid grid-cols-2 gap-3">
          {taxonomy.strategy_order.map((s) => (
            <label key={s} className="flex flex-col gap-1">
              <span className="text-xs font-medium capitalize" style={{ color: 'var(--color-muted-foreground)' }}>
                Strategy — {s}
              </span>
              <textarea
                className={inputCls}
                style={{ ...inputStyle, minHeight: 56 }}
                value={strategy[s] ?? ''}
                onChange={(e) => setStrategy((prev) => ({ ...prev, [s]: e.target.value }))}
              />
            </label>
          ))}
        </div>
      )}

      <div>
        <div className="mb-1.5 flex items-baseline justify-between">
          <span className="text-xs font-medium" style={{ color: 'var(--color-muted-foreground)' }}>
            Composition — check the roles this encounter needs, then set MIN (floor to attempt) and REC (comfortable)
          </span>
        </div>
        <div
          className="rounded-lg overflow-hidden"
          style={{ border: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          <div
            className="grid grid-cols-[auto_1fr_56px_56px] gap-3 items-center px-3 py-1.5 text-[11px] uppercase tracking-wide"
            style={{ borderBottom: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
          >
            <span />
            <span>Role (eligible classes)</span>
            <span className="text-center">MIN</span>
            <span className="text-center">REC</span>
          </div>
          {drafts.map((row, i) => (
            <div
              key={row.key}
              className={`grid grid-cols-[auto_1fr_56px_56px] gap-3 items-center px-3 py-1.5 cursor-pointer ${i % 2 === 1 ? 'bg-(--color-surface-2)/50' : ''}`}
              onClick={(e) => {
                // clicks on the checkbox / number inputs handle themselves
                if ((e.target as HTMLElement).closest('input')) return
                toggleRow(row.key, !row.include)
              }}
            >
              <input
                type="checkbox"
                checked={row.include}
                onChange={(e) => toggleRow(row.key, e.target.checked)}
              />
              <div className="flex flex-col">
                <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  {row.leaf.label || roleLabel(row.leaf.role)}
                  {row.leaf.sub && !row.leaf.label ? <span className="opacity-70"> / {subLabel(row.leaf.sub)}</span> : null}
                </span>
                <span className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
                  {classNameHint(row.leaf.classes)}
                </span>
              </div>
              <input
                className={numCls}
                style={numStyle}
                type="number"
                min={0}
                value={row.min}
                disabled={!row.include}
                ref={(el) => {
                  minRefs.current[row.key] = el
                }}
                onChange={(e) => setDraft(row.key, { min: e.target.value })}
              />
              <input
                className={numCls}
                style={numStyle}
                type="number"
                min={0}
                value={row.rec}
                disabled={!row.include}
                onChange={(e) => setDraft(row.key, { rec: e.target.value })}
              />
            </div>
          ))}
        </div>
      </div>

      {field('Notes', (
        <textarea
          className={inputCls}
          style={{ ...inputStyle, minHeight: 72 }}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
        />
      ))}
    </form>
  )
}