import { useCallback, useEffect, useState } from 'react'

import { getConfig } from '../services/api'
import type { DiscordWebhook } from '../types/config'
import { useWebSocket } from './useWebSocket'
import type { WsMessage } from './useWebSocket'

/**
 * Returns the user's saved Discord webhooks (Settings → Discord → Discord
 * Webhooks) for the trigger action editor's webhook picker. Re-fetches on
 * config:updated so adding/renaming/deleting a webhook in Settings shows up
 * in an already-open trigger editor without a manual refresh — same pattern
 * as usePiperStatus's voice-list refresh.
 */
export function useDiscordWebhooks(): DiscordWebhook[] {
  const [webhooks, setWebhooks] = useState<DiscordWebhook[]>([])

  const refresh = useCallback(() => {
    getConfig()
      .then((c) => setWebhooks(c.preferences.discord_webhooks ?? []))
      .catch(() => setWebhooks([]))
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const onMessage = useCallback(
    (msg: WsMessage) => {
      if (msg.type === 'config:updated') refresh()
    },
    [refresh],
  )
  useWebSocket(onMessage)

  return webhooks
}
