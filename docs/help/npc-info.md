---
title: "NPC Info Overlay"
slug: npc-info
section: "Live & combat"
order: 60
routes: []
keywords: []
---
# NPC Info Overlay

**Prerequisite:** `/log on`.

The moment you consider a mob, the overlay shows its level, class, HP (and max mana when it has any), resists, body type, faction, base swing timer, aggro and assist radius, and special abilities — Summon, Unmezzable, Uncharmable, See Invis, Enrage, Flurry, and so on. Pulled straight from the database via your combat log, so it works for any mob the database knows about. The aggro radius, assist radius, and swing timer sit in their own Behavior section with a Settings toggle to hide it if you don't want it.

The overlay also auto-detects your target from melee and spell events, so it stays useful even if you don't explicitly `/con`. If you target a corpse, the overlay resolves back to the underlying NPC so the panel stays useful for loot calls after the kill.

### Resists and special abilities

Resists are shown in EQ convention order — MR / CR / FR / DR / PR — and colour-coded so high resists stand out at a glance. Special abilities render as labelled badges (Summon, Mez Immune, Enrage with HP threshold, Flurry, etc.) rather than raw numeric codes. Multi-field entries like Rampage range arguments parse correctly.

### Loot table

The overlay's on-demand loot table shows every item the NPC can drop with the database drop rate. Zone-wide drops — the shared Vex Thal pool, for example — surface on every NPC in the zone, labelled with the pool name so you know it isn't mob-specific.

### Caster summary

For casting NPCs, the caster signature includes recast time, AE flag, and resist type for each known spell, alongside the spell itself — a fuller at-a-glance read of what to expect from an NPC caster than the name alone gives you.

### Copy target stats

A copy button on the overlay puts the current target's stats on your clipboard, formatted to paste straight into EQ chat and capped so it fits the in-game input limit — handy for posting a mob's resists or HP to your group without typing it out by hand. The damage line carries the swing timer too (`DMG: 294-1054 @1.8s`).

### With Zeal

Zeal adds a live HP bar driven by pipe events (so you see the bar move as the mob takes damage), and a pet-owner badge that names the player a charmed or summoned pet belongs to.

If Zeal stops confirming the target, the card and pop-out dim with a “Zeal stale · 12s” badge so an old reading never looks live. The target stays until Zeal or the log changes it, and the distance chip dims and then hides after 20 seconds. Targets picked from the log alone never dim.

### Live /con border

When you `/con` a mob, the overlay frames itself by how it regards you: red for scowls, amber for threatening, faint for dubious and apprehensive, and a standing label otherwise. The reading clears when you retarget or zone, or when your invisibility, hide, illusion, feign death or faction buffs change, since any of those can change the answer. Turn it off under NPC Overlay → Display.

### Target debuffs & timers

A strip of the target's debuff icons with time left sits under the header on the Stats and Loot tabs; click it to open Timers. The Timers tab only shows timers for the exact spawn you're fighting, splits debuffs from buffs and cooldowns, and has a Clear button for leash and evade resets.

### Upgrade scores on loot

Each item in the loot list shows its gear-upgrade score for your active character, the same score the [Gear Upgrade Finder](help:gear-upgrade-finder) uses.

### Same-name disambiguation

When several mobs in a zone share a name, the overlay uses the zone and your in-game position (via Zeal) to work out which one you're actually looking at. If the target is still ambiguous, the panel surfaces the full variant set so you can see every same-name NPC and its stats instead of guessing. The popped-out window handles the `#`\-prefixed same-name rows and their special abilities too.

With a current Zeal build the overlay also pins its pick to the specific creature you're fighting, so the panel stops flipping between candidates each time you re-target the same mob (or it gets dragged). With Zeal 1.4.8 or later, the overlay also reads the spawn's real level, class and race, so it can tell apart duplicate-named NPCs such as the Vex Thal bosses, Shissar Revenant and the Plane of Fear named, and show only the right loot table. On older Zeal the full set is still listed.

With the Zeal 1.5 pipe data (see [Zeal 1.5 features](help:pet-overlay)), the overlay also narrows same-named versions by the target's texture, gender and size. Script-spawned raid bosses such as the upstairs Rallos Zek, Jeplak, Foreman Gworknop and Emperor Ssraeshza show their real stats instead of the untargetable placeholder the zone places.

### Version labels

When a target matches several database rows, each version shows its spawn chance and what sets it apart: texture, gender, size, or an ability the others lack (summons, enrages, rampages, flurries). In Halls of Honor A, for example, the 90% Valiant Spirit doesn't summon and the 10% one does.

### Customizing the overlay

All NPC overlay settings live in the **NPC Info** overlay's panel in the **Overlays** hub, next to a live preview of the real overlay for any NPC you search for (Lord Nagafen by default). Changes show in the preview before you save.

- **Tag colors** — recolor special-ability tags by category or one at a time, and make key ones like Immune to Charm/Mez stand out with a glow.
- **Layout** — drag to reorder the Stats sections, and to put the tags you care about first.
- **Display card** — choose what shows on the dashboard panel and on the pop-out, separately, for every section and item, and per character if you want.
- **Outline rules** — outline the overlay when the target has (or lacks) chosen abilities or is one of chosen classes, for example can't be mezzed, rampages, summons, or is a caster. Each rule has its own color and optional flash, labels show which rules matched, and characters can have their own rule list.

The dashboard panel and the pop-out each reopen on Stats, Loot or Timers, whichever you left them on.

### Distance & range

With Zeal 1.4.8 or later, a distance chip shows how far away the target is, colored against your cast range and ranged range, and enemy spells that can reach you are marked. Both ranges are worked out from the active character's latest Quarmy export — cast range includes an Extended Range focus, ranged range is your bow plus arrow. Choose "Set manually" to type your own values instead.
