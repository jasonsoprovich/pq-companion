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
 * placeholder message fills that space until a valid URL is configured. The
 * body is fully transparent by default; the discord_voice_shaded_content
 * preference opts it back into the header's app-consistent tint instead.
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
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('discordVoice')

  const onDragMouseDown = useWindowDrag()

  // Drives the "not configured yet" placeholder text — hidden once a valid
  // URL is actually loaded so the overlay stays clean and minimal rather than
  // showing setup instructions over a live roster.
  const [hasContent, setHasContent] = useState(false)
  // Off by default: the content area stays fully transparent regardless of
  // overlay_opacity. Some users prefer it to match every other overlay's
  // shaded body instead — see DiscordVoiceOverlaySettings.tsx.
  const [shadedContent, setShadedContent] = useState(false)

  // Hands the main process the current StreamKit URL so it can attach/update
  // the embedded child view — main never talks to the Go config store itself.
  // Re-checked on every config poll (mirroring useOverlayZoom) so a Settings
  // change is picked up live without reopening the window.
  const syncContent = useCallback(() => {
    getConfig()
      .then((c) => {
        const enabled = c.preferences.discord_voice_enabled ?? false
        const url = c.preferences.discord_voice_url ?? ''
        const valid = enabled && isValidStreamKitVoiceUrl(url)
        setHasContent(valid)
        setShadedContent(c.preferences.discord_voice_shaded_content ?? false)
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

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
        // Deliberately no background/border here, unlike every other overlay
        // — the body below the header is a live Discord roster, and shading
        // it to match the app's opacity slider just obscures it for no
        // reason (feedback from issue #150: "not a fan of the content area
        // being shaded"). Only the header carries the app-consistent tint;
        // the body is always fully see-through to the game.
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
          borderBottom: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
          backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
          backgroundImage: 'linear-gradient(rgba(255,255,255,0.04), rgba(255,255,255,0.04))',
          flexShrink: 0,
          height: 30,
          boxSizing: 'border-box',
          userSelect: 'none',
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
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
          here as a native child view on top of whatever renders below. Fully
          transparent by default (issue #150 feedback); shadedContent opts
          back into the header's tint for users who want full visual
          consistency with every other overlay's body instead. The
          placeholder text only renders until a valid URL is actually loaded,
          so a properly configured overlay stays clean and minimal. ──────── */}
      <div
        style={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: 12,
          textAlign: 'center',
          backgroundColor: shadedContent ? `rgba(10,10,12,${chrome ? opacity : 0})` : undefined,
          transition: shadedContent ? 'background-color 0.4s ease' : undefined,
        }}
      >
        {!hasContent && (
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.4)', lineHeight: 1.5 }}>
            Enable Discord Voice and paste a StreamKit overlay URL in
            Settings → Overlays to see your voice channel here.
          </p>
        )}
      </div>
    </div>
  )
}
