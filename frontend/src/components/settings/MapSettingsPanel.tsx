import React, { useCallback, useEffect, useState } from 'react'
import { Check, Map as MapIcon, RefreshCw } from 'lucide-react'
import { getConfig, getExternalMapStatus, updateConfig } from '../../services/api'
import type { Config } from '../../types/config'
import type { ExternalMapStatus, MapRenderMode } from '../../types/map'

// The three drawings a zone can be rendered from, with what each is actually
// good for — the tradeoffs are not guessable from the names.
const STYLES: { key: MapRenderMode; label: string; blurb: string }[] = [
  {
    key: 'detailed',
    label: 'Detailed',
    blurb:
      'Everything extracted from the game files — elevation contours or every ' +
      'floor edge. The most information, and the busiest in multi-level zones.',
  },
  {
    key: 'outline',
    label: 'Outline',
    blurb:
      'One clean line drawing, the same style in every zone. Easiest to trace a ' +
      'route on, but it only draws walls — features that are not walls, like the ' +
      'lake in Oasis of Marr, are not in it.',
  },
]

// MapSettingsPanel controls how maps are drawn everywhere in the app, and
// explains how to add a third-party map pack.
export default function MapSettingsPanel(): React.ReactElement {
  // The whole config is held, not just the one field: the endpoint takes a
  // complete document, so saving a fragment would blank every other preference.
  const [config, setConfig] = useState<Config | null>(null)
  const [style, setStyle] = useState<MapRenderMode>('detailed')
  const [pack, setPack] = useState<ExternalMapStatus | null>(null)
  const [saved, setSaved] = useState(false)
  const [checking, setChecking] = useState(false)

  const loadPack = useCallback(() => {
    setChecking(true)
    getExternalMapStatus()
      .then(setPack)
      .catch(() => setPack({ available: false }))
      .finally(() => setChecking(false))
  }, [])

  useEffect(() => {
    getConfig()
      .then((c) => {
        setConfig(c)
        setStyle(c.preferences.map_style ?? 'detailed')
      })
      .catch(() => {})
    loadPack()
  }, [loadPack])

  const save = (next: MapRenderMode): void => {
    if (!config) return
    setStyle(next)
    const updated: Config = {
      ...config,
      preferences: { ...config.preferences, map_style: next },
    }
    updateConfig(updated)
      .then((c) => {
        setConfig(c)
        setSaved(true)
        setTimeout(() => setSaved(false), 2000)
      })
      // Put the old value back rather than showing a choice that did not stick.
      .catch(() => setStyle(config.preferences.map_style ?? 'detailed'))
  }

  const options = [
    ...STYLES,
    ...(pack?.available
      ? [{
          key: 'external' as MapRenderMode,
          label: pack.name ?? 'Map pack',
          blurb:
            `The map pack installed in your own EQ folder, drawn in its own ` +
            `colours — ${pack.zones} zones. Hand-drawn, so it shows things the ` +
            `extracted maps cannot, like water.`,
        }]
      : []),
  ]

  return (
    <div className="h-full overflow-y-auto px-5 py-4">
      <div className="mb-1 flex items-center gap-2">
        <MapIcon size={16} style={{ color: 'var(--color-primary)' }} />
        <h2 className="text-lg font-semibold">Maps</h2>
      </div>
      <p className="mb-4 max-w-3xl text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
        How zone maps are drawn, everywhere they appear — the Zones tab, the Live
        Map, NPC spawn maps and the overlays. You can still switch style on any
        individual map; this is what they all open with.
      </p>

      <h3
        className="mb-2 text-xs font-semibold uppercase tracking-widest"
        style={{ color: 'var(--color-muted)' }}
      >
        Default map style
      </h3>
      <div className="mb-6 flex max-w-3xl flex-col gap-1.5">
        {options.map((o) => {
          const on = style === o.key
          return (
            <button
              key={o.key}
              onClick={() => save(o.key)}
              className="rounded border px-3 py-2 text-left"
              style={{
                backgroundColor: on ? 'var(--color-surface-2)' : 'transparent',
                borderColor: on ? 'var(--color-primary)' : 'var(--color-border)',
              }}
            >
              <div className="flex items-center gap-2">
                <span
                  className="text-sm font-medium"
                  style={{ color: on ? 'var(--color-primary)' : 'var(--color-foreground)' }}
                >
                  {o.label}
                </span>
                {on && <Check size={12} style={{ color: 'var(--color-primary)' }} />}
              </div>
              <p className="mt-0.5 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                {o.blurb}
              </p>
            </button>
          )
        })}
      </div>
      {saved && (
        <p className="mb-4 text-xs" style={{ color: 'var(--color-primary)' }}>
          Saved.
        </p>
      )}

      <div className="flex items-center gap-2">
        <h3
          className="text-xs font-semibold uppercase tracking-widest"
          style={{ color: 'var(--color-muted)' }}
        >
          Map pack
        </h3>
        <button
          onClick={loadPack}
          disabled={checking}
          className="flex items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] disabled:opacity-40"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
        >
          <RefreshCw size={9} />
          {checking ? 'Checking…' : 'Check again'}
        </button>
      </div>

      {pack?.available ? (
        <div
          className="mt-2 max-w-3xl rounded border px-3 py-2 text-sm"
          style={{ borderColor: 'var(--color-primary)' }}
        >
          <div style={{ color: 'var(--color-primary)' }}>
            Found “{pack.name}” — {pack.zones} zones.
          </div>
          <div className="mt-1 font-mono text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {pack.dir}
          </div>
        </div>
      ) : (
        <div
          className="mt-2 max-w-3xl rounded border px-3 py-2 text-sm"
          style={{ borderColor: 'var(--color-border)', color: 'var(--color-muted-foreground)' }}
        >
          <p className="mb-2">
            No map pack found. PQ Companion does not ship one — but if you install
            a set of classic <code>.txt</code> EverQuest maps yourself, it will
            read them and offer them as a third style, drawn in their own colours.
          </p>
          <ol className="ml-4 list-decimal space-y-1 text-xs">
            <li>
              Download a map pack — <span className="font-mono">eqmaps.info</span>{' '}
              is the usual one.
            </li>
            <li>
              Create a folder called <span className="font-mono">maps</span> inside
              your EverQuest directory, and a folder inside that named after the
              pack — e.g. <span className="font-mono">maps\Brewall</span>. The
              folder name is what the button here will be called.
            </li>
            <li>Unzip the <span className="font-mono">.txt</span> files into it.</li>
            <li>Press “Check again” above.</li>
          </ol>
          {/* Worth saying plainly, because the published instructions describe a
              different client and would otherwise seem to contradict this. */}
          <p className="mt-2 text-xs" style={{ color: 'var(--color-muted)' }}>
            Guides written for modern EverQuest tell you to pick the pack from a
            dropdown on the in-game map window. Project Quarm's client has no such
            dropdown — installing the files still works here, it just will not
            change your in-game map. Zeal's{' '}
            <span className="font-mono">map_files</span> folder is read too, if you
            already keep maps there.
          </p>
          <p className="mt-2 text-xs" style={{ color: 'var(--color-muted)' }}>
            Nothing is bundled with or uploaded by PQ Companion — the files stay
            yours, read from your own disk.
          </p>
        </div>
      )}
    </div>
  )
}
