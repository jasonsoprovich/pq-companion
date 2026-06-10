/**
 * AlertDefaultsSettings — Settings → Preferences controls for the global
 * trigger-alert defaults:
 *
 *   - Default TTS Voice: the voice used by any text_to_speech alert whose own
 *     voice field is "App default" (empty). Per-trigger voices still win.
 *   - Default Overlay Text Position: where overlay_text alerts that have no
 *     per-trigger pinned position anchor their stack, set via the same
 *     drag-to-position session the trigger editor uses.
 *   - Default Overlay Text Style: color, glow color, font, and size used by
 *     every overlay_text alert whose own action leaves the field on
 *     "App default". Per-trigger overrides in the editor always win.
 *
 * Edits are staged into the page's config state; the page's Save button
 * persists them like every other preference.
 */
import React from 'react'
import { Crosshair, Check, X as XIcon } from 'lucide-react'
import type { Config } from '../../types/config'
import { useVoices } from '../../hooks/useVoices'
import { usePositioningSession } from '../../hooks/usePositioningSession'
import { ColorOverrideField } from '../NotificationActionEditor'
import {
  resolveOverlayTextStyle,
  overlayTextShadow,
  overlayFontFamilyCSS,
  WINDOWS_SAFE_FONTS,
} from '../../lib/overlayTextStyle'

interface AlertDefaultsSettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

