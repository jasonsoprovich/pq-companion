/**
 * DiscordVoiceOverlayWindowPage — transparent always-on-top overlay showing a
 * live Discord voice-channel roster with an active-speaker highlight. See
 * issue #150: rather than talking to Discord's API ourselves (the
 * rpc.voice.read scope isn't granted to small third-party apps), this embeds
 * Discord's own hosted StreamKit Overlay page — the same tool people paste
 * into OBS/XSplit as a browser source — via a native child WebContentsView
 * that the Electron main process attaches below this header (see
 * createDiscordVoiceOverlay/setDiscordVoiceContent in electron/main/index.ts).
 *
 * This page only renders the title-bar chrome. The body area below the header
 * is left empty on purpose — the actual Discord content is a separate native
 * view layered on top by the main process, not React-rendered here. A
 * placeholder message fills that space until a valid URL is configured.
 *
 * By default the header AND body share the same opacity/fade tint, matching
 * every other overlay for visual consistency. discord_voice_minimal_mode is
 * an opt-in "clean" look for once the window is positioned where it's
 * wanted: both fade to permanently transparent (ignoring overlay_opacity),
 * leaving just the live Discord avatars/names floating with no chrome.
 *
 * Known limitation: because the embedded content is a separate native view,
 * this window's chrome-fade and locked-mode "hover to re-enable interaction"
 * behaviour (see useOverlayChromeFade/useOverlayLock) can't detect mouse
 * activity that happens purely over the Discord content's bounds — only over
 * the header. Acceptable since the widget itself is read-only display, not
 * something users interact with directly.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Headphones } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { getConfig } from '../services/api'
import { isValidStreamKitVoiceUrl } from '../lib/overlays'

export default function DiscordVoiceOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('discordVoice')
  const chrome = useOverlayChromeFade(mode === 'display-only')

  const onDragMouseDown = useWindowDrag()

  // Drives the "not configured yet" placeholder text — hidden once a valid
  // URL is actually loaded so the overlay stays clean and minimal rather than
  // showing setup instructions over a live roster.
  const [hasContent, setHasContent] = useState(false)
  // Off by default: header + body match every other overlay's opacity/fade.
  // On: both permanently fade to transparent instead — see
  // DiscordVoiceOverlaySettings.tsx.
  const [minimalMode, setMinimalMode] = useState(false)

  // Hands the main process the current StreamKit URL so it can attach/update
  // the embedded child view — main never talks to the Go config store itself.
  // Re-checked on every config poll (mirroring useOverlayZoom) so a Settings
  // change is picked up live without reopening the window.
  const syncContent = useCallback(() => {
    getConfig()
      .then((c) => {
        const enabled = c.preferences.discord_voice_enabled ?? false
        const activeId = c.preferences.discord_voice_active_link_id ?? ''
        const url = c.preferences.discord_voice_links?.find((l) => l.id === activeId)?.url ?? ''
        const valid = enabled && isValidStreamKitVoiceUrl(url)
        setHasContent(valid)
        setMinimalMode(c.preferences.discord_voice_minimal_mode ?? false)
        if (valid) {
          window.electron?.overlay?.setDiscordVoiceUrl?.(url)
        } else {
          window.electron?.overlay?.clearDiscordVoice?.()
        }
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    syncContent()
  }, [syncContent])

  const handleMessage = useCallback(
    (msg: { type: string }) => {
      if (msg.type === WSEvent.ConfigUpdated) syncContent()
    },
    [syncContent],
  )
  useWebSocket(handleMessage)

  // The shared tint every other overlay uses — applied to both header and
  // body by default. In minimal mode both ignore this entirely and stay
  // permanently transparent instead (see class doc comment above).
  const tintBg = `rgba(10,10,12,${chrome ? opacity : 0})`
  const tintBorder = `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        color: 'rgba(255,255,255,0.9)',
      }}
    >
      {/* ── Drag handle / title bar ─────────────────────────────────────── */}
      <div
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderTopLeftRadius: 8,
          borderTopRightRadius: 8,
          borderBottom: minimalMode ? 'none' : tintBorder,
          backgroundColor: minimalMode ? 'transparent' : tintBg,
          backgroundImage: minimalMode
            ? undefined
            : 'linear-gradient(rgba(255,255,255,0.04), rgba(255,255,255,0.04))',
          flexShrink: 0,
          height: 30,
          boxSizing: 'border-box',
          userSelect: 'none',
          // Minimal mode keeps the controls always visible (no backdrop
          // behind them, but still reachable) instead of fading with chrome.
          opacity: minimalMode ? 1 : chrome ? 1 : 0,
          pointerEvents: minimalMode ? 'auto' : chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease, background-color 0.4s ease, border-color 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Headphones size={11} style={{ color: '#5865f2' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Discord Voice
          </span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeDiscordVoice()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {/* ── Body — the main process layers Discord's own StreamKit content
          here as a native child view on top of whatever renders below. Shares
          the header's tint by default, matching every other overlay; minimal
          mode drops it to permanently transparent instead. The placeholder
          text only renders until a valid URL is actually loaded, so a
          properly configured overlay stays clean and minimal. ────────────── */}
      <div
        style={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 12,
          textAlign: 'center',
          backgroundColor: minimalMode ? 'transparent' : tintBg,
          transition: 'background-color 0.4s ease',
        }}
      >
        {!hasContent && (
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', lineHeight: 1.5 }}>
            Enable Discord Voice, save a StreamKit link, and pick it as the
            active link in Settings → Overlays to see your voice channel here.
          </p>
        )}
      </div>
    </div>
  )
}
