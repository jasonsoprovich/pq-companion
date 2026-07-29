import React, { useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Check, Copy, Map as MapIcon, Search, X } from 'lucide-react'
import { getMapZones } from '../services/api'
import { useCachedState } from '../hooks/useCachedState'
import { useZoneMap } from '../hooks/useZoneMap'
import { ZoneMap } from '../components/maps/ZoneMap'
import { mapMarkerCommand, mapShowZoneCommand } from '../lib/zealMap'
import type { MapPOI, MapPOICategory, MapZone } from '../types/map'

// Layer toggles. Defaults are the categories a player is usually hunting for;
// doors, ground spawns and tradeskill containers are off because switching them
// all on at once is what makes dense zones unreadable.
const LAYERS: { key: MapPOICategory; label: string; on: boolean }[] = [
  { key: 'vendor', label: 'Vendors', on: true },
  { key: 'raid_target', label: 'Raid targets', on: true },
  { key: 'zone_line', label: 'Zone lines', on: true },
  { key: 'trap', label: 'Traps', on: true },
  { key: 'succor', label: 'Succor', on: false },
  { key: 'door', label: 'Doors', on: false },
  { key: 'ground_spawn', label: 'Ground spawns', on: false },
  { key: 'tradeskill', label: 'Tradeskills', on: false },
]

