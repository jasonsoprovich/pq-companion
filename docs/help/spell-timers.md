---
title: "Spell Timers"
slug: spell-timers
section: "Live & combat"
order: 50
routes: []
keywords: []
---
# Spell Timers

**Prerequisite:** `/log on`.

Tick-accurate countdown bars for every buff, debuff, mez, stun, and DoT. Driven directly from the combat log and keyed per spell-and-target, so casting Visions of Grandeur on three different group members produces three separate timers. Durations are extended automatically by the focus items and AAs your character actually has equipped — pulled from your latest Zeal export.

Durations follow the server's formulas, including the roughly 165 spells that keep lengthening from level 60 to 65: your own casts use your level, and other players' casts use their level from your /who history when the caster can be identified. When a new buff overwrites a conflicting one (Tranquility over Koadic's Endless Intellect, say), the old timer is removed.

### Buffs vs detrimentals

Buffs and detrimentals get their own overlay windows so you can put the bar you actually need to react to in your line of sight. Each row shows the target name (when it isn't you) and has a small dismiss <kbd>×</kbd> to clear that single timer when you know it's gone; the header eraser still bulk-clears the lot. Timers auto-cancel on resist, fade, zone, or death.

The buff overlay header has a sort-mode toggle: **remaining** (default) puts the soonest-to-expire on top, while **recent** pins newly-cast buffs to the top and freezes their order so the list stops jumping around as time ticks down. The choice syncs between the dashboard panel and the standalone pop-out window.

### Tracking mode

**Auto** (default) creates a timer for every recognised spell landing — the full firehose. **Triggers only** shows just the spells you've curated through trigger packs or custom triggers, for a focused overlay instead of full auto-coverage.

### Tracking scope

- **Self only** — buffs landing on you, nothing else.
- **Cast by me** (default) — also includes anything you cast on others, without the noise of other players buffing each other.
- **Anyone** — every recognised land, useful for seeing another chanter's debuff on the raid mob.

The optional **Filter buffs to my class** toggle drops buffs your class can't cast (paladin Spiritual Purity, shaman Talisman, bard songs) so an enchanter's overlay isn't cluttered with raid buffs from other classes.

### Display thresholds

Hide overlay rows whose remaining time exceeds a configured threshold (per-category for buffs and detrimentals, with per-trigger overrides). Set to `0` to always show — useful for keeping long-duration raid buffs out of view until they're close to expiring.

### Audio alerts

Configurable audio and TTS alerts fire when a timer's remaining time crosses thresholds you set (e.g. 30 s left, 10 s left). Triggers can also carry their own per-trigger "fading soon" alerts — handy for one-off cooldowns or specific mez/charm timers.

### Mute or hide one detrimental

The default detrimental alert fires for every debuff, DoT, mez and stun the engine tracks, which is more than you want for a spell like Tashanian. Each row on the Detrimental overlay has a bell and an eye button: the bell mutes the fading-soon alert for that spell (the timer still shows), and the eye hides that spell's timer altogether. Both are remembered by spell name and listed under **Overlays → Overlay settings → Timers → Detrimental timer alerts → Per-spell exceptions**, where you can add a spell, switch it between mute and hide, or remove it. A trigger's own alerts are never muted by this — they're your explicit choice.

### Row controls

The mute, hide and remove buttons on buff, detrimental and custom timer rows appear when you hover a row, leaving more width for the spell name and countdown. "Always show row controls" under **Overlays → Overlay settings → Timers** restores the old layout.

### Keeping expired timers

With **Keep expired timers until refreshed** on, a timer that runs out stays on the overlay as an overdue reminder until you recast it. Set separate Buff and Detrimental linger times for how long an expired row stays, plus per-spell overrides (for example Quiescence 1 minute, raid buffs 30 minutes, or 0 to drop it on expiry), so tracking raid-wide buffs with Anyone scope doesn't flood the overlays.

### Filtered Timers overlay

A third timer window you filter by spell, player or mob — including **Me** and your current target — alongside the Buff and Detrimental overlays. The beneficial by-spell view can list group or raid members who are missing a buff (needs Zeal), and the window can optionally apply the display thresholds. It has a dashboard panel and a pop-out like the other overlays.

Its current-target **Target debuffs** view is spawn-aware: when two mobs share a name, debuffs are pinned to the one they landed on, and debuffs only known by name get a dimmed “?”. A Clear button handles leash and evade resets, which strip a mob's debuffs without a log line.

### Buff timers synced to Zeal

With [Zeal](https://github.com/CoastalRedwood/Zeal) connected, your own buff timers check themselves against the client's remaining tick count for each buff and move when they're off. That fixes buffs other players cast on you, whose duration depends on their AAs and focus items. Fades are also placed on your own server tick, so a buff drops off the overlay when it actually drops in game.

### Charm timers

Charms log no land line for their caster, so the engine starts a charm timer once the cast time passes without a resist, interrupt or “did not take hold” line. No trigger pack is needed. With Zeal it also waits for a pet to appear, so a silent failure doesn't start a phantom timer. Buffs on a charmed pet survive when a same-named mob dies elsewhere in the zone.

### Bard songs

Short-buff-box spells (mostly bard songs) fade one tick after their duration runs out, and their timers now include that tick. Pack song timers default to 24 seconds.
