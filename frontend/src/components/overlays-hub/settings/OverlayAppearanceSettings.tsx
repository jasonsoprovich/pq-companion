/**
 * OverlayAppearanceSettings — settings shared by every overlay window:
 * background opacity, fade when inactive, clickable links, and restoring
 * the open windows on launch. Part of the Overlays hub's global settings.
 */
import React, { useEffect, useState } from 'react'
import type { Config } from '../../../types/config'

export default function OverlayAppearanceSettings({
  config,
  setConfig,
}: {
  config: Config
  setConfig: (c: Config) => void
}): React.ReactElement {
  // "Restore overlays on launch" — persisted in Electron userData (not the Go
  // config), since it's read at boot before the backend port is resolved.
  const [restoreOverlaysOnLaunch, setRestoreOverlaysOnLaunch] = useState(false)
  useEffect(() => {
    window.electron?.overlay?.autoOpenGet?.()
      .then((s) => setRestoreOverlaysOnLaunch(s?.enabled === true))
      .catch(() => {})
  }, [])

  return (
    <section
      className="rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        Appearance
      </h2>
      <p className="mb-4 text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
        Background transparency for popout overlay windows. To set one overlay differently (say, solid on a second monitor), use the Opacity dropdown in its own settings or in the Overlay Windows table below.
      </p>

      <div className="flex items-center gap-4">
        <div style={{ flex: 1 }}>
          <div className="mb-1 flex items-center justify-between">
            <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Opacity
            </p>
            <span className="text-xs font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
              {Math.round(config.preferences.overlay_opacity * 100)}%
            </span>
          </div>
          <input
            type="range"
            min={10}
            max={100}
            step={1}
            value={Math.round(config.preferences.overlay_opacity * 100)}
            onChange={(e) =>
              setConfig({
                ...config,
                preferences: {
                  ...config.preferences,
                  overlay_opacity: Number(e.target.value) / 100,
                },
              })
            }
            style={{ width: '100%' }}
          />
          <div className="mt-1 flex justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
            <span>10%</span>
            <span>100%</span>
          </div>
        </div>

        {/* Preview swatch — the overlay background layered over a
            colourful mock "game" backdrop. Without the backdrop the
            swatch sat directly on the dark settings page, so dragging
            the slider changed its alpha invisibly (it read as a static
            black box). */}
        <div
          style={{
            width: 48,
            height: 48,
            borderRadius: 6,
            border: '1px solid rgba(255,255,255,0.15)',
            background:
              'repeating-linear-gradient(45deg, #3b82f6 0 8px, #22c55e 8px 16px, #f97316 16px 24px)',
            position: 'relative',
            overflow: 'hidden',
            flexShrink: 0,
          }}
          title="Preview: overlay background at this opacity over game content"
        >
          <div
            style={{
              position: 'absolute',
              inset: 0,
              backgroundColor: `rgba(10,10,12,${config.preferences.overlay_opacity})`,
            }}
          />
        </div>
      </div>

      <label className="flex cursor-pointer items-center justify-between py-1 mt-4">
        <div>
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Fade When Inactive
          </p>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            A few seconds after the mouse leaves an overlay, its background,
            border, and title bar fade out completely — timer bars and stats
            stay visible. Hovering the overlay brings them back.
          </p>
        </div>
        <div
          onClick={() =>
            setConfig({
              ...config,
              preferences: {
                ...config.preferences,
                overlay_fade_enabled: !config.preferences.overlay_fade_enabled,
              },
            })
          }
          style={{
            width: 40,
            height: 22,
            borderRadius: 11,
            backgroundColor: config.preferences.overlay_fade_enabled
              ? 'var(--color-primary)'
              : 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            cursor: 'pointer',
            position: 'relative',
            flexShrink: 0,
            transition: 'background-color 0.15s',
          }}
        >
          <div
            style={{
              position: 'absolute',
              top: 2,
              left: config.preferences.overlay_fade_enabled ? 20 : 2,
              width: 16,
              height: 16,
              borderRadius: '50%',
              backgroundColor: '#fff',
              transition: 'left 0.15s',
            }}
          />
        </div>
      </label>

      {(config.preferences.overlay_fade_enabled ||
        Object.values(config.preferences.overlay_fade_modes ?? {}).includes('on')) && (
        <div className="mt-3">
          <div className="mb-1 flex items-center justify-between">
            <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Fade Delay
            </p>
            <span className="text-xs font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
              {(config.preferences.overlay_fade_delay_secs || 2.5).toFixed(1)}s
            </span>
          </div>
          <input
            type="range"
            min={5}
            max={100}
            step={5}
            value={Math.round((config.preferences.overlay_fade_delay_secs || 2.5) * 10)}
            onChange={(e) =>
              setConfig({
                ...config,
                preferences: {
                  ...config.preferences,
                  overlay_fade_delay_secs: Number(e.target.value) / 10,
                },
              })
            }
            style={{ width: '100%' }}
          />
          <div className="mt-1 flex justify-between text-xs" style={{ color: 'var(--color-muted)' }}>
            <span>0.5s</span>
            <span>10s</span>
          </div>
        </div>
      )}

      <label className="flex cursor-pointer items-center justify-between py-1 mt-4">
        <div>
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Clickable Links in Overlays
          </p>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Make entity rows in overlay windows — like the NPC overlay's loot
            items and castable spells — clickable links that bring the main
            window forward and open that item or spell in the database
            explorer. Turn off to keep those rows as plain text so a click
            never pulls focus out of the game. On by default.
          </p>
        </div>
        <div
          onClick={() =>
            setConfig({
              ...config,
              preferences: {
                ...config.preferences,
                overlay_entity_links_enabled:
                  config.preferences.overlay_entity_links_enabled === false,
              },
            })
          }
          style={{
            width: 40,
            height: 22,
            borderRadius: 11,
            backgroundColor:
              config.preferences.overlay_entity_links_enabled !== false
                ? 'var(--color-primary)'
                : 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            cursor: 'pointer',
            position: 'relative',
            flexShrink: 0,
            transition: 'background-color 0.15s',
          }}
        >
          <div
            style={{
              position: 'absolute',
              top: 2,
              left: config.preferences.overlay_entity_links_enabled !== false ? 20 : 2,
              width: 16,
              height: 16,
              borderRadius: '50%',
              backgroundColor: '#fff',
              transition: 'left 0.15s',
            }}
          />
        </div>
      </label>

      <label className="flex cursor-pointer items-center justify-between py-1 mt-4">
        <div>
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Restore Overlays on Launch
          </p>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Re-open the popout overlay windows you had open when you last
            quit, in their saved positions, so they're ready as soon as the
            app starts. Off by default.
          </p>
        </div>
        <div
          onClick={() => {
            const next = !restoreOverlaysOnLaunch
            setRestoreOverlaysOnLaunch(next)
            window.electron?.overlay?.autoOpenSetEnabled?.(next).catch(() => {})
          }}
          style={{
            width: 40,
            height: 22,
            borderRadius: 11,
            backgroundColor: restoreOverlaysOnLaunch
              ? 'var(--color-primary)'
              : 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            cursor: 'pointer',
            position: 'relative',
            flexShrink: 0,
            transition: 'background-color 0.15s',
          }}
        >
          <div
            style={{
              position: 'absolute',
              top: 2,
              left: restoreOverlaysOnLaunch ? 20 : 2,
              width: 16,
              height: 16,
              borderRadius: '50%',
              backgroundColor: '#fff',
              transition: 'left 0.15s',
            }}
          />
        </div>
      </label>
    </section>
  )
}
