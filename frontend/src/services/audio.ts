/**
 * audio.ts — Audio playback and TTS service for the PQ Companion audio engine.
 *
 * playSound  — plays a local file via an HTML5 Audio element.
 * speakText  — speaks text aloud via the Web Speech Synthesis API, OR, when the
 *              selected voice is the Piper local voice, asks the backend to
 *              synthesize a cached WAV and plays that through playSound. Any
 *              Piper failure falls back to Web Speech so an alert is never lost.
 */
import { isPiperVoice } from '../lib/piper'
import { piperSynthesize } from './api'

// Deduplication: track when each unique audio key was last fired.
// Prevents the same sound/utterance from playing twice when multiple alert
// systems fire for the same effect. This is the ONLY dedup shared across all
// three audio paths — trigger actions (useAudioEngine), spell-timer threshold
// alerts (useTimerAlerts), and respawn alerts (useRespawnAlerts) — so it is
// keyed on the actual output (sound path / TTS text), not on the matched line.
// Kept at the same 750ms window the per-line dedup in useAudioEngine uses, so
// a burst that resolves to the same sound or utterance collapses to one play
// no matter how many triggers or alert systems fired it (one mez/charm break,
// one tell beep). Distinct TTS text — e.g. per-sender tell readouts — is left
// alone, since those are genuinely different utterances.
const lastFiredAt = new Map<string, number>()
const AUDIO_DEDUP_MS = 750

// Cross-renderer single-owner gate. The dedup above only collapses repeats
// WITHIN one renderer — each renderer process has its own module state, so if
// more than one main-route renderer runs the audio hooks at once (a duplicate /
// ghost main window), each plays the same trigger sound independently and the
// user hears it 2–3×. The main process designates exactly one webContents as
// primary (window:is-primary); MainWindowLayout resolves it once at mount and
// calls setAudioOwner. Non-owner renderers stay silent. Defaults to true so a
// browser dev preview (no Electron IPC) and the brief window before the IPC
// resolves both still play — a real fire arrives long after mount.
let isAudioOwner = true

/** Set whether this renderer is allowed to emit trigger/alert audio. */
export function setAudioOwner(owner: boolean): void {
  isAudioOwner = owner
}

// Per-renderer identity for the duplicate-audio diagnostic. Generated once at
// module load, so every renderer process gets a distinct value. playSound /
// speakText log it on each play; the Electron main process captures renderer
// console output tagged with the window title (see hardenWebContents), so a
// recurrence shows whether one window played N times (one nonce repeated =
// single-renderer bug) or N windows each played once (distinct nonces =
// duplicate main renderers, which the owner gate above then suppresses).
const RENDERER_AUDIO_ID = Math.random().toString(36).slice(2, 8)

// Master volume scales every playback (sound + TTS, including test previews)
// on top of the per-action volume. Stored as 0.0–1.0; updated from
// Preferences.master_volume by useAudioPrefs(). Default 1.0 = no dampening.
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

// Default TTS voice applied whenever an alert's own voice field is empty
// ("App default" in the editor). Pushed from Preferences.default_tts_voice
// by useAudioPrefs(). Empty = the OS default voice.
let defaultTTSVoice = ''

/** Set the fallback TTS voice name used when an alert has no voice of its own. */
export function setDefaultTTSVoice(name: string): void {
  defaultTTSVoice = typeof name === 'string' ? name : ''
}

// Optional hook invoked when a Piper synthesis attempt fails and playback falls
// back to a Web Speech voice. The app can wire this to a non-blocking toast so
// the user learns their Piper setup isn't working; unset, it just logs. Kept as
// a pluggable callback so this leaf service stays decoupled from the React
// notification layer. Never throws into the audio path.
let piperFallbackNotifier: ((message: string) => void) | null = null

/** Register a callback fired (once per failure) when Piper falls back to Web Speech. */
export function setPiperFallbackNotifier(fn: ((message: string) => void) | null): void {
  piperFallbackNotifier = typeof fn === 'function' ? fn : null
}

