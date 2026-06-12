import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, RefreshCw, AlertCircle, Search, X, ChevronRight, ChevronDown } from 'lucide-react'
import { getAllInventories, getItem, listCharacters } from '../services/api'
import type { AllInventoriesResponse, Inventory, InventoryEntry } from '../types/zeal'
import type { Item } from '../types/item'
import ItemDetailModal from '../components/ItemDetailModal'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ItemIcon } from '../components/Icon'
import {
  bagContainerInfo,
  bagSlotInfo,
  describeLocation,
  isEquipmentSlot,
} from '../lib/inventoryLocations'

// ── Types ──────────────────────────────────────────────────────────────────────

interface TaggedEntry extends InventoryEntry {
  character: string
}

// ── Entry classifiers ──────────────────────────────────────────────────────────

function isBagOrBank(location: string, kind: string): boolean {
  const slot = bagSlotInfo(location)
  if (slot) return slot.kind === kind
  const cont = bagContainerInfo(location)
  return cont?.kind === kind
}

// ── Grouping ───────────────────────────────────────────────────────────────────

interface BagGroup {
  kind: string // "General" | "Bank" | "SharedBank"
  num: number
  character: string
  bag: TaggedEntry | null
  slots: TaggedEntry[]
}

function groupBags(entries: TaggedEntry[], kind: string): BagGroup[] {
  const map = new Map<string, BagGroup>()
  for (const e of entries) {
    const slot = bagSlotInfo(e.location)
    const cont = bagContainerInfo(e.location)
    const num = slot?.bag ?? cont?.bag
    const matchKind = slot?.kind ?? cont?.kind
    if (num === undefined || matchKind !== kind) continue
    const key = `${e.character}::${num}`
    if (!map.has(key)) map.set(key, { kind, num, character: e.character, bag: null, slots: [] })
    const g = map.get(key)!
    if (slot) g.slots.push(e)
    else g.bag = e
  }
  for (const g of map.values()) {
    g.slots.sort((a, b) => {
      const an = bagSlotInfo(a.location)?.slot ?? 0
      const bn = bagSlotInfo(b.location)?.slot ?? 0
      return an - bn
    })
  }
  return [...map.values()].sort((a, b) => {
    if (a.character !== b.character) return a.character.localeCompare(b.character)
    return a.num - b.num
  })
}

// ── Helpers ────────────────────────────────────────────────────────────────────

function tagEntries(inv: Inventory): TaggedEntry[] {
  return inv.entries.map((e) => ({ ...e, character: inv.character }))
}

function isEmptyEntry(e: InventoryEntry): boolean {
  return e.id === 0 || e.name.toLowerCase() === 'empty'
}

// Persisted view preferences. Survive app restarts (unlike useCachedState).
const HIDE_EMPTY_BAGS_KEY = 'pq-invtracker-hide-empty-bags'
const SHOW_UNIMPORTED_KEY = 'pq-invtracker-show-unimported'

function readStoredBool(key: string, fallback: boolean): boolean {
  try {
    const v = localStorage.getItem(key)
    return v === null ? fallback : v === 'true'
  } catch {
    return fallback
  }
}

function writeStoredBool(key: string, value: boolean): void {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // Persistence is best-effort; the in-memory state still applies.
  }
}

// ── Sub-components ─────────────────────────────────────────────────────────────

interface BagCardProps {
  group: BagGroup
  showCharBadge: boolean
  onLookup: (id: number) => void
}

