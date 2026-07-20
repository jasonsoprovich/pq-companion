import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Scale, Search, Star, Trash2, RefreshCw, AlertTriangle, Eye, X, ChevronDown, ChevronRight } from 'lucide-react'
import {
  listCharacters,
  searchFactions,
  listFactionWishlist,
  addFactionWishlistEntry,
  deleteFactionWishlistEntry,
  getFactionSession,
  resetFactionSession,
} from '../services/api'
import type { Faction, FactionSearchResult, FactionWishlistEntry, FactionTally } from '../types/faction'
import { BUCKET_ORDER, BUCKET_LABEL, BUCKET_COLOR, bucketIndex, type FactionBucket } from '../lib/factionBuckets'
import { useActiveCharacter } from '../contexts/ActiveCharacterContext'
import { useWebSocket } from '../hooks/useWebSocket'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { WSEvent } from '../lib/wsEvents'
import CharacterSubTabs from '../components/CharacterSubTabs'
import { ConfirmModal } from '../components/ConfirmModal'
import BackfillLink from '../components/BackfillLink'

// searchDebounceMs delays the faction-picker query until the user pauses
// typing — cheap against the ~2100-row faction_list table, but no reason to
// fire one request per keystroke.
const searchDebounceMs = 250

// bannerCollapsedKey persists the estimate-disclaimer banner's collapsed
// state across reloads — once a user's read it, it shouldn't reappear full-
// size every time they open the page.
const bannerCollapsedKey = 'pq-factions-banner-collapsed'

// BucketBar renders the nine classic EQ faction disposition ranges as a
// segmented scale, highlighting the faction's most recent /con reading.
// Bucket-level precision only — EQ never gives us a position within a
// bucket, so the marker sits at the whole segment, not an exact point.
function BucketBar({ bucket, suspect }: { bucket?: string; suspect?: boolean }): React.ReactElement {
  const idx = bucket ? bucketIndex(bucket) : -1
  return (
    <div className="flex items-center gap-1.5">
      <div className="flex flex-1 gap-0.5">
        {BUCKET_ORDER.map((b, i) => {
          const active = i === idx
          return (
            <div
              key={b}
              title={BUCKET_LABEL[b]}
              className="h-2 flex-1 rounded-sm transition-opacity"
              style={{
                backgroundColor: BUCKET_COLOR[b],
                opacity: idx === -1 ? 0.25 : active ? 1 : 0.2,
                outline: active ? '1px solid var(--color-foreground)' : 'none',
              }}
            />
          )
        })}
      </div>
      <span
        className="w-28 shrink-0 text-right text-[11px]"
        style={{ color: idx === -1 ? 'var(--color-muted-foreground)' : 'var(--color-foreground)' }}
        title={idx === -1 ? 'Consider an NPC of this faction in-game to set a baseline' : undefined}
      >
        {idx === -1 ? 'Needs /con' : BUCKET_LABEL[bucket as FactionBucket]}
      </span>
      {suspect && (
        <span title="Reading taken while illusioned — may not reflect your true faction">
          <Eye size={12} style={{ color: 'var(--color-warning, #f59e0b)' }} />
        </span>
      )}
    </div>
  )
}

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
      className="flex flex-col gap-2 rounded-lg px-4 py-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="flex items-center gap-3">
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
          title="Unpin this faction (its history keeps being tracked)"
          className="shrink-0 rounded p-1.5"
          style={{ color: 'var(--color-muted-foreground)' }}
        >
          <Trash2 size={14} />
        </button>
      </div>
      <BucketBar bucket={tally?.last_bucket} suspect={tally?.last_consider_suspect} />
    </div>
  )
}

