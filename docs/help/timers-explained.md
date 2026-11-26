---
title: "How timers work"
slug: timers-explained
section: "Live & combat"
order: 85
routes: []
keywords: ["timer", "buff", "detrimental", "debuff", "custom timer", "filtered", "scope", "tracking", "duration", "countdown", "why no timer"]
---
# How timers work

Every timer in PQ Companion starts from a line in your log. Knowing which
line starts which kind of timer, and which window shows it, answers most
"why don't I see a timer?" questions.

```flow
Log line | Spell engine or trigger | Timer | Overlay window
```

### Where timers come from

- **Spell landings.** When the log says a spell landed ("A gnoll has been
  mesmerized."), the spell engine looks up the spell, works out its duration
  for the caster's level and your focus items, and starts a timer keyed to
  that spell and target. No setup needed.
- **Triggers.** A trigger can start a timer when its pattern matches: a
  buff or detrimental timer for a spell, or a custom countdown with any
  name and duration (a boss ability, a camp check, an incoming tell).
- **Respawns.** Killing an NPC starts its respawn timer from the zone's
  spawn data.

### Which window shows it

| Timer | Shown in |
| --- | --- |
| Beneficial spells | **Buff Timers** |
| Debuffs, DoTs, mez, stuns, charm | **Detrimental Timers** |
| Trigger countdowns | **Custom Timers**, or a named timer window you choose on the trigger |
| A filtered subset (by spell, player or mob) | **Filtered Timers** |
| NPC respawns | **Respawn Timers** |

All of them are cards in the [Overlays hub](help:overlay-dashboard).

### What decides whether a spell timer appears

These live under **Overlays → Overlay settings → Timers**:

- **Tracking mode.** *Auto* times every recognised spell; *Triggers only*
  times just the spells covered by your trigger packs and triggers.
- **Tracking scope.** *Self only*, *Cast by me* (default) or *Anyone*. With
  *Cast by me*, another player's buff on you shows, but their buffs on
  each other don't.
- **Display thresholds.** Rows with more time left than the threshold stay
  hidden until they get close to fading. Set `0` to always show.
- **Filter buffs to my class** hides buffs your class can't cast.

### Common surprises

- **No timers at all:** the log isn't being written. Type `/log on` and
  check **Settings → General → Feature health**.
- **A long buff is missing:** it's probably above the display threshold and
  will appear later.
- **Someone else's debuff isn't shown:** switch the scope to *Anyone*.
- **A timer vanished early:** timers end on resist, fade, zone, death, or
  when a conflicting buff overwrites them.

### Fading-soon alerts

Each timer type can warn you before it ends with a sound, spoken text or
on-screen text. Set defaults in each timer overlay's settings drawer; a
trigger can add its own fading-soon alerts at any thresholds. See
[Spell Timers](help:spell-timers) and [Custom Timers](help:custom-timers)
for the details.