function notifyPiperFallback(err: unknown): void {
  const detail = err instanceof Error ? err.message : String(err)
  const message = `Piper voice unavailable — using the default voice. (${detail})`
  // eslint-disable-next-line no-console
  console.warn('[audio] piper fallback:', message)
  try {
    piperFallbackNotifier?.(message)
  } catch {
    // A broken notifier must never break the audio path.
  }
}

// Speaking rate applied to every TTS utterance (alerts + test previews).
// 1.0 = the voice's normal speed; higher is faster. Pushed from
// Preferences.tts_rate by useAudioPrefs(). The Web Speech spec allows 0.1–10,
// but voices distort badly outside a narrower band, so we clamp to 0.5–2.0 —
// the same range the Settings slider exposes.
let ttsRate = 1.0

/** Set the global TTS speaking rate. Accepts 0.5–2.0; out-of-range is clamped. */
export function setTTSRate(rate: number): void {
  if (!Number.isFinite(rate)) return
  ttsRate = Math.min(2, Math.max(0.5, rate))
}

/** Returns the current global TTS speaking rate (0.5–2.0). */
export function getTTSRate(): number {
  return ttsRate
}

// Per-trigger repeat-audio cooldown, in milliseconds. After a trigger fires
// audio, useAudioEngine suppresses further audio from that SAME trigger id for
// this long, collapsing rapid same-trigger bursts (AE mez breaking several
// mobs) to one alert. 0 = disabled (every fire plays). Pushed from
// Preferences.trigger_audio_cooldown_secs by useAudioPrefs(). The actual gate
// lives in useAudioEngine since only it knows the trigger id; this module just
// holds the configured value so the engine can read it. Experimental — see the
// config field comment for how to remove the feature.
let repeatAudioCooldownMs = 0

/** Set the per-trigger repeat-audio cooldown (ms). Non-positive/invalid = off. */
export function setRepeatAudioCooldownMs(ms: number): void {
  repeatAudioCooldownMs = Number.isFinite(ms) && ms > 0 ? ms : 0
}

/** Returns the per-trigger repeat-audio cooldown in ms (0 = disabled). */
export function getRepeatAudioCooldownMs(): number {
  return repeatAudioCooldownMs
}

function effectiveVolume(volume: number): number {
  return Math.min(1, Math.max(0, volume)) * masterVolume
}

// Strong references to every in-flight playback. A bare `new Audio().play()`
// whose element falls out of scope can be garbage-collected by Chromium mid-
// playback under rapid-fire creation — the sound stops partway through. This
// is the "it cuts off when several fire at once" symptom: a single play has
// little GC pressure and finishes fine, but a burst (AE mez breaking several
// mobs, two enchanters' alerts, a run of tells) creates many short-lived
// Audio elements and some get reclaimed before they finish. Holding each
// element here until it ends (or errors) keeps it alive for its full duration.
const activePlaybacks = new Set<HTMLAudioElement>()

function isDuplicate(key: string): boolean {
  const now = Date.now()
  const last = lastFiredAt.get(key) ?? 0
  if (now - last < AUDIO_DEDUP_MS) return true
  lastFiredAt.set(key, now)
  // Sweep entries older than the dedup window (they can never match again) so
  // the map doesn't grow unbounded. TTS keys are capture-substituted
  // (per-sender, per-mob), so a long session would otherwise accumulate one
  // entry per distinct utterance forever. Gated on size to stay cheap.
  if (lastFiredAt.size > 256) {
    for (const [k, t] of lastFiredAt) {
      if (now - t >= AUDIO_DEDUP_MS) lastFiredAt.delete(k)
    }
  }
  return false
}

