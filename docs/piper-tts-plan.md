# Piper TTS (User-Installed Local Voice) — Implementation Plan

> **Status:** Planned / not implemented. Research + design captured so we can
> resume easily later. Nothing here is built yet.
>
> **Last updated:** 2026-07-06
>
> **Sibling doc:** [`cloud-tts-plan.md`](cloud-tts-plan.md). Piper reuses almost
> all of that plan's infrastructure (the generate→cache→replay reframe, the
> `tts-cache/` dir, the `audio.ts` routing branch, the local-voice fallback).
> **Read that doc first** — this one only covers what's different for a *local,
> user-installed* engine. The recommended end state is a **single shared TTS
> provider abstraction** with cloud and Piper as two providers under it, not two
> parallel systems. See §7.

## 1. Problem & Background

Users ask for better-sounding TTS than the robotic Web Speech API voices PQC is
limited to (Chromium/OneCore wall — see `cloud-tts-plan.md` §1). One user
(Larcen, Discord 2026-07) requested integrating **Piper**, a fast local neural
TTS engine, and reported running it on an older laptop with an md5-keyed WAV
cache to hide synthesis latency.

Piper produces genuinely natural voices, runs fully offline on CPU, and is free
(no API key, no quota). The trade-off vs cloud TTS is **setup friction** — the
user has to install Piper themselves — which is why this is a **power-user,
opt-in** feature, not a default.

### The critical constraint that shapes this whole plan

The repo Larcen linked — **`OHF-Voice/piper1-gpl`** — is **not** the old
bundleable Piper. Two facts (verified 2026-07-06) drive the design:

1. **No standalone binary.** piper1-gpl ships *only* Python wheels
   (`pip install piper-tts`); every release asset is a `.whl`/`.tar.gz`. There
   is no `piper.exe` to drop in. Running it means a Python runtime, or
   self-compiling `libpiper` via CMake, or using the old frozen binary (below).
2. **GPL-3.0** (the old `rhasspy/piper` was MIT; relicensed because
   phonemization statically links GPL espeak-ng). **Bundling** a GPL binary in
   our closed-source installer drags GPL redistribution obligations onto it.

**Design consequence:** we **do not bundle Piper**. We treat it as a
**user-installed external tool** we detect and call — exactly the pattern we
already use for **Zeal** and **EQW**. Because we distribute nothing, the GPL
redistribution problem disappears entirely, install size is unchanged, and users
who don't care are completely unaffected.

> If we ever *did* want an out-of-the-box neural voice for everyone, the path is
> **sherpa-onnx** (Apache-2.0, ships a prebuilt Windows CLI that runs the same
> Piper `.onnx` voices) — that's a *separate, larger* decision with a permanent
> ~100–160 MB install cost and a maintenance burden. Out of scope here; noted so
> we don't conflate the two.

## 2. What "user-installed" means (there is no API key)

Piper is a **local program**, not a cloud service — so "linking our app to it"
is not an API key. It's one of two localhost mechanisms:

- **Mode A — Executable path + spawn (recommended for v1).** User points us at
  a Piper executable (the frozen MIT standalone exe, a `pip`-installed
  `piper`/`python -m piper` console script, or their own build) plus a voice
  model file. Our Go backend spawns it per phrase (text in → WAV out), caches
  the WAV, and plays it through the existing `pq-audio://` pipeline. Simple, no
  long-running process to babysit.
  - **Latency caveat:** the piper1-gpl CLI *reloads the model on every
    invocation*, so a cold spawn of a *new* phrase has model-load lag on weak
    hardware (this is exactly why Larcen cached). Our cache + pre-generate-on-
    save (§5) makes repeats and static callouts instant, which covers the
    timing-critical cases. Good enough for v1.
