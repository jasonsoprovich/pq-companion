import React from 'react'
import { Headphones, CheckCircle2, AlertTriangle, ExternalLink, PictureInPicture2 } from 'lucide-react'

import type { Config, Preferences } from '../../types/config'
import { updateConfig } from '../../services/api'
import { isValidStreamKitVoiceUrl } from '../../lib/overlays'

const STREAMKIT_URL = 'https://streamkit.discord.com/overlay'

// A colored status dot: green = ok, red = problem, gray = unknown/not set.
function Dot({ state }: { state: 'on' | 'off' | 'unknown' }): React.ReactElement {
  const color =
    state === 'on' ? '#22c55e' : state === 'off' ? '#ef4444' : 'var(--color-muted)'
  return (
    <span
      className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
      style={{ backgroundColor: color }}
    />
  )
}

interface DiscordVoiceOverlaySettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

/**
 * DiscordVoiceOverlaySettings is the "Discord Voice" settings card (issue
 * #150). We never talk to Discord's API ourselves — the app's own rpc.voice.read
 * request was declined. Instead the user generates a StreamKit Overlay URL
 * themselves (Discord's own hosted tool for embedding a voice roster in OBS/
 * XSplit browser sources) and pastes it here; the popout overlay window then
 * embeds that URL directly. Pinned to one guild+channel — switching channels
 * means pasting a new URL, since auto-following the current channel would
 * need the same gated Discord scope.
 */
