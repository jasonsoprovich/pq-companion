import React, { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Map as MapIcon, MapPin, RefreshCw, AlertCircle, X, Navigation, BookOpen, Ban, ChevronDown } from 'lucide-react'
import { getShoppingRoute } from '../services/api'
import type { ShoppingRoute, ShoppingStop, ShoppingSpell, ShoppingCandidateZone, ZoneAlignment } from '../types/spell'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { usePoPEnabled } from '../hooks/usePoPEnabled'
import { formatNPCName } from './SourceNPCLink'

interface Props {
  // The selected spells to plan for. Selection is owned by the checklist page
  // (per character) — this panel just renders the route for what it's given.
  spellIds: number[]
  // Remove a spell from the run; the page drops it from the selection and
  // re-passes a shorter spellIds, which re-plans the route.
  onRemoveSpell: (id: number) => void
  onClose: () => void
}

// Common starting points offered in the dropdown, in addition to whatever zones
// the route itself visits. Mostly neutral hubs plus the major hometowns.
const START_HUBS: { short: string; name: string }[] = [
  { short: 'poknowledge', name: 'Plane of Knowledge' },
  { short: 'bazaar', name: 'Bazaar' },
  { short: 'nexus', name: 'Nexus' },
  { short: 'shadowhaven', name: 'Shadow Haven' },
  { short: 'qeynos2', name: 'South Qeynos' },
  { short: 'freportw', name: 'West Freeport' },
  { short: 'gfaydark', name: 'Greater Faydark (Kelethin)' },
  { short: 'neriakb', name: 'Neriak Commons' },
]

const ALIGNMENTS: { key: ZoneAlignment; label: string }[] = [
  { key: 'good', label: 'Good' },
  { key: 'neutral', label: 'Neutral' },
  { key: 'evil', label: 'Evil' },
]

const LS_ALIGN = 'pq-companion:shop-exclude-alignments'
const LS_START = 'pq-companion:shop-start-zone'
const LS_POK = 'pq-companion:shop-include-pok'
const LS_ZONES = 'pq-companion:shop-exclude-zones'

const EMPTY_ROUTE: ShoppingRoute = {
  stops: [], unavailable: [], excluded_by_alignment: [], excluded_by_expansion: [],
  excluded_by_zone: [], candidate_zones: [], total_zones: 0, total_spells: 0,
}

function loadJSON<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(key)
    if (v) return JSON.parse(v) as T
  } catch {
    // ignore
  }
  return fallback
}
function saveJSON(key: string, value: unknown) {
  try { localStorage.setItem(key, JSON.stringify(value)) } catch { /* ignore */ }
}

const alignmentColor: Record<ZoneAlignment, string> = {
  good: '#4ade80',
  neutral: 'var(--color-muted)',
  evil: '#f87171',
}

