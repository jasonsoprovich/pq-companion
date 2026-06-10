/**
 * AlertDefaultsSettings — Settings → Preferences controls for the global
 * trigger-alert defaults:
 *
 *   - Default TTS Voice: the voice used by any text_to_speech alert whose own
 *     voice field is "App default" (empty). Per-trigger voices still win.
 *   - Default Overlay Text Position: where overlay_text alerts that have no
 *     per-trigger pinned position anchor their stack, set via the same
 *     drag-to-position session the trigger editor uses.
 *
 * Edits are staged into the page's config state; the page's Save button
 * persists them like every other preference.
 */
import React from 'react'
import { Crosshair, Check, X as XIcon } from 'lucide-react'
import type { Config } from '../../types/config'
import { useVoices } from '../../hooks/useVoices'
import { usePositioningSession } from '../../hooks/usePositioningSession'

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

  const { positioning, toggle } = usePositioningSession({
    position,
    onPositionChange: setPosition,
    testText: 'TRIGGER ALERT TEXT',
    testColor: '#ffffff',
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
    </>
  )
}
