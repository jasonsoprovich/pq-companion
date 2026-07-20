import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Scale, Search, Star, Trash2, RefreshCw, AlertTriangle } from 'lucide-react'
import {
  listCharacters,
  searchFactions,
  listFactionWishlist,
  addFactionWishlistEntry,
  deleteFactionWishlistEntry,
  getFactionSession,
  resetFactionSession,
} from '../services/api'
import type { Faction, FactionWishlistEntry, FactionTally } from '../types/faction'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import CharacterSubTabs from '../components/CharacterSubTabs'

// searchDebounceMs delays the faction-picker query until the user pauses
// typing — cheap against the ~2100-row faction_list table, but no reason to
// fire one request per keystroke.
const searchDebounceMs = 250

function TallyRow({
  entry,
  tally,
  onRemove,
}: {
  entry: FactionWishlistEntry
  tally?: FactionTally
  onRemove: () => void
}): React.ReactElement {
  const better = tally?.better ?? 0
  const worse = tally?.worse ?? 0
  const net = tally?.estimated_net ?? 0
  const unresolved = tally?.unresolved ?? 0
  const netColor =
    net > 0 ? 'var(--color-success)' : net < 0 ? '#f87171' : 'var(--color-muted-foreground)'

  return (
    <div
      className="flex items-center gap-3 rounded-lg px-4 py-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
          {entry.faction_name}
        </div>
        <div className="mt-1 flex items-center gap-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <span style={{ color: 'var(--color-success)' }}>{better} better</span>
          <span style={{ color: '#f87171' }}>{worse} worse</span>
          {unresolved > 0 && <span>{unresolved} unresolved</span>}
        </div>
      </div>
      <div className="shrink-0 text-right">
        <div className="text-xs uppercase tracking-wide" style={{ color: 'var(--color-muted-foreground)' }}>
          Est. net
        </div>
        <div className="text-sm font-semibold tabular-nums" style={{ color: netColor }}>
          {net > 0 ? '+' : ''}
          {net}
        </div>
      </div>
      <button
        type="button"
        onClick={onRemove}
        title="Stop tracking this faction"
        className="shrink-0 rounded p-1.5"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        <Trash2 size={14} />
      </button>
    </div>
  )
}