- **Mode B — Local HTTP server (phase 2, power users).** Piper ships
  `python -m piper.http_server`, which loads the model once and stays warm.
  User runs it; we POST text to `http://localhost:<port>` and get a WAV back.
  Warm model = fast enough for live/dynamic text. Cost: the user has to keep a
  server running (or we offer to spawn it if they gave us a runnable command).

**v1 = Mode A only.** It's the smallest surface and the cache neutralizes the
latency downside for the callouts people actually care about.

### Because it's local + free, dynamic text is cheaper than cloud

Cloud TTS restricts token-bearing text (`"Mez breaking on {mob}"`) because each
unique resolved string costs credits. Piper has **no per-phrase cost**, so we
can cache-by-resolved-text freely — the only downside is a one-time cold-spawn
lag the first time a given resolved string is seen. So Piper can support dynamic
text where cloud recommends against it (still cache by resolved value; still
recommend static text for the most timing-critical fires).

## 3. Getting Piper installed (user's responsibility, we guide)

We ship nothing and we don't manage updates — same posture as Zeal/EQW. The
settings UI links to install instructions and detects what's present:

- **Easiest for users:** the old **frozen MIT standalone** `piper_windows_amd64`
  binary (`rhasspy/piper` release `2023.11.14-2`) — a true self-contained exe,
  stdin/args → WAV, no Python. Unmaintained (2023 engine/voices) but perfectly
  functional for alert callouts. This is likely what most of the "handful" will
  use, and it dodges both the Python-runtime and (being MIT, if we *ever*
  reconsidered bundling) the GPL issues.
- **Maintained path:** `pip install piper-tts` → `python -m piper` (needs
  Python). For users who already have Python and want current voices.
- **Voice models:** user downloads their own `*.onnx` + `*.onnx.json` from
  Hugging Face `rhasspy/piper-voices` (~60 MB medium, ~120–140 MB high). We link
  to the catalog; we don't host or ship models. Each voice carries its own
  (usually CC-BY) license — the user's relationship, not ours; a one-line note
  in settings suffices.

## 4. Architecture Decision: where does synthesis run?

**Go backend, not the renderer** — same rationale as `cloud-tts-plan.md` §4, plus:

- **Spawning subprocesses is a backend concern.** Note: the Go backend has
  *zero* `exec.Command` usage today — Piper would introduce the **first**
  subprocess it manages. Keep it contained in one package with tight timeouts,
  output-size limits, and no shell interpolation (pass args as a slice, never a
  shell string).
- Go already owns `user.db`, file I/O, and the `~/.pq-companion` tree where the
  cache lives.
- Renderer stays thin: "this voice is a Piper voice → ask backend for the cached
  WAV path → play it through the existing pipeline." Identical to the cloud
  branch.

## 5. Backend (Go) Design

Prefer a **shared TTS abstraction** so cloud and Piper are sibling providers
(see §7). If built standalone first, package `backend/internal/pipertts/`:

```
internal/pipertts/            (or a provider under internal/tts/)
  detect.go     // locate/validate piper executable + model; version probe
  synth.go      // spawn piper (Mode A) → WAV bytes; timeouts, arg-slice, no shell
  server.go     // Mode B (phase 2): POST to local http_server
  cache.go      // content-addressed WAV cache (shared with cloud — see §7)
  service.go    // resolve voice → cache hit or synth → return path
  models.go     // PiperConfig, PiperVoice, request/response types
```

### Config (stored in `user.db` or config.yaml, mirrors Zeal/EQW settings)

```
piper_enabled        bool     // master on/off
piper_exe_path       string   // path to piper executable (Mode A)
piper_model_path     string   // path to a .onnx voice (+ implied .onnx.json)
piper_mode           string   // "spawn" (v1) | "http" (phase 2)
piper_server_url     string   // Mode B only
```

No API key, no secrets — so no DPAPI/leak concerns like the cloud plan has.

### Cache (content-addressed — SHARE with cloud-tts)