function BagCard({ group, showCharBadge, onLookup }: BagCardProps): React.ReactElement {
  const [open, setOpen] = useState(true)
  const visibleSlots = group.slots.filter((s) => !isEmptyEntry(s))
  const bagName = group.bag && !isEmptyEntry(group.bag) ? group.bag.name : '(empty bag)'
  const label = group.kind === 'General' ? `Bag ${group.num}` : `${group.kind} ${group.num}`

  return (
    <div
      className="rounded"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {label}
        </span>
        <span className="text-xs truncate" style={{ color: 'var(--color-muted-foreground)' }}>
          {bagName}
        </span>
        {showCharBadge && (
          <span
            className="shrink-0 text-[10px] px-1.5 py-0.5 rounded ml-1"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            {group.character}
          </span>
        )}
        <span className="ml-auto text-[10px]" style={{ color: 'var(--color-muted)' }}>
          {visibleSlots.length}/{group.slots.length}
        </span>
      </button>
      {open && (
        <div
          className="grid gap-1.5 px-3 pb-3 pt-1"
          style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(160px, 1fr))' }}
        >
          {visibleSlots.length === 0 ? (
            <p className="text-[11px] italic col-span-full" style={{ color: 'var(--color-muted)' }}>
              Empty
            </p>
          ) : (
            visibleSlots.map((s, i) => (
              <button
                key={`${s.location}-${i}`}
                onClick={() => onLookup(s.id)}
                className="flex items-center gap-1.5 rounded px-2 py-1 text-left text-xs"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
                title={s.name}
              >
                <ItemIcon id={s.icon} name={s.name} size={18} />
                <span className="truncate flex-1">{s.name}</span>
                {s.count > 1 && (
                  <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                    ×{s.count}
                  </span>
                )}
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}

interface SectionTitleProps {
  label: string
  count: number
}

function SectionTitle({ label, count }: SectionTitleProps): React.ReactElement {
  return (
    <div className="flex items-center justify-between mb-2 mt-4">
      <span
        className="text-[10px] font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        {label}
      </span>
      <span className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
        {count}
      </span>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function InventoryTrackerPage(): React.ReactElement {
  const [data, setData] = useState<AllInventoriesResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // Empty string = "All"; a name = single-character view. Matches CharacterSubTabs.
  const [selectedChar, setSelectedChar] = useState<string>('')
  const [query, setQuery] = useState('')
  const [hideEmptyBags, setHideEmptyBags] = useState(() => readStoredBool(HIDE_EMPTY_BAGS_KEY, true))
  const [showUnimported, setShowUnimported] = useState(() => readStoredBool(SHOW_UNIMPORTED_KEY, false))
  // Lowercased names of characters stored on the Characters page. null until
  // loaded (or on failure), which falls back to showing every export.
  const [importedNames, setImportedNames] = useState<Set<string> | null>(null)
  const [modalItem, setModalItem] = useState<Item | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    getAllInventories()
      .then(setData)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    listCharacters()
      .then((res) => setImportedNames(new Set(res.characters.map((c) => c.name.toLowerCase()))))
      .catch(() => setImportedNames(null))
  }, [])

  function handleLookup(id: number) {
    if (id === 0) return
    getItem(id)
      .then((item) => {
        setModalItem(item)
        setModalOpen(true)
      })
      .catch(() => {
        navigate(`/items?select=${id}`)
      })
  }

  // ── Derived data ─────────────────────────────────────────────────────────────

  // Exports from characters not imported on the Characters page are hidden
  // unless the user opts in via "Show unimported".
  const inventories = useMemo<Inventory[]>(() => {
    if (!data) return []
    if (showUnimported || importedNames === null) return data.characters
    return data.characters.filter((c) => importedNames.has(c.character.toLowerCase()))
  }, [data, importedNames, showUnimported])

  const unimportedNames = useMemo<string[]>(() => {
    if (!data || importedNames === null) return []
    return data.characters
      .filter((c) => !importedNames.has(c.character.toLowerCase()))
      .map((c) => c.character)
  }, [data, importedNames])

  // CharacterSubTabs only lists stored characters; unimported ones become
  // extra tabs while the toggle is on.
  const extraTabNames = useMemo<string[]>(
    () => (showUnimported ? unimportedNames : []),
    [showUnimported, unimportedNames],
  )

  // Collapse to the single character when only one export is visible; clear a
  // selection that disappeared (e.g. its unimported tab was just toggled off).
  useEffect(() => {
    if (inventories.length === 1) {
      setSelectedChar(inventories[0].character)
      return
    }
    setSelectedChar((prev) =>
      prev === '' || inventories.some((c) => c.character === prev) ? prev : '',
    )
  }, [inventories])

  const allTagged = useMemo<TaggedEntry[]>(() => {
    if (selectedChar === '') return inventories.flatMap(tagEntries)
    const inv = inventories.find((c) => c.character === selectedChar)
    return inv ? tagEntries(inv) : []
  }, [inventories, selectedChar])

  const showCharBadge = selectedChar === '' && inventories.length > 1

  const equipped = useMemo(
    () => allTagged.filter((e) => isEquipmentSlot(e.location) && !isEmptyEntry(e)),
    [allTagged],
  )

  const bagGroups = useMemo<BagGroup[]>(
    () => groupBags(allTagged.filter((e) => isBagOrBank(e.location, 'General')), 'General'),
    [allTagged],
  )

  const bankGroups = useMemo<BagGroup[]>(
    () => groupBags(allTagged.filter((e) => isBagOrBank(e.location, 'Bank')), 'Bank'),
    [allTagged],
  )

  const sharedBankGroups = useMemo<BagGroup[]>(() => {
    if (!data) return []
    const tagged: TaggedEntry[] = data.shared_bank.map((e) => ({ ...e, character: '' }))
    return groupBags(tagged.filter((e) => isBagOrBank(e.location, 'SharedBank')), 'SharedBank')
  }, [data])

  // Rechargeable items: limited-charge clickies the character holds anywhere
  // (equipped, bags, or bank). The API flags these by setting max_charges (only
  // for clickeffect>0, maxcharges>1); when set, `count` is the current charge
  // count, so we show current/max. Multiple copies appear as separate rows.
  const rechargeables = useMemo<TaggedEntry[]>(
    () =>
      allTagged
        .filter((e) => !isEmptyEntry(e) && (e.max_charges ?? 0) > 1)
        .sort((a, b) => a.name.localeCompare(b.name)),
    [allTagged],
  )

  // A search term switches the page to a flat result list instead of filtering
  // the bag grid in place — no more scrolling past "No matches" cards. Matches
  // cover everything held: equipment, bag contents, the bags themselves, bank,
  // shared bank (tagged with an empty character), and cursor.
  const searchActive = query.trim().length > 0

  const flatMatches = useMemo<TaggedEntry[]>(() => {
    if (!searchActive || !data) return []
    const q = query.trim().toLowerCase()
    const sharedTagged: TaggedEntry[] = data.shared_bank.map((e) => ({ ...e, character: '' }))
    return [...allTagged, ...sharedTagged]
      .filter((e) => !isEmptyEntry(e) && !e.location.endsWith('-Coin'))
      .filter((e) => e.name.toLowerCase().includes(q))
      .sort((a, b) => a.name.localeCompare(b.name) || a.character.localeCompare(b.character))
  }, [allTagged, data, query, searchActive])

  const matchHolderCount = useMemo(
    () => new Set(flatMatches.map((e) => e.character || 'Shared Bank')).size,
    [flatMatches],
  )

  const hasEmptyBag = useMemo(
    () =>
      bagGroups.some((g) => g.slots.every(isEmptyEntry)) ||
      bankGroups.some((g) => g.slots.every(isEmptyEntry)) ||
      sharedBankGroups.some((g) => g.slots.every(isEmptyEntry)),
    [bagGroups, bankGroups, sharedBankGroups],
  )

  const visibleBagGroups = useMemo(
    () => (hideEmptyBags ? bagGroups.filter((g) => g.slots.some((s) => !isEmptyEntry(s))) : bagGroups),
    [bagGroups, hideEmptyBags],
  )
  const visibleBankGroups = useMemo(
    () => (hideEmptyBags ? bankGroups.filter((g) => g.slots.some((s) => !isEmptyEntry(s))) : bankGroups),
    [bankGroups, hideEmptyBags],
  )
  const visibleSharedBankGroups = useMemo(
    () =>
      hideEmptyBags
        ? sharedBankGroups.filter((g) => g.slots.some((s) => !isEmptyEntry(s)))
        : sharedBankGroups,
    [sharedBankGroups, hideEmptyBags],
  )

  // ── Loading / error states ───────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8">
        <AlertCircle size={32} style={{ color: 'var(--color-danger)' }} />
        <p className="text-sm text-center" style={{ color: 'var(--color-muted-foreground)' }}>
          {error}
        </p>
        <button
          onClick={load}
          className="text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          Retry
        </button>
      </div>
    )
  }

  if (!data?.configured) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 p-8 max-w-md mx-auto text-center">
        <Package size={40} style={{ color: 'var(--color-muted)' }} />
        <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          EQ Path Not Configured
        </h2>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Set your EverQuest install path in{' '}
          <button
            className="underline"
            style={{ color: 'var(--color-primary)' }}
            onClick={() => navigate('/settings')}
          >
            Settings
          </button>
          , then log out of EverQuest to generate Zeal inventory exports.
        </p>
      </div>
    )
  }

  if (data.characters.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 p-8 max-w-md mx-auto text-center">
        <Package size={40} style={{ color: 'var(--color-muted)' }} />
        <h2 className="text-base font-semibold" style={{ color: 'var(--color-foreground)' }}>
          No Inventory Exports Found
        </h2>
        <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
          Zeal writes inventory files named{' '}
          <code
            className="px-1 py-0.5 rounded text-xs"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
          >
            &lt;CharName&gt;-Inventory.txt
          </code>{' '}
          to your EQ directory when you log out. Log out of each character to generate their export.
        </p>
        <button
          onClick={load}
          className="flex items-center gap-2 text-xs px-3 py-1.5 rounded"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: 'var(--color-foreground)',
            border: '1px solid var(--color-border)',
          }}
        >
          <RefreshCw size={12} />
          Check Again
        </button>
      </div>
    )
  }

  // ── Main render ──────────────────────────────────────────────────────────────

  const totalBagItems = bagGroups.reduce(
    (sum, g) => sum + g.slots.filter((s) => !isEmptyEntry(s)).length,
    0,
  )
  const totalBankItems = bankGroups.reduce(
    (sum, g) => sum + g.slots.filter((s) => !isEmptyEntry(s)).length,
    0,
  )
  const totalSharedBankItems = sharedBankGroups.reduce(
    (sum, g) => sum + g.slots.filter((s) => !isEmptyEntry(s)).length,
    0,
  )

  return (
    <div className="flex h-full flex-col">
      <ItemDetailModal
        item={modalItem}
        open={modalOpen}
        onClose={() => setModalOpen(false)}
      />

      {/* Header bar */}
      <div
        className="flex items-center gap-3 border-b px-4 py-3 shrink-0"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <Package size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold shrink-0" style={{ color: 'var(--color-foreground)' }}>
          Inventory Tracker
        </span>

        <div
          className="flex-1 flex items-center gap-2 rounded px-2.5 py-1"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            maxWidth: 320,
          }}
        >
          <Search size={12} style={{ color: 'var(--color-muted)', flexShrink: 0 }} />
          <input
            ref={searchRef}
            type="text"
            placeholder="Search items…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="flex-1 bg-transparent text-xs outline-none"
            style={{ color: 'var(--color-foreground)' }}
          />
          {query && (
            <button onClick={() => setQuery('')} className="shrink-0">
              <X size={11} style={{ color: 'var(--color-muted)' }} />
            </button>
          )}
        </div>

        <div className="ml-auto flex items-center gap-2">
          {unimportedNames.length > 0 && (
            <label
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded cursor-pointer select-none"
              style={{
                backgroundColor: showUnimported ? 'var(--color-surface-3)' : 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
              title="Include inventory exports from characters not imported on the Characters page"
            >
              <input
                type="checkbox"
                checked={showUnimported}
                onChange={(e) => {
                  setShowUnimported(e.target.checked)
                  writeStoredBool(SHOW_UNIMPORTED_KEY, e.target.checked)
                }}
                className="h-3 w-3"
              />
              Show unimported ({unimportedNames.length})
            </label>
          )}
          {!searchActive && hasEmptyBag && (
            <label
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded cursor-pointer select-none"
              style={{
                backgroundColor: hideEmptyBags ? 'var(--color-surface-3)' : 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
              title="Hide bags whose slots are all empty"
            >
              <input
                type="checkbox"
                checked={hideEmptyBags}
                onChange={(e) => {
                  setHideEmptyBags(e.target.checked)
                  writeStoredBool(HIDE_EMPTY_BAGS_KEY, e.target.checked)
                }}
                className="h-3 w-3"
              />
              Hide empty bags
            </label>
          )}
          <button
            onClick={load}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{
              backgroundColor: 'var(--color-surface-2)',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
        </div>
      </div>

      {/* Character sub-tabs */}
      <CharacterSubTabs
        value={selectedChar}
        onChange={setSelectedChar}
        allowAll
        extraNames={extraTabNames}
      />

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-4">
        {inventories.length === 0 ? (
          <div className="flex flex-col items-center gap-2 p-8 text-center">
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              All {data.characters.length} inventory exports belong to characters that
              haven't been imported yet.
            </p>
            <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
              Enable "Show unimported" above, or add them on the Characters page.
            </p>
          </div>
        ) : searchActive ? (
          <div>
            <p className="mb-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {flatMatches.length === 0
                ? `No items matching "${query.trim()}"`
                : `${flatMatches.length} ${flatMatches.length === 1 ? 'match' : 'matches'} across ${matchHolderCount} ${matchHolderCount === 1 ? 'character' : 'characters'}`}
            </p>
            <div className="flex flex-col gap-1.5" style={{ maxWidth: 560 }}>
              {flatMatches.map((e, i) => (
                <button
                  key={`fm-${e.character}-${e.location}-${i}`}
                  onClick={() => handleLookup(e.id)}
                  className="flex items-center gap-2 rounded px-2 py-1.5 text-left text-xs"
                  style={{
                    backgroundColor: 'var(--color-surface-2)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                  }}
                  title={e.name}
                >
                  <ItemIcon id={e.icon} name={e.name} size={18} />
                  <span className="truncate">{e.name}</span>
                  {e.count > 1 && (
                    <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                      ×{e.count}
                    </span>
                  )}
                  <span
                    className="ml-auto shrink-0 text-[10px] px-1.5 py-0.5 rounded"
                    style={{
                      backgroundColor: 'var(--color-surface-3)',
                      color: 'var(--color-muted-foreground)',
                      border: '1px solid var(--color-border)',
                    }}
                  >
                    {e.character || 'Shared Bank'}
                  </span>
                  <span
                    className="shrink-0 text-[10px]"
                    style={{ color: 'var(--color-muted-foreground)', minWidth: 110 }}
                  >
                    {describeLocation(e.location)}
                  </span>
                </button>
              ))}
            </div>
          </div>
        ) : (
          <>
            {/* Rechargeable — limited-charge clickies, current/max charges. Hidden
                entirely when the held set has none (or none match the search). */}
            {rechargeables.length > 0 && (
              <div>
                <SectionTitle label="Rechargeable" count={rechargeables.length} />
                <div
                  className="grid gap-1.5"
                  style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))' }}
                >
                  {rechargeables.map((e, i) => {
                    const max = e.max_charges ?? 0
                    const current = e.count
                    const below = current < max
                    return (
                      <button
                        key={`rc-${e.character}-${e.location}-${i}`}
                        onClick={() => handleLookup(e.id)}
                        className="flex items-center gap-1.5 rounded px-2 py-1.5 text-left text-xs"
                        style={{
                          backgroundColor: 'var(--color-surface-2)',
                          border: '1px solid var(--color-border)',
                          color: 'var(--color-foreground)',
                        }}
                        title={`${e.name} — ${current} of ${max} charges${showCharBadge ? ` (${e.character})` : ''}`}
                      >
                        <ItemIcon id={e.icon} name={e.name} size={18} />
                        <span className="truncate flex-1">{e.name}</span>
                        {showCharBadge && (
                          <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                            {e.character}
                          </span>
                        )}
                        <span
                          className="shrink-0 tabular-nums font-semibold rounded px-1.5 py-0.5 text-[10px]"
                          style={{
                            color: below ? 'var(--color-warning)' : 'var(--color-success)',
                            backgroundColor: 'var(--color-surface-3)',
                          }}
                        >
                          {current} / {max}
                        </span>
                      </button>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Equipped — flat slot tiles, matching the bag-slot layout below. The
                in-game equipment grid lives on Character Info → Gear instead. */}
            <div>
              <SectionTitle label="Equipped" count={equipped.length} />
              {equipped.length === 0 ? (
                <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
                  No equipped items
                </p>
              ) : (
                <div
                  className="grid gap-1.5"
                  style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))' }}
                >
                  {equipped.map((e, i) => (
                    <button
                      key={`eq-${i}`}
                      onClick={() => handleLookup(e.id)}
                      className="flex items-center gap-1.5 rounded px-2 py-1.5 text-left text-xs"
                      style={{
                        backgroundColor: 'var(--color-surface-2)',
                        border: '1px solid var(--color-border)',
                        color: 'var(--color-foreground)',
                      }}
                      title={e.name}
                    >
                      <ItemIcon id={e.icon} name={e.name} size={18} />
                      <span className="truncate flex-1">{e.name}</span>
                      <span className="shrink-0 text-[10px]" style={{ color: 'var(--color-muted-foreground)' }}>
                        {showCharBadge ? e.character : e.location}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Bags */}
            {visibleBagGroups.length > 0 && (
              <>
                <SectionTitle label="Bags" count={totalBagItems} />
                <div className="flex flex-col gap-2">
                  {visibleBagGroups.map((g) => (
                    <BagCard
                      key={`bag-${g.character}-${g.num}`}
                      group={g}
                      showCharBadge={showCharBadge}
                      onLookup={handleLookup}
                    />
                  ))}
                </div>
              </>
            )}

            {/* Bank */}
            {visibleBankGroups.length > 0 && (
              <>
                <SectionTitle label="Bank" count={totalBankItems} />
                <div className="flex flex-col gap-2">
                  {visibleBankGroups.map((g) => (
                    <BagCard
                      key={`bank-${g.character}-${g.num}`}
                      group={g}
                      showCharBadge={showCharBadge}
                      onLookup={handleLookup}
                    />
                  ))}
                </div>
              </>
            )}

            {/* Shared Bank */}
            {visibleSharedBankGroups.length > 0 && (
              <>
                <SectionTitle label="Shared Bank" count={totalSharedBankItems} />
                <div className="flex flex-col gap-2">
                  {visibleSharedBankGroups.map((g) => (
                    <BagCard
                      key={`sb-${g.num}`}
                      group={g}
                      showCharBadge={false}
                      onLookup={handleLookup}
                    />
                  ))}
                </div>
              </>
            )}
          </>
        )}
      </div>
    </div>
  )
}