// ── TTS voice list ────────────────────────────────────────────────────────────
//
// Chromium loads the speech-synthesis voice list asynchronously: getVoices()
// returns [] until the first call kicks off enumeration and `voiceschanged`
// fires. Nothing primed that load in the main window at startup, so the first
// in-game trigger fire resolved its saved voice name against an empty list and
// silently fell back to the system default voice — until the user happened to
// open the trigger editor, whose voice dropdown (useVoices) primed the list.
// Prime it eagerly at module load and keep a cache fresh via voiceschanged.
let cachedVoices: SpeechSynthesisVoice[] = []

function refreshVoices(): void {
  if (!window.speechSynthesis) return
  const list = window.speechSynthesis.getVoices()
  if (list.length > 0) cachedVoices = list
}

if (typeof window !== 'undefined' && window.speechSynthesis) {
  refreshVoices()
  window.speechSynthesis.addEventListener('voiceschanged', refreshVoices)
}

function resolveVoice(name: string): SpeechSynthesisVoice | null {
  if (!name) return null
  if (cachedVoices.length === 0) refreshVoices()
  return cachedVoices.find((v) => v.name === name) ?? null
}

/**
 * Speak an utterance, applying the named voice if it can be resolved. If the
 * voice list hasn't loaded yet, hold the utterance until voiceschanged (or a
 * short timeout) so a trigger that fires right after app launch still speaks
 * with its configured voice instead of the system default.
 */
