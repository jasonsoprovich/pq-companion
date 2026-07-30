import React, { useCallback, useEffect, useState } from 'react'
import { Check, Map as MapIcon, Trash2 } from 'lucide-react'
import {
  getGameMapExportStatus,
  removeGameMapExport,
  runGameMapExport,
} from '../../services/api'
import type { GameMapExportResult, GameMapExportStatus } from '../../types/map'

// Categories offered for export, with why each is or is not on by default.
//
// The default set is what the in-game map does not already show. Existing map
// packs mark vendors, zone lines and raid targets, so exporting those adds a
// second copy of information the player already has; traps, locked doors and
// in-zone teleports are the categories no map set carries.
const CATEGORIES: { key: string; label: string; note?: string }[] = [
  { key: 'trap', label: 'Traps', note: 'no existing map set has these' },
  { key: 'locked', label: 'Locked doors', note: 'names the key' },
  { key: 'teleport', label: 'In-zone teleports' },
  { key: 'switch', label: 'Switches and lifts' },
  { key: 'wall', label: 'Walls (your markers)' },
  { key: 'hazard', label: 'Hazards (your markers)' },
  { key: 'note', label: 'Notes (your markers)' },
  { key: 'vendor', label: 'Vendors', note: 'usually already on your in-game map' },
  { key: 'raid_target', label: 'Raid targets', note: 'usually already there' },
  { key: 'zone_line', label: 'Zone lines', note: 'usually already there' },
  { key: 'ground_spawn', label: 'Ground spawns' },
  { key: 'tradeskill', label: 'Tradeskill containers' },
]

// GameMapExportPanel writes our markers into Zeal's external map files, so they
// appear on the in-game map rather than only in the app.
export default function GameMapExportPanel(): React.ReactElement {
  const [status, setStatus] = useState<GameMapExportStatus | null>(null)
  const [selected, setSelected] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<GameMapExportResult | null>(null)
  const [removed, setRemoved] = useState<{ removed: number; kept: number } | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(() => {
    getGameMapExportStatus()
      .then((s) => {
        setStatus(s)
        setSelected((prev) => (prev.length ? prev : (s.default_categories ?? [])))
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  useEffect(refresh, [refresh])

  const toggle = (key: string): void =>
    setSelected((prev) => (prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]))

  const run = (): void => {
    setBusy(true)
    setError(null)
    setRemoved(null)
    runGameMapExport(selected)
      .then((r) => { setResult(r); refresh() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  const remove = (): void => {
    setBusy(true)
    setError(null)
    setResult(null)
    removeGameMapExport()
      .then((r) => { setRemoved(r); refresh() })
      .catch((e: Error) => setError(e.message))
      .finally(() => setBusy(false))
  }

  return (
    <div className="h-full overflow-y-auto px-5 py-4">
      <div className="mb-1 flex items-center gap-2">
        <MapIcon size={16} style={{ color: 'var(--color-primary)' }} />
        <h2 className="text-lg font-semibold">In-Game Map Markers</h2>
      </div>
      <p className="mb-4 max-w-3xl text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        Writes PQ Companion's markers into Zeal's <code>map_files</code> folder so they
        appear on the map in game, not just here. This is the same information as the
        “Pin in game” button, except it covers every zone at once and persists —
        Akheva's 129 trap markers instead of 129 copy-pastes.
      </p>

      {status && !status.ready && (
        <p
          className="mb-4 rounded border px-3 py-2 text-sm"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
        >
          {status.reason}
        </p>
      )}

      {status?.ready && (
        <>
          <div
            className="mb-4 rounded border px-3 py-2 text-xs"
            style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
          >
            <div>EQ folder: <span className="font-mono">{status.eq_path}</span></div>
            <div className="mt-1">
              {status.existing_files === 0
                ? 'No map files installed yet — we will create them.'
                : `${status.existing_files} map file(s) present, ${status.foreign_files} not ours.`}
              {status.exported_files > 0 && ` ${status.exported_files} written by PQ Companion.`}
            </div>
            {/* The one guarantee worth stating outright, since this writes into
                the user's game folder. */}
            <div className="mt-1" style={{ color: 'var(--color-muted)' }}>
              Files from other map packs are never overwritten — ours go in the next free
              slot, and Remove only deletes files we wrote and still recognise.
            </div>
          </div>

          <h3 className="mb-2 text-xs font-semibold uppercase tracking-widest"
              style={{ color: 'var(--color-muted)' }}>
            What to export
          </h3>
          <div className="mb-4 flex flex-wrap gap-1.5">
            {CATEGORIES.map((c) => {
              const on = selected.includes(c.key)
              return (
                <button
                  key={c.key}
                  onClick={() => toggle(c.key)}
                  title={c.note}
                  className="rounded border px-2 py-1 text-xs"
                  style={{
                    backgroundColor: on ? 'var(--color-surface-2)' : 'transparent',
                    borderColor: on ? 'var(--color-primary)' : 'var(--color-border)',
                    color: on ? 'var(--color-primary)' : 'var(--color-muted)',
                  }}
                >
                  {c.label}
                </button>
              )
            })}
          </div>

          <div className="flex items-center gap-2">
            <button
              onClick={run}
              disabled={busy || selected.length === 0}
              className="rounded px-3 py-1.5 text-xs font-medium disabled:opacity-40"
              style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
            >
              {busy ? 'Working…' : 'Write map files'}
            </button>
            {status.exported_files > 0 && (
              <button
                onClick={remove}
                disabled={busy}
                className="flex items-center gap-1 rounded border px-3 py-1.5 text-xs disabled:opacity-40"
                style={{ borderColor: 'var(--color-border)', color: '#f87171' }}
              >
                <Trash2 size={12} />
                Remove ours
              </button>
            )}
          </div>

          {result && (
            <div className="mt-3 rounded border px-3 py-2 text-sm"
                 style={{ borderColor: 'var(--color-primary)' }}>
              <div className="flex items-center gap-1.5" style={{ color: 'var(--color-primary)' }}>
                <Check size={13} />
                Wrote {result.points} markers across {result.written} zones.
              </div>
              {result.skipped > 0 && (
                <div className="mt-1 text-xs" style={{ color: 'var(--color-muted)' }}>
                  {result.skipped} zone(s) skipped — all 11 map file slots already in use.
                </div>
              )}
              {/* Without this the export appears to do nothing: external data is
                  off by default. */}
              <div className="mt-2 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                In game, run <code>/map data_mode both</code> once to switch external
                data on. Maps reload when you zone.
              </div>
            </div>
          )}

          {removed && (
            <div className="mt-3 text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              Removed {removed.removed} file(s).
              {removed.kept > 0 &&
                ` ${removed.kept} left alone — changed since we wrote them, so they are no longer ours to delete.`}
            </div>
          )}
        </>
      )}

      {error && (
        <p className="mt-3 text-sm" style={{ color: '#f87171' }}>{error}</p>
      )}
    </div>
  )
}
