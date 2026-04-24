import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Check, Copy, Filter, Search, X } from 'lucide-react'
import { searchItems, getItem, getItemSources } from '../services/api'
import type { ItemSearchFilter } from '../services/api'
import type { Item, ItemSourceNPC, ItemSources, ItemForageZone, ItemGroundSpawnZone, ItemTradeskillEntry } from '../types/item'
import {
  baneBodyLabel,
  baneRaceLabel,
  classesLabel,
  effectiveItemTypeLabel,
  isLoreItem,
  itemTypeLabel,
  priceLabel,
  racesLabel,
  sizeLabel,
  slotsLabel,
  weightLabel,
} from '../lib/itemHelpers'

// ── Filter definitions ─────────────────────────────────────────────────────────

const ITEM_CLASSES: { value: number; label: string }[] = [
  { value: 1, label: 'Warrior' },
  { value: 2, label: 'Cleric' },
  { value: 4, label: 'Paladin' },
  { value: 8, label: 'Ranger' },
  { value: 16, label: 'Shadow Knight' },
  { value: 32, label: 'Druid' },
  { value: 64, label: 'Monk' },
  { value: 128, label: 'Bard' },
  { value: 256, label: 'Rogue' },
  { value: 512, label: 'Shaman' },
  { value: 1024, label: 'Necromancer' },
  { value: 2048, label: 'Wizard' },
  { value: 4096, label: 'Magician' },
  { value: 8192, label: 'Enchanter' },
  { value: 16384, label: 'Beastlord' },
]

const ITEM_RACES: { value: number; label: string }[] = [
  { value: 1, label: 'Human' },
  { value: 2, label: 'Barbarian' },
  { value: 4, label: 'Erudite' },
  { value: 8, label: 'Wood Elf' },
  { value: 16, label: 'High Elf' },
  { value: 32, label: 'Dark Elf' },
  { value: 64, label: 'Half Elf' },
  { value: 128, label: 'Dwarf' },
  { value: 256, label: 'Troll' },
  { value: 512, label: 'Ogre' },
  { value: 1024, label: 'Halfling' },
  { value: 2048, label: 'Gnome' },
  { value: 4096, label: 'Iksar' },
  { value: 8192, label: 'Vah Shir' },
]

const ITEM_SLOTS: { value: number; label: string }[] = [
  { value: 0x000001, label: 'Charm' },
  { value: 0x000012, label: 'Ear' },
  { value: 0x000004, label: 'Head' },
  { value: 0x000008, label: 'Face' },
  { value: 0x000020, label: 'Neck' },
  { value: 0x000040, label: 'Shoulder' },
  { value: 0x000080, label: 'Arms' },
  { value: 0x000100, label: 'Back' },
  { value: 0x000600, label: 'Wrist' },
  { value: 0x000800, label: 'Range' },
  { value: 0x001000, label: 'Hands' },
  { value: 0x002000, label: 'Primary' },
  { value: 0x004000, label: 'Secondary' },
  { value: 0x018000, label: 'Finger' },
  { value: 0x020000, label: 'Chest' },
  { value: 0x040000, label: 'Legs' },
  { value: 0x080000, label: 'Feet' },
  { value: 0x100000, label: 'Waist' },
  { value: 0x800000, label: 'Ammo' },
]

const ITEM_TYPES: { value: number; label: string }[] = [
  { value: 0, label: '1H Slashing' },
  { value: 1, label: '2H Slashing' },
  { value: 2, label: '1H Piercing' },
  { value: 3, label: '1H Blunt' },
  { value: 4, label: '2H Blunt' },
  { value: 5, label: 'Archery' },
  { value: 7, label: 'Throwing' },
  { value: 8, label: 'Shield' },
  { value: 10, label: 'Armor' },
  { value: 11, label: 'Miscellaneous' },
  { value: 14, label: 'Food' },
  { value: 15, label: 'Drink' },
  { value: 17, label: 'Combinable' },
  { value: 20, label: 'Spell Scroll' },
  { value: 21, label: 'Potion' },
  { value: 22, label: 'Tradeskill' },
  { value: 28, label: 'Jewelry' },
  { value: 30, label: 'Book' },
  { value: 32, label: 'Key' },
  { value: 34, label: '2H Piercing' },
  { value: 40, label: 'Poison' },
  { value: 52, label: 'Martial' },
]

interface FilterState {
  race: number
  class: number
  minLevel: string
  maxLevel: string
  slot: number
  itemType: number
  minHP: string
  minMana: string
  minAC: string
  minSTR: string
  minSTA: string
  minAGI: string
  minDEX: string
  minWIS: string
  minINT: string
  minCHA: string
  minMR: string
  minCR: string
  minDR: string
  minFR: string
  minPR: string
}

