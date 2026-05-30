# PQ Companion — Known Limitations

This document tracks things PQ Companion **cannot do perfectly or correctly**
given the information currently available to it. The app draws on three data
sources, each with hard boundaries:

1. **Log files** (`eqlog_<Char>_pq.proj.txt`) — only contains what *your*
   client wrote to chat/log. No NPC IDs, no other players' private events, no
   instance metadata. Capture range is limited to what your client renders.
2. **`quarm.db`** — the static EverQuest game database (items, spells, NPCs,
   zones). Authoritative for *templates*, but cannot identify a specific live
   spawn or distinguish duplicate-named NPCs.
3. **ZealPipes** — a live, ~10 Hz mirror of **one** client's local game state
   (your player, your target, your group, your buffs). It is **not omniscient**
   — it only exposes what your own EQ client can see.

Each entry records the limitation, its root cause, which data sources were
checked, and whether a **future Zeal version** (or other future data source)
could plausibly resolve it. When a new Zeal release ships, re-check the "Could
a future data source fix this?" column against the new capabilities.

> **Maintenance rule:** Whenever we discover a *new* limitation during
> development, add it to this file. This is the canonical place to check
> against each new Zeal release to see what we can finally resolve.

---

## 1. Combat / DPS tracking

### 1.1 DPS is not 100% accurate due to log capture range

- **Limitation:** The DPS meter can undercount damage from combatants who are
  far enough away that your client never renders their combat messages.
- **Root cause:** The log only contains lines your client actually received.
  EverQuest does not send distant/out-of-range combat spam to your client, so
  those hits never reach the parser.
- **Sources checked:** Log (only source for damage lines).
- **Could a future data source fix this?** **No.** Zeal mirrors your client's
  state, which has the same range gate. Only server-side parse data could fix
  this, which neither logs nor Zeal expose.

### 1.2 Necromancer (and other) DoT DPS does not appear in other players' logs

- **Limitation:** A DoT cast by *another* player does not produce tick lines in
  *your* log, so their DoT damage is invisible to your meter. This most often
  hits necromancers, whose damage is heavily DoT-based, making their parsed DPS
  look far lower than reality.
- **Root cause:** DoT tick messages ("X has taken N damage from your spell.")
  are written only to the *caster's* log. Other clients never see them.
- **Sources checked:** Log (caster-only), Zeal (only your own casts/target).
- **Could a future data source fix this?** **Partially, self only.** Zeal's
  `CastingSpellName` + `TargetName` lets us bind *your own* DoTs to their target
  reliably. Other players' DoT damage remains unobservable.

### 1.3 Multi-target / AoE damage attribution is fuzzy

- **Limitation:** When fighting several identically named mobs (e.g. 3 gnolls),
  a line "You slash a gnoll for 150" does not say *which* gnoll. AoE damage
  across multiple targets cannot be split per-target with certainty.
- **Root cause:** Logs carry no target/spawn ID. The parser falls back to
  "current target" inference from the last `/con` or last-hit heuristic.
- **Sources checked:** Log (no ID), DB (templates only), Zeal (`TargetName` is
  also just a label).
- **Could a future data source fix this?** **Partially.** Zeal's live
  `TargetName` stream makes *single-target* trash attribution near-perfect
  (hits within ±200 ms of a target switch are attributable). True AoE splitting
  across same-named mobs remains impossible without spawn IDs.

### 1.4 Pet / charmed-pet damage attribution requires inference

- **Limitation:** Pet and charmed-pet damage shows as a separate combatant; a
  charmed "spider eye" is indistinguishable from a hostile mob in the log.
- **Root cause:** Logs don't link a pet's hits to its owner except at bind time
  ("My leader is X."). Charm breaks/re-charms need re-inference.
- **Sources checked:** Log (bind-time only), Zeal (`PlayerPetName`,
  `TargetPetOwner`).
- **Could a future data source fix this?** **Yes (your pet only).** Zeal's
  `PlayerPetName` is always the current pet, enabling automatic owner-merge for
  *your* pet/charm. Other players' pet ownership is still log-inference only.

---

## 2. Healing / HPS tracking

### 2.1 Cannot accurately compute HPS for other players' heals

- **Limitation:** A real, general HPS meter for a group's main healer is not
  possible. You only see heal lines for heals *you* cast.
- **Root cause:** Incoming heals from other players are never written to your
  log file.
- **Sources checked:** Log (own heals only), Zeal.
- **Could a future data source fix this?** **Partially.** Zeal's
  `GroupMemberXHPPerc` deltas can *infer* healing landing on group members (HP%
  rising between ticks), but it can't attribute *which* healer did it, can't
  separate overheal, and works for group members only — not raid.

### 2.2 Heal-over-time (HoT) ticks on others are invisible

- **Limitation:** HoT ticks only log when they fire on *you*; ticks on other
  players are not in your log.
- **Root cause:** Same as 2.1 — other players' heal/HoT events aren't logged
  client-side.
- **Sources checked:** Log, Zeal.
- **Could a future data source fix this?** **Partially.** Same HP-delta
  inference as 2.1 (group only), with the same attribution/overheal caveats.

### 2.3 Cross-raid cure prioritization is not feasible

- **Limitation:** We cannot tell which *other* raid/group member has a curse,
  poison, disease, or debuff that needs curing.
- **Root cause:** Only the afflicted player's own client knows their debuffs.
  Logs don't carry others' detrimentals; Zeal streams others' HP% but not their
  buff/debuff slots.
- **Sources checked:** Log, Zeal (`Buff0`–`Buff30` are *your* slots only).
- **Could a future data source fix this?** **No** for others. **Yes** for a
  self-only "a curse landed on you" alert.

---

## 3. NPC identification

