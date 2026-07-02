import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Package, RefreshCw, Trash2, AlertCircle, X, ArrowUp, ArrowDown } from 'lucide-react'
import { getLootMeta, listLoot, clearLoot, searchItems, searchZones } from '../services/api'
import type { LootEntry } from '../types/loot'
import type { Item } from '../types/item'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import MissingLogNotice from '../components/MissingLogNotice'
import BackfillLink from '../components/BackfillLink'
import ItemDetailModal from '../components/ItemDetailModal'

function formatTimestamp(unix: number): string {
  if (!unix) return ''
  return new Date(unix * 1000).toLocaleString([], {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

type SortField = 'time' | 'item' | 'player' | 'zone'
type SortDir = 'asc' | 'desc'

// Rows rendered before the "Show more" button — the tracker can hold
// thousands of entries and a 5-column grid row per entry makes the full
// list expensive to mount at once.
const DISPLAY_STEP = 500

export default function LootTrackerPage(): React.ReactElement {
  const [characters, setCharacters] = useState<string[]>([])
  const [selectedChar, setSelectedChar] = useState<string>('')
  const [metaLoaded, setMetaLoaded] = useState(false)
  const [players, setPlayers] = useState<string[]>([])
  const [zones, setZones] = useState<string[]>([])

  const [rows, setRows] = useState<LootEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [playerFilter, setPlayerFilter] = useState('')
  const [zoneFilter, setZoneFilter] = useState('')
  const [sortField, setSortField] = useState<SortField>('time')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [confirmClearOpen, setConfirmClearOpen] = useState(false)
  const [modalItem, setModalItem] = useState<Item | null>(null)
  const [modalOpen, setModalOpen] = useState(false)
  const navigate = useNavigate()

  // Resolve a loot item name to its DB record and open the item popup. Picks an
  // exact (case-insensitive) name match, falling back to the first result.
  async function openItem(name: string) {
    try {
      const r = await searchItems(name, 10)
      const exact = r.items.find((i) => i.name.toLowerCase() === name.toLowerCase())
      const item = exact ?? r.items[0]
      if (item) { setModalItem(item); setModalOpen(true) }
    } catch { /* best-effort */ }
  }

  // Resolve a zone long name to its DB record and jump to the Zone browser.
  async function openZone(zoneName: string) {
    try {
      const r = await searchZones(zoneName)
      const exact = r.items.find((z) => z.long_name.toLowerCase() === zoneName.toLowerCase())
      const zone = exact ?? r.items[0]
      if (zone) navigate(`/zones?select=${zone.id}`)
    } catch { /* best-effort */ }
  }

  const loadMeta = useCallback(() => {
    getLootMeta(selectedChar || undefined)
      .then((m) => {
        const chars = m.characters ?? []
        setCharacters(chars)
        setPlayers(m.players ?? [])
        setZones(m.zones ?? [])
        setSelectedChar((cur) => {
          if (cur && chars.includes(cur)) return cur
          if (m.active && chars.includes(m.active)) return m.active
          return chars[0] ?? m.active ?? ''
        })
      })
      .catch(() => { /* best effort */ })
      .finally(() => setMetaLoaded(true))
  }, [selectedChar])

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    listLoot({ character: selectedChar || undefined, search, player: playerFilter, zone: zoneFilter, sort: 'desc', limit: 5000 })
      .then((r) => setRows(r.loot))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [selectedChar, search, playerFilter, zoneFilter])

  // Wait for meta to resolve the active character before the first loot
  // fetch — loading immediately would fetch the unfiltered list, then
  // refetch for the resolved character, flashing the table twice.
  useEffect(() => { if (metaLoaded) load() }, [load, metaLoaded])

  // Refresh player/zone dropdowns when switching character.
  useEffect(() => { loadMeta() }, [selectedChar]) // eslint-disable-line react-hooks/exhaustive-deps

  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onWs = useCallback((msg: { type: string }) => {
    if (msg.type !== WSEvent.LootNew) return
    if (reloadTimer.current) clearTimeout(reloadTimer.current)
    reloadTimer.current = setTimeout(() => load(), 500)
  }, [load])
  useWebSocket(onWs)

  const showNPC = useMemo(() => rows.some((r) => r.npc), [rows])

  const [displayLimit, setDisplayLimit] = useState(DISPLAY_STEP)
  useEffect(() => { setDisplayLimit(DISPLAY_STEP) }, [selectedChar, search, playerFilter, zoneFilter])

  const sortedRows = useMemo(() => {
    const dir = sortDir === 'asc' ? 1 : -1
    const cmp = (a: LootEntry, b: LootEntry): number => {
      switch (sortField) {
        case 'time': return (a.ts - b.ts) * dir
        case 'item': return a.item.localeCompare(b.item) * dir
        case 'player': return a.player.localeCompare(b.player) * dir
        case 'zone': return (a.zone || '').localeCompare(b.zone || '') * dir
        default: return 0
      }
    }
    return [...rows].sort(cmp)
  }, [rows, sortField, sortDir])

  function handleSort(field: SortField) {
    if (sortField === field) setSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))
    else { setSortField(field); setSortDir(field === 'time' ? 'desc' : 'asc') }
  }

  async function doClear() {
    setConfirmClearOpen(false)
    try {
      await clearLoot(selectedChar || undefined)
      load()
      loadMeta()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const gridCols = showNPC ? 'auto 1fr auto auto auto' : 'auto 1fr auto auto'

  return (
    <div className="flex h-full flex-col">
      <ItemDetailModal item={modalItem} open={modalOpen} onClose={() => setModalOpen(false)} />

      {/* Header */}
      <div className="flex items-center gap-3 border-b px-4 py-3 shrink-0" style={{ borderColor: 'var(--color-border)' }}>
        <Package size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>Loot Tracker</span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>{rows.length} item{rows.length === 1 ? '' : 's'}</span>
        <div className="ml-auto flex items-center gap-2">
          <BackfillLink />
          <button onClick={() => { load(); loadMeta() }} className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}>
            <RefreshCw size={11} /> Refresh
          </button>
          <button onClick={() => setConfirmClearOpen(true)} disabled={rows.length === 0}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-danger)', border: '1px solid var(--color-border)' }}>
            <Trash2 size={11} /> Clear all
          </button>
        </div>
      </div>

      {/* Character tabs. The outer div carries the border; the inner scroll
          container overhangs it by 1px (instead of each tab carrying a
          negative margin) so the active tab's surface-colored bottom border
          covers the row border without creating a 1px vertical overflow —
          overflow-x:auto forces overflow-y to auto too, and that overflow
          rendered as a stray scrollbar on the right of the row. While meta
          is still loading, reserve the bar's height with one invisible tab
          so the content below doesn't jump when the tabs resolve. */}
      {!metaLoaded ? (
        <div className="border-b shrink-0" style={{ borderColor: 'var(--color-border)' }} aria-hidden>
          <div className="px-3 pt-2 flex items-end gap-1 overflow-x-auto" style={{ marginBottom: -1 }}>
            <button className="rounded-t px-3 py-1.5 text-xs font-medium whitespace-nowrap"
              style={{ visibility: 'hidden', border: '1px solid transparent' }} tabIndex={-1}>
              &nbsp;
            </button>
          </div>
        </div>
      ) : characters.length > 1 ? (
        <div className="border-b shrink-0" style={{ borderColor: 'var(--color-border)' }}>
          <div className="px-3 pt-2 flex items-end gap-1 overflow-x-auto" style={{ marginBottom: -1 }}>
            {characters.map((name) => {
              const active = name === selectedChar
              return (
                <button key={name} onClick={() => setSelectedChar(name)}
                  className="rounded-t px-3 py-1.5 text-xs font-medium whitespace-nowrap"
                  style={{
                    backgroundColor: active ? 'var(--color-surface)' : 'transparent',
                    color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                    border: '1px solid', borderColor: active ? 'var(--color-border)' : 'transparent',
                    borderBottom: active ? '1px solid var(--color-surface)' : '1px solid transparent',
                  }}>
                  {name}
                </button>
              )
            })}
          </div>
        </div>
      ) : null}

      {/* Filters */}
      <div className="border-b px-4 py-2 shrink-0 flex items-center gap-2 flex-wrap" style={{ borderColor: 'var(--color-border)' }}>
        <input type="text" value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search item or player…"
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)', minWidth: '14rem' }} />
        <select value={playerFilter} onChange={(e) => setPlayerFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}>
          <option value="">All looters</option>
          {players.map((p) => <option key={p} value={p}>{p}</option>)}
        </select>
        <select value={zoneFilter} onChange={(e) => setZoneFilter(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}>
          <option value="">All zones</option>
          {zones.map((z) => <option key={z} value={z}>{z}</option>)}
        </select>
        {(search || playerFilter || zoneFilter) && (
          <button onClick={() => { setSearch(''); setPlayerFilter(''); setZoneFilter('') }}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}>
            <X size={11} /> Clear filters
          </button>
        )}
      </div>

      {/* Body. Refetches (filter keystrokes, character switches, WS-driven
          reloads) keep the current table rendered and just dim it — swapping
          the whole table for a spinner made the page flash on every reload.
          The centered spinner only shows before the first rows arrive. */}
      <div className="flex-1 overflow-y-auto p-4">
        <MissingLogNotice />
        {loading && rows.length === 0 && (
          <div className="flex flex-1 items-center justify-center py-12">
            <RefreshCw size={18} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {error && !loading && (
          <div className="flex items-start gap-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <AlertCircle size={14} style={{ color: 'var(--color-danger)' }} />
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>
          </div>
        )}
        {!loading && !error && rows.length === 0 && (
          <div className="flex flex-col items-center justify-center gap-2 py-12">
            <Package size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>No loot tracked yet.</p>
            <p className="text-[11px] max-w-md text-center" style={{ color: 'var(--color-muted)' }}>
              Items looted by you and others in your group/raid are captured here as you play. To backfill from a character's log, use the <span className="font-medium">Backfill</span> button above (Settings → Logs).
            </p>
          </div>
        )}

        {!error && rows.length > 0 && (
          <div style={{ opacity: loading ? 0.6 : 1, transition: 'opacity 150ms' }}>
            <div className="grid gap-x-3 text-xs" style={{ gridTemplateColumns: gridCols, color: 'var(--color-muted-foreground)' }}>
              <SortHeader label="Time" field="time" activeField={sortField} dir={sortDir} onClick={handleSort} />
              <SortHeader label="Item" field="item" activeField={sortField} dir={sortDir} onClick={handleSort} />
              <SortHeader label="Looted by" field="player" activeField={sortField} dir={sortDir} onClick={handleSort} />
              <SortHeader label="Zone" field="zone" activeField={sortField} dir={sortDir} onClick={handleSort} />
              {showNPC && <span className="font-semibold border-b pb-1" style={{ color: 'var(--color-muted)', borderColor: 'var(--color-border)' }}>NPC</span>}
              {sortedRows.slice(0, displayLimit).map((r) => {
                const mine = r.player.toLowerCase() === selectedChar.toLowerCase()
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
                    {showNPC && <span className="py-1 truncate" style={{ color: 'var(--color-muted-foreground)' }}>{r.npc || '—'}</span>}
                  </React.Fragment>
                )
              })}
            </div>
            {sortedRows.length > displayLimit && (
              <div className="mt-3 flex items-center gap-3">
                <button onClick={() => setDisplayLimit((l) => l + DISPLAY_STEP)}
                  className="text-xs px-2 py-1 rounded"
                  style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}>
                  Show {Math.min(DISPLAY_STEP, sortedRows.length - displayLimit)} more
                </button>
                <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
                  Showing {displayLimit} of {sortedRows.length}
                </span>
              </div>
            )}
            {!showNPC && (
              <p className="mt-3 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                EverQuest loot lines don't name the source corpse, so the NPC column isn't available.
              </p>
            )}
          </div>
        )}
      </div>

      {confirmClearOpen && (
        <ConfirmModal
          title="Clear loot tracker?"
          body={`Delete all tracked loot for ${selectedChar || 'the active character'}. This cannot be undone.`}
          confirmLabel="Clear all"
          onCancel={() => setConfirmClearOpen(false)}
          onConfirm={doClear}
        />
      )}
    </div>
  )
}

