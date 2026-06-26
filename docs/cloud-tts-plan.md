# Cloud TTS (Bring-Your-Own-Key) — Implementation Plan

> **Status:** Planned / not implemented. This document captures the design so we
> can pull the trigger on implementation later. Nothing here is built yet.
>
> **Last updated:** 2026-06-26

## 1. Problem & Background

Users repeatedly ask for better-sounding TTS voices. PQC's trigger and alert TTS
currently sounds robotic because of a hard platform limitation, not a code
choice we can easily tweak.

### Why we only see robotic voices today

PQC speaks through the browser **Web Speech API** (`window.speechSynthesis`)
inside Chromium/Electron. Relevant code:

- `frontend/src/services/audio.ts` — voice enumeration (`getVoices()`), speech
  (`SpeechSynthesisUtterance`), the `activePlaybacks` Set for file playback, and
  test playback.
- `frontend/src/hooks/useVoices.ts` — exposes the voice list to the UI.
- `frontend/src/hooks/useAudioEngine.ts` — dispatches trigger actions to TTS.
- `frontend/src/components/NotificationActionEditor.tsx` — per-trigger TTS voice
  dropdown (`TextToSpeechFields`).
- `frontend/src/components/settings/AlertDefaultsSettings.tsx` — "Default TTS
  Voice" dropdown (`config.preferences.default_tts_voice`).
- `electron/main/index.ts` (~lines 63–69) — `autoplay-policy` switch so TTS/audio
  works before a user gesture.

**The wall:** Chromium on Windows reads only the **OneCore** voice list and
deliberately ignores SAPI5 voices whenever any OneCore voice exists (known,
unfixed upstream Chromium behavior). So:

| Engine | Registry | Who reads it |
|---|---|---|
| OneCore | `HKLM\…\Speech_OneCore\Voices` (Settings → Speech → Manage Voices) | **Chromium → PQC** |
| SAPI5 | `HKLM\…\Speech\Voices` (Control Panel) | .NET apps (eqlogparser) |
| Narrator Natural | private store | Narrator only |

This is why eqlogparser (a .NET/SAPI app) sees more voices than PQC, why
Woof's "Manage Voices" trick worked (it adds *OneCore* voices), and why the
good Narrator Natural / Azure neural voices are unreachable from inside the Web
Speech API. We cannot unlock high-quality voices through `speechSynthesis`; we
have to **bypass it**.

### The core design idea

> **Treat a cloud voice as a custom audio file that is generated on demand and
> cached.** PQC already plays custom audio files well (the `activePlaybacks`
> path in `audio.ts`). If we synthesize each unique trigger phrase **once**,
> write it to disk, and replay the cached file, then the network cost is paid
> at edit time and the fire-time latency equals a local sound file.

This single reframe is what makes the feature viable and keeps the
implementation small.

## 2. Latency Strategy (the make-or-break concern)

Live synthesis of a short phrase costs ~300ms–1.5s (Azure/Google ~300–700ms
streaming; ElevenLabs ~400ms–1.5s, faster on Flash/Turbo models). That is **too
slow** for timing-critical calls (mez/charm/CH breaks). We never synthesize live
in the hot path. Instead:

1. **Pre-generate at save time.** When a user assigns a cloud voice + static text
   to a trigger, synthesize the audio immediately and cache it keyed by
   `(provider, voiceId, model, text)`. Fire-time = local disk read.
2. **Lazy fallback + warm cache.** Anything not yet cached is generated on first
   encounter, then cached; at worst one fire is slow. Optionally pre-warm the
   whole cache on launch.
3. **Dynamic/token text** (e.g. `"Mez breaking on ${mob}"`) cannot be
   pre-cached because the string is unknown until fire time. Options:
   - Cache by *resolved* text (second occurrence of the same value is instant).
   - **Recommended:** restrict cloud voices to **static text**; token-bearing
     TTS stays on local Web Speech voices. Most timing-critical callouts
     (`"Mez break"`, `"GET OUT"`) are static anyway.

## 3. Provider: ElevenLabs first

- **API access is available on the free plan** (10,000 credits/month ≈ ~10 min
  of audio; ~1 credit/char on Multilingual v2, ~0.5/char on Flash).
