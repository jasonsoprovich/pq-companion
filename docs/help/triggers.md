---
title: "Custom Triggers & Alerts"
slug: triggers
section: "Live & combat"
order: 130
routes: ["/triggers"]
keywords: []
---
# Custom Triggers & Alerts

**Prerequisite:** `/log on`.

A regex trigger engine for any log line. Each trigger fires one or more actions:

- **On-screen text** with draggable position and configurable font size.
- **Local sound files** via the native file picker, with per-action volume.
- **Text-to-speech** for hands-busy alerts.
- **Copy to clipboard** — drop matched text straight onto your clipboard, handy for capturing a name or a number to paste elsewhere.
- **Discord webhook** — post the matched alert straight to a Discord channel via a webhook URL you paste in, for alerts you want your raid or guild to see outside the game.

### Starting from a template

**New Trigger** opens a short picker: a **Text alert** (speak or show a message when a line appears), a **Spell timer** (pick a spell; its timer and fading warning are filled in), a **Mob emote warning** (paste the emote, get a red on-screen warning plus speech), a **Tell or chat alert** (tells, group, guild, raid, OOC, shout or auction, optionally from one person), a **Countdown timer**, **Start blank**, or **Install a pack**. Each one creates the trigger in a click or two, or opens it in the full editor first.

### Plain text or regex

The editor groups its fields into **Basics**, **Actions**, **Timer** and **Advanced**; Timer and Advanced stay folded until they hold something.

Patterns can be typed as **Plain text**: write the line as it appears in your log and it matches anywhere in the line. `{name}` (any word you like) captures whatever text is there, and you can reuse it in the alert, for example `{name} tells you, '{msg}'` with the alert `{name} sent a tell`. `{C}` is your character's name, `{N}` a number, and `{{` `}}` are literal braces. The trigger always stores the regex, so switching to **Regex** shows exactly what will match. A regex that uses real regex features (like `\w+` or alternation) opens in Regex mode and stays there; the editor says why.

### Multiple patterns & GINA tokens

A single trigger can hold several regex patterns, each with its own on/off toggle, so different log lines that mean the same thing all drive one alert. Patterns understand GINA-style `{c}` (your character name) and `{target}` tokens, so shared community triggers work without rewriting them, and any regex capture group can be substituted into the alert text.

### Group members: {groupmember}

`{groupmember}` matches a player name only when that player is in your group. Built-in **Group Member Died** uses it, so it no longer fires for other deaths. It needs Zeal's live pipe to know your group; without Zeal (or during a log replay) it matches any name.

### Capturing the target name

A capture group — say, the name of the mob you just mezzed — can flow straight into both the alert text and the timer the trigger starts, so the bar on your buff or detrimental overlay is labelled with the actual target instead of a generic spell name. Per-pattern timer overrides let each pattern start its own capture-keyed timer.

### Custom categories

Organise triggers into your own categories and drag to reorder both the categories and the triggers inside them. Triggers that came from a pack keep a pack-origin tag even after you move them, so you always know where one came from. An Export button next to rename/delete on each category turns just that category into its own shareable trigger pack — no need to export everything to hand a guild your "Raid Triggers" category.

### Alert text style

Customise the on-screen alert text — colour, glow, font, size, and left/center/right text alignment — with a positioning card that doubles as a live preview, so you can see exactly how an alert will look before it fires. A one-click Reset Style returns any trigger to the app defaults.

### Voice rate

A global speed control under **Settings → Audio** speeds up or slows down every spoken alert, with a Test button that previews the rate live before you save it.

### Local TTS voices — Piper & Kokoro

Text-to-speech alerts can use one of two locally-run neural voices instead of the browser's built-in speech synthesis, for a more natural read. Both live under **Settings → Audio**, and once enabled and ready, their voice shows up in every alert and trigger voice dropdown alongside the browser voices. PQ Companion bundles neither — you install or host them yourself.

#### Piper — a locally-installed executable

Piper runs as a program on your machine that PQ Companion spawns to synthesize speech. To set it up:

