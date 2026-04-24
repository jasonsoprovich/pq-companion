/**
 * audio.ts — Audio playback and TTS service for the PQ Companion audio engine.
 *
 * playSound  — plays a local file via an HTML5 Audio element.
 * speakText  — speaks text aloud via the Web Speech Synthesis API.
 */

// Deduplication: track when each unique audio key was last fired.
// Prevents the same sound/utterance from playing twice when multiple alert
// systems (event alerts + trigger actions) fire for the same log line.
const lastFiredAt = new Map<string, number>()
const AUDIO_DEDUP_MS = 400

function isDuplicate(key: string): boolean {
  const now = Date.now()
  const last = lastFiredAt.get(key) ?? 0
  if (now - last < AUDIO_DEDUP_MS) return true
  lastFiredAt.set(key, now)
  return false
}

/**
 * Play a local sound file at the given volume.
 *
 * @param filePath  Absolute path to the audio file. Electron serves local
 *                  files at the file:// scheme without extra configuration.
 * @param volume    Playback volume in the range 0.0–1.0. Defaults to 1.0.
 */
export function playSound(filePath: string, volume = 1.0): void {
  if (!filePath) return
  if (isDuplicate(`sound:${filePath}`)) return

  // Normalise Windows back-slashes and ensure the file:// scheme is present.
  const normalised = filePath.replace(/\\/g, '/')
  const url = normalised.startsWith('file://') ? normalised : `file:///${normalised}`

  const audio = new Audio(url)
  audio.volume = Math.min(1, Math.max(0, volume))
  audio.play().catch(() => {
    // Silently ignore playback errors (e.g. file not found) — the trigger
    // still fires its other actions.
  })
}

/**
 * Speak text aloud using the Web Speech Synthesis API.
 *
 * @param text    The string to speak.
 * @param voice   Optional voice name matching a `SpeechSynthesisVoice.name`.
 *                Pass an empty string to use the system default.
 * @param volume  Speech volume in the range 0.0–1.0. Defaults to 1.0.
 */
export function speakText(text: string, voice = '', volume = 1.0): void {
  if (!text || !window.speechSynthesis) return
  if (isDuplicate(`tts:${text}`)) return

  // Cancel any queued utterances so a rapid sequence of triggers doesn't pile up.
  window.speechSynthesis.cancel()

  const utterance = new SpeechSynthesisUtterance(text)
  utterance.volume = Math.min(1, Math.max(0, volume))

  if (voice) {
    const voices = window.speechSynthesis.getVoices()
    const match = voices.find((v) => v.name === voice)
    if (match) utterance.voice = match
  }

  window.speechSynthesis.speak(utterance)
}

/**
 * Returns the list of available TTS voice names, sorted alphabetically.
 * May be empty until the browser has loaded the voice list.
 */
export function getAvailableVoices(): string[] {
  if (!window.speechSynthesis) return []
  return window.speechSynthesis
    .getVoices()
    .map((v) => v.name)
    .sort()
}