const EMPTY_FILTER: FilterState = {
  race: 0, class: 0, minLevel: '', maxLevel: '',
  slot: 0, itemType: -1,
  minHP: '', minMana: '', minAC: '',
  minSTR: '', minSTA: '', minAGI: '', minDEX: '',
  minWIS: '', minINT: '', minCHA: '',
  minMR: '', minCR: '', minDR: '', minFR: '', minPR: '',
}

function filterToApiParams(f: FilterState): ItemSearchFilter {
  return {
    race: f.race > 0 ? f.race : undefined,
    class: f.class > 0 ? f.class : undefined,
    minLevel: parseInt(f.minLevel) > 0 ? parseInt(f.minLevel) : undefined,
    maxLevel: parseInt(f.maxLevel) > 0 ? parseInt(f.maxLevel) : undefined,
    slot: f.slot > 0 ? f.slot : undefined,
    itemType: f.itemType >= 0 ? f.itemType : undefined,
    minHP: parseInt(f.minHP) > 0 ? parseInt(f.minHP) : undefined,
    minMana: parseInt(f.minMana) > 0 ? parseInt(f.minMana) : undefined,
    minAC: parseInt(f.minAC) > 0 ? parseInt(f.minAC) : undefined,
    minSTR: parseInt(f.minSTR) > 0 ? parseInt(f.minSTR) : undefined,
    minSTA: parseInt(f.minSTA) > 0 ? parseInt(f.minSTA) : undefined,
    minAGI: parseInt(f.minAGI) > 0 ? parseInt(f.minAGI) : undefined,
    minDEX: parseInt(f.minDEX) > 0 ? parseInt(f.minDEX) : undefined,
    minWIS: parseInt(f.minWIS) > 0 ? parseInt(f.minWIS) : undefined,
    minINT: parseInt(f.minINT) > 0 ? parseInt(f.minINT) : undefined,
    minCHA: parseInt(f.minCHA) > 0 ? parseInt(f.minCHA) : undefined,
    minMR: parseInt(f.minMR) > 0 ? parseInt(f.minMR) : undefined,
    minCR: parseInt(f.minCR) > 0 ? parseInt(f.minCR) : undefined,
    minDR: parseInt(f.minDR) > 0 ? parseInt(f.minDR) : undefined,
    minFR: parseInt(f.minFR) > 0 ? parseInt(f.minFR) : undefined,
    minPR: parseInt(f.minPR) > 0 ? parseInt(f.minPR) : undefined,
  }
}

function activeChips(f: FilterState): { key: keyof FilterState; label: string }[] {
  const chips: { key: keyof FilterState; label: string }[] = []
  if (f.race > 0) chips.push({ key: 'race', label: `Race: ${ITEM_RACES.find(r => r.value === f.race)?.label ?? f.race}` })
  if (f.class > 0) chips.push({ key: 'class', label: `Class: ${ITEM_CLASSES.find(c => c.value === f.class)?.label ?? f.class}` })
  if (parseInt(f.minLevel) > 0) chips.push({ key: 'minLevel', label: `Min Lvl: ${f.minLevel}` })
  if (parseInt(f.maxLevel) > 0) chips.push({ key: 'maxLevel', label: `Max Lvl: ${f.maxLevel}` })
  if (f.slot > 0) chips.push({ key: 'slot', label: `Slot: ${ITEM_SLOTS.find(s => s.value === f.slot)?.label ?? f.slot}` })
  if (f.itemType >= 0) chips.push({ key: 'itemType', label: `Type: ${ITEM_TYPES.find(t => t.value === f.itemType)?.label ?? f.itemType}` })
  if (parseInt(f.minHP) > 0) chips.push({ key: 'minHP', label: `HP ≥ ${f.minHP}` })
  if (parseInt(f.minMana) > 0) chips.push({ key: 'minMana', label: `Mana ≥ ${f.minMana}` })
  if (parseInt(f.minAC) > 0) chips.push({ key: 'minAC', label: `AC ≥ ${f.minAC}` })
  if (parseInt(f.minSTR) > 0) chips.push({ key: 'minSTR', label: `STR ≥ ${f.minSTR}` })
  if (parseInt(f.minSTA) > 0) chips.push({ key: 'minSTA', label: `STA ≥ ${f.minSTA}` })
  if (parseInt(f.minAGI) > 0) chips.push({ key: 'minAGI', label: `AGI ≥ ${f.minAGI}` })
  if (parseInt(f.minDEX) > 0) chips.push({ key: 'minDEX', label: `DEX ≥ ${f.minDEX}` })
  if (parseInt(f.minWIS) > 0) chips.push({ key: 'minWIS', label: `WIS ≥ ${f.minWIS}` })
  if (parseInt(f.minINT) > 0) chips.push({ key: 'minINT', label: `INT ≥ ${f.minINT}` })
  if (parseInt(f.minCHA) > 0) chips.push({ key: 'minCHA', label: `CHA ≥ ${f.minCHA}` })
  if (parseInt(f.minMR) > 0) chips.push({ key: 'minMR', label: `MR ≥ ${f.minMR}` })
  if (parseInt(f.minCR) > 0) chips.push({ key: 'minCR', label: `CR ≥ ${f.minCR}` })
  if (parseInt(f.minDR) > 0) chips.push({ key: 'minDR', label: `DR ≥ ${f.minDR}` })
  if (parseInt(f.minFR) > 0) chips.push({ key: 'minFR', label: `FR ≥ ${f.minFR}` })
  if (parseInt(f.minPR) > 0) chips.push({ key: 'minPR', label: `PR ≥ ${f.minPR}` })
  return chips
}

