import React, { useState } from 'react'
import { Webhook, AlertTriangle, Pencil, Trash2, Plus } from 'lucide-react'

import type { Config, Preferences, DiscordWebhook } from '../../types/config'
import { updateConfig } from '../../services/api'
import { isValidDiscordWebhookUrl } from '../../lib/discordWebhook'

function newWebhookId(): string {
  return typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `webhook-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

interface DiscordWebhookSettingsProps {
  config: Config
  setConfig: (c: Config) => void
}

/**
 * DiscordWebhookSettings — the Settings → Discord tab's "Discord Webhooks"
 * card. Lets the user save named Discord incoming webhooks (created in a
 * channel's Server Settings → Integrations → Webhooks) so trigger
 * discord_webhook actions can reference one by name (NotificationActionEditor's
 * DiscordWebhookFields).
 *
 * Deliberately no single "active" webhook (unlike DiscordVoiceOverlaySettings'
 * links) — each trigger action picks whichever saved webhook it wants
 * independently, since different alerts (guild deaths, raid calls, ...) may
 * post to different channels.
 */
export default function DiscordWebhookSettings({
  config,
  setConfig,
}: DiscordWebhookSettingsProps): React.ReactElement {
  const webhooks = config.preferences.discord_webhooks ?? []

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
  function startEdit(webhook: DiscordWebhook): void {
    setEditingId(webhook.id)
    setDraftName(webhook.name)
    setDraftUrl(webhook.url)
  }
  function cancelEdit(): void {
    setEditingId(null)
  }
  function saveEdit(): void {
    if (!editingId) return
    const name = draftName.trim() || 'Untitled webhook'
    const url = draftUrl.trim()
    const nextWebhooks: DiscordWebhook[] =
      editingId === 'new'
        ? [...webhooks, { id: newWebhookId(), name, url }]
        : webhooks.map((w) => (w.id === editingId ? { ...w, name, url } : w))
    saveNow({ discord_webhooks: nextWebhooks })
    setEditingId(null)
  }
  function deleteWebhook(id: string): void {
    saveNow({ discord_webhooks: webhooks.filter((w) => w.id !== id) })
    if (editingId === id) setEditingId(null)
  }

  return (
    <section
      className="mt-4 rounded-lg p-4"
      style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)' }}
    >
      <h2
        className="mb-1 flex items-center gap-2 text-sm font-semibold uppercase tracking-wide"
        style={{ color: 'var(--color-muted)' }}
      >
        <Webhook size={13} /> Discord Webhooks
      </h2>

      <p className="mb-2 text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
        Lets a trigger post text to a Discord channel — e.g. broadcasting a
        guild-wide server message like a raid boss kill. PQ Companion never
        talks to Discord's API on your behalf; it only posts to the exact
        webhook URL you generate and paste in below. Once saved, pick a
        webhook from the &ldquo;Discord Webhook&rdquo; action type when
        editing a trigger's actions on the Triggers page.
      </p>

      <ol
        className="mb-3 flex list-decimal flex-col gap-1 pl-4 text-xs leading-relaxed"
        style={{ color: 'var(--color-muted-foreground)' }}
      >
        <li>In Discord, open the target channel's settings and go to <strong>Integrations</strong>.</li>
        <li>Click <strong>Webhooks</strong>, then <strong>New Webhook</strong> (or use an existing one).</li>
        <li>Click <strong>Copy Webhook URL</strong>.</li>
        <li>Paste it below and give it a name (e.g. &ldquo;Guild Announcements&rdquo;).</li>
      </ol>

      <div className="flex flex-col gap-3">
        <div>
          <div className="mb-1 text-sm" style={{ color: 'var(--color-foreground)' }}>
            Saved webhooks
          </div>
          <div className="flex flex-col gap-1">
            {webhooks.map((w) => (
              <div
                key={w.id}
                className="flex items-center gap-2 rounded px-2 py-1.5"
                style={{ backgroundColor: 'var(--color-surface-2)' }}
              >
                <span
                  className="min-w-0 flex-1 truncate text-xs"
                  style={{ color: 'var(--color-foreground)' }}
                  title={w.url}
                >
                  {w.name}
                </span>
                {!isValidDiscordWebhookUrl(w.url) && (
                  <AlertTriangle size={12} style={{ color: '#f59e0b' }} />
                )}
                <button
                  onClick={() => startEdit(w)}
                  title="Rename or update this webhook's URL"
                  className="flex items-center rounded p-1"
                  style={{ color: 'var(--color-muted-foreground)' }}
                >
                  <Pencil size={12} />
                </button>
                <button
                  onClick={() => deleteWebhook(w.id)}
                  title="Delete this webhook"
                  className="flex items-center rounded p-1"
                  style={{ color: 'var(--color-danger, #ef4444)' }}
                >
                  <Trash2 size={12} />
                </button>
              </div>
            ))}
            {webhooks.length === 0 && (
              <p className="text-xs" style={{ color: 'var(--color-muted)' }}>
                No webhooks saved yet.
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
                placeholder="Name, e.g. Guild Announcements"
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
                placeholder="https://discord.com/api/webhooks/..."
                className="w-full rounded px-2 py-1 text-xs font-mono outline-none"
                style={{
                  backgroundColor: 'var(--color-surface)',
                  border: '1px solid var(--color-border)',
                  color: 'var(--color-foreground)',
                }}
              />
              {draftUrl && !isValidDiscordWebhookUrl(draftUrl) && (
                <span className="flex items-center gap-1.5 text-[11px]" style={{ color: '#f59e0b' }}>
                  <AlertTriangle size={11} /> Only discord.com/discordapp.com webhook URLs are accepted.
                </span>
              )}
              <div className="flex items-center gap-2">
                <button
                  onClick={saveEdit}
                  disabled={!isValidDiscordWebhookUrl(draftUrl)}
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
              <Plus size={11} /> Add webhook
            </button>
          )}
        </div>

        <p className="text-[11px] leading-relaxed" style={{ color: 'var(--color-muted)' }}>
          A webhook URL lets anyone who has it post into that channel — treat
          it like a password. Deleting a webhook here doesn't touch it on
          Discord's side; any trigger action still pointing at it will simply
          stop firing until you pick a different one.
        </p>
      </div>
    </section>
  )
}
