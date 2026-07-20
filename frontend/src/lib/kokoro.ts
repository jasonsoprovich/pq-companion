/**
 * kokoro.ts — shared constants + helpers for the user-installed Kokoro local
 * TTS provider, the sibling of piper.ts for a second local-TTS voice.
 *
 * Same namespaced-sentinel-value trick as Piper: a trigger/alert `voice`
 * field carries KOKORO_VOICE_ID unchanged through the (many) places that
 * store a voice string, and audio.ts recognizes it and routes to the backend
 * instead of Web Speech.
 */

/** Sentinel `voice` value selecting the configured Kokoro voice. */
export const KOKORO_VOICE_ID = 'kokoro:local'

/** Label shown for the Kokoro voice in the same dropdown as Web Speech voices. */
export const KOKORO_VOICE_LABEL = '🔊 Kokoro (local)'

/** True when a stored voice value routes to the Kokoro backend, not Web Speech. */
export function isKokoroVoice(voice: string | undefined | null): boolean {
  return typeof voice === 'string' && voice.startsWith('kokoro:')
}

/**
 * Backend Kokoro status (GET /api/kokoro/status). Mirrors the Go
 * kokoroStatusResponse (kokorotts.Status flattened + cache fields). Unlike
 * Piper, there's no exe/model path detection — `reachable` and `voices` come
 * from one live GET /v1/audio/voices call against the configured service.
 */
export interface KokoroStatus {
  enabled: boolean
  base_url?: string
  voice?: string
  // True when the configured service answered GET /v1/audio/voices.
  reachable: boolean
  // Every voice id the service reported — powers the Settings voice
  // dropdown. Empty when unreachable.
  voices?: string[]
  ready: boolean
  error?: string
  // Cached TTS WAV count / total size, for the "Clear cache" UI. Shared with
  // Piper's cache — see internal/tts — so this reflects every provider's
  // entries, not just Kokoro's.
  cache_files?: number
  cache_bytes?: number
}
