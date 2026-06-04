import React, { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Map as MapIcon, MapPin, RefreshCw, AlertCircle, X, Plus, Navigation } from 'lucide-react'
import { getShoppingRoute } from '../services/api'
import type { ShoppingRoute, ShoppingStop, ShoppingSpell, ZoneAlignment } from '../types/spell'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { formatNPCName } from './SourceNPCLink'

interface Props {
  spellIds: number[]
  classIndex: number // used to scope the per-class exclusion list
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
const lsExcludeKey = (classIndex: number) => `pq-companion:shop-excluded:${classIndex}`

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
  stop, index, onExcludeSpell,
}: {
  stop: ShoppingStop
  index: number
  onExcludeSpell: (s: ShoppingSpell) => void
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

export default function ShoppingRoutePanel({ spellIds, classIndex, onClose }: Props): React.ReactElement {
  useEscapeToClose(onClose)

  const [excluded, setExcluded] = useState<ShoppingSpell[]>(() => loadJSON(lsExcludeKey(classIndex), []))
  const [excludeAlignments, setExcludeAlignments] = useState<ZoneAlignment[]>(() => loadJSON(LS_ALIGN, []))
  const [startZone, setStartZone] = useState<string>(() => loadJSON<string>(LS_START, ''))

  const [route, setRoute] = useState<ShoppingRoute | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Persist preferences.
  useEffect(() => { saveJSON(lsExcludeKey(classIndex), excluded) }, [excluded, classIndex])
  useEffect(() => { saveJSON(LS_ALIGN, excludeAlignments) }, [excludeAlignments])
  useEffect(() => { saveJSON(LS_START, startZone) }, [startZone])

  const excludedIds = useMemo(() => new Set(excluded.map((s) => s.id)), [excluded])
  const activeIds = useMemo(() => spellIds.filter((id) => !excludedIds.has(id)), [spellIds, excludedIds])

  // Stable dependency keys so the fetch only re-runs on real changes.
  const activeKey = activeIds.join(',')
  const alignKey = [...excludeAlignments].sort().join(',')

  useEffect(() => {
    let cancelled = false
    if (activeIds.length === 0) {
      setRoute({ stops: [], unavailable: [], excluded_by_alignment: [], total_zones: 0, total_spells: 0 })
      setLoading(false)
      setError(null)
      return
    }
    setLoading(true)
    setError(null)
    getShoppingRoute(activeIds, { excludeAlignments, startZone })
      .then((r) => { if (!cancelled) setRoute(r) })
      .catch((err: Error) => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeKey, alignKey, startZone])

  const excludeSpell = (s: ShoppingSpell) =>
    setExcluded((prev) => (prev.some((e) => e.id === s.id) ? prev : [...prev, s]))
  const restoreSpell = (id: number) => setExcluded((prev) => prev.filter((e) => e.id !== id))

  const toggleAlignment = (a: ZoneAlignment) =>
    setExcludeAlignments((prev) => (prev.includes(a) ? prev.filter((x) => x !== a) : [...prev, a]))

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
        </div>

        {/* Body */}
        <div className="flex flex-1 flex-col gap-2 overflow-y-auto px-5 py-4">
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
              {activeIds.length === 0
                ? 'No spells selected — every spell has been removed below.'
                : 'No vendor route found for these spells.'}
            </p>
          )}

          {!loading && !error && route && route.stops.map((stop, i) => (
            <StopCard key={stop.zone_short} stop={stop} index={i} onExcludeSpell={excludeSpell} />
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

          {/* Spells the user removed — restorable */}
          {excluded.length > 0 && (
            <div
              className="mt-1 rounded border px-3 py-2"
              style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
            >
              <div className="mb-1 flex items-center justify-between text-xs font-semibold" style={{ color: 'var(--color-muted-foreground)' }}>
                <span>Removed from route ({excluded.length})</span>
                <button onClick={() => setExcluded([])} className="text-[11px] underline" style={{ color: 'var(--color-muted)' }}>
                  restore all
                </button>
              </div>
              <div className="flex flex-wrap gap-1">
                {excluded.map((s) => (
                  <button
                    key={s.id}
                    onClick={() => restoreSpell(s.id)}
                    className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px]"
                    style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
                    title="Add this spell back to the route"
                  >
                    <Plus size={10} />
                    {s.name || `Spell ${s.id}`}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