### 3.1 No reliable NPC ID — duplicate-named NPCs cannot be told apart

- **Limitation:** When two distinct NPCs share a name, the app cannot tell which
  one you're fighting. Concrete cases:
  - The duplicate-named boss in **Vex Thal**.
  - The **Shissar Revenant** in **Ssraeshza Temple**.
- **Root cause:** Logs never emit a spawn ID or `npc_types.id`; they only carry
  the display name. `quarm.db` may hold *multiple* `npc_types` rows with the
  same `name`, and nothing in the log/Zeal feed disambiguates which row is the
  live spawn.
- **Sources checked:** Log (name only), DB (multiple rows, no live binding),
  Zeal (`TargetName` is the same display label).
- **Could a future data source fix this?** **No** with current Zeal. Would
  require Zeal to expose the target's actual spawn/NPC ID. Re-check on each Zeal
  release.

### 3.2 Cannot determine level / class of duplicate-named NPCs

- **Limitation:** For same-named NPCs that map to different `npc_types` rows
  (different level, class, resists, HP), the NPC overlay can't be sure which
  row to show.
- **Root cause:** Same as 3.1 — no way to bind the live target to a specific DB
  row. Level/class are only visible if the user `/con`s and the parser maps it,
  and even then ambiguity remains across same-named rows.
- **Sources checked:** Log, DB, Zeal.
- **Could a future data source fix this?** **No** without a target spawn/NPC ID
  from Zeal.

### 3.3 Loot tables for duplicate-named bosses are ambiguous

- **Limitation:** The duplicate-named Vex Thal boss can't have its loot table
  shown accurately, because we can't determine *which* `npc_types` entry the
  live boss corresponds to — and the candidates have different loot tables.
- **Root cause:** Loot tables are keyed off `npc_types.id` (via
  `loottable`/`loottable_entries`), but we can't resolve the live spawn to one
  ID. See 3.1.
- **Sources checked:** DB (loot tables exist per ID), Log/Zeal (no ID binding).
- **Could a future data source fix this?** **No** without a target NPC ID.

### 3.4 NPC database stats are templates, not live state

- **Limitation:** NPC level, HP, class, and resists in the overlay come from the
  static template, not the actual live spawn (which may be buffed, scaled, or a
  different variant).
- **Root cause:** Only `quarm.db` template data is available; Zeal exposes
  `TargetName` and `TargetHPPerc` (a percentage bar) but no absolute live stats.
- **Sources checked:** DB (template), Zeal (name + HP%).
- **Could a future data source fix this?** **Partially.** Zeal gives a live HP%
  bar, but absolute live stats remain template-derived.

---

## 4. Instances & respawn timers

### 4.1 Cannot distinguish regular vs. PvP vs. guild instances

- **Limitation:** When zoning into an instanced area, we can't tell whether it's
  a normal instance, a PvP instance, or a guild instance.
- **Root cause:** The log's "You have entered <Zone>." line carries only the
  zone name, not the instance type or instance ID. Zeal's `Player.zone` is a
  zone ID, not an instance descriptor.
- **Sources checked:** Log (zone name only), Zeal (`Player.zone` = zone ID).
- **Could a future data source fix this?** **No** unless Zeal exposes the
  instance ID/type. Re-check on each Zeal release.

### 4.2 Respawn timers are inaccurate inside instances

- **Limitation:** Spawn/respawn timers for instanced areas can be wrong, because
  instanced spawns and PvP/guild instance rules differ from the open-world
  timers we model.
- **Root cause:** A consequence of 4.1 — without knowing the instance type, we
  can't apply the right respawn rules (instance reset behavior, Quarm
  fast-respawn variations, etc.).
- **Sources checked:** Log, DB (open-world spawn data), Zeal.
- **Could a future data source fix this?** **No** without instance metadata from
  a future data source.

---

## 5. Raid-scope data

### 5.1 No information about raid members through Zeal

- **Limitation:** Zeal does not expose raid roster, raid members' HP, buffs,
  cooldowns, or positions — only the player's own **group** (up to 5 others).
- **Root cause:** ZealPipes mirrors one client's local state; the EQ client's
  raid window data is not exposed over the pipe.
- **Sources checked:** Log (only roster-join chat lines, if in-zone), Zeal
  (group fields only).
- **Could a future data source fix this?** **Maybe.** If a future Zeal version
  exposes raid-window data, raid-scope features become possible. Until then,
  raid-wide tracking is out of scope. Re-check on each Zeal release.

---

## 6. General log-format constraints

These are inherent to log-file parsing and affect multiple features:

### 6.1 Target is never logged directly

- **Limitation:** The log never states your current target; it must be inferred
  from combat/spell context.
- **Could a future data source fix this?** **Yes.** Zeal's `TargetName` provides
  authoritative live target detection (see 1.3).

### 6.2 No backfill for events before the app started

- **Limitation:** Live Zeal streams have no history — if the app starts mid-
  fight, early events from the pipe are missed.
- **Mitigation:** The log parser backfills from the on-disk log; Zeal does not.
- **Could a future data source fix this?** Partially mitigated today via log
  backfill; Zeal itself is live-only by design.

### 6.3 Server-wide events are log-only

- **Limitation:** Zone-wide GM messages, server restarts, and similar events are
  only available if they hit the chat log.
- **Could a future data source fix this?** No change expected from Zeal.

---

## Template for new entries

```
### X.Y <short title>

- **Limitation:** What can't be done / what's wrong.
- **Root cause:** Why — which data is missing or ambiguous.
- **Sources checked:** Log / DB / Zeal — what each does or doesn't provide.
- **Could a future data source fix this?** Yes / No / Partially — and what
  specific capability (e.g. a Zeal field) would be required.
```
