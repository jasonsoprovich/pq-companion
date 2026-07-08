/**
 * piper.ts — shared constants + helpers for the user-installed Piper local TTS
 * provider (issue #147, docs/piper-tts-plan.md).
 *
 * A trigger/alert `voice` field is normally a bare Web Speech voice name. To
 * route a voice to Piper without touching the (many) places that store a voice
 * string, we reserve a namespaced sentinel value — PIPER_VOICE_ID — that the
 * dropdown offers and audio.ts recognizes. Everything else (Go trigger models,
 * WebSocket, config round-trip) carries it as an opaque string unchanged.
 */

/** Sentinel `voice` value selecting the configured Piper voice. */
export const PIPER_VOICE_ID = 'piper:local'

/** Label shown for the Piper voice in the same dropdown as Web Speech voices. */
export const PIPER_VOICE_LABEL = '🔊 Piper (local)'

/** True when a stored voice value routes to the Piper backend, not Web Speech. */
export function isPiperVoice(voice: string | undefined | null): boolean {
  return typeof voice === 'string' && voice.startsWith('piper:')
}

/**
 * Display label for a voice value in a dropdown. Web Speech voices show their
 * bare name (value === label); the Piper sentinel shows a friendly, namespaced
 * label so it's unambiguous next to the OS voices.
 */
export function voiceLabel(voice: string): string {
  return voice === PIPER_VOICE_ID ? PIPER_VOICE_LABEL : voice
}

/**
 * Backend Piper install status (GET /api/piper/status). Mirrors the Go
 * piperStatusResponse (pipertts.Status flattened + mode/warm/cache fields).
 */
export interface PiperStatus {
  enabled: boolean
  exe_path?: string
  model_path?: string
  exe_found: boolean
  model_found: boolean
  model_config_found: boolean
  version?: string
  voice_name?: string
  ready: boolean
  error?: string
  // "spawn" | "warm" — the currently configured synthesis mode.
  mode?: string
  // Whether a persistent warm-mode worker is currently alive. Meaningless
  // when mode is "spawn" (there is never a persistent worker in that mode).
  warm_running?: boolean
  // Most recent warm-worker failure, if any (e.g. it crashed and hasn't been
  // replaced by a new request yet).
  warm_error?: string
  // Cached TTS WAV count / total size, for the "Clear cache" UI.
  cache_files?: number
  cache_bytes?: number
}
