import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, X, ArrowUp, ArrowDown } from 'lucide-react'
import { getLootMeta, listCharacters, listLoot, searchItems, searchZones } from '../services/api'
import type { LootEntry } from '../types/loot'
import type { Item } from '../types/item'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { itemTypeLabel } from '../lib/enumsCache'
import ItemDetailModal from './ItemDetailModal'
import BackfillLink from './BackfillLink'

function formatTimestamp(unix: number): string {
  if (!unix) return ''
  return new Date(unix * 1000).toLocaleString([], {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

type SortDir = 'asc' | 'desc'

const DISPLAY_STEP = 25

interface CharacterRecapLootProps {
  characterName: string
}

// Compact, character-scoped loot listing embedded in the Recap tab —
// filterable by item type and searchable by item/looter, zone, and time.
// Scoped to the user's own tracked characters: `character` filters to rows
// witnessed in this character's own log, and the ownCharacters filter below
// further excludes rows where a *groupmate* (not one of the user's own
// alts) was the looter — the log records other players' loot too when
// grouped/raiding with them, which isn't "this app's user's loot."
// NPC-looted-from isn't surfaced here: the EQ loot log line never names the
// source corpse, so it's always empty (see LootTrackerPage's identical
// caveat).
export default function CharacterRecapLoot({ characterName }: CharacterRecapLootProps): React.ReactElement {
  const [zones, setZones] = useState<string[]>([])
  const [rows, setRows] = useState<LootEntry[]>([])
  const [ownCharacters, setOwnCharacters] = useState<Set<string> | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const seqRef = useRef(0)

  const [search, setSearch] = useState('')
  const [zoneFilter, setZoneFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState<string>('')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [displayLimit, setDisplayLimit] = useState(DISPLAY_STEP)

  const [modalItem, setModalItem] = useState<Item | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    getLootMeta(characterName).then((m) => setZones(m.zones ?? [])).catch(() => { /* best-effort */ })
  }, [characterName])

  // The user's own tracked character roster, used to exclude groupmates'
  // loot (see the ownCharacters filter below). Loaded once — it doesn't
  // depend on which character's recap is being viewed.
  useEffect(() => {
    listCharacters()
      .then((r) => setOwnCharacters(new Set((r.characters ?? []).map((c) => c.name.toLowerCase()))))
      .catch(() => setOwnCharacters(new Set()))
  }, [])

  const load = useCallback(() => {
    if (!characterName) { setRows([]); setLoading(false); return undefined }
    const seq = ++seqRef.current
    setLoading(true)
    setError(null)
    return listLoot({ character: characterName, search, zone: zoneFilter, sort: sortDir, limit: 1000 })
      .then((r) => { if (seq === seqRef.current) setRows(r.loot) })
      .catch((e: Error) => { if (seq === seqRef.current) setError(e.message) })
      .finally(() => { if (seq === seqRef.current) setLoading(false) })
  }, [characterName, search, zoneFilter, sortDir])

  useEffect(() => { load() }, [load])
  useEffect(() => { setDisplayLimit(DISPLAY_STEP) }, [characterName, search, zoneFilter, typeFilter])

  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onWs = useCallback((msg: { type: string }) => {
    if (msg.type !== WSEvent.LootNew) return
    if (reloadTimer.current) clearTimeout(reloadTimer.current)
    reloadTimer.current = setTimeout(() => load(), 500)
  }, [load])
  useWebSocket(onWs)

  // Exclude groupmates' loot — only rows looted by one of the user's own
  // tracked characters. While the roster is still loading, fall back to the
  // unfiltered rows rather than flashing an empty table.
  const ownRows = useMemo(() => {
    if (!ownCharacters) return rows
    return rows.filter((r) => ownCharacters.has(r.player.toLowerCase()))
  }, [rows, ownCharacters])

  const types = useMemo(() => {
    const seen = new Set<number>()
    for (const r of ownRows) seen.add(r.item_type)
    return Array.from(seen).sort((a, b) => a - b)
  }, [ownRows])

  const filteredRows = useMemo(() => {
    if (typeFilter === '') return ownRows
    const t = Number(typeFilter)
    return ownRows.filter((r) => r.item_type === t)
  }, [ownRows, typeFilter])

  async function openItem(name: string) {
    try {
      const r = await searchItems(name, 10)
      const exact = r.items.find((i) => i.name.toLowerCase() === name.toLowerCase())
      const item = exact ?? r.items[0]
      if (item) { setModalItem(item); setModalOpen(true) }
    } catch { /* best-effort */ }
  }

  async function openZone(zoneName: string) {
    try {
      const r = await searchZones(zoneName)
      const exact = r.items.find((z) => z.long_name.toLowerCase() === zoneName.toLowerCase())
      const zone = exact ?? r.items[0]
      if (zone) navigate(`/zones?select=${zone.id}`)
    } catch { /* best-effort */ }
  }

  const hasFilters = search || zoneFilter || typeFilter

  return (
    <div className="rounded-lg" style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}>
      <ItemDetailModal item={modalItem} open={modalOpen} onClose={() => setModalOpen(false)} />

      <div className="flex items-center gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <Package size={13} style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>Loot</span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {filteredRows.length} item{filteredRows.length === 1 ? '' : 's'}
        </span>
      </div>

      <div className="flex flex-wrap items-center gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search item or looter…"
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)', minWidth: '12rem' }}
        />
        <select
          value={zoneFilter}
          onChange={(e) => setZoneFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
        >
          <option value="">All zones</option>
          {zones.map((z) => <option key={z} value={z}>{z}</option>)}
        </select>
        <select
          value={typeFilter}
          onChange={(e) => setTypeFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
        >
          <option value="">All types</option>
          {types.map((t) => <option key={t} value={t}>{itemTypeLabel(t)}</option>)}
        </select>
        <button
          onClick={() => setSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))}
          className="flex items-center gap-1 text-xs px-2 py-1 rounded"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          title="Toggle time sort order"
        >
          {sortDir === 'desc' ? <ArrowDown size={11} /> : <ArrowUp size={11} />} Time
        </button>
        {hasFilters && (
          <button
            onClick={() => { setSearch(''); setZoneFilter(''); setTypeFilter('') }}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            <X size={11} /> Clear filters
          </button>
        )}
      </div>

      <div className="p-3">
        {loading && rows.length === 0 && (
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
        )}
        {error && !loading && (
          <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>
        )}
        {!loading && !error && ownRows.length === 0 && (
          <div className="flex flex-col items-center gap-1.5 py-6 text-center">
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>No loot tracked yet for this character.</p>
            <div className="flex items-center gap-2 text-[11px]" style={{ color: 'var(--color-muted)' }}>
              Have older loot in your log file?
              <BackfillLink />
            </div>
          </div>
        )}
        {!loading && !error && ownRows.length > 0 && filteredRows.length === 0 && (
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>No loot matches the current filters.</p>
        )}

        {filteredRows.length > 0 && (
          <>
            <div className="grid gap-x-3 text-xs" style={{ gridTemplateColumns: 'auto 1fr auto auto', color: 'var(--color-muted-foreground)' }}>
              <span className="font-semibold border-b pb-1" style={{ borderColor: 'var(--color-border)' }}>Time</span>
              <span className="font-semibold border-b pb-1" style={{ borderColor: 'var(--color-border)' }}>Item</span>
              <span className="font-semibold border-b pb-1" style={{ borderColor: 'var(--color-border)' }}>Looted by</span>
              <span className="font-semibold border-b pb-1" style={{ borderColor: 'var(--color-border)' }}>Zone</span>
              {filteredRows.slice(0, displayLimit).map((r) => {
                const mine = r.player.toLowerCase() === characterName.toLowerCase()
                return (
                  <React.Fragment key={r.id}>
                    <span className="py-1 tabular-nums whitespace-nowrap" style={{ color: 'var(--color-muted)' }}>{formatTimestamp(r.ts)}</span>
                    <button
                      onClick={() => openItem(r.item)}
                      className="py-1 text-left hover:underline"
                      style={{ color: 'var(--color-primary)', background: 'transparent', border: 'none', cursor: 'pointer' }}
                      title="View item details"
                    >
                      {r.item}
                    </button>
                    <span className="py-1 whitespace-nowrap font-medium" style={{ color: mine ? 'var(--color-primary)' : 'var(--color-foreground)' }}>{r.player}</span>
                    {r.zone ? (
                      <button
                        onClick={() => openZone(r.zone)}
                        className="py-1 truncate text-left hover:underline"
                        style={{ color: 'var(--color-primary)', background: 'transparent', border: 'none', cursor: 'pointer' }}
                        title="Open this zone in the Zone browser"
                      >
                        {r.zone}
                      </button>
                    ) : (
                      <span className="py-1 truncate" style={{ color: 'var(--color-muted-foreground)' }}>—</span>
                    )}
                  </React.Fragment>
                )
              })}
            </div>
            {filteredRows.length > displayLimit && (
              <div className="mt-2 flex items-center gap-3">
                <button
                  onClick={() => setDisplayLimit((l) => l + DISPLAY_STEP)}
                  className="text-xs px-2 py-1 rounded"
                  style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
                >
                  Show {Math.min(DISPLAY_STEP, filteredRows.length - displayLimit)} more
                </button>
                <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  Showing {Math.min(displayLimit, filteredRows.length)} of {filteredRows.length}
                </span>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