1. Install Piper — grab the [standalone executable](https://github.com/rhasspy/piper/releases/tag/2023.11.14-2) (no Python needed) or follow the [full install guide](https://github.com/OHF-Voice/piper1-gpl).
2. Download a voice from the [voice catalog](https://huggingface.co/rhasspy/piper-voices/tree/main) — you need both the `.onnx` model file and its matching `.onnx.json` config, kept side by side.
3. In **Settings → Audio → Local TTS (Piper)**, turn it on and point the executable and voice-model fields at the files from steps 1–2 (use Browse, or paste the paths). Green dots confirm each path resolves; a readiness banner confirms the voice is usable, and Test Voice plays a sample line.

Choose a synthesis mode: **Spawn per phrase** is simple but a brand-new phrase may lag briefly, while **Keep Piper running (warm)** keeps the model loaded in the background for near-instant synthesis at the cost of a bit of memory. Generated audio is cached so repeat lines are instant either way, and PQ Companion falls back to the browser voice automatically if anything goes wrong. The Piper engine (GPL) and voice models (usually CC-BY) are licensed separately from PQ Companion.

#### Kokoro — a self-hosted server

Kokoro is a second local voice option with its own catalog. Unlike Piper, PQ Companion doesn't spawn it — instead it talks to a [Kokoro-FastAPI](https://github.com/remsky/Kokoro-FastAPI) server you run yourself (for example via `docker run`) over plain HTTP, so there's nothing to download into the app itself. To set it up:

1. Start a Kokoro-FastAPI server (see the project's own instructions for Docker or a bare-metal install).
2. In **Settings → Audio → Local TTS (Kokoro)**, turn it on and enter the server's URL (e.g. `http://localhost:8880`).
3. Once connected, the voice dropdown populates live from the server's own voice catalog — pick one and hit Test Voice to confirm.

Generated audio is cached (the same cache Piper uses — Clear TTS Cache on either card clears both), and it falls back to the browser voice automatically if the server becomes unreachable.

### Exclude patterns

Each trigger can carry a list of exclude patterns (one regex per line). When any exclude matches the same log line as the primary pattern, firing is suppressed — perfect for "fire on player tells, but not on pet `Master.` responses or merchant/banker/trainer canned phrases." The General Triggers pack's Incoming Tell trigger ships with sensible defaults; add bazaar trader names you recognise to silence those too.

### Triggers that drive spell timers

A trigger can do more than fire an alert — it can also start a countdown on the buff or detrimental overlay. Set a timer duration, optionally a worn-off pattern that clears the row before its natural expiry, and per-trigger "fading soon" audio cues at any thresholds you want. Each trigger can carry a per-trigger display threshold that overrides the global category default.

### Pipe-source triggers (with Zeal)

When Zeal is connected, triggers can fire off the live pipe stream instead of log text. Pipe sources include target name change, target HP crossing a threshold percentage, a named buff landing or fading on you, and your own in-game `/pipe <text>` commands. Pipe triggers don't depend on the log file, so they're immune to log latency and to emote-only events the log doesn't capture.

### Pack-grouped UI

Built-in and imported packs render as collapsible groups on the Triggers page, with a pack filter and sort in the header. Always-on event alerts (deaths, zoning, resists, interrupts) live under the General Triggers group rather than as a separate page. Triggers that are shared across packs — the same Mez Break alert appearing in Enchanter and Bard packs, for example — carry a **shared** badge and use a `dedup_key` so a single log line fires the alert once, not once per pack.

### Per-character scoping

Triggers can be scoped to specific characters — handy for class-specific packs that should only fire when you're on that character. Leave the character list empty to fire for any active character.

### Built-in packs

Class packs ship for all 15 classes (Bard, Beastlord, Cleric, Druid, Enchanter, Magician, Monk, Necromancer, Paladin, Ranger, Rogue, Shadowknight, Shaman, Warrior, Wizard). Each bundles the relevant CC-break and cast-failure alerts plus spell timers for the class's core buffs, debuffs, roots, mezzes, charms, and pacify lines — including their Planes of Power upgrades, such as Command of Druzzil, Voice of Quellious, Tranquility, Virtue and Seduction of Saryrn. Nine general packs sit alongside them: General Triggers (incoming tells, deaths, zoning), Caster Alerts, Crit Alerts, Spell Breaks, Group Alerts, Raid Alerts, Tracking, Misc Alerts and Slows. Raid packs sit under their own Raid Packs header: Raid: Scripted Event Timers plus one pack per raid era with boss timers, cure alerts and Who cards.

### Raid: Scripted Event Timers pack

Countdown timers for scripted raid events that run on a server clock: Paffa's waves, Coirnav's final-phase reinforcements, the Plane of Time instance clock, Plane of Air boss despawns, the Emperor and his room curse, Doomshade, Ring of Fear, the Great Divide Ring War, Hollowshade invasions and the Grimling war. It covers every era and works alongside the era raid packs, which leave these event clocks to it. Install it from the Raid Packs section of the Packs tab.

### Non-destructive pack updates

When a built-in pack ships changes, a review panel on the Packs tab shows a per-field diff between your installed version and the new default. Apply it install-by-install, and choose per field whether to preserve your own customization or take the new default — an update never silently overwrites edits you made on purpose.

### Action templates & bulk editing

Save an action as a template and apply it to many triggers at once instead of editing each one by hand. Bulk editing also covers converting text-to-speech alerts to sound (including timer alerts) across a whole selection, with control over exactly which triggers a bulk edit touches.

### Test with log replay

Replay mode plays a recorded segment of your log back through the live pipeline, so you can confirm a trigger fires (and an overlay behaves) against real lines without being in game. Triggers, timers, meters and overlays all run during a replay, but it never re-records old events — kill counts, loot, lockouts, PoP flags and the like stay as they were — and Discord webhook actions don't fire. The Log Feed page also has a read-only Browse mode for reading old logs out of game.

### Multi-format import

A unified **Import Triggers** wizard reads the four common share formats and converts them into PQ Companion triggers, so you don't have to rewrite community packs by hand:

- **GINA** shares as `.xml` or `.gtp`
- **EQNag** trigger databases, with `${Var}` placeholders mapped to named captures
- **EQLogParser** `.tgf` trigger trees
- **PQ Companion** packs exported as JSON from another install

The wizard detects the format, previews the triggers it found, and lets you commit them into a category you choose — so you see exactly what you're importing before it lands.

### Refire cooldowns

A noisy log line can fire the same trigger many times in a second. Each trigger has a **refire cooldown** (a lockout that suppresses re-fires for a set window) and a separate **repeat-audio cooldown** that lets the on-screen alert keep up while stopping the sound from stacking on itself — so a burst of matching lines doesn't turn into a wall of overlapping alerts.

### Repeat count: {repeated}

A per-trigger **Repeat count window** counts matches that land within N seconds of each other, so a PBAE trigger can show how many mobs it hit. `{repeated}` and `{count}` work in action and timer-alert text, and follow-up matches update the existing on-screen alert instead of stacking a new one. The EQLogParser importer maps RepeatedResetTime for triggers that use `{REPEATED}`.

### Tips

- Use the live test button beside each trigger to fire it without waiting for the log line — handy for tuning position, font size, and volume.
- Drag the on-screen alert to where you want it; the position persists across sessions.

### Target HP filters, Enrage Soon & strip callouts

Target HP pipe triggers can be narrowed to a mob name or to targets with a chosen special ability. The Raid Alerts pack uses this for **Enrage Soon**, a banner and spoken alert when a mob that enrages drops to 12%, just ahead of its 10% enrage (needs Zeal for target HP).

Two conditions call out dispels: **buff stripped** fires when one of your buffs that Zeal has confirmed leaves early and stays gone (clicking a buff off counts too, since the pipe can't tell them apart), and **debuff stripped** fires when one of your detrimentals wears off early alongside a dispel on that mob, from the log alone. Both take an optional spell filter, and the alert text can use `{spell}` and `{target}`.

### Lookarounds & backreferences

GINA, EQLogParser and EQNag exports use .NET regular expressions. Patterns with lookahead, lookbehind or backreferences, which used to import disabled, now run as written. Pipe triggers can be saved without a regex pattern.