export default function AlertDefaultsSettings({
  config,
  setConfig,
}: AlertDefaultsSettingsProps): React.ReactElement {
  const voices = useVoices()
  const defaultVoice = config.preferences.default_tts_voice ?? ''
  const position = config.preferences.default_overlay_position ?? null

  function setPosition(p: { x: number; y: number } | null): void {
    setConfig({
      ...config,
      preferences: { ...config.preferences, default_overlay_position: p },
    })
  }

  // Global default overlay text style. Empty/0 = the built-in look (white,
  // glow matching the text color, system-ui, 20px). `resolved` is what an
  // un-customized alert will actually render as — it drives the swatches,
  // the size placeholder, and the live preview.
  const textColor = config.preferences.default_overlay_text_color ?? ''
  const glowColor = config.preferences.default_overlay_glow_color ?? ''
  const fontFamily = config.preferences.default_overlay_font_family ?? ''
  const fontSize = config.preferences.default_overlay_font_size ?? 0
  const resolved = resolveOverlayTextStyle(null, config.preferences)

  function setStylePref(patch: Partial<Config['preferences']>): void {
    setConfig({
      ...config,
      preferences: { ...config.preferences, ...patch },
    })
  }

  const { positioning, toggle } = usePositioningSession({
    position,
    onPositionChange: setPosition,
    testText: 'TRIGGER ALERT TEXT',
    testColor: resolved.color,
    testDurationSecs: 8,
  })

  return (
    <>
      <div className="mt-4">
        <div className="mb-1">
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Default TTS Voice
          </p>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Spoken by every trigger and alert whose voice is set to &ldquo;App default&rdquo;.
            Triggers with their own voice keep it.
          </p>
        </div>
        <select
          value={defaultVoice}
          onChange={(e) =>
            setConfig({
              ...config,
              preferences: { ...config.preferences, default_tts_voice: e.target.value },
            })
          }
          className="mt-1 w-full max-w-xs rounded px-2 py-1 text-xs outline-none"
          style={{
            backgroundColor: 'var(--color-surface-2)',
            border: '1px solid var(--color-border)',
            color: 'var(--color-foreground)',
            appearance: 'none',
          }}
        >
          <option value="">System default</option>
          {voices.map((v) => (
            <option key={v} value={v}>{v}</option>
          ))}
        </select>
      </div>

      <div className="mt-4">
        <div className="mb-1 flex items-center justify-between gap-3">
          <div>
            <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
              Default Overlay Text Position
            </p>
            <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
              Where trigger alert text appears when a trigger has no pinned position of its
              own. Unset = centered on the overlay monitor.
            </p>
          </div>
          <button
            type="button"
            onClick={toggle}
            className="flex items-center gap-1 rounded px-2 py-1 text-[11px] shrink-0"
            style={{
              backgroundColor: positioning ? '#16a34a' : 'var(--color-primary)',
              color: positioning ? '#fff' : 'var(--color-background)',
              border: '1px solid transparent',
              cursor: 'pointer',
            }}
            title={
              positioning
                ? 'Drag the on-screen card to position, then click here (or press Esc to cancel)'
                : 'Pop up a sample alert in the overlay so you can drag it into position'
            }
          >
            {positioning ? <Check size={11} /> : <Crosshair size={11} />}
            {positioning ? 'Done — Keep Position' : 'Set Default Position'}
          </button>
        </div>
        {position && (
          <div
            className="mt-1 flex items-center gap-1.5 text-[10px] rounded px-2 py-1 max-w-xs"
            style={{
              color: 'var(--color-muted-foreground)',
              backgroundColor: 'var(--color-surface-2)',
              border: '1px solid var(--color-border)',
              fontFamily: 'monospace',
            }}
          >
            <span>Anchored at x={position.x}, y={position.y}</span>
            <button
              type="button"
              onClick={() => setPosition(null)}
              className="ml-auto flex items-center gap-1 px-1 py-0.5 rounded"
              style={{
                backgroundColor: 'transparent',
                color: 'var(--color-muted)',
                border: '1px solid var(--color-border)',
                cursor: 'pointer',
                fontFamily: 'inherit',
              }}
              title="Clear the default position (centered stacking)"
            >
              <XIcon size={9} />
              Reset
            </button>
          </div>
        )}
      </div>

      <div className="mt-4">
        <div className="mb-1">
          <p className="text-sm" style={{ color: 'var(--color-foreground)' }}>
            Default Overlay Text Style
          </p>
          <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            Color, glow, font, and size for every trigger alert whose own style fields are on
            &ldquo;App default&rdquo;. Triggers customized in the editor keep their own style.
            Fonts listed all ship with Windows. Unset = the classic look (white text, matching
            glow, system font, 20px).
          </p>
        </div>
        <div className="mt-2 flex gap-3 items-center flex-wrap">
          <ColorOverrideField
            label="Color"
            value={textColor}
            resolved={resolved.color}
            onChange={(v) => setStylePref({ default_overlay_text_color: v })}
            resetTitle="Reset to the built-in text color (white)"
          />
          <ColorOverrideField
            label="Glow"
            value={glowColor}
            resolved={resolved.glowColor}
            onChange={(v) => setStylePref({ default_overlay_glow_color: v })}
            resetTitle="Reset to the built-in glow (matches the text color)"
          />
          <div className="flex items-center gap-1.5">
            <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
              Font
            </label>
            <select
              value={fontFamily}
              onChange={(e) => setStylePref({ default_overlay_font_family: e.target.value })}
              className="rounded px-2 py-0.5 text-xs outline-none max-w-40"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
                appearance: 'none',
                fontFamily: fontFamily ? `'${fontFamily}'` : undefined,
              }}
            >
              <option value="">System default</option>
              {WINDOWS_SAFE_FONTS.map((f) => (
                <option key={f} value={f} style={{ fontFamily: `'${f}'` }}>{f}</option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-1.5">
            <label className="text-[11px] shrink-0" style={{ color: 'var(--color-muted-foreground)' }}>
              Size
            </label>
            <input
              type="number"
              min={8}
              max={96}
              value={fontSize > 0 ? fontSize : ''}
              placeholder={String(resolved.fontSize)}
              onChange={(e) =>
                setStylePref({ default_overlay_font_size: Math.max(0, parseInt(e.target.value) || 0) })
              }
              className="w-14 rounded px-2 py-0.5 text-xs outline-none text-center"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
              title="Overlay font size in pixels (blank = 20)"
            />
          </div>
        </div>
        {/* Live preview on a dark backdrop, rendered exactly like the overlay
            (same shadow + font fallback helpers). */}
        <div
          className="mt-2 rounded px-3 py-2 max-w-md flex items-center justify-center"
          style={{
            backgroundColor: 'rgba(10,10,12,0.9)',
            border: '1px solid var(--color-border)',
            minHeight: 48,
            overflow: 'hidden',
          }}
        >
          <span
            style={{
              fontSize: resolved.fontSize,
              fontWeight: 800,
              letterSpacing: '0.04em',
              color: resolved.color,
              fontFamily: overlayFontFamilyCSS(resolved.fontFamily),
              textShadow: overlayTextShadow(resolved.glowColor),
              whiteSpace: 'nowrap',
              userSelect: 'none',
            }}
          >
            MEZ BROKE!
          </span>
        </div>
      </div>
    </>
  )
}
