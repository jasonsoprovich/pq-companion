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
 * Build a renderer-loadable URL for a local audio file. Electron's default
 * webSecurity blocks file:// from non-file:// origins, so we route playback
 * through the custom pq-audio:// scheme registered in the main process. In
 * a browser dev preview (no Electron) we fall back to file:// — playback
 * won't work there but the URL shape is still inspectable.
 */
function audioUrl(filePath: string): string {
  let p = filePath.replace(/\\/g, '/')
  if (p.startsWith('file://')) p = p.replace(/^file:\/+/, '')
  if (p.startsWith('pq-audio://')) return filePath
  // pq-audio uses URL form pq-audio:///<absolute-path>; the empty host means
  // unix paths keep their leading slash and windows drive letters appear as
  // /C:/foo, which the main-process handler normalizes back to C:/foo.
  if (!p.startsWith('/')) p = '/' + p
  return 'pq-audio://' + p
}

/**
 * Play a local sound file at the given volume.
 *
 * @param filePath  Absolute path to the audio file. Loaded via the custom
 *                  pq-audio:// scheme registered in the Electron main process.
 * @param volume    Playback volume in the range 0.0–1.0. Defaults to 1.0.
 */
export function playSound(filePath: string, volume = 1.0): void {
  if (!filePath) return
  if (isDuplicate(`sound:${filePath}`)) return

  const audio = new Audio(audioUrl(filePath))
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

// ── Test playback ─────────────────────────────────────────────────────────────
//
// Trigger / alert configuration UIs expose Test buttons that preview a sound
// file or TTS utterance at the configured volume. Test playback is single-
// channel: starting a new test stops the previous one, and clicking the same
// button while it's playing stops it. The onEnd callback fires for both
// natural completion and external interruption so the caller can flip the
// button icon back to play.

let activeStop: (() => void) | null = null

/** Stop whichever test sound or TTS is currently playing, if any. */
export function stopTestPlayback(): void {
  const stop = activeStop
  activeStop = null
  if (stop) stop()
}

/** Returns true if a test sound or TTS is currently playing. */
export function isTestPlaying(): boolean {
  return activeStop !== null
}

/**
 * Play a local sound file once at the given volume, stopping any prior
 * test playback. Bypasses the production dedup window. The onEnd callback
 * fires when playback finishes naturally or is interrupted.
 */
export function playSoundForTest(filePath: string, volume = 1.0, onEnd?: () => void): void {
  stopTestPlayback()
  if (!filePath) {
    onEnd?.()
    return
  }
  const audio = new Audio(audioUrl(filePath))
  audio.volume = Math.min(1, Math.max(0, volume))

  let done = false
  const finish = () => {
    if (done) return
    done = true
    audio.removeEventListener('ended', finish)
    audio.removeEventListener('error', finish)
    if (activeStop === stop) activeStop = null
    onEnd?.()
  }
  const stop = () => {
    audio.pause()
    finish()
  }
  audio.addEventListener('ended', finish)
  audio.addEventListener('error', finish)
  activeStop = stop
  audio.play().catch(finish)
}

/**
 * Speak text via Web Speech at the given volume / voice, stopping any prior
 * test playback. Bypasses the production dedup window. The onEnd callback
 * fires when speech finishes naturally or is interrupted.
 */
export function speakTextForTest(
  text: string,
  voice = '',
  volume = 1.0,
  onEnd?: () => void,
): void {
  stopTestPlayback()
  if (!text || !window.speechSynthesis) {
    onEnd?.()
    return
  }
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.volume = Math.min(1, Math.max(0, volume))
  if (voice) {
    const voices = window.speechSynthesis.getVoices()
    const match = voices.find((v) => v.name === voice)
    if (match) utterance.voice = match
  }

  let done = false
  const finish = () => {
    if (done) return
    done = true
    utterance.removeEventListener('end', finish)
    utterance.removeEventListener('error', finish)
    if (activeStop === stop) activeStop = null
    onEnd?.()
  }
  const stop = () => {
    window.speechSynthesis.cancel()
    finish()
  }
  utterance.addEventListener('end', finish)
  utterance.addEventListener('error', finish)
  activeStop = stop
  window.speechSynthesis.speak(utterance)
}
