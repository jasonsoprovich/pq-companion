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

### 2.4 CH-chain tracking relies on chat callouts, not actual casts

- **Limitation:** The CH Chain overlay and CH Metronome cannot see other
  clerics' Complete Heal casts or the heals landing on the tank. They are
  driven entirely by the chain *callout* lines clerics post to raid chat. If a
  guild doesn't call its chain in chat (e.g. uses a silent `/pause`-timed
  macro), there is nothing to track.
- **Root cause:** Other players' spell casts log only as the generic
  "Soandso begins to cast a spell." with no spell name, and heals on a third
  party (the tank) aren't in your log at all. Only the chat callout carries the
  caster + position + target.
- **Sources checked:** Log (own casts named; others' casts nameless; no
  third-party heal lines), Zeal (own client data only).
- **Could a future data source fix this?** **No.** Even ZealPipes exposes only
  the local client's state. Chat callouts are the only viable signal, so the
  feature is built on the user-configurable callout regex.

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

### 3.5 Duplicate-named bosses have no "active version" flag

- **Limitation:** Several raid bosses carry *multiple* `npc_types` rows of the
  same name that are all assigned to spawn in the same zone — a real raid
  version plus low-HP siblings. Concrete cases (Plane of Fear): **Cazic Thule**
  (450k-HP raid row + 32k rows) and **A Dracoliche** (175k-HP raid row + 32k
  rows), present in both `fearplane` and `fear_instanced`. The overlay can't
  *prove* which one the live spawn is.
- **Root cause:** Same as 3.1 (no live spawn/NPC ID), compounded by the fact
  that the data itself flags more than one same-name row as spawnable in the
  zone — `raid_target` is set on several of them, so there is no single
  authoritative "this is the live version" column.
- **Mitigation in app:** The overlay now headlines the strongest candidate
  (`raid_target` first, then highest `hp`, then lowest `id`), which reliably
  matches the raid boss being fought, and collapses the remaining same-name
  rows under a "N other DB version(s)" disclosure instead of stacking them.
  This is a heuristic, not a resolution — if a group fought a low-HP variant the
  headline would still show the raid row.
- **Sources checked:** DB (`npc_types` name/hp/raid_target, `spawn2`/`spawnentry`
  per zone), Log (name only), Zeal (`TargetName` + HP%, no ID).
- **Could a future data source fix this?** **No** without a target spawn/NPC ID
  from Zeal. Re-check on each Zeal release.

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

## 7. Character stat derivation

### 7.1 Derived stats assume skills are trained to the class/level cap

- **Limitation:** The Character Info → Stats tab derives the displayed AC and
  ATK rating assuming the character's Defense, Offense, and weapon skills are at
  the class/level cap, and that the equipped weapon matches the character's
  best-trained melee skill. A character below cap, or wielding an off-skill
  weapon, will read slightly high.
- **Root cause:** The Quarmy/Zeal export carries base attributes, level, class,
  race, inventory, and AAs — but **no live skill values** and no equipped-weapon
  skill type. The skill caps come from `quarm.db` (`skill_caps`).
- **Sources checked:** Quarmy export (no skills), `quarm.db` (caps only), Zeal
  (no skill snapshot today).
- **Could a future data source fix this?** **Partially.** A future Zeal field
  exposing live skill values and the equipped-weapon type would let us use the
  character's actual skills instead of the cap assumption.

### 7.2 Skill Tracker has no full skill snapshot — DISABLED behind a dev flag

- **Limitation:** The character Skills tab can only be built from `EventSkillUp`
  log lines ("You have become better at X! (N)"), which fire *only when a skill
  rises*. A character already at cap — or whose older logs were purged — emits
  no such lines, so the page can never populate and looks broken. Running a Log
  Backfill doesn't help: the historical lines simply aren't there. Because this
  makes the feature unusable for most players (it only works for skills raised
  while actively logging), the Skills tab and its backfill row are **hidden
  behind the `DEV_SKILLS` flag** (`VITE_DEV_SKILLS=true`), mirroring the
  disabled HPS meter. The backend tracking/parser/store still run, so data
  accumulates for if/when a snapshot source appears and the feature is
  re-enabled.