- Because we cache, steady-state credit use is near zero — credits are spent
  once per unique phrase. A 50-trigger setup spends well under 1,000 credits
  total and never re-spends unless text changes.
- **Default to the Flash model**: half the credit cost and lowest latency
  (matters for the one-time generation and lazy fallback).
- Free tier is **non-commercial + attribution required**; user brings their own
  key and account, so the license relationship is theirs. Add a one-line note in
  the settings UI. No liability for PQC.

Provider is abstracted (see §5) so Azure Neural / Google can be added later
without touching the trigger/audio plumbing.

## 4. Architecture Decision: where does synthesis run?

**Recommendation: synthesize in the Go backend, not the renderer.**

Rationale:
- Keeps API keys out of the renderer / out of WebSocket traffic; key lives in
  `user.db` and never leaves the Go process except in the outbound provider call.
- Go already owns `user.db`, file I/O, and HTTP clients; the cache dir is a
  natural fit next to existing user data.
- Matches the project convention ("all business logic lives in Go").

Renderer's job stays thin: when an action's voice is a cloud voice, request the
cached file path from the backend and play it through the existing audio
pipeline.

## 5. Backend (Go) Design

New package: `backend/internal/cloudtts/`.

```
internal/cloudtts/
  provider.go      // Provider interface + registry
  elevenlabs.go    // ElevenLabs implementation (TTS + voice list)
  cache.go         // content-addressed audio cache
  service.go       // orchestration: resolve voice → cache hit or synth → path
  models.go        // CloudVoice, ProviderConfig, request/response types
```

### Provider interface

```go
type Provider interface {
    ID() string                                   // "elevenlabs"
    ListVoices(ctx context.Context) ([]CloudVoice, error)
    Synthesize(ctx context.Context, req SynthRequest) ([]byte, string, error) // audio bytes, mime
    ValidateKey(ctx context.Context) error
}

type CloudVoice struct {
    Provider string `json:"provider"`
    VoiceID  string `json:"voice_id"`
    Name     string `json:"name"`
    Model    string `json:"model"`     // default flash
}
```

### Cache (content-addressed)