export default function DiscordVoiceOverlaySettings({
  config,
  setConfig,
}: DiscordVoiceOverlaySettingsProps): React.ReactElement {
  const prefs = config.preferences
  const enabled = prefs.discord_voice_enabled ?? false
  const url = prefs.discord_voice_url ?? ''
  const urlValid = isValidStreamKitVoiceUrl(url)

  function stage(patch: Partial<Preferences>): void {
    setConfig({ ...config, preferences: { ...config.preferences, ...patch } })
  }
  function saveNow(patch: Partial<Preferences>): void {
    const next: Config = { ...config, preferences: { ...config.preferences, ...patch } }
    setConfig(next)
    void updateConfig(next)
  }

  return (
    <section
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <div className="mb-1 flex items-center justify-between">
        <h2
          className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
          style={{ color: 'var(--color-muted)' }}
        >
          <Headphones size={13} /> Discord Voice
        </h2>
        <label className="flex cursor-pointer items-center gap-2">
          <span className="text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
            {enabled ? 'Enabled' : 'Disabled'}
          </span>
          <div
            onClick={() => saveNow({ discord_voice_enabled: !enabled })}
            className="relative h-5 w-9 rounded-full transition-colors"
            style={{ backgroundColor: enabled ? 'var(--color-primary)' : 'var(--color-surface-2)' }}
          >
            <span
              className="absolute top-0.5 h-4 w-4 rounded-full bg-white transition-transform"
              style={{ transform: enabled ? 'translateX(18px)' : 'translateX(2px)' }}
            />
          </div>
        </label>
      </div>

      <p className="mb-2 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
        Shows a live Discord voice-channel roster with a speaking highlight in
        an overlay. PQ Companion never talks to Discord's API — instead it
        embeds Discord's own{' '}
        <a
          href={STREAMKIT_URL}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-0.5 underline"
          style={{ color: 'var(--color-primary)' }}
        >
          StreamKit Overlay <ExternalLink size={10} />
        </a>{' '}
        tool — the same thing streamers paste into OBS as a browser source.
      </p>

      <ol
        className="mb-3 flex list-decimal flex-col gap-1 pl-4 text-xs leading-relaxed"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        <li>
          Open{' '}
          <a
            href={STREAMKIT_URL}
            target="_blank"
            rel="noreferrer noopener"
            className="inline-flex items-center gap-0.5 underline"
            style={{ color: 'var(--color-primary)' }}
          >
            streamkit.discord.com/overlay <ExternalLink size={10} />
          </a>{' '}
          and log in with Discord if it asks.
        </li>
        <li>
          Click <strong>Install for OBS</strong> (or the similar install/copy
          button) — this opens the widget configuration panel, including the
          link box you'll need at the end.
        </li>
        <li>Choose the <strong>Voice</strong> widget (not Status or Chat).</li>
        <li>Pick your server, then pick the specific voice channel from the dropdowns.</li>
        <li>
          Optional: toggle display settings like "Small Avatars" or "Show
          Speaking Users Only" to taste.
        </li>
        <li>Copy the link from that box and paste it into the field below.</li>
      </ol>

      {enabled && (
        <div className="flex flex-col gap-3">
          <div>
            <div className="mb-1 flex items-center gap-2">
              <Dot state={url ? (urlValid ? 'on' : 'off') : 'unknown'} />
              <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                StreamKit overlay URL
              </span>
            </div>
            <input
              type="text"
              value={url}
              placeholder="https://streamkit.discord.com/overlay/voice/..."
              onChange={(e) => stage({ discord_voice_url: e.target.value })}
              onBlur={() => saveNow({ discord_voice_url: url })}
              className="w-full rounded px-2 py-1 text-xs font-mono outline-none"
              style={{
                backgroundColor: 'var(--color-surface-2)',
                border: '1px solid var(--color-border)',
                color: 'var(--color-foreground)',
              }}
            />
          </div>

          {url && (
            <div
              className="flex items-start gap-2 rounded px-2 py-1.5 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)' }}
            >
              {urlValid ? (
                <>
                  <CheckCircle2 size={13} style={{ color: '#22c55e' }} />
                  <span style={{ color: 'var(--color-foreground)' }}>
                    Looks like a valid StreamKit voice link. The overlay window
                    will show this channel's roster.
                  </span>
                </>
              ) : (
                <>
                  <AlertTriangle size={13} style={{ color: '#f59e0b' }} />
                  <span style={{ color: 'var(--color-muted-foreground)' }}>
                    Doesn't look like a StreamKit voice overlay link. It should
                    look like streamkit.discord.com/overlay/voice/&lt;server
                    id&gt;/&lt;channel id&gt;.
                  </span>
                </>
              )}
            </div>
          )}

          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
            This link is pinned to the one server + voice channel you picked
            when generating it — Discord doesn't let third-party apps like
            this one auto-detect "whichever channel I'm in right now."
            Switching voice channels means repeating the StreamKit steps above
            and pasting the new link. Only streamkit.discord.com links are
            accepted — anything else is rejected and won't load. The link is
            saved here and persists across restarts, so you only need to
            paste it once per channel; if the roster ever stops updating,
            the link itself may have expired on Discord's end — regenerate
            it the same way and paste the new one.
          </p>

          <label
            className="flex cursor-pointer items-center gap-2 text-sm"
            style={{ color: 'var(--color-foreground)' }}
          >
            <input
              type="checkbox"
              checked={prefs.discord_voice_shaded_content ?? false}
              onChange={(e) => saveNow({ discord_voice_shaded_content: e.target.checked })}
            />
            Shade content area to match app opacity
          </label>
          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
            Off by default: the roster stays fully see-through to the game
            no matter what the Overlays opacity slider is set to. Turn this on
            if you'd rather the content area match every other overlay's
            shaded look — only the header is shaded either way.
          </p>

          {urlValid && (
            <button
              onClick={() => window.electron?.overlay?.toggleDiscordVoice()}
              className="flex items-center gap-1.5 self-start rounded px-2.5 py-1 text-xs font-medium"
              style={{
                backgroundColor: 'var(--color-primary)',
                color: '#fff',
                border: '1px solid var(--color-border)',
              }}
            >
              <PictureInPicture2 size={11} /> Open overlay window
            </button>
          )}
        </div>
      )}
    </section>
  )
}
