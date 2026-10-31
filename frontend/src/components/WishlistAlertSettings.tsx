/**
 * WishlistAlertSettings — modal on the Wishlist page's "Alerts" button.
 * Configures the backend wishlist watcher (internal/wishlistwatch): an alert
 * fired when a wishlisted item's name appears in the active character's log
 * (loot lines, chat, raid calls, ...). Not a trigger — the match set is a
 * live join of every character's wishlist, which the trigger editor's static
 * regex model has no way to represent — but it reuses the same overlay/TTS/
 * sound alert plumbing (see NotificationActionEditor) under the hood.
 *
 * Overlay text and TTS speech share a single `template` (like RespawnAlert's
 * tts_template), so the action sections come from the compact shared
 * AlertActionFields rather than the full OverlayTextFields/TextToSpeechFields
 * — those bundle a per-action text input that would duplicate the template.
 */
import React, { useEffect, useState } from 'react'
import { Bell, Loader2 } from 'lucide-react'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import { useVoices } from '../hooks/useVoices'
import { useTTSVoices } from '../hooks/usePiperStatus'
import { getConfig, saveConfigChanges } from '../services/api'
import type { Config, WishlistWatchSettings } from '../types/config'
import AlertActionFields from './AlertActionFields'

interface WishlistAlertSettingsProps {
  open: boolean
  onClose: () => void
}

const DEFAULT_TEMPLATE = "{item} is on {character}'s wishlist"

const inputStyle: React.CSSProperties = {
  backgroundColor: 'var(--color-surface-2)',
  border: '1px solid var(--color-border)',
  color: 'var(--color-foreground)',
}

export default function WishlistAlertSettings({
  open,
  onClose,
}: WishlistAlertSettingsProps): React.ReactElement | null {
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

  const prefs: WishlistWatchSettings = config?.preferences.wishlist_watch ?? { enabled: false }

  function patch(p: Partial<WishlistWatchSettings>): void {
    setConfig((c) =>
      c
        ? {
            ...c,
            preferences: {
              ...c.preferences,
              wishlist_watch: { ...c.preferences.wishlist_watch, ...p },
            },
          }
        : c,
    )
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
            Wishlist Alerts
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
                  onChange={(e) => patch({ enabled: e.target.checked })}
                  className="mt-0.5"
                />
                <span>
                  <span className="block">Alert when a wishlisted item appears in your log</span>
                  <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                    Watches loot lines and chat for item names on any character&rsquo;s wishlist
                    while you&rsquo;re playing — a raid officer calling a drop, a looted
                    component, an item someone is selling.
                  </span>
                </span>
              </label>

              {prefs.enabled && (
                <>
                  <label className="flex items-start gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={prefs.include_other_chars ?? false}
                      onChange={(e) => patch({ include_other_chars: e.target.checked })}
                      className="mt-0.5"
                    />
                    <span>
                      <span className="block">Include other characters&rsquo; wishlists</span>
                      <span className="block text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
                        Also alerts for items other characters wishlisted — including no-drop
                        items, for swapping in an alt to loot for them. Off by default: most
                        players only want reminders about their active character.
                      </span>
                    </span>
                  </label>

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
                      {'{item}'} and {'{character}'} expand to the matched item and the
                      wishlisting character(s) — {'{character}'} lists every character that
                      has the item wishlisted, e.g. &ldquo;Bard and Cleric&rdquo;.
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
