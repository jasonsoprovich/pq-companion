import React, { useState } from 'react'
import {
  Headphones,
  CheckCircle2,
  AlertTriangle,
  ExternalLink,
  PictureInPicture2,
  Pencil,
  Trash2,
  Plus,
} from 'lucide-react'

import type { Config, Preferences, DiscordVoiceLink } from '../../types/config'
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

function newLinkId(): string {
  return typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `link-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

interface DiscordVoiceOverlaySettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

/**
 * DiscordVoiceOverlaySettings is the "Discord Voice" settings card (issue
 * #150). We never talk to Discord's API ourselves — the app's own rpc.voice.read
 * request was declined. Instead the user generates StreamKit Overlay links
 * themselves (Discord's own hosted tool for embedding a voice roster in OBS/
 * XSplit browser sources) and saves them here, named, one per voice channel
 * they use — each link is pinned to one guild+channel, so switching channels
 * is picking a different saved link from the Active link dropdown rather
 * than re-pasting a URL every time.
 */
export default function DiscordVoiceOverlaySettings({
  config,
  setConfig,
}: DiscordVoiceOverlaySettingsProps): React.ReactElement {
  const prefs = config.preferences
  const enabled = prefs.discord_voice_enabled ?? false
  const links = prefs.discord_voice_links ?? []
  const activeId = prefs.discord_voice_active_link_id ?? ''
  const activeLink = links.find((l) => l.id === activeId)
  const activeUrl = activeLink?.url ?? ''
  const activeUrlValid = isValidStreamKitVoiceUrl(activeUrl)

  // Inline add/edit form state — 'new' for a not-yet-saved link, an existing
  // link's id to rename/re-paste its URL, or null when the form is closed.
  const [editingId, setEditingId] = useState<string | null>(null)
  const [draftName, setDraftName] = useState('')
  const [draftUrl, setDraftUrl] = useState('')

  function saveNow(patch: Partial<Preferences>): void {
    const next: Config = { ...config, preferences: { ...config.preferences, ...patch } }
    setConfig(next)
    void updateConfig(next)
  }

  function startAdd(): void {
    setEditingId('new')
    setDraftName('')
    setDraftUrl('')
  }
  function startEdit(link: DiscordVoiceLink): void {
    setEditingId(link.id)
    setDraftName(link.name)
    setDraftUrl(link.url)
  }
  function cancelEdit(): void {
    setEditingId(null)
  }
  function saveEdit(): void {
    if (!editingId) return
    const name = draftName.trim() || 'Untitled link'
    const url = draftUrl.trim()
    let nextLinks: DiscordVoiceLink[]
    let nextActiveId = activeId
    if (editingId === 'new') {
      const id = newLinkId()
      nextLinks = [...links, { id, name, url }]
      // First link ever saved becomes active automatically — otherwise
      // enabling + saving a link would still show nothing until the user
      // separately picks it from the dropdown.
      if (!activeId) nextActiveId = id
    } else {
      nextLinks = links.map((l) => (l.id === editingId ? { ...l, name, url } : l))
    }
    saveNow({ discord_voice_links: nextLinks, discord_voice_active_link_id: nextActiveId })
    setEditingId(null)
  }
  function deleteLink(id: string): void {
    const nextLinks = links.filter((l) => l.id !== id)
    const nextActiveId = activeId === id ? '' : activeId
    saveNow({ discord_voice_links: nextLinks, discord_voice_active_link_id: nextActiveId })
    if (editingId === id) setEditingId(null)
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
        <li>Copy the link from that box and save it below.</li>
      </ol>

      {enabled && (
        <div className="flex flex-col gap-3">
          {links.length > 0 && (
            <div>
              <div className="mb-1 flex items-center gap-2">
                <Dot state={activeId ? (activeUrlValid ? 'on' : 'off') : 'unknown'} />
                <span className="text-sm" style={{ color: 'var(--color-foreground)' }}>
                  Active link
                </span>
              </div>
              <select
                value={activeId}
                onChange={(e) => saveNow({ discord_voice_active_link_id: e.target.value })}
                className="w-full rounded px-2 py-1 text-xs outline-none"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              >
                <option value="">— none —</option>
                {links.map((l) => (
                  <option key={l.id} value={l.id}>
                    {l.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {activeId && (
            <div
              className="flex items-start gap-2 rounded px-2 py-1.5 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)' }}
            >
              {activeUrlValid ? (
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

          {/* Saved links list + inline add/edit form */}
          <div>
            <div className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
              Saved links
            </div>
            <div className="flex flex-col gap-1">
              {links.map((l) => (
                <div
                  key={l.id}
                  className="flex items-center gap-2 rounded px-2 py-1.5"
                  style={{ backgroundColor: 'var(--color-surface-2)' }}
                >
                  <span
                    className="min-w-0 flex-1 truncate text-xs"
                    style={{ color: 'var(--color-foreground)' }}
                    title={l.url}
                  >
                    {l.name}
                    {l.id === activeId && (
                      <span className="ml-1.5" style={{ color: 'var(--color-primary)' }}>
                        (active)
                      </span>
                    )}
                  </span>
                  <button
                    onClick={() => startEdit(l)}
                    title="Rename or update this link's URL"
                    className="flex items-center rounded p-1"
                    style={{ color: 'var(--color-muted-foreground)' }}
                  >
                    <Pencil size={12} />
                  </button>
                  <button
                    onClick={() => deleteLink(l.id)}
                    title="Delete this link"
                    className="flex items-center rounded p-1"
                    style={{ color: 'var(--color-danger, #ef4444)' }}
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              ))}
              {links.length === 0 && (
                <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
                  No links saved yet.
                </p>
              )}
            </div>

            {editingId ? (
              <div
                className="mt-2 flex flex-col gap-2 rounded p-2"
                style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)' }}
              >
                <input
                  type="text"
                  value={draftName}
                  onChange={(e) => setDraftName(e.target.value)}
                  placeholder="Name, e.g. Raid channel"
                  className="w-full rounded px-2 py-1 text-xs outline-none"
                  style={{
                    backgroundColor: 'var(--color-surface)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                  }}
                />
                <input
                  type="text"
                  value={draftUrl}
                  onChange={(e) => setDraftUrl(e.target.value)}
                  placeholder="https://streamkit.discord.com/overlay/voice/..."
                  className="w-full rounded px-2 py-1 text-xs font-mono outline-none"
                  style={{
                    backgroundColor: 'var(--color-surface)',
                    border: '1px solid var(--color-border)',
                    color: 'var(--color-foreground)',
                  }}
                />
                {draftUrl && !isValidStreamKitVoiceUrl(draftUrl) && (
                  <span className="flex items-center gap-1.5 text-[11px]" style={{ color: '#f59e0b' }}>
                    <AlertTriangle size={11} /> Only streamkit.discord.com voice links are accepted.
                  </span>
                )}
                <div className="flex items-center gap-2">
                  <button
                    onClick={saveEdit}
                    disabled={!isValidStreamKitVoiceUrl(draftUrl)}
                    className="rounded px-2.5 py-1 text-xs font-medium disabled:opacity-50"
                    style={{ backgroundColor: 'var(--color-primary)', color: '#fff' }}
                  >
                    Save
                  </button>
                  <button
                    onClick={cancelEdit}
                    className="rounded px-2.5 py-1 text-xs font-medium"
                    style={{
                      backgroundColor: 'var(--color-surface)',
                      color: 'var(--color-muted-foreground)',
                      border: '1px solid var(--color-border)',
                    }}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <button
                onClick={startAdd}
                className="mt-2 flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium"
                style={{
                  backgroundColor: 'var(--color-surface-2)',
                  color: 'var(--color-foreground)',
                  border: '1px solid var(--color-border)',
                }}
              >
                <Plus size={11} /> Add link
              </button>
            )}
          </div>

          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
            Each link is pinned to the one server + voice channel you picked
            when generating it — Discord doesn't let third-party apps like
            this one auto-detect "whichever channel I'm in right now." Save
            one link per voice channel you use and switch between them with
            the Active link dropdown above. Only streamkit.discord.com links
            are accepted — anything else is rejected and won't load. Links
            persist across restarts. There's no documented fixed expiry for
            these, but Discord can invalidate one if you revoke the
            "Streamkit Overlay" app from your Discord connections, reset
            permissions, or reinstall Discord — if a roster ever stops
            updating, regenerate that link the same way and update it here.
          </p>

          <label
            className="flex cursor-pointer items-center gap-2 text-sm"
            style={{ color: 'var(--color-foreground)' }}
          >
            <input
              type="checkbox"
              checked={prefs.discord_voice_minimal_mode ?? false}
              onChange={(e) => saveNow({ discord_voice_minimal_mode: e.target.checked })}
            />
            Minimal mode (fade header + background, avatars only)
          </label>
          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
            Off by default: the header and content area both match every
            other overlay's opacity/fade settings. Turn this on once you've
            positioned the window where you want it — both the header and the
            background tint fade to fully transparent, leaving just the live
            avatars and names. The lock/close buttons stay reachable, just
            without a shaded box behind them.
          </p>

          {activeUrlValid && (
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