- Key: `sha256("piper" + modelPath + normalizedText)`. Namespacing by provider
  lets the *same* `tts-cache/` dir hold cloud and Piper files without collision.
- File: `~/.pq-companion/tts-cache/<hash>.wav`.
- Same GC/"Clear cache" story as `cloud-tts-plan.md` §5 — build it once, both
  providers use it.

### API endpoints (`internal/api/pipertts.go`, or merged into a `tts` handler)

- `GET  /api/piper/status` — detected? exe/model valid? version string? Powers
  the settings status card (Zeal/EQW-style: installed / not-found / bad-path).
- `POST /api/piper/validate` — store + test the configured exe+model path.
- `POST /api/piper/synthesize` — `{text}` → cached WAV path (synth on miss).
  Used by trigger save (pre-generate), settings "Test voice", and lazy
  fire-time fallback. Same contract shape as `/api/cloudtts/synthesize` so the
  frontend can route to one abstraction.

## 6. Frontend Design

Nearly identical to `cloud-tts-plan.md` §6 — reuse it:

- **Voice list merge:** Piper voice(s) appear in the same voice dropdown used by
  `NotificationActionEditor.tsx` and `AlertDefaultsSettings.tsx`, namespaced so
  they're unambiguous, e.g. `🔊 Amy (Piper, local)`. Store a **structured voice
  ref** (`voice_provider: "piper"` + model id), never a bare name string — the
  same field the cloud plan adds. **This is the one shared frontend change both
  plans depend on; do it once.**
- **Playback routing in `audio.ts`:** the branch is provider-agnostic —
  ```
  if (voice is not a local Web Speech voice) {
      path = await api.ttsSynthesize({provider, voiceRef, text})  // piper OR cloud
      playAudioFile(path, volume)   // existing activePlaybacks pipeline
  } else {
      // existing window.speechSynthesis path (unchanged)
  }
  ```
- **Settings UI — new "Local TTS (Piper)" section**, styled like the Zeal/EQW
  status panels:
  - Enable toggle.
  - Piper executable path picker + **"Test voice"** button → status
    (found/valid/version, or a clear "not found — [install guide]" link).
  - Voice model path picker (with a link to the HF voice catalog).
  - One-line note: Piper + voice models are user-installed and separately
    licensed (GPL engine / CC-BY voices); PQC bundles neither.
  - "Clear TTS cache" (shared control with cloud).
- **Trigger editor:** Piper voices appear in the same dropdown; on save with a
  Piper voice + static text, call synthesize to pre-generate (one-time spawn,
  show a brief "generating…"). Token text is *allowed* (unlike cloud) but warn
  that the first fire of each new resolved string may lag on slow hardware.

## 7. The shared-abstraction recommendation (important)

Both this plan and `cloud-tts-plan.md` reduce to the **same** reframe: *"a
non-Web-Speech voice = a WAV generated once, cached, and replayed like any sound
file."* They differ only in *how the WAV is produced* (local subprocess vs cloud
HTTP + key). **Build the abstraction once:**

```
internal/tts/
  provider.go   // Provider interface: ListVoices, Synthesize, Validate, ID
  cache.go      // one content-addressed cache, provider-namespaced keys
  service.go    // resolve voice ref → provider → cache hit or synth → path
  cloud/        // elevenlabs, etc. (from cloud-tts-plan)
  piper/        // local spawn / http
```

- **One** `voice_provider`/`voice_id` action-field change (not two).
- **One** `audio.ts` routing branch (not two).
- **One** cache + "Clear cache" control.
- **One** `POST /api/tts/synthesize` the frontend calls regardless of engine.

Whichever plan ships first should build this shared layer; the second becomes
"add a provider." If Piper ships first, it's arguably the *better* one to build
the abstraction around because it has no key/quota/network complexity to muddy
the interface.

## 8. Edge Cases & Risks