// PreviewCard shows a temporarily-selected (not necessarily pinned) faction
// from search — clicking a result's name previews it here without requiring
// it be starred first. Mirrors TallyRow's layout but with a pin toggle and a
// dismiss button instead of an unpin-only trash icon.
function PreviewCard({
  faction,
  pinned,
  onTogglePin,
  onDismiss,
}: {
  faction: FactionSearchResult
  pinned: boolean
  onTogglePin: () => void
  onDismiss: () => void
}): React.ReactElement {
  const tally = faction.tally
  return (
    <div
      className="mb-4 flex flex-col gap-2 rounded-lg px-4 py-3"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px dashed var(--color-border)' }}
    >
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <div className="text-[10px] font-semibold uppercase tracking-wide" style={{ color: 'var(--color-muted-foreground)' }}>
            Preview
          </div>
          <div className="truncate text-sm font-medium" style={{ color: 'var(--color-foreground)' }}>
            {faction.name}
          </div>
          <div className="mt-1 flex items-center gap-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span style={{ color: 'var(--color-success)' }}>{tally?.better ?? 0} better</span>
            <span style={{ color: '#f87171' }}>{tally?.worse ?? 0} worse</span>
            {!tally && <span>No kill or /con data recorded yet</span>}
          </div>
        </div>
        <button
          type="button"
          onClick={onTogglePin}
          title={pinned ? 'Unpin this faction' : 'Pin this faction'}
          className="shrink-0 rounded p-1.5"
        >
          <Star size={14} fill={pinned ? 'var(--color-primary)' : 'none'} style={{ color: pinned ? 'var(--color-primary)' : 'var(--color-muted-foreground)' }} />
        </button>
        <button
          type="button"
          onClick={onDismiss}
          title="Dismiss preview"
          className="shrink-0 rounded p-1.5"
          style={{ color: 'var(--color-muted-foreground)' }}
        >
          <X size={14} />
        </button>
      </div>
      <BucketBar bucket={tally?.last_bucket} suspect={tally?.last_consider_suspect} />
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
  const [confirmClear, setConfirmClear] = useState(false)
  const [bannerCollapsed, setBannerCollapsed] = useState(() => {
    try { return localStorage.getItem(bannerCollapsedKey) === '1' } catch { return false }
  })
  const toggleBanner = (): void => {
    setBannerCollapsed((prev) => {
      const next = !prev
      try { localStorage.setItem(bannerCollapsedKey, next ? '1' : '0') } catch { /* noop */ }
      return next
    })
  }

  const [query, setQuery] = useState('')
  const [results, setResults] = useState<FactionSearchResult[]>([])
  const [searching, setSearching] = useState(false)
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const [preview, setPreview] = useState<FactionSearchResult | null>(null)
  const searchWrapRef = useRef<HTMLDivElement>(null)
  const dropdownVisible = dropdownOpen && query.trim() !== ''

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

  // The tracker engine only ever watches one character at a time — whichever
  // one's log is actively tailed — so *live* updates (new kills/con/faction
  // lines) only happen for the active character. A non-active character's
  // persisted history is still readable (getFactionSession(characterID)),
  // it just won't change again until that character becomes active — the WS
  // broadcast below only ever reflects the active character's engine.
  const isViewingActive = viewedCharacter !== '' && viewedCharacter.toLowerCase() === active.toLowerCase()

  useEffect(() => {
    if (!viewedCharID) {
      setTallies([])
      return
    }
    getFactionSession(isViewingActive ? undefined : viewedCharID)
      .then((s) => setTallies(s.tallies ?? []))
      .catch(() => setTallies([]))
  }, [viewedCharID, isViewingActive])

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
      searchFactions(query, viewedCharID || undefined)
        .then((r) => { if (seq === searchSeq.current) setResults(r.factions ?? []) })
        .catch(() => { if (seq === searchSeq.current) setResults([]) })
        .finally(() => { if (seq === searchSeq.current) setSearching(false) })
    }, searchDebounceMs)
    return () => clearTimeout(id)
  }, [query, viewedCharID])

  // Close the results dropdown on an outside click — previously nothing
  // dismissed it short of clearing the search text or navigating away and
  // back, trapping the user in the search input.
  useEffect(() => {
    if (!dropdownVisible) return
    const handler = (e: MouseEvent) => {
      if (!searchWrapRef.current?.contains(e.target as Node)) setDropdownOpen(false)
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [dropdownVisible])

  // Escape also closes the dropdown (without clearing what was typed, so
  // refocusing the input reopens it) — a guaranteed exit even if the
  // outside-click handler somehow misses.
  useEscapeToClose(() => setDropdownOpen(false), dropdownVisible)

  // Reset transient search UI when switching which character is being viewed.
  useEffect(() => {
    setQuery('')
    setDropdownOpen(false)
    setPreview(null)
  }, [viewedCharID])

  const trackedIDs = useMemo(() => new Set(entries.map((e) => e.faction_id)), [entries])

  const handleAdd = (faction: Faction): void => {
    if (!viewedCharID || trackedIDs.has(faction.id)) return
    addFactionWishlistEntry(viewedCharID, faction.id)
      .then((entry) => {
        setEntries((prev) => [...prev, entry])
        setPreview((prev) => (prev?.id === faction.id ? null : prev))
      })
      .catch((err: Error) => setError(err.message))
  }

  const handleRemove = (factionID: number): void => {
    if (!viewedCharID) return
    setEntries((prev) => prev.filter((e) => e.faction_id !== factionID))
    deleteFactionWishlistEntry(viewedCharID, factionID).catch((err: Error) => setError(err.message))
  }

  const handleClearHistory = (): void => {
    setConfirmClear(false)
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
          <BackfillLink />
          {isViewingActive && (
            <button
              onClick={() => setConfirmClear(true)}
              className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                color: 'var(--color-muted-foreground)',
                border: '1px solid var(--color-border)',
              }}
            >
              <RefreshCw size={11} />
              Clear history
            </button>
          )}
        </div>
      </div>

      <CharacterSubTabs value={viewedCharacter} onChange={setViewedCharacter} />

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <div
          className="mb-4 rounded-lg px-4 py-3"
          style={{
            border: '1px solid var(--color-warning, #f59e0b)',
            backgroundColor: 'color-mix(in srgb, var(--color-warning, #f59e0b) 12%, transparent)',
          }}
        >
          <button
            type="button"
            onClick={toggleBanner}
            className="flex w-full items-center gap-2 text-left"
            title={bannerCollapsed ? 'Expand' : 'Collapse'}
          >
            <AlertTriangle size={16} className="shrink-0" style={{ color: 'var(--color-warning, #f59e0b)' }} />
            <strong className="text-sm" style={{ color: 'var(--color-warning, #f59e0b)' }}>
              Estimate, not your real standing.
            </strong>
            <span className="ml-auto shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
              {bannerCollapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
            </span>
          </button>
          {!bannerCollapsed && (
            <p className="mt-2 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Every faction this character has ever killed toward or <code>/con</code>&rsquo;d is tracked
              automatically, the same way the Player and Lockout trackers record everything encountered —
              pinning just keeps a faction on this list. EQ never logs a faction&rsquo;s actual value or point
              amount, though: better/worse counts and the estimated net come from tallying &ldquo;got
              better/worse&rdquo; lines and, where possible, tying them to a resolved kill&rsquo;s known point
              value. The bar shows your last <code>/con</code> reading, which is real (bucket-level only) — a{' '}
              <Eye size={12} className="inline align-text-bottom" /> marker means that reading was taken
              while you had an illusion active and may not be reliable. <strong>Backfill</strong> only recovers
              this bar — the most recent <code>/con</code> reading per faction already in your log — not
              better/worse or estimated net, which only build up from here forward. And since that illusion
              check only runs live, a backfilled reading can never carry the suspect marker (or catch a
              faction-perception spell like Alliance on the NPC), even if it was taken under one at the time.
            </p>
          )}
        </div>

        {!viewedCharID ? (
          <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
            Select a character to track factions.
          </p>
        ) : (
          <>
            {!isViewingActive && (
              <p className="mb-3 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                Showing {viewedCharacter}&rsquo;s tracked history — it won&rsquo;t update further until
                this character becomes active ({active || 'none'} is currently active).
              </p>
            )}

            {/* Faction picker */}
            <div className="relative mb-4" ref={searchWrapRef}>
              <Search
                size={14}
                className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2"
                style={{ color: 'var(--color-muted-foreground)' }}
              />
              <input
                type="text"
                value={query}
                onChange={(e) => { setQuery(e.target.value); setDropdownOpen(true) }}
                onFocus={() => { if (query.trim()) setDropdownOpen(true) }}
                placeholder="Search factions…"
                className="w-full rounded-lg py-2 pl-9 pr-3 text-sm outline-none"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
              {dropdownVisible && (
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
                    const pinned = trackedIDs.has(f.id)
                    const t = f.tally
                    return (
                      <div
                        key={f.id}
                        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
                        style={{ color: 'var(--color-foreground)' }}
                      >
                        <button
                          type="button"
                          onClick={() => (pinned ? handleRemove(f.id) : handleAdd(f))}
                          title={pinned ? 'Unpin this faction' : 'Pin this faction'}
                          className="shrink-0"
                        >
                          <Star
                            size={13}
                            fill={pinned ? 'var(--color-primary)' : 'none'}
                            style={{ color: pinned ? 'var(--color-primary)' : 'var(--color-muted-foreground)' }}
                          />
                        </button>
                        <button
                          type="button"
                          onClick={() => { setPreview(f); setDropdownOpen(false) }}
                          title="Preview this faction"
                          className="min-w-0 flex-1 truncate text-left"
                        >
                          {f.name}
                        </button>
                        <span
                          className="shrink-0 text-xs"
                          style={{ color: 'var(--color-muted-foreground)' }}
                          title={
                            t
                              ? `${t.better} better / ${t.worse} worse`
                              : 'No kill or /con data recorded yet'
                          }
                        >
                          {t
                            ? t.last_bucket
                              ? BUCKET_LABEL[t.last_bucket as FactionBucket] ?? 'Has data'
                              : `${t.better + t.worse} event${t.better + t.worse === 1 ? '' : 's'}`
                            : 'No data yet'}
                        </span>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            {preview && (
              <PreviewCard
                faction={preview}
                pinned={trackedIDs.has(preview.id)}
                onTogglePin={() => (trackedIDs.has(preview.id) ? handleRemove(preview.id) : handleAdd(preview))}
                onDismiss={() => setPreview(null)}
              />
            )}

            {loading && (
              <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>Loading…</p>
            )}
            {error && (
              <p className="mb-3 text-sm" style={{ color: '#f87171' }}>{error}</p>
            )}
            {!loading && entries.length === 0 && (
              <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
                No factions pinned yet — search above to pin one. Every faction you kill toward or{' '}
                <code>/con</code> is still tracked automatically; pinning just keeps it on this list.
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

      {confirmClear && (
        <ConfirmModal
          title="Clear faction history?"
          message="This permanently discards the tracked better/worse counts, estimated net, and last /con reading for EVERY faction recorded for this character — not just pinned ones, every faction ever killed toward or considered. Pins themselves are unaffected — pinned factions stay pinned and start from zero again."
          confirmLabel="Clear history"
          tone="danger"
          onConfirm={handleClearHistory}
          onCancel={() => setConfirmClear(false)}
        />
      )}
    </div>
  )
}