- **Root cause:** EQ logs a skill only when it *changes*, never a full list, and
  **no data source exposes a skill snapshot:** the Quarmy/Zeal export carries no
  skill values, and the ZealPipes `LabelType` enum (verified against the
  OkieDan/ZealPipes source on 2026-06-03 — 40 named entries) has **zero**
  skill-related labels (no Skill*, Defense, Offense, tradeskills, etc.).
  Secondary nuance: for the five caster Specialize <school> skills only one may
  exceed 50 on Quarm; the chosen school can only be inferred from observed
  values (whichever is already >50), and `skill_caps` doesn't encode the rule.
- **Sources checked:** Log (skill-up *deltas* only, no snapshot), `quarm.db`
  (`skill_caps` caps only), Quarmy export (no skills), ZealPipes (no skill
  `LabelType` — confirmed in the enum source).
- **Could a future data source fix this?** **Only with new data.** A Zeal /
  ZealPipes addition exposing the live skill list (ideally with the chosen
  specialization) would make the tab viable; re-enable by flipping `DEV_SKILLS`.
  Re-check this against each new Zeal release.

---

## 8. Tradeskills / recipes

### 8.1 Component cost coverage is partial

- **Limitation:** The recipe view shows a component's cost only when that
  component is sold by at least one merchant, and even then it's the item's
  flat base price — not what a vendor actually charges. Most tradeskill
  components (drops, sub-combines, quest pieces) are not vendor-sold, so they
  show no price, and there is no whole-recipe cost rollup.
- **Root cause:** `quarm.db` stores a static `items.price` and a
  `merchantlist` membership, but final merchant pricing is computed live from
  Charisma, faction, and the merchant's markup — none of which exist in the
  static DB. Items with no `merchantlist` row have no price reference at all.
- **Sources checked:** `quarm.db` (`items.price`, `merchantlist`), Log (no
  pricing), Zeal (no merchant snapshot today).
- **Could a future data source fix this?** **Partially.** A future Zeal field
  exposing the live merchant window (item → actual buy price) would give real,
  CHA/faction-adjusted costs, but only for merchants the player has open.

### 8.2 Recipes don't model success rate, skill-ups, or yield variance

- **Limitation:** The recipe view shows the trivial and the
  container/component/product list, but not the chance of success at a given
  skill, expected skill-ups, or any randomized/bonus yield.
- **Root cause:** `tradeskill_recipe` only stores `trivial`, `skillneeded`,
  and a `nofail` flag. The success curve and skill-up odds are server-side
  formulas in the EQMacEmu fork, not data in the dump.
- **Sources checked:** `quarm.db` (`tradeskill_recipe` / `_entries`), Log
  (combine results only, post-hoc), Zeal (no tradeskill state).
- **Could a future data source fix this?** **No** for the formula itself
  (it's server code); a documented constant set could be hardcoded if Quarm
  publishes it.

---

## 9. Spell acquisition

### 9.1 Quest-given spells can't be sourced

- **Limitation:** The spell "How to acquire" panel covers vendors, drops,
  tradeskill/research recipes, forage, and ground spawns, but NOT spells that
  are handed out by a quest. For those (and any genuinely unobtainable scroll)
  it shows "likely a quest reward or a starting spell."
- **Root cause:** Quest rewards live in the server's Perl quest scripts, not
  in `quarm.db`. There's no table linking a quest turn-in to the spell scroll
  it grants.
- **Sources checked:** `quarm.db` (`items.scrolleffect` → merchantlist /
  loot / tradeskill / forage / ground_spawns), Log (no quest reward data),
  Zeal (none).
- **Could a future data source fix this?** **Partially** — only if a
  quest→reward dataset were exported from the server scripts and added to the
  dump.

---

### 9.2 Shopping-route travel cost is a heuristic, not real EQ travel

- **Limitation:** The "plan shopping route" travel model measures distance as
  zone-line hops over the `zone_points` graph, plus a flat one-hop link from the
  **Nexus** to every Druid/Wizard teleport destination. It does NOT model: ports
  originating anywhere other than the Nexus, gating to bind, boats, the PoK book
  network (intentionally off — see the Plane of Knowledge toggle), Call of the
  Hero, run speed, or class. So "nearest source" is an ease-of-travel proxy
  anchored at the Nexus, not a true shortest path for an arbitrary character.
- **Root cause:** Real travel depends on the player's class, bind point, group,
  and live world state, none of which are in the DB. Teleport destinations are
  derived from `spells_new` (Druid/Wizard spells with effect SPA 83 Teleport or
  104 Translocate, joined to a real `zone`); the Nexus is used as the hub
  because most Quarm players bind there.
- **Sources checked:** `quarm.db` (`zone_points` for adjacency, `spells_new`
  teleport effects/`teleport_zone` for ports), Log (none), Zeal (none).
- **Could a future data source fix this?** **Partially** — modeling per-class
  ports from an arbitrary start, bind points, or boats would need that state as
  input; the current model is deliberately Nexus-centric and start-zone-driven.

---

## 10. NPC caster summary (overlay)

### 10.1 Caster highlights are heuristic, not the NPC's real-time cast list

- **Limitation:** The NPC overlay's "Spells & Abilities" section shows the
  NPC's *potential* caster AI (curated highlights like Complete Heal / Gate /
  AE, procs, signature spells, and a count of inherited class lists). It does
  NOT show which spell the NPC is casting right now, nor guarantee it will ever
  cast a given spell in a fight.