- Key: `sha256(provider + voiceId + model + normalizedText)`.
- File: `~/.pq-companion/tts-cache/<hash>.mp3`.
- Index row in `user.db` for bookkeeping / GC (optional but useful for "clear
  cache" and orphan cleanup).
- **Invalidation:** key includes text+voice+model, so any edit naturally
  produces a new key. Old files GC'd by a sweep that drops hashes no longer
  referenced by any trigger/default + an LRU age cap.

### API endpoints (`internal/api/cloudtts.go`)

- `POST /api/cloudtts/providers/{id}/validate` — store + validate key.
- `GET  /api/cloudtts/voices` — merged list across configured providers.
- `POST /api/cloudtts/synthesize` — `{provider, voiceId, model, text}` →
  returns cached file path (synthesizes + caches on miss). Used by:
  - trigger save (pre-generate),
  - settings "test voice" button,
  - lazy fallback at fire time.
- `POST /api/cloudtts/prewarm` — batch pre-generate for all cloud-voiced
  triggers (called on launch or after import).

### Key storage

- Store provider keys in `user.db` (new `cloudtts_providers` table:
  `provider, api_key, model, enabled, updated_at`).
- At minimum keep the key out of logs and out of WS broadcasts. Consider
  OS-level protection later (DPAPI on Windows) — note as a follow-up, not a
  blocker for v1.

## 6. Frontend Design

### Voice list merge

- Extend `useVoices()` (or add `useCloudVoices()`) to fetch
  `GET /api/cloudtts/voices` and merge into the dropdown used by both
  `NotificationActionEditor.tsx` and `AlertDefaultsSettings.tsx`.
- Namespace cloud voices so they're visually distinct and unambiguous, e.g.
  `☁ Rachel (ElevenLabs)`. Store a structured voice ref, not just a display
  name, so we can tell local vs cloud at fire time. (Today the action stores a
  bare voice *name* string — we either add a `voice_provider` field or encode a
  prefix like `cloud:elevenlabs:<voiceId>`. Prefer an explicit field to avoid
  parsing.)

### Playback routing in `audio.ts`

In `speakText()` / `speakTextForTest()`:

```
if (voice is a cloud voice) {
    path = await api.cloudTtsSynthesize({provider, voiceId, model, text}) // cache hit = instant
    playAudioFile(path, volume)   // existing activePlaybacks pipeline
} else {
    // existing window.speechSynthesis path (unchanged)
}
```

The existing `activePlaybacks` GC fix (unreferenced `Audio()` getting collected
mid-play) already covers cached-file playback — reuse it, don't reinvent.

### Settings UI

New "Cloud TTS" section in settings:
- Provider picker (ElevenLabs to start).
- API key input + "Validate" button (calls validate endpoint).
- Model selector (default Flash).
- Free-tier / licensing note line.
- "Clear TTS cache" button.

### Trigger editor

- Cloud voices appear in the same dropdown.
- On trigger save with a cloud voice + static text → call synthesize to
  pre-generate (show a small "generating…" state; it's a one-time ~0.5–1s).
- If text contains tokens and a cloud voice is selected → warn and either block
  or fall back to local voice (per §2 decision).

## 7. Config / Types

- `frontend/src/types/config.ts` — add cloud-voice ref shape; keep
  `default_tts_voice` working for both local and cloud (structured value).
- `backend/internal/trigger/models.go` — `ActionTextToSpeech` gains optional
  `voice_provider` / `voice_id` / `model` (back-compat: empty = local Web
  Speech, existing behavior unchanged).

## 8. Edge Cases & Risks

- **Offline / API down:** synthesize fails → fall back to local Web Speech voice
  for that fire so the user still gets *an* alert. Log + surface a non-blocking
  toast. Cached phrases keep working offline.
- **Quota exhausted:** provider returns 401/429 → same fallback + a clear
  settings banner ("ElevenLabs quota reached").
- **Cache miss on a timing-critical fire** (un-prewarmed dynamic text): one slow
  fire, then cached. Document the static-text recommendation to avoid it.
- **Cache growth:** GC sweep (orphans + LRU age cap) + manual "Clear cache".
- **Key leakage:** never log keys, never send over WS, never expose via GET.
- **Linux/Wine users:** overlays/TTS already unsupported there; cloud TTS via
  cached file playback *might* actually work where Web Speech doesn't — nice
  side benefit, but not a v1 goal; don't advertise until tested.

## 9. Phased Rollout

**Phase 1 — MVP (ElevenLabs, static text only)**
- `cloudtts` package + ElevenLabs provider (Flash model).
- Key storage + validate endpoint + settings section.
- Voice list merge into the existing dropdown.
- Synthesize-on-save + cache + `audio.ts` routing.
- Local-voice fallback on any failure.
- Block/warn cloud voice on token-bearing text.

**Phase 2 — Robustness & UX**
- Prewarm-on-launch for all cloud-voiced triggers.
- Cache GC + "Clear cache" + cache-size display.
- Quota/error banners.
- "Test voice" button in settings + trigger editor.

**Phase 3 — More providers / dynamic text**
- Azure Neural and/or Google provider behind the same interface.
- Resolved-text caching for token triggers (opt-in).
- Optional DPAPI key encryption on Windows.

## 10. Open Questions

- Structured voice ref vs prefixed string for storing cloud voice selection —
  prefer a structured field; confirm against current `config.ts` shape before
  implementing.
- Do we want a single "default cloud provider" or allow multiple providers
  configured simultaneously? (Interface supports multiple; UI can start with
  one.)
- Should pre-generation be synchronous on save (blocks the save button briefly)
  or fire-and-forget with a background status? Lean synchronous for v1 so the
  user knows it worked.
- Cache location: `~/.pq-companion/tts-cache/` confirmed acceptable alongside
  `user.db`.

## 11. Effort Estimate

Medium. The architecture is intentionally small because it reduces to "generate
a cloud voice into a cached audio file and play it like any other sound file."
The real work is: provider client + key storage, the cache with invalidation,
the settings UI, and the `audio.ts` routing branch. No changes to the timer
engine, overlays, or WS protocol are required.
