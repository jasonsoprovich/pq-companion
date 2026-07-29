/**
 * Validates a Discord incoming-webhook URL, e.g.
 * https://discord.com/api/webhooks/123456789012345678/abcDEF-123_xyz — the
 * URL a channel's Integrations → Webhooks page issues. Shared by the
 * Settings "Discord" tab (saving a webhook) and the trigger action editor's
 * "test" affordance, so both agree on what "looks valid" means. Mirrors
 * discordWebhookURLRe in backend/internal/trigger/engine.go — keep in sync.
 */
const DISCORD_WEBHOOK_URL_RE = /^https:\/\/(?:discord|discordapp)\.com\/api\/webhooks\/\d+\/[\w-]+$/

export function isValidDiscordWebhookUrl(url: string): boolean {
  return DISCORD_WEBHOOK_URL_RE.test(url.trim())
}
