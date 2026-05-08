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

// Master volume scales every playback (sound + TTS, including test previews)
// on top of the per-action volume. Stored as 0.0–1.0; updated from
// Preferences.master_volume by useMasterVolume(). Default 1.0 = no dampening.
let masterVolume = 1.0

/**
 * Set the master volume multiplier applied to every subsequent playback.
 * Accepts 0.0–1.0; values outside the range are clamped.
 */
export function setMasterVolume(value: number): void {
  if (!Number.isFinite(value)) return
  masterVolume = Math.min(1, Math.max(0, value))
}

/** Returns the current master volume multiplier (0.0–1.0). */
export function getMasterVolume(): number {
  return masterVolume
}

function effectiveVolume(volume: number): number {
  return Math.min(1, Math.max(0, volume)) * masterVolume
}

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
  // pq-audio is registered as a standard scheme. Chromium does NOT allow an
  // empty host on a standard URL — pq-audio:///Users/foo gets normalized to
  // pq-audio://users/foo (the first path segment is promoted to host and
  // lowercased), which loses both case and a path component. Use a fixed
  // sentinel host ("local") so the absolute path lands in URL.pathname intact.
  if (!p.startsWith('/')) p = '/' + p
  // Encode each path segment so spaces and URL-reserved chars (#, ?, %, etc.)
  // in filenames don't produce an invalid URL or get reinterpreted as a
  // query/fragment. The main-process handler runs decodeURIComponent before
  // hitting the file system.
  const encoded = p
    .split('/')
    .map((seg) => encodeURIComponent(seg))
    .join('/')
  return 'pq-audio://local' + encoded
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
  audio.volume = effectiveVolume(volume)
  audio.play().catch((err: unknown) => {
    // Surface the failure in DevTools — file-not-found and autoplay-blocked
    // both look identical to the user (silence) so a console line is the
    // only signal something's wrong with their trigger.
    // eslint-disable-next-line no-console
    console.warn('[audio] playSound failed', { filePath, error: err })
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

  // NOTE: do NOT call speechSynthesis.cancel() here. We used to, on the
  // theory that it kept rapid alerts from piling up, but it cancels the
  // previous utterance mid-sentence whenever a second trigger fires within
  // a second of the first — the symptom the user saw as "Mez broke" being
  // chopped off when "Charm broke" arrived right after. Letting utterances
  // queue naturally is the correct behaviour; the dedup window above
  // already keeps a single trigger from spamming the same line.

  const utterance = new SpeechSynthesisUtterance(text)
  utterance.volume = effectiveVolume(volume)
  utterance.onerror = (e) => {
    // eslint-disable-next-line no-console
    console.warn('[audio] speakText failed', { text, voice, error: e.error })
  }

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
  audio.volume = effectiveVolume(volume)

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
  utterance.volume = effectiveVolume(volume)
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