function speakWithVoice(utterance: SpeechSynthesisUtterance, voiceName: string): void {
  const synth = window.speechSynthesis
  const match = resolveVoice(voiceName)
  if (match) utterance.voice = match

  if (!voiceName || match || cachedVoices.length > 0) {
    // No specific voice wanted, voice resolved, or the list IS loaded and the
    // name just doesn't exist anymore — speak now (default voice in that case).
    synth.speak(utterance)
    return
  }

  // Voice list still loading: wait for it, but never hold the alert for more
  // than a beat — a late default-voice alert beats a missing one.
  let spoken = false
  const speakNow = (): void => {
    if (spoken) return
    spoken = true
    synth.removeEventListener('voiceschanged', speakNow)
    const late = resolveVoice(voiceName)
    if (late) utterance.voice = late
    synth.speak(utterance)
  }
  synth.addEventListener('voiceschanged', speakNow)
  setTimeout(speakNow, 2000)
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
  if (!isAudioOwner) return
  if (isDuplicate(`sound:${filePath}`)) return

  // Duplicate-audio diagnostic: emitted once per actual play. If a single fire
  // triggers multiple serves, matching these across window titles / nonces in
  // the Electron log pinpoints whether it's one renderer or several.
  // Fields are inlined into the message string, not passed as an object: the
  // Electron main-process console capture only serializes the formatted
  // message, so an object arg lands in the log as "[object Object]" and the
  // renderer id / path are lost (which is exactly what happened the first time
  // we tried to debug this).
  // eslint-disable-next-line no-console
  console.warn(`[audio-diag] play sound r=${RENDERER_AUDIO_ID} path=${filePath}`)

  const audio = new Audio(audioUrl(filePath))
  audio.volume = effectiveVolume(volume)
  // Retain the element until playback finishes so it isn't garbage-collected
  // mid-sound (see activePlaybacks above), then release it.
  activePlaybacks.add(audio)
  const release = (): void => {
    activePlaybacks.delete(audio)
    audio.removeEventListener('ended', release)
    audio.removeEventListener('error', release)
  }
  audio.addEventListener('ended', release)
  audio.addEventListener('error', release)
  audio.play().catch((err: unknown) => {
    // Surface the failure in DevTools — file-not-found and autoplay-blocked
    // both look identical to the user (silence) so a console line is the
    // only signal something's wrong with their trigger.
    release()
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
  if (!text) return
  if (!isAudioOwner) return
  if (isDuplicate(`tts:${text}`)) return

  // eslint-disable-next-line no-console
  console.warn(`[audio-diag] speak tts r=${RENDERER_AUDIO_ID} text=${text}`)

  // A "piper:" voice is synthesized by the backend into a cached WAV and played
  // as a sound file; anything else is a local Web Speech voice. The effective
  // voice honours the app-default fallback, so a default of Piper routes here.
  if (isPiperVoice(voice || defaultTTSVoice)) {
    piperSynthesize(text)
      .then(({ path }) => {
        // playSound has its own owner check and a sound-keyed dedup window,
        // distinct from the tts key consumed above, so this plays exactly once
        // through the GC-safe activePlaybacks pipeline.
        if (path) playSound(path, volume)
      })
      .catch((err: unknown) => {
        notifyPiperFallback(err)
        speakViaWebSpeech(text, '', volume) // OS default voice as the fallback
      })
    return
  }

  speakViaWebSpeech(text, voice, volume)
}

/**
 * Speak via the Web Speech Synthesis API with the given voice name (empty =
 * app default / OS default). The production TTS path and the Piper fallback
 * both funnel through here.
 */
function speakViaWebSpeech(text: string, voice: string, volume: number): void {
  if (!window.speechSynthesis) return

  // NOTE: do NOT call speechSynthesis.cancel() here. We used to, on the
  // theory that it kept rapid alerts from piling up, but it cancels the
  // previous utterance mid-sentence whenever a second trigger fires within
  // a second of the first — the symptom the user saw as "Mez broke" being
  // chopped off when "Charm broke" arrived right after. Letting utterances
  // queue naturally is the correct behaviour; the dedup window above
  // already keeps a single trigger from spamming the same line.

  const utterance = new SpeechSynthesisUtterance(text)
  utterance.volume = effectiveVolume(volume)
  utterance.rate = ttsRate
  utterance.onerror = (e) => {
    // eslint-disable-next-line no-console
    console.warn('[audio] speakText failed', { text, voice, error: e.error })
  }

  speakWithVoice(utterance, voice || defaultTTSVoice)
}

/**
 * Returns the list of available TTS voice names, sorted alphabetically.
 * May be empty until the browser has loaded the voice list.
 */
export function getAvailableVoices(): string[] {
  if (!window.speechSynthesis) return []
  refreshVoices()
  return cachedVoices.map((v) => v.name).sort()
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
  rate?: number,
): void {
  stopTestPlayback()
  if (!text) {
    onEnd?.()
    return
  }

  // Piper preview: synthesize (cache hit = instant, miss = a one-time spawn)
  // then play the WAV as a file test. On any failure, preview the Web Speech
  // fallback so the button still resolves and the user hears *something*.
  if (isPiperVoice(voice || defaultTTSVoice)) {
    piperSynthesize(text)
      .then(({ path }) => {
        if (path) playSoundForTest(path, volume, onEnd)
        else onEnd?.()
      })
      .catch((err: unknown) => {
        notifyPiperFallback(err)
        speakViaWebSpeechForTest(text, '', volume, onEnd, rate)
      })
    return
  }

  speakViaWebSpeechForTest(text, voice, volume, onEnd, rate)
}

function speakViaWebSpeechForTest(
  text: string,
  voice: string,
  volume: number,
  onEnd?: () => void,
  rate?: number,
): void {
  if (!window.speechSynthesis) {
    onEnd?.()
    return
  }
  const utterance = new SpeechSynthesisUtterance(text)
  utterance.volume = effectiveVolume(volume)
  // An explicit rate lets a settings preview reflect the live slider value
  // before it's been saved and pushed back through useAudioPrefs; otherwise
  // fall back to the configured global rate. Clamped to the same 0.5–2.0 band.
  utterance.rate =
    typeof rate === 'number' && rate > 0
      ? Math.min(2, Math.max(0.5, rate))
      : ttsRate
  // No deferred speak here — tests run from an editor whose voice dropdown has
  // already primed the list, and the play/stop bookkeeping wants an immediate
  // utterance. Blank voice previews with the app default so the test matches
  // what a real fire will sound like.
  const match = resolveVoice(voice || defaultTTSVoice)
  if (match) utterance.voice = match

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