// ── Filter Modal ───────────────────────────────────────────────────────────────

interface FilterModalProps {
  filter: FilterState
  onChange: (f: FilterState) => void
  onClose: () => void
}

function FilterModal({ filter, onChange, onClose }: FilterModalProps): React.ReactElement {
  const [draft, setDraft] = useState<FilterState>(filter)

  function set<K extends keyof FilterState>(key: K, value: FilterState[K]) {
    setDraft((prev) => ({ ...prev, [key]: value }))
  }

  function apply() {
    onChange(draft)
    onClose()
  }

  function clearAll() {
    setDraft(EMPTY_FILTER)
    onChange(EMPTY_FILTER)
    onClose()
  }

  const selectStyle = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
  }

  const inputStyle = {
    backgroundColor: 'var(--color-surface-2)',
    color: 'var(--color-foreground)',
    border: '1px solid var(--color-border)',
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-16"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="flex w-full max-w-lg flex-col overflow-hidden rounded-lg shadow-2xl"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', maxHeight: 'calc(100vh - 8rem)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between border-b px-4 py-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Filter Items
          </span>
          <button onClick={onClose}>
            <X size={14} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto px-4 py-3 flex flex-col gap-4">

          {/* Usability */}
          <section>
            <div className="mb-2 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
              Usability
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div className="flex flex-col gap-1">
                <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Race</label>
                <select value={draft.race} onChange={(e) => set('race', Number(e.target.value))} className="rounded px-2 py-1 text-xs outline-none" style={selectStyle}>
                  <option value={0}>Any Race</option>
                  {ITEM_RACES.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
                </select>
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Class</label>
                <select value={draft.class} onChange={(e) => set('class', Number(e.target.value))} className="rounded px-2 py-1 text-xs outline-none" style={selectStyle}>
                  <option value={0}>Any Class</option>
                  {ITEM_CLASSES.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
                </select>
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Min Level</label>
                <input type="number" min={0} max={255} placeholder="0" value={draft.minLevel} onChange={(e) => set('minLevel', e.target.value)} className="rounded px-2 py-1 text-xs outline-none" style={inputStyle} />
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Max Level</label>
                <input type="number" min={0} max={255} placeholder="0" value={draft.maxLevel} onChange={(e) => set('maxLevel', e.target.value)} className="rounded px-2 py-1 text-xs outline-none" style={inputStyle} />
              </div>
            </div>
          </section>

          {/* Equipment */}
          <section>
            <div className="mb-2 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
              Equipment
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div className="flex flex-col gap-1">
                <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Slot</label>
                <select value={draft.slot} onChange={(e) => set('slot', Number(e.target.value))} className="rounded px-2 py-1 text-xs outline-none" style={selectStyle}>
                  <option value={0}>Any Slot</option>
                  {ITEM_SLOTS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
                </select>
              </div>
              <div className="flex flex-col gap-1">
                <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>Item Type</label>
                <select value={draft.itemType} onChange={(e) => set('itemType', Number(e.target.value))} className="rounded px-2 py-1 text-xs outline-none" style={selectStyle}>
                  <option value={-1}>Any Type</option>
                  {ITEM_TYPES.map((t) => <option key={t.value} value={t.value}>{t.label}</option>)}
                </select>
              </div>
            </div>
          </section>

          {/* Stats */}
          <section>
            <div className="mb-2 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
              Min Stats
            </div>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  ['minHP', 'HP'], ['minMana', 'Mana'], ['minAC', 'AC'],
                  ['minSTR', 'STR'], ['minSTA', 'STA'], ['minAGI', 'AGI'],
                  ['minDEX', 'DEX'], ['minWIS', 'WIS'], ['minINT', 'INT'],
                  ['minCHA', 'CHA'],
                ] as [keyof FilterState, string][]
              ).map(([key, label]) => (
                <div key={key} className="flex flex-col gap-1">
                  <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>{label}</label>
                  <input
                    type="number" min={0} placeholder="0"
                    value={draft[key] as string}
                    onChange={(e) => set(key, e.target.value)}
                    className="rounded px-2 py-1 text-xs outline-none"
                    style={inputStyle}
                  />
                </div>
              ))}
            </div>
          </section>

          {/* Resists */}
          <section>
            <div className="mb-2 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
              Min Resists
            </div>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  ['minMR', 'Magic'], ['minCR', 'Cold'], ['minDR', 'Disease'],
                  ['minFR', 'Fire'], ['minPR', 'Poison'],
                ] as [keyof FilterState, string][]
              ).map(([key, label]) => (
                <div key={key} className="flex flex-col gap-1">
                  <label className="text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>{label}</label>
                  <input
                    type="number" min={0} placeholder="0"
                    value={draft[key] as string}
                    onChange={(e) => set(key, e.target.value)}
                    className="rounded px-2 py-1 text-xs outline-none"
                    style={inputStyle}
                  />
                </div>
              ))}
            </div>
          </section>
        </div>

        {/* Footer */}
        <div
          className="flex items-center justify-between border-t px-4 py-3"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <button
            onClick={clearAll}
            className="text-xs"
            style={{ color: 'var(--color-muted)' }}
          >
            Clear All
          </button>
          <button
            onClick={apply}
            className="rounded px-3 py-1.5 text-xs font-medium"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-primary-foreground)' }}
          >
            Apply Filters
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Search pane ────────────────────────────────────────────────────────────────