- **Root cause:** The data is the static `npc_spells` / `npc_spells_entries`
  list (with `parent_list` inheritance) plus `spells_new` effect/target columns
  — the server picks spells at runtime by level gate, mana, recast, and AI
  state, none of which are logged. Highlights are derived heuristically from
  SPA effect ids and targettype/aoerange, so categories needing a base-value
  comparison that we don't surface (e.g. snare vs SoW, slow vs self-haste) are
  intentionally omitted to avoid false positives.
- **Sources checked:** DB (`npc_spells*`, `spells_new`) — full static list;
  Log (NPC spell casts appear as generic "begins casting"/landed messages with
  no caster→list mapping); Zeal (none).
- **Could a future data source fix this?** **Partially** — real-time "what is
  this NPC casting" would need a Zeal/log feed of NPC cast events tied to the
  target; the static potential list is already as complete as the DB allows.

---

## 11. Buff duration modeling

### 11.1 Quarm's `spell_modifiers` table is not in the public dumps

- **Limitation:** Buff timer durations can be wrong for any spell Quarm has
  tuned via its server-side `spell_modifiers` table (per-spell or per-zone
  fixed tick counts, multipliers, and additive tick adjustments applied in
  `CalcBuffDuration_modification`, EQMacEmu `zone/spells.cpp`). The rule
  `Quarm:ClientBeneficialSpellDurationModifier` is **true** in the dumped
  `rule_values`, so the path is live for client-cast beneficial buffs.
- **Root cause:** The public Quarm MySQL dumps we convert into `quarm.db`
  do not include the `spell_modifiers` table, so we cannot see which spells
  (if any) carry overrides. All durations we have verified against in-game
  observations (KEI, Aegolism, Soul Energy, Forlorn Deeds, …) match the plain
  formula + AA + focus math, so the table appears to be sparse — but any
  mismatch reported by users that survives the AA/focus math should be
  checked against this first.
- **Sources checked:** DB (`rule_values` confirms the rule is enabled; table
  absent from `sql/` dumps); EQMacEmu source (`CalcBuffDuration_modification`
  reads `spell_modifiers` at zone boot); Log/Zeal (no duration feedback).
- **Could a future data source fix this?** **Yes** — the table being added to
  the public dump, or a one-off export from the Quarm team, would let the
  timer engine apply the same overrides.

### 11.2 Buffs received from other players use the *caster's* AAs/focuses

- **Limitation:** When another player buffs the active character (e.g. a
  cleric casting Aegolism on you), the real duration depends on the caster's
  Spell Casting Reinforcement ranks and worn duration focus — none of which
  are knowable from the recipient's side. The timer engine applies the active
  character's own modifiers, which is only correct for self-cast/self-clicked
  buffs.