// StopCard renders one zone in the itinerary.
function StopCard({
  stop, index, onExcludeSpell, onExcludeZone,
}: {
  stop: ShoppingStop
  index: number
  onExcludeSpell: (s: ShoppingSpell) => void
  onExcludeZone: (zoneShort: string) => void
}): React.ReactElement {
  const navigate = useNavigate()
  return (
    <div
      className="rounded border px-3 py-2"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      <div className="flex items-center gap-2">
        <span
          className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold tabular-nums"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-primary)' }}
        >
          {index + 1}
        </span>
        <button
          onClick={() => navigate(`/zones?select=${stop.zone_short}`)}
          className="text-sm font-semibold underline decoration-dotted"
          style={{ color: 'var(--color-primary)' }}
        >
          {stop.zone_name || stop.zone_short}
        </button>
        <span
          className="text-[10px] font-semibold uppercase tracking-wide"
          style={{ color: alignmentColor[stop.alignment] }}
          title={`${stop.alignment} town`}
        >
          {stop.alignment}
        </span>
        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>
          {stop.spells.length} {stop.spells.length === 1 ? 'spell' : 'spells'}
        </span>
        {stop.reason === 'anchor' && (
          <span
            className="rounded px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
            title="This zone is the only source of at least one spell, so the trip is mandatory."
          >
            only source
          </span>
        )}
        <div className="flex-1" />
        <button
          onClick={() => onExcludeZone(stop.zone_short)}
          className="shrink-0 opacity-50 transition-opacity hover:opacity-100"
          style={{ color: 'var(--color-muted)' }}
          title="Skip this town — re-route its spells to the next-best source"
        >
          <Ban size={13} />
        </button>
      </div>

      {/* Spells covered at this stop — each removable */}
      <div className="mt-1.5 flex flex-wrap gap-1">
        {stop.spells.map((s) => (
          <span
            key={s.id}
            className="group flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px]"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
          >
            {s.name}
            <button
              onClick={() => onExcludeSpell(s)}
              className="opacity-40 transition-opacity hover:opacity-100"
              title="Remove this spell from the route"
            >
              <X size={10} />
            </button>
          </span>
        ))}
      </div>

      {/* Vendors */}
      {stop.vendors.length > 0 && (
        <div className="mt-2 flex flex-col gap-0.5">
          {stop.vendors.map((v) => (
            <div key={v.id} className="flex items-center justify-between gap-3 text-sm">
              <button
                onClick={() => navigate(`/npcs?select=${v.id}`)}
                className="min-w-0 truncate text-left underline decoration-dotted"
                style={{ color: 'var(--color-primary)' }}
              >
                {formatNPCName(v.name)}
              </button>
              <span
                className="flex shrink-0 items-center gap-1 text-xs tabular-nums"
                style={{ color: 'var(--color-muted)' }}
                title="In-game location (X, Y)"
              >
                <MapPin size={10} />
                {Math.round(v.x)}, {Math.round(v.y)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default function ShoppingRoutePanel({ spellIds, onRemoveSpell, onClose }: Props): React.ReactElement {
  useEscapeToClose(onClose)

  // Once the Planes of Power era flag is on, PoK is a normal source (the
  // backend ignores include_pok) and the opt-in toggle below is hidden.
  const popEnabled = usePoPEnabled()

  const [excludeAlignments, setExcludeAlignments] = useState<ZoneAlignment[]>(() => loadJSON(LS_ALIGN, []))
  // Default to the Nexus: it's the common bind point and the teleport hub, so
  // out of the box the route reflects easy travel from there.
  const [startZone, setStartZone] = useState<string>(() => loadJSON<string>(LS_START, 'nexus'))
  // Plane of Knowledge is off by default — the Planes of Power hub isn't on this
  // server's timeline yet, so it shouldn't be a recommended source.
  const [includePoK, setIncludePoK] = useState<boolean>(() => loadJSON<boolean>(LS_POK, false))
  // Towns the player never wants routed through (persisted preference).
  const [excludeZones, setExcludeZones] = useState<string[]>(() => loadJSON<string[]>(LS_ZONES, []))
  const [showZonePicker, setShowZonePicker] = useState(false)

  const [route, setRoute] = useState<ShoppingRoute | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Persist preferences. (The spell selection itself is owned and persisted by
  // the checklist page, per character.)
  useEffect(() => { saveJSON(LS_ALIGN, excludeAlignments) }, [excludeAlignments])
  useEffect(() => { saveJSON(LS_START, startZone) }, [startZone])
  useEffect(() => { saveJSON(LS_POK, includePoK) }, [includePoK])
  useEffect(() => { saveJSON(LS_ZONES, excludeZones) }, [excludeZones])

  // Stable dependency keys so the fetch only re-runs on real changes.
  const activeKey = spellIds.join(',')
  const alignKey = [...excludeAlignments].sort().join(',')
  const zonesKey = [...excludeZones].sort().join(',')

  useEffect(() => {
    let cancelled = false
    if (spellIds.length === 0) {
      setRoute(EMPTY_ROUTE)
      setLoading(false)
      setError(null)
      return
    }
    setLoading(true)
    setError(null)
    getShoppingRoute(spellIds, { excludeAlignments, startZone, includePoK, excludeZones })
      .then((r) => { if (!cancelled) setRoute(r) })
      .catch((err: Error) => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeKey, alignKey, startZone, includePoK, zonesKey])

  const toggleAlignment = (a: ZoneAlignment) =>
    setExcludeAlignments((prev) => (prev.includes(a) ? prev.filter((x) => x !== a) : [...prev, a]))

  const excludedZoneSet = useMemo(() => new Set(excludeZones), [excludeZones])
  const toggleZone = (short: string) =>
    setExcludeZones((prev) => (prev.includes(short) ? prev.filter((z) => z !== short) : [...prev, short]))

  // Resolve an excluded zone's display name from the candidate list (it may not
  // be in the current route once excluded), falling back to the short name.
  const zoneNameOf = (short: string) =>
    route?.candidate_zones.find((c) => c.zone_short === short)?.zone_name || short

  // Start-zone options: hubs + any zone the current route visits, deduped.
  const startOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const h of START_HUBS) seen.set(h.short, h.name)
    for (const stop of route?.stops ?? []) {
      if (!seen.has(stop.zone_short)) seen.set(stop.zone_short, stop.zone_name || stop.zone_short)
    }
    return Array.from(seen, ([short, name]) => ({ short, name }))
  }, [route])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="relative flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-lg"
        style={{ backgroundColor: 'var(--color-background)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="flex shrink-0 items-center gap-2 px-5 pt-4 pb-3"
          style={{ borderBottom: '1px solid var(--color-border)' }}
        >
          <MapIcon size={18} style={{ color: 'var(--color-primary)' }} />
          <h2 className="text-base font-bold" style={{ color: 'var(--color-primary)' }}>
            Shopping route
          </h2>
          {route && !loading && (
            <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {route.total_spells} {route.total_spells === 1 ? 'spell' : 'spells'} ·{' '}
              {route.total_zones} {route.total_zones === 1 ? 'zone' : 'zones'}
            </span>
          )}
          <div className="flex-1" />
          <button onClick={onClose} title="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
        </div>

        {/* Controls */}
        <div
          className="flex shrink-0 flex-wrap items-center gap-x-4 gap-y-2 px-5 py-2.5"
          style={{ borderBottom: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)' }}
        >
          {/* Start zone */}
          <label className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <Navigation size={12} />
            Start
            <select
              value={startZone}
              onChange={(e) => setStartZone(e.target.value)}
              className="rounded px-1.5 py-0.5 text-xs outline-none"
              style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
            >
              <option value="">Anywhere</option>
              {startOptions.map((o) => (
                <option key={o.short} value={o.short}>{o.name}</option>
              ))}
            </select>
          </label>

          {/* Alignment filter */}
          <div className="flex items-center gap-1.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            <span>Towns</span>
            <div className="flex overflow-hidden rounded" style={{ border: '1px solid var(--color-border)' }}>
              {ALIGNMENTS.map((a, i) => {
                const included = !excludeAlignments.includes(a.key)
                return (
                  <button
                    key={a.key}
                    onClick={() => toggleAlignment(a.key)}
                    className="px-2 py-0.5 text-[11px] transition-colors"
                    style={{
                      backgroundColor: included ? 'var(--color-surface-2)' : 'transparent',
                      color: included ? alignmentColor[a.key] : 'var(--color-muted)',
                      textDecoration: included ? 'none' : 'line-through',
                      borderRight: i < ALIGNMENTS.length - 1 ? '1px solid var(--color-border)' : 'none',
                    }}
                    title={included ? `Including ${a.label.toLowerCase()} towns — click to exclude` : `Excluding ${a.label.toLowerCase()} towns — click to include`}
                  >
                    {a.label}
                  </button>
                )
              })}
            </div>
          </div>

          {/* Plane of Knowledge source toggle (off by default — not on this
              server's timeline yet). Once the PoP era flag is on, PoK is a
              normal source server-side, so the opt-in toggle disappears. */}
          {!popEnabled && <button
            onClick={() => setIncludePoK((v) => !v)}
            className="flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] transition-colors"
            style={{
              backgroundColor: includePoK ? 'var(--color-surface-2)' : 'transparent',
              color: includePoK ? 'var(--color-primary)' : 'var(--color-muted)',
              border: '1px solid var(--color-border)',
            }}
            title={includePoK
              ? 'Including Plane of Knowledge as a source — click to disable'
              : 'Plane of Knowledge is disabled (not on this server yet) — click to include it'}
          >
            <BookOpen size={11} />
            Plane of Knowledge
          </button>}

          {/* Skip-towns picker toggle */}
          <button
            onClick={() => setShowZonePicker((v) => !v)}
            className="flex items-center gap-1.5 rounded px-2 py-0.5 text-[11px] transition-colors"
            style={{
              backgroundColor: excludeZones.length > 0 ? 'var(--color-surface-2)' : 'transparent',
              color: excludeZones.length > 0 ? '#f87171' : 'var(--color-muted)',
              border: '1px solid var(--color-border)',
            }}
            title="Choose towns to never route through"
          >
            <Ban size={11} />
            Skip towns{excludeZones.length > 0 ? ` (${excludeZones.length})` : ''}
            <ChevronDown size={11} style={{ transform: showZonePicker ? 'rotate(180deg)' : 'none' }} />
          </button>
        </div>

        {/* Skip-towns picker: every candidate source town, checkable */}
        {showZonePicker && (
          <div
            className="shrink-0 max-h-48 overflow-y-auto px-5 py-2"
            style={{ borderBottom: '1px solid var(--color-border)', backgroundColor: 'var(--color-surface)' }}
          >
            {(route?.candidate_zones.length ?? 0) === 0 ? (
              <p className="py-2 text-center text-[11px]" style={{ color: 'var(--color-muted)' }}>
                No candidate towns to skip.
              </p>
            ) : (
              <div className="flex flex-col gap-0.5">
                {route?.candidate_zones.map((c) => {
                  const excluded = excludedZoneSet.has(c.zone_short)
                  return (
                    <label
                      key={c.zone_short}
                      className="flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 text-xs"
                      style={{ color: excluded ? 'var(--color-muted)' : 'var(--color-foreground)' }}
                    >
                      <input
                        type="checkbox"
                        checked={excluded}
                        onChange={() => toggleZone(c.zone_short)}
                      />
                      <span className="flex-1 truncate" style={{ textDecoration: excluded ? 'line-through' : 'none' }}>
                        {c.zone_name || c.zone_short}
                      </span>
                      <span className="text-[10px] uppercase tracking-wide" style={{ color: alignmentColor[c.alignment] }}>
                        {c.alignment}
                      </span>
                      <span className="tabular-nums text-[10px]" style={{ color: 'var(--color-muted)' }}>
                        {c.spell_count} {c.spell_count === 1 ? 'spell' : 'spells'}
                      </span>
                    </label>
                  )
                })}
              </div>
            )}
          </div>
        )}

        {/* Body */}
        <div className="flex flex-1 flex-col gap-2 overflow-y-auto px-5 py-4">
          {/* Skipped-towns summary — each chip restores the town on click */}
          {excludeZones.length > 0 && (
            <div className="flex flex-wrap items-center gap-1 text-[11px]">
              <span style={{ color: 'var(--color-muted)' }}>Skipping:</span>
              {excludeZones.map((z) => (
                <button
                  key={z}
                  onClick={() => toggleZone(z)}
                  className="flex items-center gap-1 rounded px-1.5 py-0.5"
                  style={{ backgroundColor: 'var(--color-surface-2)', color: '#f87171' }}
                  title="Stop skipping this town"
                >
                  {zoneNameOf(z)}
                  <X size={10} />
                </button>
              ))}
              <button
                onClick={() => setExcludeZones([])}
                className="ml-1 underline"
                style={{ color: 'var(--color-muted)' }}
              >
                restore all
              </button>
            </div>
          )}

          {loading && (
            <div className="flex items-center justify-center py-10">
              <RefreshCw size={20} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          )}

          {!loading && error && (
            <div className="flex flex-col items-center gap-2 py-8">
              <AlertCircle size={28} style={{ color: 'var(--color-danger)' }} />
              <p className="text-center text-sm" style={{ color: 'var(--color-muted-foreground)' }}>{error}</p>
            </div>
          )}

          {!loading && !error && route && route.stops.length === 0 && (
            <p className="py-8 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
              {spellIds.length === 0
                ? 'No spells selected — pick some spells on the checklist to plan a run.'
                : 'No vendor route found for these spells.'}
            </p>
          )}

          {!loading && !error && route && route.stops.map((stop, i) => (
            <StopCard
              key={stop.zone_short}
              stop={stop}
              index={i}
              onExcludeSpell={(s) => onRemoveSpell(s.id)}
              onExcludeZone={toggleZone}
            />
          ))}

          {/* Spells whose only towns were filtered out by alignment */}
          {!loading && !error && route && (route.excluded_by_alignment?.length ?? 0) > 0 && (
            <div
              className="mt-1 rounded border px-3 py-2"
              style={{ backgroundColor: 'rgba(248,113,113,0.06)', borderColor: 'rgba(248,113,113,0.3)' }}
            >
              <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold" style={{ color: '#f87171' }}>
                <AlertCircle size={12} />
                Only sold in filtered-out towns ({route.excluded_by_alignment.length})
              </div>
              <div className="flex flex-wrap gap-1">
                {route.excluded_by_alignment.map((s) => (
                  <span key={s.id} className="rounded px-1.5 py-0.5 text-[11px]" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
                    {s.name || `Spell ${s.id}`}
                  </span>
                ))}
              </div>
              <p className="mt-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Re-enable the relevant town alignment above to include these.
              </p>
            </div>
          )}

          {/* Spells whose only towns are ones the player chose to skip */}
          {!loading && !error && route && (route.excluded_by_zone?.length ?? 0) > 0 && (
            <div
              className="mt-1 rounded border px-3 py-2"
              style={{ backgroundColor: 'rgba(248,113,113,0.06)', borderColor: 'rgba(248,113,113,0.3)' }}
            >
              <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold" style={{ color: '#f87171' }}>
                <Ban size={12} />
                Only sold in towns you're skipping ({route.excluded_by_zone.length})
              </div>
              <div className="flex flex-wrap gap-1">
                {route.excluded_by_zone.map((s) => (
                  <span key={s.id} className="rounded px-1.5 py-0.5 text-[11px]" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
                    {s.name || `Spell ${s.id}`}
                  </span>
                ))}
              </div>
              <p className="mt-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Restore one of the skipped towns above to route these.
              </p>
            </div>
          )}

          {/* Spells whose only source is a zone that isn't released yet (PoK) */}
          {!loading && !error && route && (route.excluded_by_expansion?.length ?? 0) > 0 && (
            <div
              className="mt-1 rounded border px-3 py-2"
              style={{ backgroundColor: 'rgba(96,165,250,0.06)', borderColor: 'rgba(96,165,250,0.3)' }}
            >
              <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold" style={{ color: '#60a5fa' }}>
                <BookOpen size={12} />
                Only sold in Plane of Knowledge ({route.excluded_by_expansion.length})
              </div>
              <div className="flex flex-wrap gap-1">
                {route.excluded_by_expansion.map((s) => (
                  <span key={s.id} className="rounded px-1.5 py-0.5 text-[11px]" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
                    {s.name || `Spell ${s.id}`}
                  </span>
                ))}
              </div>
              <p className="mt-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                Plane of Knowledge isn't on this server yet. Toggle it on above to
                route there anyway.
              </p>
            </div>
          )}

          {/* Spells no vendor sells anywhere */}
          {!loading && !error && route && (route.unavailable?.length ?? 0) > 0 && (
            <div
              className="mt-1 rounded border px-3 py-2"
              style={{ backgroundColor: 'rgba(255,200,50,0.06)', borderColor: 'rgba(255,200,50,0.3)' }}
            >
              <div className="mb-1 flex items-center gap-1.5 text-xs font-semibold" style={{ color: '#f59e0b' }}>
                <AlertCircle size={12} />
                Not sold by any vendor ({route.unavailable.length})
              </div>
              <div className="flex flex-wrap gap-1">
                {route.unavailable.map((s) => (
                  <span key={s.id} className="rounded px-1.5 py-0.5 text-[11px]" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}>
                    {s.name || `Spell ${s.id}`}
                  </span>
                ))}
              </div>
              <p className="mt-1.5 text-[11px]" style={{ color: 'var(--color-muted)' }}>
                These come from drops, tradeskills, or quests — check each spell's
                "How to acquire" for details.
              </p>
            </div>
          )}

          {/* Removing a spell here unchecks it on the checklist; re-add it there. */}
        </div>
      </div>
    </div>
  )
}
