---
title: "Pet Overlay"
slug: pet-overlay
section: "Live & combat"
order: 70
routes: []
keywords: []
---
# Pet Overlay

One card for your charmed or summoned pet. It switches between three modes on its own:

- **Pet** — name, level and class, an HP bar (live from Zeal, or from `/pet health` with the reading's age), the charm countdown pinned on top, the timers on the pet, its DPS, max hit, delay and magic resist, and badges for summoning, gating and immunities.
- **Charm broke** — a red card with the mob's magic resist and your re-charm odds, counting down before it swaps to the finder.
- **Finder** — with no pet, the zone's charmable NPCs ranked by DPS with land chance, using your strongest scribed charm (or one you pick), like the [Charm Pet Finder](help:charm-pet-finder).

### Pet DPS

A live line shows the pet's damage in the current fight (or the last one, dimmed) and its share of yours, following the DPS meter's Personal / Raid / Encounter mode so the two agree. It works even when you haven't dealt any damage yourself.

### Summoned pets

A summoned pet is matched to its database row from the summoning cast, or from targeting it with Zeal. A magician's four elementals differ only by texture, so telling them apart needs the Zeal 1.5 pipe data described below; without it the pick is marked ambiguous.

### Alerts & settings

The **Pet** overlay's settings in the **Overlays** hub set the swap delay, the finder's row count, and three alerts, each with its own text, sound or TTS: charm expiring (default 30 seconds left), pet low HP (default 30%), and buff missing, checked against a watch list you keep per character for charmed and summoned pets.

### Zeal 1.5 features

A few features use pipe data added in the Zeal [v1.5.0-alpha0](https://github.com/CoastalRedwood/Zeal/releases/tag/v1.5.0-alpha0) test build: the [Item Info overlay](help:item-info), telling a magician's elementals apart, telling same-named NPCs apart by their look, and catching deaths your log doesn't show (for kill counts and clearing timers). They switch on by themselves when you run that alpha, or the official Zeal 1.5 release once it's out — there's nothing to change in PQ Companion.