- **Root cause:** EQMacEmu computes duration on the caster
  (`CalcBuffDuration` uses `caster->GetAA(...)`, `ApplyDurationFocus` walks
  the caster's worn slots); the log line on the recipient's side carries no
  caster identity or AA/gear state.
- **Sources checked:** Log (land messages don't name the caster); DB (no
  per-character data); Zeal (pipe reports buff slots but not durations).
- **Could a future data source fix this?** **Partially** — a future Zeal
  build exposing the client's real per-slot buff tick counts would make all
  received-buff timers exact regardless of caster.

---

## 12. Gear upgrade finder

### 12.1 No quest-reward sourcing

- **Limitation:** The finder can rank a quest-reward item if it exists in the
  catalog, but cannot tell you it comes from a quest, and the "where it drops"
  panel shows nothing for quest-only items.
- **Root cause:** `quarm.db` has no quest→item reward mapping. Item sourcing is
  derived from loot tables, merchants, forage, ground spawns, and tradeskill
  recipes only.
- **Sources checked:** DB (no quest reward tables); Log/Zeal (irrelevant).
- **Could a future data source fix this?** **Partially** — only a curated
  quest-reward dataset (hand-maintained or imported) would add this.

### 12.2 No "keyed / flagged / leveled-for" gating

- **Limitation:** Cannot filter to "only items I'm keyed or flagged for."
  Level usability *is* honored (an item's required level), but access gating
  (keys, flags, lockouts) is not.
- **Root cause:** No per-character key/flag/lockout state is stored, and the DB
  doesn't model item access requirements beyond class/race/level.
- **Sources checked:** DB (class/race/`reqlevel` only); user.db (no key/flag
  state).
- **Could a future data source fix this?** **Partially** — would need both a
  per-character flag/key record and item access metadata.

### 12.3 No clean raid-vs-group or zone/source filtering of results

- **Limitation:** Results can't be filtered to "group-obtainable only,"
  "raid only," or "drops in zone X." Only class/race/level usability,
  tradeable-vs-nodrop, and focus presence filter the list.
- **Root cause:** There is no raid/group flag on items or NPCs to gate on, and
  resolving the drop source for *every* candidate before ranking is too slow
  (`GetItemSources` is ~39 ms/item — multiple seconds for a full slot). Source
  is therefore loaded lazily per row, after ranking.
- **Sources checked:** DB (no raid/group marker; sourcing is an expensive
  multi-table join).
- **Could a future data source fix this?** **Partially** — a precomputed
  item→source/zone index (built offline during the data-release workflow)
  would make source/zone filtering affordable.

### 12.4 Scoring uses raw item stats, not all derived effects

- **Limitation:** The upgrade score weights flat item stats (HP, mana, AC, the
  seven attributes, resists). It does **not** score ATK, worn haste, weapon
  ratio/procs, or click/proc effects, and AC is scored on raw item AC without
  the class/level mitigation softcap.
- **Root cause:** ATK and worn haste are derived from worn-effect spells, not
  flat columns, and weapon value (ratio/proc) is a different model than armor
  stat-stacking. These were scoped out of phase 1.
- **Sources checked:** DB (items carry no flat ATK/haste column).
- **Could a future data source fix this?** **N/A (design choice)** — these can
  be added to the scoring model later; the data exists, it's just not wired in.

### 12.5 Focus effects are surfaced, not scored

- **Limitation:** A focus effect shows as a badge and can be filtered on, but
  does not contribute to the numeric upgrade score, so a focus item may rank
  below a higher-stat item.
- **Root cause:** Deliberate — focus effects don't stack (only the best of each
  type applies), so summing them into a per-item stat score is incorrect.
  Whether a focus is worth keeping is a loadout-level, playstyle judgment.
- **Sources checked:** DB (focus spell id/name available per item).
- **Could a future data source fix this?** **N/A (design choice)** — a future
  pass could add loadout-aware focus expected-value as a separate ranked axis.

---

## 13. UI rendering / performance

### 13.1 Chat History re-fetches the whole feed on each live update

- **Limitation:** When the log is live, every `chat:new` event triggers a full
  background re-fetch of the current channel's feed (up to 3000 rows) and
  swaps the whole array into React, which reconciles every row even though
  only one line is actually new. The feed list is not virtualized. On a very
  busy channel with a large backfilled history this could feel slightly less
  snappy on each reload.
- **Root cause:** Filtering/sorting is server-authoritative, so the client
  re-queries rather than appending the new line locally. A delta-append (push
  the new message straight off the WebSocket payload) would avoid the churn
  entirely, but it would require replicating the server's filter logic
  (channel/character/search/date/NPC-reply filtering) client-side — any drift
  there means missing or duplicated lines.
- **Sources checked:** N/A (this is an app-side rendering tradeoff, not a
  data-availability limit).
- **Could a future data source fix this?** **N/A (design choice)** — accepted
  for now because a single session rarely approaches the 3000-row cap and the
  reload is no longer jarring (no spinner flash or scroll reset). If lag is
  observed on a busy `ooc`/`auction` feed on real Windows hardware, the fix is
  a delta-append with a full-reload fallback on filter changes (verify with a
  Windows smoke test, since reconciliation perf is hard to judge in Mac dev).

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
