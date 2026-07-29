import React, { useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Map as MapIcon, Search, X } from 'lucide-react'
import { useCachedState } from '../hooks/useCachedState'
import { useMapZones } from '../hooks/useMapZones'
import { ZoneMapPanel } from '../components/maps/ZoneMapPanel'

// MapsPage is the standalone map browser: a zone picker beside the shared
// ZoneMapPanel. All the layer/POI behaviour lives in the panel so this page and
// the Zones tab cannot drift apart.
export default function MapsPage(): React.ReactElement {
  const [params, setParams] = useSearchParams()
  const [query, setQuery] = useCachedState('maps.query', '')
  const { zones, loaded } = useMapZones()

  const zoneName = params.get('zone')

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = q ? zones.filter((z) => z.zone.toLowerCase().includes(q)) : zones
    return [...list].sort((a, b) => a.zone.localeCompare(b.zone))
  }, [zones, query])

  // A build without maps.db reports zero zones. Say so plainly rather than
  // showing an empty picker that looks broken.
  if (loaded && zones.length === 0) {
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
      <div
        className="flex w-60 shrink-0 flex-col border-r"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div
          className="flex items-center gap-2 border-b px-3 py-2"
          style={{ borderColor: 'var(--color-border)' }}
        >
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
            <button onClick={() => setQuery('')}>
              <X size={12} style={{ color: 'var(--color-muted)' }} />
            </button>
          )}
        </div>
        <div className="flex-1 overflow-y-auto">
          {filtered.map((z) => (
            <button
              key={z.zone}
              onClick={() => setParams({ zone: z.zone })}
              className="w-full px-3 py-1.5 text-left text-sm transition-colors"
              style={{
                backgroundColor: z.zone === zoneName ? 'var(--color-surface-2)' : 'transparent',
                borderLeft:
                  z.zone === zoneName
                    ? '2px solid var(--color-primary)'
                    : '2px solid transparent',
                color: z.zone === zoneName ? 'var(--color-primary)' : 'var(--color-foreground)',
              }}
            >
              {z.zone}
            </button>
          ))}
        </div>
      </div>

      <div className="flex min-w-0 flex-1 flex-col gap-2 p-3">
        <div className="flex items-center gap-2">
          <MapIcon size={16} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold">{zoneName ?? 'Select a zone'}</h1>
        </div>
        {/* Remounting per zone resets pan/zoom and any POI selection, so
            switching zones never leaves a stale pin from the previous one. */}
        <div className="min-h-0 flex-1">
          <ZoneMapPanel key={zoneName ?? ''} zoneShortName={zoneName} />
        </div>
      </div>
    </div>
  )
}
