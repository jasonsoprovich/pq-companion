import React from 'react'
import { Navigation } from 'lucide-react'
import { useLiveZone } from '../hooks/useLiveZone'
import { ZoneMapPanel } from '../components/maps/ZoneMapPanel'

// LiveMapPage is the map of wherever you are standing, as opposed to the Zones
// tab and the Maps page, which show a zone you looked up.
//
// Worth its own nav entry precisely because the distinction is not cosmetic: a
// browsing surface must not move under you, so those two deliberately do *not*
// follow the player between zones — the Maps page offers a "You are in <zone>"
// button and leaves the choice to you. Here following is the entire point, so
// the zone tracks the game, follow-me defaults on, and POI search is enabled so
// "where's the vendor / the key door / the traps" is one query away from a pin.
export default function LiveMapPage(): React.ReactElement {
  const { zone, live } = useLiveZone()

  if (!zone) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
        <Navigation size={22} style={{ color: 'var(--color-muted)' }} />
        <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
          Waiting for your position
        </p>
        <p className="max-w-sm text-xs" style={{ color: 'var(--color-muted)' }}>
          This needs Zeal running in game — position comes over its named pipe.
          The Zones tab and Maps page work without it.
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col gap-2 p-3">
      <div className="flex shrink-0 items-center gap-2">
        <Navigation size={15} style={{ color: 'var(--color-primary)' }} />
        <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
          {zone}
        </h1>
        {/* The zone is sticky across the silence while zoning, so it can be shown
            while the position behind it is stale. Say which it is rather than
            leaving a map that looks live but is not. */}
        <span
          className="rounded px-1.5 py-0.5 text-[10px] font-medium"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            color: live ? 'var(--color-primary)' : 'var(--color-muted)',
          }}
          title={
            live
              ? 'Position is current'
              : 'No position right now — zoning, or Zeal has stopped. Showing the last zone you were in.'
          }
        >
          {live ? 'Live' : 'Last known'}
        </span>
      </div>
      {/* Remount per zone so pan/zoom and any selection reset on a zone change
          rather than leaving the previous zone's pin selected. */}
      <div className="min-h-0 flex-1">
        <ZoneMapPanel key={zone} zoneShortName={zone} live />
      </div>
    </div>
  )
}