export default function FactionsPage(): React.ReactElement {
  const { active } = useActiveCharacter()
  const [viewedCharacter, setViewedCharacter] = useState('')
  const [characters, setCharacters] = useState<{ id: number; name: string }[]>([])
  const [entries, setEntries] = useState<FactionWishlistEntry[]>([])
  const [tallies, setTallies] = useState<FactionTally[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Faction[]>([])
  const [searching, setSearching] = useState(false)

  useEffect(() => {
    listCharacters().then((r) => setCharacters(r.characters)).catch(() => setCharacters([]))
  }, [])
  const viewedCharID = useMemo(
    () => characters.find((c) => c.name.toLowerCase() === viewedCharacter.toLowerCase())?.id ?? 0,
    [characters, viewedCharacter],
  )

  useEffect(() => {
    if (!viewedCharacter && active) setViewedCharacter(active)
  }, [active, viewedCharacter])

  const load = useCallback(() => {
    if (!viewedCharID) {
      setEntries([])
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    listFactionWishlist(viewedCharID)
      .then((r) => setEntries(r.entries ?? []))
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [viewedCharID])

  useEffect(() => { load() }, [load])

  // The session tally is global to the active character in the backend (the
  // tracker follows whichever character the log tailer is watching) — it
  // isn't per-viewed-character the way the wishlist itself is. Only load/show
  // it while viewing the active character.
  const isViewingActive = viewedCharacter !== '' && viewedCharacter.toLowerCase() === active.toLowerCase()

  useEffect(() => {
    if (!isViewingActive) {
      setTallies([])
      return
    }
    getFactionSession().then((s) => setTallies(s.tallies ?? [])).catch(() => setTallies([]))
  }, [isViewingActive])

  useWebSocket((msg) => {
    if (msg.type === WSEvent.OverlayFactions && isViewingActive) {
      setTallies((msg.data as { tallies: FactionTally[] }).tallies ?? [])
    }
  })

  const tallyByFactionID = useMemo(() => {
    const m = new Map<number, FactionTally>()
    for (const t of tallies) m.set(t.faction_id, t)
    return m
  }, [tallies])

  // Debounced faction-picker search.
  const searchSeq = useRef(0)
  useEffect(() => {
    if (!query.trim()) {
      setResults([])
      setSearching(false)
      return
    }
    setSearching(true)
    const seq = ++searchSeq.current
    const id = setTimeout(() => {
      searchFactions(query)
        .then((r) => { if (seq === searchSeq.current) setResults(r.factions ?? []) })
        .catch(() => { if (seq === searchSeq.current) setResults([]) })
        .finally(() => { if (seq === searchSeq.current) setSearching(false) })
    }, searchDebounceMs)
    return () => clearTimeout(id)
  }, [query])

  const trackedIDs = useMemo(() => new Set(entries.map((e) => e.faction_id)), [entries])

  const handleAdd = (faction: Faction): void => {
    if (!viewedCharID || trackedIDs.has(faction.id)) return
    addFactionWishlistEntry(viewedCharID, faction.id)
      .then((entry) => setEntries((prev) => [...prev, entry]))
      .catch((err: Error) => setError(err.message))
  }

  const handleRemove = (factionID: number): void => {
    if (!viewedCharID) return
    setEntries((prev) => prev.filter((e) => e.faction_id !== factionID))
    deleteFactionWishlistEntry(viewedCharID, factionID).catch((err: Error) => setError(err.message))
  }

  const handleReset = (): void => {
    resetFactionSession().then((s) => setTallies(s.tallies ?? [])).catch((err: Error) => setError(err.message))
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div
        className="flex items-center gap-2 border-b px-4 py-2.5 shrink-0"
        style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
      >
        <Scale size={16} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Factions
        </span>
        <div className="ml-auto flex items-center gap-2">
          {isViewingActive && (
            <button
              onClick={handleReset}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <RefreshCw size={11} />
              Reset session
            </button>
          )}
        </div>
      </div>

      <CharacterSubTabs value={viewedCharacter} onChange={setViewedCharacter} />

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <div
          className="mb-4 flex items-start gap-2 rounded-lg px-4 py-3"
          style={{
            border: '1px solid var(--color-warning, #f59e0b)',
            backgroundColor: 'color-mix(in srgb, var(--color-warning, #f59e0b) 12%, transparent)',
          }}
        >
          <AlertTriangle size={16} className="mt-0.5 shrink-0" style={{ color: 'var(--color-warning, #f59e0b)' }} />
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            <strong style={{ color: 'var(--color-warning, #f59e0b)' }}>Session-only estimate.</strong>{' '}
            EQ never logs a faction&rsquo;s real value or point amount — this tally counts
            &ldquo;got better/worse&rdquo; lines for the factions below and adds a best-effort
            point estimate only when a change can be tied to a resolved kill. It resets on
            every restart and character switch, and can&rsquo;t tell you your actual standing.
          </p>
        </div>

        {!viewedCharID ? (
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Select a character to track factions.
          </p>
        ) : (
          <>
            {!isViewingActive && (
              <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Live session tallies only run for the active character ({active || 'none'}).
                You can still edit this character&rsquo;s wishlist here.
              </p>
            )}

            {/* Faction picker */}
            <div className="relative mb-4">
              <Search
                size={14}
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2"
                style={{ color: 'var(--color-muted-foreground)' }}
              />
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search factions to track…"
                className="w-full rounded-lg py-2 pl-9 pr-3 text-sm outline-none"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
              {query.trim() && (
                <div
                  className="absolute z-10 mt-1 max-h-64 w-full overflow-y-auto rounded-lg shadow-lg"
                  style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
                >
                  {searching && (
                    <div className="px-3 py-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      Searching…
                    </div>
                  )}
                  {!searching && results.length === 0 && (
                    <div className="px-3 py-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                      No matching factions.
                    </div>
                  )}
                  {results.map((f) => {
                    const tracked = trackedIDs.has(f.id)
                    return (
                      <button
                        key={f.id}
                        type="button"
                        disabled={tracked}
                        onClick={() => handleAdd(f)}
                        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
                        style={{
                          color: tracked ? 'var(--color-muted-foreground)' : 'var(--color-foreground)',
                          cursor: tracked ? 'default' : 'pointer',
                        }}
                      >
                        <Star size={13} style={{ color: tracked ? 'var(--color-primary)' : 'var(--color-muted-foreground)' }} />
                        {f.name}
                        {tracked && <span className="ml-auto text-xs">Tracked</span>}
                      </button>
                    )
                  })}
                </div>
              )}
            </div>

            {loading && (
              <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
            )}
            {error && (
              <p className="mb-3 text-sm" style={{ color: '#f87171' }}>{error}</p>
            )}
            {!loading && entries.length === 0 && (
              <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                No factions tracked yet — search above to star one.
              </p>
            )}

            <div className="flex flex-col gap-2">
              {entries.map((e) => (
                <TallyRow
                  key={e.id}
                  entry={e}
                  tally={tallyByFactionID.get(e.faction_id)}
                  onRemove={() => handleRemove(e.faction_id)}
                />
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