interface SearchPaneProps {
  selectedId: number | null
  onSelect: (item: Item) => void
}

function SearchPane({ selectedId, onSelect }: SearchPaneProps): React.ReactElement {
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<FilterState>(EMPTY_FILTER)
  const [showModal, setShowModal] = useState(false)
  const [items, setItems] = useState<Item[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const chips = activeChips(filter)
  const hasFilter = chips.length > 0

  const runSearch = useCallback((q: string, f: FilterState) => {
    setLoading(true)
    setError(null)
    searchItems(q, 50, 0, filterToApiParams(f))
      .then((res) => {
        setItems(res.items ?? [])
        setTotal(res.total)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => runSearch(query, filter), 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, filter, runSearch])

  useEffect(() => {
    runSearch('', EMPTY_FILTER)
  }, [runSearch])

  function dismissChip(key: keyof FilterState) {
    setFilter((prev) => ({ ...prev, [key]: EMPTY_FILTER[key] }))
  }

  return (
    <div
      className="flex w-72 shrink-0 flex-col border-r"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {/* Search + filter button row */}
      <div
        className="flex items-center gap-2 border-b px-3 py-2"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
        <input
          type="text"
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
          style={{ color: 'var(--color-foreground)' }}
          placeholder="Search items…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          spellCheck={false}
        />
        {query && (
          <button onClick={() => setQuery('')} className="shrink-0">
            <X size={12} style={{ color: 'var(--color-muted)' }} />
          </button>
        )}
        <button
          onClick={() => setShowModal(true)}
          title="Filter"
          className="shrink-0 rounded p-0.5 transition-colors"
          style={{
            color: hasFilter ? 'var(--color-primary)' : 'var(--color-muted)',
            backgroundColor: hasFilter ? 'color-mix(in srgb, var(--color-primary) 15%, transparent)' : 'transparent',
          }}
        >
          <Filter size={14} />
        </button>
      </div>

      {/* Active filter chips */}
      {chips.length > 0 && (
        <div
          className="flex flex-wrap gap-1 border-b px-3 py-2"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {chips.map((chip) => (
            <span
              key={chip.key}
              className="flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium"
              style={{
                backgroundColor: 'color-mix(in srgb, var(--color-primary) 15%, transparent)',
                color: 'var(--color-primary)',
                border: '1px solid color-mix(in srgb, var(--color-primary) 30%, transparent)',
              }}
            >
              {chip.label}
              <button onClick={() => dismissChip(chip.key)} className="shrink-0 leading-none">
                <X size={10} />
              </button>
            </span>
          ))}
          <button
            onClick={() => setFilter(EMPTY_FILTER)}
            className="text-[10px]"
            style={{ color: 'var(--color-muted)' }}
          >
            Clear all
          </button>
        </div>
      )}

      {/* Result count */}
      <div
        className="border-b px-3 py-1.5 text-[11px]"
        style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted)' }}
      >
        {loading ? 'Searching…' : error ? 'Error' : `${total.toLocaleString()} items`}
      </div>

      {/* Results list */}
      <div className="flex-1 overflow-y-auto">
        {error && (
          <p className="px-3 py-4 text-xs" style={{ color: 'var(--color-destructive)' }}>
            {error}
          </p>
        )}
        {!error &&
          items.map((item) => (
            <button
              key={item.id}
              onClick={() => onSelect(item)}
              className="w-full px-3 py-2 text-left transition-colors"
              style={{
                backgroundColor:
                  selectedId === item.id ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  selectedId === item.id
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              <div
                className="truncate text-sm font-medium"
                style={{
                  color:
                    selectedId === item.id
                      ? 'var(--color-primary)'
                      : 'var(--color-foreground)',
                }}
              >
                {item.name}
              </div>
              <div className="mt-0.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                {effectiveItemTypeLabel(item.item_class, item.item_type)}
                {item.req_level > 0 && ` · Req ${item.req_level}`}
              </div>
            </button>
          ))}
      </div>

      {showModal && (
        <FilterModal
          filter={filter}
          onChange={setFilter}
          onClose={() => setShowModal(false)}
        />
      )}
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

interface StatRowProps {
  label: string
  value: string | number
}

function StatRow({ label, value }: StatRowProps): React.ReactElement {
  return (
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <span className="min-w-0 break-words text-right" style={{ color: 'var(--color-foreground)' }}>{value}</span>
    </div>
  )
}

interface SectionProps {
  title: string
  children: React.ReactNode
}

function Section({ title, children }: SectionProps): React.ReactElement {
  return (
    <div>
      <div
        className="mb-1 text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {title}
      </div>
      <div
        className="rounded border px-3 py-1"
        style={{
          backgroundColor: 'var(--color-surface)',
          borderColor: 'var(--color-border)',
        }}
      >
        {children}
      </div>
    </div>
  )
}

function formatNPCName(name: string): string {
  return name.replace(/_/g, ' ')
}

interface SpellEffectRowProps {
  label: string
  spellId: number
  name: string
}

function SpellEffectRow({ label, spellId, name }: SpellEffectRowProps): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div className="flex justify-between gap-3 py-0.5 text-sm">
      <span className="shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>{label}</span>
      <button
        onClick={() => navigate(`/spells?select=${spellId}`)}
        className="min-w-0 truncate text-right underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}
        title="View spell details"
      >
        {name}
      </button>
    </div>
  )
}

interface SourceNPCLinkProps {
  npc: ItemSourceNPC
  showRate?: boolean
}

function SourceNPCLink({ npc, showRate }: SourceNPCLinkProps): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div className="flex w-full items-center justify-between gap-3 py-0.5 text-sm">
      <button
        onClick={() => navigate(`/npcs?select=${npc.id}`)}
        className="min-w-0 truncate text-left underline decoration-dotted"
        style={{ color: 'var(--color-primary)' }}
      >
        {formatNPCName(npc.name)}
      </button>
      <div className="flex shrink-0 items-center gap-2">
        {showRate && npc.drop_rate != null && npc.drop_rate > 0 && (
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {npc.drop_rate.toFixed(2)}%
          </span>
        )}
        {npc.zone_name && (
          <button
            onClick={() => navigate(`/zones?select=${npc.zone_short_name}`)}
            className="text-xs underline decoration-dotted"
            style={{ color: 'var(--color-muted)' }}
          >
            {npc.zone_name}
          </button>
        )}
      </div>
    </div>
  )
}

function EmptyTabMessage({ message }: { message: string }): React.ReactElement {
  return (
    <p className="py-4 text-sm" style={{ color: 'var(--color-muted)' }}>{message}</p>
  )
}

function tradeskillLabel(id: number): string {
  const labels: Record<number, string> = {
    0: 'No Tradeskill',
    55: 'Fishing',
    56: 'Make Poison',
    57: 'Smithing',
    58: 'Tailoring',
    59: 'Baking',
    60: 'Alchemy',
    61: 'Brewing',
    63: 'Research',
    64: 'Pottery',
    65: 'Fletching',
    68: 'Jewelry Making',
    69: 'Tinkering',
    75: 'Begging',
  }
  return labels[id] ?? `Tradeskill ${id}`
}

// ── Tab: Overview ──────────────────────────────────────────────────────────────

interface OverviewTabProps {
  item: Item
  copied: boolean
  onCopy: () => void
}

function OverviewTab({ item, copied, onCopy }: OverviewTabProps): React.ReactElement {
  const flags: string[] = []
  if (item.magic) flags.push('MAGIC')
  if (isLoreItem(item.lore)) flags.push('LORE')
  if (item.nodrop === 0) flags.push('NO DROP')
  if (item.norent === 0) flags.push('NO RENT')

  const hasCombat = item.damage > 0 || item.ac > 0
  const hasBane = item.bane_amt > 0 || item.bane_body > 0 || item.bane_race > 0
  const hasStats =
    item.hp > 0 || item.mana > 0 || item.str > 0 || item.sta > 0 || item.agi > 0 ||
    item.dex > 0 || item.wis > 0 || item.int > 0 || item.cha > 0
  const hasResists = item.mr > 0 || item.cr > 0 || item.dr > 0 || item.fr > 0 || item.pr > 0
  const hasEffects =
    (item.click_effect > 0 && !!item.click_name) ||
    (item.proc_effect > 0 && !!item.proc_name) ||
    (item.worn_effect > 0 && !!item.worn_name) ||
    (item.focus_effect > 0 && !!item.focus_name)

  return (
    <div className="flex flex-col gap-3">
      {/* Item header in tab */}
      <div>
        <div className="flex items-start justify-between gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              {effectiveItemTypeLabel(item.item_class, item.item_type)}
            </span>
            {flags.map((f) => (
              <span
                key={f}
                className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-primary)',
                  border: '1px solid var(--color-border)',
                }}
              >
                {f}
              </span>
            ))}
          </div>
          <button
            onClick={onCopy}
            title="Copy In-Game Link"
            className="flex shrink-0 items-center gap-1.5 rounded border px-2 py-1 text-[11px] font-medium transition-colors"
            style={{
              backgroundColor: 'var(--color-surface)',
              borderColor: 'var(--color-border)',
              color: copied ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
            }}
          >
            {copied ? <Check size={12} /> : <Copy size={12} />}
            {copied ? 'Copied!' : 'Copy Link'}
          </button>
        </div>
        {item.lore && (
          <p className="mt-1.5 text-xs italic" style={{ color: 'var(--color-muted-foreground)' }}>
            {item.lore.startsWith('*') ? item.lore.slice(1) : item.lore}
          </p>
        )}
      </div>

      {hasCombat && (
        <Section title="Combat">
          {item.damage > 0 && (
            <StatRow label="Damage / Delay" value={`${item.damage} / ${item.delay}`} />
          )}
          {item.range > 0 && <StatRow label="Range" value={item.range} />}
          {item.ac > 0 && <StatRow label="AC" value={item.ac} />}
        </Section>
      )}

      {hasBane && (
        <Section title="Bane Damage">
          {item.bane_amt > 0 && <StatRow label="Bane Damage" value={`+${item.bane_amt}`} />}
          {item.bane_body > 0 && <StatRow label="Bane vs Body" value={baneBodyLabel(item.bane_body)} />}
          {item.bane_race > 0 && <StatRow label="Bane vs Race" value={baneRaceLabel(item.bane_race)} />}
        </Section>
      )}

      {hasStats && (
        <Section title="Stats">
          {item.hp > 0 && <StatRow label="HP" value={`+${item.hp}`} />}
          {item.mana > 0 && <StatRow label="Mana" value={`+${item.mana}`} />}
          {item.str > 0 && <StatRow label="STR" value={`+${item.str}`} />}
          {item.sta > 0 && <StatRow label="STA" value={`+${item.sta}`} />}
          {item.agi > 0 && <StatRow label="AGI" value={`+${item.agi}`} />}
          {item.dex > 0 && <StatRow label="DEX" value={`+${item.dex}`} />}
          {item.wis > 0 && <StatRow label="WIS" value={`+${item.wis}`} />}
          {item.int > 0 && <StatRow label="INT" value={`+${item.int}`} />}
          {item.cha > 0 && <StatRow label="CHA" value={`+${item.cha}`} />}
        </Section>
      )}

      {hasResists && (
        <Section title="Resists">
          {item.mr > 0 && <StatRow label="Magic" value={`+${item.mr}`} />}
          {item.cr > 0 && <StatRow label="Cold" value={`+${item.cr}`} />}
          {item.dr > 0 && <StatRow label="Disease" value={`+${item.dr}`} />}
          {item.fr > 0 && <StatRow label="Fire" value={`+${item.fr}`} />}
          {item.pr > 0 && <StatRow label="Poison" value={`+${item.pr}`} />}
        </Section>
      )}

      {hasEffects && (
        <Section title="Effects">
          {item.click_effect > 0 && item.click_name && (
            <SpellEffectRow label="Click" spellId={item.click_effect} name={item.click_name} />
          )}
          {item.proc_effect > 0 && item.proc_name && (
            <SpellEffectRow label="Proc" spellId={item.proc_effect} name={item.proc_name} />
          )}
          {item.worn_effect > 0 && item.worn_name && (
            <SpellEffectRow label="Worn" spellId={item.worn_effect} name={item.worn_name} />
          )}
          {item.focus_effect > 0 && item.focus_name && (
            <SpellEffectRow label="Focus" spellId={item.focus_effect} name={item.focus_name} />
          )}
        </Section>
      )}

      <Section title="Restrictions">
        {item.req_level > 0 && <StatRow label="Required Level" value={item.req_level} />}
        {item.rec_level > 0 && <StatRow label="Recommended Level" value={item.rec_level} />}
        <StatRow label="Slots" value={slotsLabel(item.slots)} />
        <StatRow label="Classes" value={classesLabel(item.classes)} />
        <StatRow label="Races" value={racesLabel(item.races)} />
      </Section>

      <Section title="Info">
        <StatRow label="Weight" value={weightLabel(item.weight)} />
        <StatRow label="Size" value={sizeLabel(item.size)} />
        {item.stackable > 0 && item.stack_size > 1 && (
          <StatRow label="Stack Size" value={item.stack_size} />
        )}
        {item.item_class === 1 && (
          <>
            <StatRow label="Bag Slots" value={item.bag_slots} />
            <StatRow label="Bag Size" value={sizeLabel(item.bag_size)} />
          </>
        )}
        {item.price > 0 && <StatRow label="Value" value={priceLabel(item.price)} />}
        <StatRow label="Item ID" value={item.id} />
      </Section>
    </div>
  )
}

