import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Map, MapPin, RefreshCw, AlertCircle, X } from 'lucide-react'
import { getShoppingRoute } from '../services/api'
import type { ShoppingRoute, ShoppingStop } from '../types/spell'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { formatNPCName } from './SourceNPCLink'

interface Props {
  spellIds: number[]
  onClose: () => void
}

// StopCard renders one zone in the itinerary: its order, name (links to the
// zone page), why it was chosen, the spells covered there, and the vendors
// carrying them with in-game coordinates.
function StopCard({ stop, index }: { stop: ShoppingStop; index: number }): React.ReactElement {
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

      {/* Spells covered at this stop */}
      <div className="mt-1.5 flex flex-wrap gap-1">
        {stop.spells.map((s) => (
          <span
            key={s.id}
            className="rounded px-1.5 py-0.5 text-[11px]"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}
          >
            {s.name}
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

export default function ShoppingRoutePanel({ spellIds, onClose }: Props): React.ReactElement {
  useEscapeToClose(onClose)
  const [route, setRoute] = useState<ShoppingRoute | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    getShoppingRoute(spellIds)
      .then((r) => { if (!cancelled) setRoute(r) })
      .catch((err: Error) => { if (!cancelled) setError(err.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [spellIds])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ backgroundColor: 'rgba(0,0,0,0.6)' }}
      onClick={onClose}
    >
      <div
        className="relative flex max-h-[80vh] w-full max-w-lg flex-col overflow-hidden rounded-lg"
        style={{ backgroundColor: 'var(--color-background)', border: '1px solid var(--color-border)' }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div
          className="flex shrink-0 items-center gap-2 px-5 pt-4 pb-3"
          style={{ borderBottom: '1px solid var(--color-border)' }}
        >
          <Map size={18} style={{ color: 'var(--color-primary)' }} />
          <h2 className="text-base font-bold" style={{ color: 'var(--color-primary)' }}>
            Shopping route
          </h2>
          {route && !loading && (
            <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              {route.total_spells} {route.total_spells === 1 ? 'spell' : 'spells'} across{' '}
              {route.total_zones} {route.total_zones === 1 ? 'zone' : 'zones'}
            </span>
          )}
          <div className="flex-1" />
          <button onClick={onClose} title="Close">
            <X size={16} style={{ color: 'var(--color-muted)' }} />
          </button>
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

          {!loading && !error && route && route.stops.length === 0 && route.unavailable.length === 0 && (
            <p className="py-8 text-center text-sm" style={{ color: 'var(--color-muted)' }}>
              Nothing to buy — every spell is already known or has no list entries.
            </p>
          )}

          {!loading && !error && route && route.stops.map((stop, i) => (
            <StopCard key={stop.zone_short} stop={stop} index={i} />
          ))}

          {/* Spells no vendor sells */}
          {!loading && !error && route && route.unavailable.length > 0 && (
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
                  <span
                    key={s.id}
                    className="rounded px-1.5 py-0.5 text-[11px]"
                    style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)' }}
                  >
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
        </div>
      </div>
    </div>
  )
}