function SortHeader({
  label, field, activeField, dir, onClick,
}: {
  label: string
  field: SortField
  activeField: SortField
  dir: SortDir
  onClick: (f: SortField) => void
}): React.ReactElement {
  const active = activeField === field
  return (
    <button onClick={() => onClick(field)} className="font-semibold border-b pb-1 flex items-center gap-1"
      style={{ color: active ? 'var(--color-foreground)' : 'var(--color-muted)', borderColor: 'var(--color-border)', background: 'transparent', cursor: 'pointer' }}>
      <span>{label}</span>
      {active && (dir === 'asc' ? <ArrowUp size={10} /> : <ArrowDown size={10} />)}
    </button>
  )
}

function ConfirmModal({
  title, body, confirmLabel, onCancel, onConfirm,
}: {
  title: string
  body: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: () => void
}): React.ReactElement {
  useEscapeToClose(onCancel)
  return (
    <div onClick={onCancel} style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}>
      <div onClick={(e) => e.stopPropagation()} className="rounded-lg p-4 space-y-3" style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', width: '100%', maxWidth: 420 }}>
        <div className="flex items-center gap-2">
          <AlertCircle size={16} style={{ color: 'var(--color-danger)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{title}</p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>{body}</p>
        <div className="flex justify-end gap-2 pt-1">
          <button onClick={onCancel} className="text-xs px-3 py-1.5 rounded font-medium" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}>Cancel</button>
          <button onClick={onConfirm} className="text-xs px-3 py-1.5 rounded font-medium" style={{ backgroundColor: 'var(--color-danger)', color: '#fff', border: '1px solid transparent' }}>{confirmLabel}</button>
        </div>
      </div>
    </div>
  )
}