// ── Tab: Drops From ────────────────────────────────────────────────────────────

function DropsFromTab({ drops }: { drops: ItemSourceNPC[] }): React.ReactElement {
  if (drops.length === 0) return <EmptyTabMessage message="No drop sources found." />
  return (
    <div>
      <div className="mb-1 flex justify-between text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
        <span>NPC</span>
        <span>Drop Rate / Zone</span>
      </div>
      {drops.map((npc) => (
        <SourceNPCLink key={npc.id} npc={npc} showRate />
      ))}
    </div>
  )
}

// ── Tab: Purchased From ────────────────────────────────────────────────────────

function PurchasedFromTab({ merchants }: { merchants: ItemSourceNPC[] }): React.ReactElement {
  if (merchants.length === 0) return <EmptyTabMessage message="Not sold by any merchant." />
  return (
    <div>
      {merchants.map((npc) => (
        <SourceNPCLink key={npc.id} npc={npc} />
      ))}
    </div>
  )
}

// ── Tab: Foraged From ──────────────────────────────────────────────────────────

function ForagedFromTab({ zones }: { zones: ItemForageZone[] }): React.ReactElement {
  const navigate = useNavigate()
  if (zones.length === 0) return <EmptyTabMessage message="Not forageable in any zone." />
  return (
    <div>
      {zones.map((fz, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <button
            onClick={() => navigate(`/zones?select=${fz.zone_short_name}`)}
            className="min-w-0 truncate text-left underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
          >
            {fz.zone_name || fz.zone_short_name}
          </button>
          {fz.chance > 0 && (
            <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {fz.chance}%
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

// ── Tab: Ground Spawns ─────────────────────────────────────────────────────────

function GroundSpawnsTab({ spawns }: { spawns: ItemGroundSpawnZone[] }): React.ReactElement {
  const navigate = useNavigate()
  if (spawns.length === 0) return <EmptyTabMessage message="No ground spawns found." />
  return (
    <div>
      {spawns.map((gs, i) => (
        <div key={i} className="flex items-center justify-between gap-3 py-0.5 text-sm">
          <button
            onClick={() => navigate(`/zones?select=${gs.zone_short_name}`)}
            className="min-w-0 truncate text-left underline decoration-dotted"
            style={{ color: 'var(--color-primary)' }}
          >
            {gs.zone_name || gs.zone_short_name}
          </button>
          <span className="shrink-0 text-xs" style={{ color: 'var(--color-muted)' }}>
            {gs.name}
          </span>
        </div>
      ))}
    </div>
  )
}

// ── Tab: Tradeskills ───────────────────────────────────────────────────────────

function TradeskillsTab({ entries }: { entries: ItemTradeskillEntry[] }): React.ReactElement {
  if (entries.length === 0) return <EmptyTabMessage message="Not used in any tradeskill recipe." />
  const products = entries.filter((e) => e.role === 'product')
  const ingredients = entries.filter((e) => e.role === 'ingredient')
  return (
    <div className="flex flex-col gap-3">
      {products.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Produced by
          </div>
          {products.map((ts) => (
            <div key={ts.recipe_id} className="flex items-center justify-between gap-3 py-0.5 text-sm">
              <span style={{ color: 'var(--color-foreground)' }}>{ts.recipe_name}</span>
              <div className="flex shrink-0 items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                <span>{tradeskillLabel(ts.tradeskill)}</span>
                <span>Trivial {ts.trivial}</span>
              </div>
            </div>
          ))}
        </div>
      )}
      {ingredients.length > 0 && (
        <div>
          <div className="mb-1 text-[10px] font-semibold uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
            Used as ingredient in
          </div>
          {ingredients.map((ts) => (
            <div key={ts.recipe_id} className="flex items-center justify-between gap-3 py-0.5 text-sm">
              <span style={{ color: 'var(--color-foreground)' }}>{ts.recipe_name}</span>
              <div className="flex shrink-0 items-center gap-2 text-xs" style={{ color: 'var(--color-muted)' }}>
                <span>{tradeskillLabel(ts.tradeskill)}</span>
                {ts.count > 1 && <span>×{ts.count}</span>}
                <span>Trivial {ts.trivial}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Detail panel ───────────────────────────────────────────────────────────────

type ItemTabKey = 'overview' | 'drops' | 'merchants' | 'forage' | 'ground-spawns' | 'tradeskills'

const ITEM_TABS: { key: ItemTabKey; label: string }[] = [
  { key: 'overview', label: 'Overview' },
  { key: 'drops', label: 'Drops From' },
  { key: 'merchants', label: 'Purchased From' },
  { key: 'forage', label: 'Foraged From' },
  { key: 'ground-spawns', label: 'Ground Spawns' },
  { key: 'tradeskills', label: 'Tradeskills' },
]

interface DetailPanelProps {
  item: Item | null
}

function DetailPanel({ item }: DetailPanelProps): React.ReactElement {
  const [sources, setSources] = useState<ItemSources | null>(null)
  const [activeTab, setActiveTab] = useState<ItemTabKey>('overview')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    setActiveTab('overview')
    if (!item) { setSources(null); return }
    getItemSources(item.id)
      .then(setSources)
      .catch(() => setSources({ drops: [], merchants: [], forage_zones: [], ground_spawns: [], tradeskills: [] }))
  }, [item?.id])

  function copyIngameLink() {
    if (!item) return
    const link = `\\aITEM ${item.id} 0 0 0 0 0:${item.name}\\a/`
    navigator.clipboard.writeText(link).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  if (!item) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Select an item to view details
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Header */}
      <div
        className="shrink-0 border-b px-5 pt-4 pb-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <h2 className="text-xl font-bold leading-tight" style={{ color: 'var(--color-primary)' }}>
          {item.name}
        </h2>

        {/* Tabs */}
        <div className="mt-3 flex gap-0 overflow-x-auto">
          {ITEM_TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className="shrink-0 px-3 py-1.5 text-xs font-medium transition-colors"
              style={{
                color: activeTab === tab.key ? 'var(--color-primary)' : 'var(--color-muted)',
                borderBottom:
                  activeTab === tab.key
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto px-5 py-4">
        {activeTab === 'overview' && (
          <OverviewTab item={item} copied={copied} onCopy={copyIngameLink} />
        )}
        {activeTab === 'drops' && (
          <DropsFromTab drops={sources?.drops ?? []} />
        )}
        {activeTab === 'merchants' && (
          <PurchasedFromTab merchants={sources?.merchants ?? []} />
        )}
        {activeTab === 'forage' && (
          <ForagedFromTab zones={sources?.forage_zones ?? []} />
        )}
        {activeTab === 'ground-spawns' && (
          <GroundSpawnsTab spawns={sources?.ground_spawns ?? []} />
        )}
        {activeTab === 'tradeskills' && (
          <TradeskillsTab entries={sources?.tradeskills ?? []} />
        )}
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function ItemsPage(): React.ReactElement {
  const [selected, setSelected] = useState<Item | null>(null)
  const [searchParams, setSearchParams] = useSearchParams()

  useEffect(() => {
    const id = Number(searchParams.get('select'))
    if (!id) return
    getItem(id)
      .then(setSelected)
      .catch(() => {/* ignore */})
      .finally(() => setSearchParams({}, { replace: true }))
  }, [searchParams, setSearchParams])

  return (
    <div className="flex h-full" style={{ backgroundColor: 'var(--color-background)' }}>
      <SearchPane selectedId={selected?.id ?? null} onSelect={setSelected} />
      <DetailPanel item={selected} />
    </div>
  )
}
