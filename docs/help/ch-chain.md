---
title: "CH Chain & Metronome"
slug: ch-chain
section: "Live & combat"
order: 120
routes: []
keywords: []
---
# CH Chain & Metronome

**Prerequisite:** `/log on`, plus your cleric chain calling its heals out loud in a channel the log captures (raid, group, or the heal callout channel your raid uses).

A pair of overlays for running and following a Complete Heal chain. The app watches for CH callouts in your log and tracks the rotation in real time — who's healing, who's up next, and whether the chain is keeping pace — so a cleric in the chain (or the person calling it) can see the whole rotation at a glance instead of counting in their head.

### How it tracks the chain

CH-chain tracking is callout-driven: it reads the heal announcements your clerics post, not the spell mechanics themselves, so it works off the same log line everyone in the raid already sees. Each cast draws a cast-to-land bar so you can watch the heal travel from cast to landing, and the overlay measures the live cadence between casts and flags a **stall** when the next healer is late — the early warning that a chain is about to break. The default callout pattern is broad enough to catch the common raid conventions; tune it with a custom trigger if your raid uses an unusual format.

### Possible-miss detection

Each chain callout's captured target is watched for the spell's own heal-landed confirmation text — Complete Healing's exact bystander line for Clerics. If that confirmation never arrives before the callout's cast window elapses, the timer is flagged and the bar renders **red** instead of silently expiring, surfacing fizzles, interrupts, and skipped casts that would otherwise slip by unnoticed. It's on by default for Clerics. Druids filling a chain slot can opt in to the same detection for Superior Healing (the CH Chain overlay's settings in the **Overlays** hub) — it's opt-in there because Superior Healing's confirmation line is shared with over a dozen unrelated heal spells, so watching it unconditionally risks a filler heal on the same tank masking an actual missed cast.

### Personal CH metronome

The metronome overlay is the cleric's-eye view: a standalone window that paces *your* slot in the rotation, so you know exactly when to start casting to keep the cadence steady. Pop it out over the game like any other overlay. If the watched cleric misses a cast, the metronome keeps advancing by projecting off whichever other slot's callout is freshest, instead of stalling indefinitely.

When a cleric drops out for good, the raid compresses the chain. Once the clerics on both sides of a slot have kept calling for about two missed turns, the metronome treats that slot as out and follows the compressed chain, instead of cueing everyone behind it a beat late.

### Audio & TTS alerts

The metronome can fire an optional sound or spoken cue at two moments — countdown start and "cast now" — reusing the same sound/TTS picker as triggers (including Piper and Kokoro voices, see [Custom Triggers & Alerts](help:triggers)). Both cues are independently toggleable in the **CH Metronome** overlay's settings in the **Overlays** hub, and fire whether or not either metronome overlay window is open.

### Where it lives

Both the CH Chain panel and the CH Metronome are first-class dashboard panels with their own toggles in the **Overlays** view — switch them on only on the characters and fights where you need them, and pop either out as its own click-through window.
