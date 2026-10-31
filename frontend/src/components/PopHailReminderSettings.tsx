/**
 * PopHailReminderSettings — modal on the PoP Flagging page's "Reminders"
 * button. Configures the backend zone-in hail reminder (internal/popflag
 * Reminder): on entering Tranquility, Justice, Innovation or Knowledge, an
 * alert lists the flag NPCs the active character can hail right now. Like
 * the wishlist watcher it is not a trigger — the match is per-character
 * progression state — but it reuses the same overlay/TTS/sound alert
 * plumbing via AlertActionFields.
 */
import React, { useEffect, useState } from 'react'
import { Bell, Loader2 } from 'lucide-react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { useVoices } from '../hooks/useVoices'
import { useTTSVoices } from '../hooks/usePiperStatus'
import { getConfig, saveConfigChanges } from '../services/api'
import type { Config, PoPHailReminderSettings } from '../types/config'
import AlertActionFields from './AlertActionFields'

interface PopHailReminderSettingsProps {
  open: boolean
  onClose: () => void
}

const DEFAULT_TEMPLATE = 'Flag hail available: {hails}'

const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

export default function PopHailReminderSettings({
  open,
  onClose,
}: PopHailReminderSettingsProps): React.ReactElement | null {
  useEscapeToClose(onClose, open)
  const voices = useTTSVoices(useVoices())

  const [config, setConfig] = useState<Config | null>(null)
  // The config as loaded, so Save writes only this dialog's edits.
  const [baseConfig, setBaseConfig] = useState<Config | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setConfig(null)
    setError(null)
    getConfig()
      .then((c) => {
        setConfig(c)
        setBaseConfig(c)
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)))
  }, [open])

  if (!open) return null

  const prefs: PoPHailReminderSettings =
    config?.preferences.pop_hail_reminder ?? { enabled: false }

  function patch(p: Partial<PoPHailReminderSettings>): void {
    setConfig((c) =>
      c
        ? {
            ...c,
            preferences: {
              ...c.preferences,
              pop_hail_reminder: {
                enabled: false,
                ...c.preferences.pop_hail_reminder,
                ...p,
              },
            },
          }
        : c,
    )
  }

  function setEnabled(enabled: boolean): void {
    // Turning the reminder on with no action selected would fire nothing, so
    // default to overlay text — the user can switch it after.
    const noAction =
      !prefs.overlay_enabled && !prefs.tts_enabled && !prefs.sound_enabled
    patch(enabled && noAction ? { enabled, overlay_enabled: true } : { enabled })
  }

  async function handleSave(): Promise<void> {
    if (!config || !baseConfig) return
    setSaving(true)
    setError(null)
    try {
      await saveConfigChanges(config, baseConfig)
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      onClick={onClose}
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0,0,0,0.6)',
        zIndex: 1100,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg flex flex-col"
        style={{
          backgroundColor: 'var(--color-surface)',
          border: '1px solid var(--color-primary)',
          width: '100%',
          maxWidth: 560,
          maxHeight: '85vh',
        }}
      >
        <div className="flex items-center gap-2 px-4 py-3 border-b" style={{ borderColor: 'var(--color-border)' }}>
          <Bell size={16} style={{ color: 'var(--color-primary)' }} />
          <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Flag Hail Reminders
          </span>
        </div>

        <div className="px-4 py-3 space-y-4 overflow-y-auto text-sm" style={{ color: 'var(--color-foreground)' }}>
          {!config && !error && (
            <div className="flex items-center justify-center h-24">
              <Loader2 size={18} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            </div>
          )}
          {error && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>}

          {config && (
            <>
              <label className="flex items-start gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={prefs.enabled}
                  onChange={(e) => setEnabled(e.target.checked)}
                  className="mt-0.5"
                />
                <span>
                  <span className="block">Remind me of flag hails when I zone in</span>
                  <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    On entering Tranquility, Justice, Innovation or Knowledge,
                    lists the pre/post-flag NPCs your active character can hail
                    right now — e.g. Elder Poxbourne after a Terris Thule kill.
                  </span>
                </span>
              </label>

              <p className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                A hail is ticked off automatically the moment the NPC grants
                it (its reply followed by &ldquo;You have received a character
                flag!&rdquo;), so the reminder stops on its own. Characters with
                no synced or checked flags are skipped — run{' '}
                <span className="font-mono">#popflags</span> once on each alt so
                the reminder knows what it still needs.
              </p>

              {prefs.enabled && (
                <>
                  <div>
                    <label className="block text-xs mb-1" style={{ color: 'var(--color-muted-foreground)' }}>
                      Alert text
                    </label>
                    <input
                      type="text"
                      value={prefs.template ?? ''}
                      placeholder={DEFAULT_TEMPLATE}
                      onChange={(e) => patch({ template: e.target.value })}
                      className="w-full rounded px-2 py-1 text-xs outline-none font-mono"
                      style={inputStyle}
                    />
                    <p className="mt-1 text-[10px]" style={{ color: 'var(--color-muted)' }}>
                      {'{hails}'} lists the NPCs to hail, e.g. &ldquo;Elder
                      Poxbourne and Elder Fuirstel&rdquo;. {'{zone}'} and{' '}
                      {'{character}'} expand to the zone entered and the active
                      character.
                    </p>
                  </div>

                  <AlertActionFields prefs={prefs} patch={patch} voices={voices} />
                </>
              )}
            </>
          )}
        </div>

        <div className="flex justify-end gap-2 px-4 py-3 border-t" style={{ borderColor: 'var(--color-border)' }}>
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm rounded"
            style={{
              backgroundColor: 'transparent',
              color: 'var(--color-muted-foreground)',
              border: '1px solid var(--color-border)',
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={!config || saving}
            className="px-3 py-1.5 text-sm font-medium rounded disabled:opacity-50"
            style={{
              backgroundColor: 'var(--color-primary)',
              color: 'var(--color-primary-foreground, #fff)',
              border: 'none',
            }}
          >
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  )
}