- **Piper not installed / bad path:** `status` reports not-found; TTS actions
  with a Piper voice fall back to local Web Speech for that fire + a non-blocking
  toast. Never hard-fail an alert.
- **Spawn failure / timeout / garbage output:** enforce a synth timeout and a
  max output size; on failure, fall back to Web Speech + log. A broken external
  tool must never wedge the alert pipeline.
- **Subprocess safety:** args as a slice (no shell), validate the exe path is a
  file we can execute, cap concurrent spawns, kill on timeout. This is the first
  subprocess the backend spawns — get the hygiene right up front.
- **Cold-spawn lag on weak hardware:** mitigated by pre-generate-on-save + cache
  (Larcen's exact approach). Document that the first utterance of a brand-new
  phrase may lag; everything cached is instant. Mode B (warm server) is the
  phase-2 answer for people who want zero-lag dynamic text.
- **Cache growth / license notes / offline:** same as `cloud-tts-plan.md` §8.
  Offline is a non-issue — Piper *is* offline.
- **Windows AV false positives:** we ship no binary, so **none of our concern** —
  the user's own Piper install is theirs. A genuine advantage of this posture.
- **Linux/Wine:** cached-WAV playback might work where Web Speech doesn't (same
  potential side benefit noted in the cloud plan); not a v1 goal.

## 9. Phased Rollout

**Phase 1 — MVP (Mode A, spawn + cache)**
- Shared `internal/tts` abstraction + cache (or `pipertts` if built standalone).
- Detect/validate Piper exe + model; `status` + `validate` + `synthesize`
  endpoints.
- Structured voice-ref action field + `audio.ts` routing branch (shared with
  future cloud work).
- Settings "Local TTS (Piper)" section (Zeal/EQW-style status card) + "Test
  voice".
- Pre-generate-on-save + cache; Web Speech fallback on any failure.

**Phase 2 — Warm server & robustness**
- Mode B: local `http_server` support (optionally spawn+supervise it).
- Cache GC + "Clear cache" + size display (shared control).
- Better dynamic-text UX now that latency is gone with a warm model.

**Phase 3 — Convergence**
- Fold cloud TTS (`cloud-tts-plan.md`) in as a sibling provider under the same
  interface if not already done.
- Optional: bundle a curated voice for one-click setup **only if** we revisit
  the sherpa-onnx (Apache-2.0) route — separate decision, separate size budget.

## 10. Open Questions

- **Build shared `internal/tts` now, or `pipertts` standalone and refactor
  later?** Leaning shared-from-the-start (§7) since the cloud plan already
  defines the interface, but standalone is a valid smaller v1.
- **Mode A vs Mode B for v1:** plan says A (simplest, cache covers latency).
  Confirm no one actually needs zero-lag *uncached dynamic* text on day one.
- **Which Piper do we tell users to install** in the guide — lead with the frozen
  MIT standalone exe (easiest, no Python) or `pip install piper-tts` (maintained,
  needs Python)? Probably document both, recommend the standalone for
  non-technical users.
- **Voice model UX:** single configured voice for v1, or allow several Piper
  voices selectable per-trigger? Interface supports many; UI can start with one.
- **Do we help users fetch a voice** (a "download a starter voice" button hitting
  HF) or purely link out? Linking out is zero-liability for v1.

## 11. Effort Estimate

**Low–Medium.** Smaller than the cloud plan in some ways (no API-key storage, no
secret handling, no quota/billing, no network-error taxonomy — Piper is offline
and free), but adds the backend's **first subprocess management** (do the safety
hygiene properly) and an install-detection/status UX. If the shared `internal/
tts` layer is built here, some of the effort is really "infrastructure both
features share," and cloud TTS later becomes a small add-on.

The core is the same one-line idea as the cloud plan: **generate a voice into a
cached WAV and play it like any other sound file** — Piper just produces that WAV
from a local process instead of a cloud call. No changes to the timer engine,
overlays, or WS protocol are required.