export default function MapsPage(): React.ReactElement {
  const [params, setParams] = useSearchParams()
  const navigate = useNavigate()
  const [zones, setZones] = useState<MapZone[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [query, setQuery] = useCachedState('maps.query', '')
  const [enabled, setEnabled] = useCachedState<MapPOICategory[]>(
    'maps.layers',
    LAYERS.filter((l) => l.on).map((l) => l.key),
  )
  const [selected, setSelected] = useState<MapPOI | null>(null)
  const [copied, setCopied] = useState<string | null>(null)

  const zoneName = params.get('zone')
  const { zone, geometry, pois, loading, error } = useZoneMap(zoneName)

  useEffect(() => {
    getMapZones()
      .then((r) => setZones(r.zones ?? []))
      .catch((e: Error) => setLoadErr(e.message))
  }, [])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = q ? zones.filter((z) => z.zone.toLowerCase().includes(q)) : zones
    return [...list].sort((a, b) => a.zone.localeCompare(b.zone))
  }, [zones, query])

  const visible = useMemo(() => new Set(enabled), [enabled])

  const counts = useMemo(() => {
    const m: Partial<Record<MapPOICategory, number>> = {}
    for (const p of pois) m[p.category] = (m[p.category] ?? 0) + 1
    return m
  }, [pois])

  const copy = (key: string, text: string): void => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  if (loadErr) {
    return (
      <div className="p-4">
        <p className="text-sm" style={{ color: 'var(--color-destructive)' }}>{loadErr}</p>
      </div>
    )
  }

  // A build without maps.db reports zero zones. Say so plainly rather than
  // showing an empty picker that looks broken.
  if (zones.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          No map data installed.
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-full">
      {/* Zone picker */}
      <div className="flex w-60 shrink-0 flex-col border-r" style={{ borderColor: 'var(--color-border)' }}>
        <div className="flex items-center gap-2 border-b px-3 py-2" style={{ borderColor: 'var(--color-border)' }}>
          <Search size={14} style={{ color: 'var(--color-muted)' }} className="shrink-0" />
          <input
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-(--color-muted)"
            style={{ color: 'var(--color-foreground)' }}
            placeholder="Search zones…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            spellCheck={false}
          />
          {query && (
            <button onClick={() => setQuery('')}><X size={12} style={{ color: 'var(--color-muted)' }} /></button>
          )}
        </div>
        <div className="flex-1 overflow-y-auto">
          {filtered.map((z) => (
            <button
              key={z.zone}
              onClick={() => { setSelected(null); setParams({ zone: z.zone }) }}
              className="w-full px-3 py-1.5 text-left text-sm transition-colors"
              style={{
                backgroundColor: z.zone === zoneName ? 'var(--color-surface-2)' : 'transparent',
                borderLeft: z.zone === zoneName ? '2px solid var(--color-primary)' : '2px solid transparent',
                color: z.zone === zoneName ? 'var(--color-primary)' : 'var(--color-foreground)',
              }}
            >
              {z.zone}
            </button>
          ))}
        </div>
      </div>

      {/* Map */}
      <div className="flex min-w-0 flex-1 flex-col gap-2 p-3">
        <div className="flex items-center gap-2">
          <MapIcon size={16} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold">{zone?.zone ?? 'Select a zone'}</h1>
          {loading && <span className="text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</span>}
          {zone && (
            <button
              onClick={() => copy('showzone', mapShowZoneCommand(zone.zone))}
              title="Copy /map show_zone — preview this zone on your in-game map"
              className="ml-auto flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
              style={{
                backgroundColor: 'var(--color-surface)',
                borderColor: copied === 'showzone' ? 'var(--color-primary)' : 'var(--color-border)',
                color: copied === 'showzone' ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
              }}
            >
              {copied === 'showzone' ? <Check size={9} /> : <Copy size={9} />}
              {copied === 'showzone' ? 'Copied' : 'Show in game'}
            </button>
          )}
        </div>

        {error && <p className="text-sm" style={{ color: 'var(--color-destructive)' }}>{error}</p>}

        {zone && (
          <>
            <div className="flex flex-wrap gap-1">
              {LAYERS.map((l) => {
                const n = counts[l.key] ?? 0
                const active = visible.has(l.key)
                return (
                  <button
                    key={l.key}
                    disabled={n === 0}
                    onClick={() =>
                      setEnabled((prev) =>
                        prev.includes(l.key) ? prev.filter((k) => k !== l.key) : [...prev, l.key],
                      )
                    }
                    className="rounded border px-1.5 py-0.5 text-[10px] font-medium disabled:opacity-30"
                    style={{
                      backgroundColor: active ? 'var(--color-surface-2)' : 'transparent',
                      borderColor: active ? 'var(--color-primary)' : 'var(--color-border)',
                      color: active ? 'var(--color-primary)' : 'var(--color-muted)',
                    }}
                  >
                    {l.label} {n > 0 && <span className="opacity-60">{n}</span>}
                  </button>
                )
              })}
            </div>

            <ZoneMap
              zone={zone}
              geometry={geometry}
              pois={pois}
              visibleCategories={visible}
              highlights={selected ? [{ x: selected.x, y: selected.y, z: selected.z }] : []}
              onPOIClick={setSelected}
              height={560}
            />

            {selected && (
              <div
                className="flex items-center gap-2 rounded border px-2 py-1.5 text-sm"
                style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
              >
                <span className="text-[10px] uppercase tracking-widest" style={{ color: 'var(--color-muted)' }}>
                  {selected.category.replace('_', ' ')}
                </span>
                <span className="truncate" style={{ color: 'var(--color-foreground)' }}>{selected.label}</span>
                <span className="font-mono text-xs" style={{ color: 'var(--color-muted)' }}>
                  {/* Displayed in /loc order, matching what the game prints. */}
                  {-selected.y}, {-selected.x}, {selected.z}
                </span>
                <div className="ml-auto flex items-center gap-1">
                  {selected.ref_id ? (
                    <button
                      onClick={() =>
                        navigate(
                          selected.category === 'ground_spawn'
                            ? `/items?select=${selected.ref_id}`
                            : `/npcs?select=${selected.ref_id}`,
                        )
                      }
                      className="rounded border px-1.5 py-0.5 text-[10px]"
                      style={{ borderColor: 'var(--color-border)', color: 'var(--color-primary)' }}
                    >
                      Open
                    </button>
                  ) : null}
                  <button
                    onClick={() =>
                      // POIs are stored in map space; the Zeal command wants
                      // game coordinates, which is the same negation back.
                      copy('pin', mapMarkerCommand(-selected.x, -selected.y, selected.label))
                    }
                    className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px]"
                    style={{
                      borderColor: copied === 'pin' ? 'var(--color-primary)' : 'var(--color-border)',
                      color: copied === 'pin' ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                    }}
                  >
                    {copied === 'pin' ? <Check size={9} /> : <Copy size={9} />}
                    {copied === 'pin' ? 'Copied' : 'Pin in game'}
                  </button>
                  <button onClick={() => setSelected(null)}>
                    <X size={12} style={{ color: 'var(--color-muted)' }} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
