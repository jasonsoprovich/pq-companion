# Zeal v1.4.6 adoption + unspent-AA + /mystats — implementation plan

**Created 2026-09-08.** Supersedes the "waiting on a release" status of
`docs/zeal-spawn-id-plan.md` — the release is now live.

Covers three requests from the 2026-08-31 Discord thread + follow-up:

1. **Zeal v1.4.6 spawn ids** — the big one. Adopt the new pipe fields, work
   through `LIMITATIONS.md` limitation-by-limitation, ship what they unlock.
2. **Unspent AA count** on Character Info → AA (snapshot-when-live approach).
3. **`/mystats` history** — issue #154 filed, deferred pending sample logs.

---

## Part 0 — What actually shipped in v1.4.6

`gh release view v1.4.6 --repo CoastalRedwood/Zeal`:

> v1.4.6 is the same as v1.4.5 except for the addition of spawn ids to the
> named pipe output.

- **PR #229** (`davehess`, merged 2026-09-01) — the only functional change.
- **PR #230** — one line, `Bump to v1.4.6` in `zeal.h`. Not a feature.
- Release commit `5ae20e3`, tag `v1.4.6`, published 2026-09-07.

### Exact merged diff (`Zeal/named_pipe.cpp`)

Verified against the real merged code, not the PR description.

```cpp
// MsgRaid (type 5), per member, only when the entity is in your zone:
raid_data["spawn_id"] = entity->SpawnId;          // NEW in 1.4.6

// MsgGroup (type 6), per member:
group_data["spawn_id"] = member->SpawnId;         // NEW in 1.4.6

// MsgPlayer (type 3):
player_data["spawn_id"] = get_self()->SpawnId;                       // NEW — always
if (target)                 player_data["target_id"] = target->SpawnId;   // NEW — omitted if no target
if (actor_info->PetID > 0)  player_data["pet_id"]    = actor_info->PetID; // NEW — omitted if no pet
```

`SpawnId` = the zone server's live entity id. Unique per spawn for the life of
that spawn, **reused after a zone reset**. Not `npc_types.id`; does not name a
DB row, loot table, or level.

### DISCOVERY: MsgRaid / MsgGroup already carry a full roster (pre-#229)

Reading the current `named_pipe.cpp` `main_loop()` in full — not just the
#229 hunks — the type-5 and type-6 messages have been emitting far more than
we assumed. **We drop both entirely today** (`main.go` pipe switch hits
`default: return`).

**MsgRaid (type 5)** — array, one object per raid member, up to
`kRaidMaxMembers` (72). Emitted whenever `raid_info->is_in_raid()`:

| Key | Source | Always present? |
|---|---|---|
| `name` | `member.Name` | yes |
| `level` | `member.PlayerLevel` | yes |
| `class` | `member.Class` | yes |
| `group` | `member.GroupNumber` (`"1"`–`"12"`, `"0"` = ungrouped) | yes |
| `rank` | `"Raid Leader"` / `"Group Leader"` / `""` | yes |
| `spawn_id` | `entity->SpawnId` | only if member is in your zone |
| `loc` | `entity->Position` | only if in your zone |
| `heading` | `entity->Heading` | only if in your zone |
| `hp_current`, `hp_max`, `zone_id` | entity | only if in your zone **and** `PipeVerbose` on |

**MsgGroup (type 6)** — array, one object per group member:

| Key | Source | Always present? |
|---|---|---|
| `name` | `group_info->Names[i]` | yes |
| `spawn_id` | `member->SpawnId` | yes (member must be non-null) |
| `loc`, `heading` | `member->Position` / `->Heading` | yes |
| `hp_current`, `hp_max`, `class`, `level`, `zone_id` | member | only if `PipeVerbose` on |

`PipeVerbose` = `ZealSetting<bool>{false, "Zeal", "PipeVerbose"}` → **off by
default**; user enables with `/pipe verbose on`. So HP for raid/group members
is opt-in; **names + level + class + group assignment + leader flags +
positions are unconditional.**

**Consequence:** `LIMITATIONS.md` §5.1 ("No information about raid members
through Zeal") is already substantially wrong, independent of spawn ids. See
Part 1 §E.

---

## Part 1 — Zeal v1.4.6 spawn-id integration

### A. Decode layer (`backend/internal/zealpipe/`) — foundation, no behaviour change

All additive. `encoding/json` already ignores unknown keys, so nothing breaks
before this; we opt in field by field.

1. **`events.go` — extend `Player`:**
   ```go
   type Player struct {
       Zone       int      `json:"zone"`
       Location   Location `json:"location"`
       Heading    float64  `json:"heading"`
       AutoAttack bool     `json:"autoattack"`
       SpawnID    *int     `json:"spawn_id,omitempty"`   // NEW
       TargetID   *int     `json:"target_id,omitempty"`  // NEW — nil when no target
       PetID      *int     `json:"pet_id,omitempty"`     // NEW — nil when no pet
   }
   ```
   Pointers: Zeal omits `target_id`/`pet_id` when absent, and we must
   distinguish "no target" from "target id 0". Absent on any pre-1.4.6 Zeal.

2. **`events.go` — new `RaidMember` / `GroupMember` types + decoders:**
   ```go
   type RaidMember struct {
       Name    string     `json:"name"`
       Level   int        `json:"level"`
       Class   int        `json:"class"`
       Group   string     `json:"group"`  // "0"=ungrouped, "1".."12"
       Rank    string     `json:"rank"`   // "Raid Leader" | "Group Leader" | ""
       SpawnID *int       `json:"spawn_id,omitempty"`
       Loc     *Location  `json:"loc,omitempty"`
       Heading *float64   `json:"heading,omitempty"`
       HPCur   *int       `json:"hp_current,omitempty"` // PipeVerbose only
       HPMax   *int       `json:"hp_max,omitempty"`
       ZoneID  *int       `json:"zone_id,omitempty"`
   }
   type GroupMember struct { /* name, spawn_id, loc, heading, + verbose hp/class/level/zone */ }

   func DecodeRaid(payload string) ([]RaidMember, error)
   func DecodeGroup(payload string) ([]GroupMember, error)
   ```
   Note Zeal's `loc` here is `toJson(Position)` → same `{x,y,z}` transposed
   convention as `MsgPlayer.location`; reuse `Location.GameX()/GameY()`.

3. **`schema.go`** — document that `MsgRaid`/`MsgGroup` now have real decoders;
   drop the "we drop those envelopes" comment once §D/§E land.

4. **Tests** — `events_test.go` fixtures: a real captured type-3 with all three
   ids, type-3 with `target_id`/`pet_id` omitted, type-5 with and without
   `PipeVerbose`, type-6 likewise, and a pre-1.4.6 type-3 (no id keys) to pin
   graceful degradation. **Capture these from a live 1.4.6 client first** —
   don't hand-write fixtures for a wire format we haven't seen.

### B. P1 — `pet_id` → charm / pet binding — ✅ DONE (commit `04907900`)

Supersedes the interim `IsCharm` name-exemption tradeoff (commit `a92c3996`) —
the exemption in `removeOnKill` stays as the no-pipe/older-Zeal fallback, and
the new `pet_id` signal is the positive clear it couldn't safely do.

- `spelltimer.Engine.SetPipePetID(*int)` — pet_id absent for 3 frames (~300 ms
  debounce) OR changed to a new non-nil id → `removeCharmTimers()`. nil-when-nil
  is a no-op. `ResetPipePetID()` on disconnect drops the id without clearing.
- `combat.Tracker.SetPipePetID(*int)` — a changed id revokes the stale
  pipe pet-name owner binding (name is identical across a same-name re-charm);
  `SetPipePetName` re-binds the current pet.
- Wired into `main.go` MsgPlayer; state cleared on pipe disconnect.
- Tests: miss-streak debounce, single-frame tolerance, id-change immediate
  clear, older-Zeal no-op, disconnect-doesn't-clear (spelltimer); recharm
  binding revoke + older-Zeal no-op (combat).

Original plan notes follow.

Retires the interim `IsCharm` name-exemption hack (commit `a92c3996`,
`spelltimer/engine.go` `removeOnKill`).

**Plumbing:**
- `main.go` `MsgPlayer` case: pull `p.PetID`, call new
  `combatTracker.SetPipePetID(*int)` and `timerEngine.SetPipePetID(*int)`.
- `combat/tracker.go`: key charm-pet damage rows on `pet_id` when present;
  fall back to `PlayerPetName` (existing `SetPipePetName`) when nil (older
  Zeal / pipe down). A re-charm = a new `pet_id` = a clean new combatant
  binding instead of the current name-collision folding.
- `spelltimer/engine.go`: bind charm timers (`IsCharm`) to the live `pet_id`.

**The "pet died / charm broke" signal we currently lack:**
EQ writes **no** "Your charm spell has worn off." line when a charmed pet is
killed under your control (confirmed with enchanters — see
`LIMITATIONS.md` 1.4 + `spelltimer/models.go:144`). With `pet_id`:
- `pet_id` **present → absent** across consecutive `MsgPlayer` snapshots = the
  pet is gone. Clear the charm timer then (debounce one or two ticks against a
  transient omission).
- Keep the existing Zeal corpse-target path as a second positive clear.
- Remove the `!t.IsCharm` exemption in `removeOnKill` once the id path is in —
  behind the version gate, with the name-exemption kept as the pre-1.4.6
  fallback.

**LIMITATIONS.md 1.4** → downgrade from "Yes (your pet only), by inference" to
**"Resolved for your pet (stable id + authoritative loss signal)."**

### C. P2 — `target_id` → sticky same-name resolution — ✅ DONE (commit `62c15ff4`)

- `overlay/npc.go`: `variantCache` (spawn id → memoised NPC + variant set +
  abilities + summary), zone-scoped. `SetPipeTargetID` (from MsgPlayer, a beat
  after the MsgLabel name) is the authority — hit applies the memoised
  resolution with no DB, miss resolves once and caches. `setTarget` also reads
  it (read-only) so the common re-pull doesn't flicker. Entry trusted only
  while its stored name matches the live target. Flushed on every zone change
  (log + pipe) and on disconnect; 512-entry soft cap. nil on older Zeal =
  no-op, resolves exactly as before.
- Combat/threat per-instance rows keyed by `target_id` = deeper, stays a
  follow-up.

Original plan notes follow.



**Plumbing:** add `TargetID *int` alongside every existing `SetPipeTarget(name)`
call site — `npcTracker`, `combatTracker`, `threatTracker`,
`raidThreatAssembler`, `timerEngine` corpse path. New paired setters
`SetPipeTargetID(*int)` or fold into the existing signature.

**NPC overlay (`overlay/npc.go`):** the win is **sticky** variant resolution.
Today `lookupNPCVariants` re-runs `filterVariantsByPlayerPosition` /
`sortVariantsByStrength` on every re-target — a coin-flip each time for the
duplicate-named Vex Thal / PoF bosses. With `target_id`:
- Resolve the `npc_types` variant once per `target_id`, cache it, reuse for the
  life of that id. Re-targeting the same live mob no longer re-rolls.
- Flush the whole `target_id`-keyed cache on any zone event (`SpawnId` is
  reused after a zone reset — never let it survive a zone line).
- The **initial** pick still needs the position/strength heuristic —
  `target_id` disambiguates *live instances*, not DB rows. No change there.

**Combat / threat:** allow two same-named live mobs to hold distinct rows keyed
by `target_id` when both are the current target at different moments.

**LIMITATIONS.md 3.1 / 3.2 / 3.3 / 3.5** → "No" becomes
**"Partial — sticky per live target within a session."** Loot/level for a
duplicate-named boss still depends on the heuristic pick; the id only makes the
pick stop flip-flopping.
**LIMITATIONS.md 1.3 / 6.1** → single-target trash attribution becomes exact
(id, not an article-cased name label). True AoE splitting across same-named
mobs still impossible (we only get *our* target's id).

### D. P3 — MsgGroup decode → groupmates on the live map — ✅ DONE (commits `cbba2641`, `20343313`)

- `playerpos.Tracker.UpdateGroup` — in-zone groupmates (loc present) negated to
  map space, broadcast as `player:group_positions`, rate-limited like the self
  arrow (floor + heartbeat + per-member change test). `ResetGroup` emits one
  empty frame on pipe disconnect.
- `main.go` MsgGroup case; `useGroupPositions` hook; `ZoneMap` draws each as a
  faint sky-blue arrow + cased name label, below the player arrow, only when
  the player's own position already puts this map on screen. Threaded through
  ZoneMapPanel / LiveMapPanel / LiveMap overlay window.
- Verbose-only group HP (better than labels 30–39) deferred — the labels
  already work and PipeVerbose is opt-in.
- Settings toggle deferred — default-on, faint, only shows when grouped on
  1.4.6.

Original plan notes follow.



- `main.go`: add a `case zealpipe.MsgGroup:` arm, `DecodeGroup`, feed:
  - `playerpos` / maps: draw groupmates' arrows on the live map from
    `member.loc` + `heading` (same transform as the player arrow). New
    `player:group_positions` WS event; frontend renders faint secondary arrows.
  - group HP: when `PipeVerbose` is on, `hp_current`/`hp_max` are absolute —
    better than labels 30–39 (`GroupMemberXHPPerc`, a percentage string). Keep
    the labels as the non-verbose fallback.
- Low risk; purely additive consumer. Gate the map arrows behind a Settings
  toggle (default on when pipe + 1.4.6).

### E. P4 — MsgRaid decode → raid roster → players store — ✅ DONE (commit `cbba2641`)

- `main.go` MsgRaid case → `playerStore.Upsert` per member (name / level /
  class / group), deduped to "on change" so a static roster doesn't churn
  `sightings_count`. Self + local character skipped. The Players tab now
  populates from a raid with no `/who`, and combat's class resolver (which
  reads the sightings store) can attribute raid-threat hate by class.
- `LIMITATIONS.md` §5.1 rewritten: roster (name/level/class/group/rank) = yes
  always; in-zone positions + spawn id = yes; member HP = yes with
  `/pipe verbose on`; buffs / cooldowns / mana = still no.
- A dedicated raid-window overlay (roster grid, group layout, HP bars) remains
  a separate follow-up — this pass just lands the decode + the store feed.

Original plan notes follow.



The roster is already on the wire (Part 0 discovery). Decoding it gives:

- **`players` store feed:** `players.Store.Upsert` already ingests
  name/class/level/guild/zone sightings with backfill. Each `MsgRaid` member
  is a live sighting — populate the Players tab from your raid without anyone
  having to `/who` or `/con`.
- **Raid threat (`raidthreat/assembler.go`):** it already aspires to raid
  scope but is fed only by log damage + the pipe target. A real roster
  (names + group split + class) lets it show every raider grouped correctly,
  seed class hate-mods per member automatically, and stop inventing player
  rows from log lines alone.
- **Foundation for a raid-window overlay** (roster, group layout, per-member
  level/class, leader flags; HP when `PipeVerbose`). Scope its own follow-up —
  don't build the overlay in this pass, just land the decode + players/threat
  feeds and confirm against a real capture.
- **Cross-zone caveat:** `spawn_id`/`loc`/`hp` are omitted for raiders not in
  your zone; `name`/`level`/`class`/`group`/`rank` are always there. Handle
  members as "known but not in zone."

**LIMITATIONS.md 5.1** → rewrite. New reality:
- Raid roster (name, level, class, group number, raid/group-leader flag): **yes,
  always, while in a raid.**
- Raid member positions + spawn id: **yes, for members in your zone.**
- Raid member HP: **yes, for members in your zone, when `/pipe verbose on`.**
- Raid member buffs / debuffs / cooldowns / mana: **still no** (not on the
  wire — `Buff*` labels are the local client's slots only).

**LIMITATIONS.md 2.1 / 2.2** (other players' heals/HoTs) → the `hp_current`
deltas per raid member (verbose) widen the HP-inference net from group-only to
whole-raid-in-zone, with the same "can't attribute which healer / can't see
overheal" caveats. Note it; don't over-promise.

### F. Version gating

`zeal.MinSupportedVersion` stays `1.4.0` (log features still work on old Zeal).
Per-feature gate on `1.4.6` using the existing `versionAtLeast` pattern (the
1.4.3 skills feature already does this). Every id-dependent path degrades to
today's name-based behaviour when the pointer is nil — the gate only hard-
blocks a feature that is *impossible* without the id (e.g. the pet-loss
signal). Soft-fail = the existing "update Zeal" banner.

### G. Risks / invariants (carry into every sub-task)

- **`SpawnId` ≠ `npc_types.id`.** Disambiguates live instances, not DB rows.
- **`SpawnId` is ephemeral** — reused after a zone reset. Treat every zone
  event as a full flush of any id-keyed cache. Never persist to `user.db`.
- **Log kill lines are still id-less.** Acting on a spawn's death by id needs a
  pipe signal (`target_id` → corpse/cleared, `pet_id` → absent).
- **`PipeVerbose` is off by default.** Any feature that needs member HP must
  detect its absence and either degrade or prompt the user to enable it.
- **Additive decode only.** The three `Player` pointers are safe now; the
  type-5/type-6 decoders are new surface — own tests + real-capture fixtures.
- **Capture before coding.** Get raw 1.4.6 pipe dumps (type 3/5/6, verbose on
  and off) and commit them as fixtures before writing consumers.

### H. Deliverables checklist

- [ ] `zealpipe` decode: `Player` pointers, `RaidMember`/`GroupMember` +
      `DecodeRaid`/`DecodeGroup`, tests w/ real fixtures.
- [ ] P1 pet_id: combat + spelltimer plumbing, pet-loss clear, remove interim
      `IsCharm` exemption behind the gate.
- [ ] P2 target_id: plumb to all `SetPipeTarget` sites, sticky variant cache in
      `overlay/npc.go` w/ zone-flush.
- [ ] P3 MsgGroup: `main.go` arm, group map arrows (Settings toggle), verbose
      group HP w/ label fallback.
- [ ] P4 MsgRaid: `main.go` arm, `players` store feed, `raidthreat` roster
      feed. (Raid overlay = separate follow-up issue.)
- [ ] `zeal.versionAtLeast("1.4.6")` gates where required.
- [ ] **`LIMITATIONS.md` rewrites:** 1.3, 1.4, 2.1/2.2 (note), 3.1, 3.2, 3.3,
      3.5, 5.1, 6.1. Add a "Resolved by Zeal 1.4.6" note to each with the date.
- [ ] `docs/zeal-spawn-id-plan.md` → mark superseded by this doc.
- [ ] Changelog + `FEATURES.md` notes (at release time per project rules).

---

## Part 2 — Unspent AA count on Character Info → AA

**User's proposal (confirmed feasible, this is the right design):** read the
value from Zeal while the game is live, persist the last-known number in
`user.db`, show it offline, refresh on next login.

### Source: pipe Label type 71 `CurrentAAPoints`

`Zeal/named_pipe.cpp:94` `LabelNames = { ... {71, "CurrentAAPoints"}, {72,
"CurrentAAPerc"}, ... }`. Label 71 is the native EQ UI "AA points available to
spend" number (`char_info->AlternateAdvancementUnspent`, see
`Zeal/experience.cpp:160`) — exactly the unspent pool, live, ~10 Hz. Label 72
is the % progress to the next point.

- **Not** in any file export. `-Quarmy.txt` (`zeal/reader.go:547`) stops at
  `BaseWIS` in the header and only lists purchased `AAIndex\tRank` rows — this
  is why we only show "spent" today.

### Design

1. **Decode:** `zealpipe` already decodes `MsgLabel`. In `main.go`'s label
   loop add `case 71:` (define `LabelCurrentAAPoints LabelType = 71`, and
   `72` for the perc if we want the progress bar too). Parse int, hand to a new
   `charStore.SetUnspentAA(character, n, at)`.
2. **Persist:** new columns on `characters` (additive migration, matches the
   existing `last_zone` / `last_zone_at` pattern in `character/store.go`):
   ```sql
   ALTER TABLE characters ADD COLUMN unspent_aa        INTEGER NOT NULL DEFAULT -1; -- -1 = never seen
   ALTER TABLE characters ADD COLUMN unspent_aa_at     INTEGER NOT NULL DEFAULT 0;  -- unix seconds
   ```
   (Keep it on `characters`, not a history table — we only need the latest.
   `/mystats` in Part 3 is where a history table belongs.)
3. **Seed from the log (secondary):** we already parse
   `You have gained an ability point!  You now have N ability points.` as
   `EventAAGain` (`logparser/parser.go:323`), and per `models.go:202` that `N`
   *is* the unspent pool at that instant. `progress/consumer.go` stores it as
   `KindAA` `Value`. Use the most-recent `KindAA` event as a fallback seed when
   the pipe has never populated `unspent_aa` — flagged clearly as
   "as of \<date>, may be stale."
4. **Display:** Character Info → AA tab, `AAPanel` header
   (`CharacterProgressPage.tsx:1493`). Next to `AA Points Spent: N` add
   `Unspent: M`. Include the freshness: `Unspent: 12 (as of 2h ago)` or
   `(as of Sep 6)`. If `unspent_aa == -1`, show `Unspent: —` with a tooltip
   ("Seen automatically while EverQuest is running with Zeal").
5. **API:** add `unspent_aa` + `unspent_aa_at` to the character payload the AA
   tab already fetches (`GET /api/characters/:id/aas` or the character detail —
   whichever the panel reads).

### Caveats to surface in the UI

- The number is a **snapshot** — it goes stale the moment the player spends
  points offline or on another client. Always show the "as of" time; never
  present it as authoritative-now.
- Only updates while EQ is running with the Zeal pipe connected (Windows).
  Same constraint as the live map arrow / target overlay.
- `CurrentAAPerc` (label 72) is optional polish — a thin "progress to next
  point" bar under the count. Nice-to-have, not required for v1.

### Deliverables

- [ ] `LabelCurrentAAPoints` (+ optional `LabelCurrentAAPerc`) in `schema.go`.
- [ ] `main.go` label arm → `charStore.SetUnspentAA`.
- [ ] `characters` migration: `unspent_aa`, `unspent_aa_at`.
- [ ] Store method + API field + type mirror.
- [ ] AA tab header: `Unspent: N (as of …)`, `—` when never seen.
- [ ] Fallback seed from latest `progress` `KindAA` event.
- [ ] `LIMITATIONS.md`: no new entry needed — this closes a gap rather than
      recording one. (Optionally note under 17.3 that live unspent AA is now
      shown even though net-spent history still isn't.)

---

## Part 3 — `/mystats` history  *(no longer deferred — real sample in hand)*

Issue **#154**. A real capture arrived 2026-09-08 (character Kess, a
Shadowknight):

```
[Tue Sep 08 13:51:06 2026] ---- Misc stats ----
[Tue Sep 08 13:51:06 2026] Movement speed: 0%
[Tue Sep 08 13:51:06 2026] Movement modifier: +55%
[Tue Sep 08 13:51:06 2026] ---- Defensive stats ----
[Tue Sep 08 13:51:06 2026] AC (display): 1783 = (Mit: 1012  + Avoidance: 499) * 1000/847
[Tue Sep 08 13:51:06 2026] Mitigation: 497 (softcap: 451)
[Tue Sep 08 13:51:06 2026] Avoidance: 558 (with AAs)
[Tue Sep 08 13:51:06 2026] ---- Melee Primary: Goldenrod ----
[Tue Sep 08 13:51:06 2026] Offense: 395 (Skill 225 + Stat 90 + SpellAtk 10 + ItemAtk 70 + Class 0)
[Tue Sep 08 13:51:06 2026] To Hit: 457
[Tue Sep 08 13:51:06 2026] Display ATK: 1145 = (offense + to hit) * 1000 / 744
[Tue Sep 08 13:51:06 2026] Dmg = BonusDmg + BaseDmg * MitFactor * DmgMultiplier
[Tue Sep 08 13:51:06 2026] Dmg = 30 + 40 * (0.1 to 2.0x) * (1 to 2.64, ave = 1.62)
[Tue Sep 08 13:51:06 2026] Dmg = 34.00 to 242.99, ave = 95.19
[Tue Sep 08 13:51:06 2026] DPS = 10.62 to 75.93, ave = 29.75
[Tue Sep 08 13:51:06 2026] DPS = 20.00 to 142.94, ave = 56.00 (81% haste)
```

### What each field is (this IS the "what is what" the request asks for)

| Line | Field | Plain meaning |
|---|---|---|
| `Movement speed: 0%` | run-speed bonus from the active movement buff/AA | +% over base run |
| `Movement modifier: +55%` | net movement-rate modifier | SoW etc., after snare/enc |
| `AC (display): 1783` | the AC number on your inventory screen | `(rawMit + rawAvoid) * 1000/847` |
| `Mit: 1012` (in AC line) | raw mitigation AC before the era softcap | pre-cap |
| `Avoidance: 499` (in AC line) | raw avoidance before combat-agility AA | pre-AA |
| `Mitigation: 497 (softcap: 451)` | mitigation AC **after** the softcap | what actually reduces hits; `hardcap` label pre-Luclin |
| `Avoidance: 558 (with AAs)` | avoidance **including** Combat Agility etc. | chance to take zero damage |
| `Offense: 395 (Skill 225 + Stat 90 + SpellAtk 10 + ItemAtk 70 + Class 0)` | melee offense + its decomposition | drives mitigation factor + damage mult |
| `To Hit: 457` | to-hit rating vs the target's avoidance | hit chance |
| `Display ATK: 1145` | the ATK number on your inventory screen | `(offense + toHit) * 1000/744` |
| `Dmg = 30 + 40 * (0.1 to 2.0x) * (1 to 2.64, ...)` | bonus dmg, base dmg, mit-factor range, dmg-multiplier range | per-swing formula |
| `Dmg = 34.00 to 242.99, ave = 95.19` | resulting per-swing damage | min / max / average |
| `DPS = 10.62 to 75.93, ave = 29.75` | sustained melee DPS, no haste | baseline |
| `DPS = 20.00 to 142.94, ave = 56.00 (81% haste)` | same, with current haste | effective |

### What it's beneficial for (beyond the raw request)

1. **Authoritative cross-check for `eqstat` / Character Info → Stats.** Our
   engine *assumes* skills are at the class/level cap (`LIMITATIONS.md` §7.1).
   `/mystats` reports the client's **actual** numbers, and the `Offense:` line
   even hands us the real weapon-skill value (`Skill 225`) and the STR / spell
   / item / class split. A capture lets the Stats tab show
   "Zeal reports ATK 1145 / AC 1783" next to our estimate — or flag a gap.
   It does not *fix* §7.1 (still needs the user to run the command) but it's
   the first ground-truth we've had.
2. **Gear / bandolier A-B testing.** The sample is preceded by
   `Loading bandolier set [DPS]` — the user swapped gear and ran `/mystats` to
   see the effect. Snapshot + side-by-side diff = exactly the ask.
3. **`/mystats <item link>`** (future) prints just the two melee blocks for a
   hypothetical weapon → a "what if I wielded this" compare on the item page.

### Parser spec (from the real sample)

**Delimiting:** `/mystats` with no args always starts with `---- Misc stats ----`
(Zeal source, unconditional). Lines are all within the same log-second. The
block has **no terminator line** — end it on the first line that matches no
`/mystats` pattern, on a new `---- Misc stats ----`, or on a >2s timestamp gap.

**Sections:**
- `---- Misc stats ----` → `Movement speed: N%`, `Movement modifier: ±N%`
- `---- Defensive stats ----` → the AC / Mitigation / Avoidance lines above
  (regex must tolerate the double space in `(Mit: %i  + Avoidance…`; accept
  `softcap` **or** `hardcap`)
- `---- Melee <Primary|Secondary>: <weapon name> ----` → 0–2 blocks (the
  sample has only Primary — a 2H wielder or a Zeal build that skips an empty
  HandToHand secondary; treat Secondary as optional). Each block:
  `Offense:` (+ breakdown), `To Hit:`, `Display ATK:` (primary only),
  two `Dmg =` lines, `DPS =` line, optional `DPS = … (N% haste)` line.
- Ignore the `Dmg = BonusDmg + BaseDmg * …` legend line (constant text).
- `/mystats info` and `/mystats affects` output: recognise-and-skip for now
  (don't snapshot the formula reference).

**Implementation shape** (mirrors the `/who` multi-line pattern —
"consumers buffer entry rows and flush on summary"):
- New `internal/mystats` package: a **pure** `Parse([]string) (Snapshot, error)`
  + a `Consumer` that buffers raw lines from `HandleLine(ts, msg)`, detects the
  block, and persists a `Snapshot` on finalize.
- `user.db` table `character_stat_snapshots` (history belongs here, **not** on
  `characters`): `id, character COLLATE NOCASE, captured_at, raw_text,
  parsed_json`. Store the raw block too so a future parser rev can re-parse.
- API: `GET /api/characters/{id}/stat-snapshots` (list, newest first),
  `DELETE …/{snapId}`. Capture is automatic from the log; no POST.
- Fixtures: commit the sample above verbatim as
  `testdata/.../mystats_sk_2h.txt`; add a dual-wield capture when one arrives.
- **Zeal-version note in the package doc:** `/mystats` is Beta (disciplines
  ignored, anti-twink logic unverified, no ranged/DW/double-attack yet) and
  its line text may drift between Zeal releases — the parser is best-effort
  and stores raw text so a drift doesn't lose the capture.

**Frontend (its own phase):** Character Info → new "Stat Snapshots" view —
list of captures, pick two, render a labeled side-by-side with the plain-
meaning column above and per-field deltas. Design pass needed; not blocking
the backend.

### Deliverables — DONE (commit `604c7595`)

- [x] `internal/mystats`: `Parse` + `Consumer` + tests against the real sample
      (+ synthetic dual-wield, non-/mystats, dedup, back-to-back blocks).
- [x] `character_stat_snapshots` table + store + `Consumer` wired into
      `dispatchLine` (live + replay, with `Flush()` on replay end).
- [x] `GET/DELETE /api/characters/{id}/stat-snapshots`.
- [x] Frontend "Stat Snapshots" tab — pick-2 side-by-side + deltas + per-row
      plain-English hints. Refreshes on `character:stat_snapshot` WS event.
- [ ] `LIMITATIONS.md` §7.1 note (cross-check now available) — pending.
- [ ] Confirm the Secondary-block text against a real dual-wield capture — the
      current assertion for it is synthetic. Not blocking.

---

## Suggested sequencing for the next release

1. **Part 1 §A** (decode foundation + real-capture fixtures) — ✅ **DONE**
   (commit `b63bbcec`). Unblocks everything, ships nothing user-visible.
2. **Part 2** (unspent AA) — ✅ **DONE** (commit `2f3ed03c`). Exercises the
   label path end to end.
3. **Part 3** (`/mystats`) — ✅ **DONE** (commit `604c7595`). Parser + store +
   API + Stat Snapshots tab.
4. **Part 1 §B** (pet_id / charm binding) — ✅ **DONE** (commit `04907900`).
5. **Part 1 §C** (target_id / sticky variants) — ✅ **DONE** (commit `62c15ff4`).
6. **Part 1 §D + §E** (group / raid decode) — ✅ **DONE** (commits `cbba2641`,
   `20343313`). Raid-window overlay + verbose group HP + a Settings toggle for
   the map arrows remain follow-ups.
7. **`LIMITATIONS.md` pass** — ✅ **DONE** (§5.1 in `cbba2641`; §1.3, §1.4,
   §2.1/§2.2, §3.1/§3.2/§3.3/§3.5, §6.1, §7.1 in `80c22072`).

**All of Part 1 + Parts 2 & 3 + the LIMITATIONS sweep are now shipped**, plus
the groupmate-arrow Settings toggle (`22a7bbd2`, pref `map_show_group`).
Remaining Zeal-1.4.6 work is optional follow-ups, none blocking a release:
- a dedicated raid-window overlay (roster grid / group layout / HP bars) —
  **deferred, out of current scope**
- combat holding per-instance damage rows keyed by `target_id` / `pet_id`
- verbose group HP fed into an HPS estimate — **deferred**, and gated on
  whether an HPS meter is worth building at all (see §2.1 — attribution,
  overheal, HoT-vs-direct, and cross-zone all still block a real parse)

### Regression / fallback audit (2026-09-08)

Everything in this batch is additive and nil-safe for pre-1.4.6 Zeal:
- **Decode layer** — the new `Player` id fields are `*int`, nil when the key
  is absent; every consumer treats nil as "use the old path".
- **`pet_id`** — `SetPipePetID(nil)` is a no-op in both the spelltimer and
  combat trackers, so charm timers behave exactly as before `a92c3996`
  (linger to expiry) with no pipe or old Zeal. `ResetPipePetID` on
  disconnect drops the id **without** clearing timers.
- **`target_id`** — `SetPipeTargetID(nil)` is an early return; `setTarget`'s
  cache read is skipped for a nil id, so resolution is byte-identical to
  before. The cache is flushed on every zone change (log + pipe) and
  disconnect.
- **`MsgRaid` / `MsgGroup`** — these message types and their roster / `loc`
  fields **predate PR #229** (only `spawn_id` is 1.4.6-new, and neither path
  uses it), so the Players-tab population and the group map arrows work on
  *any* Zeal that emits type 5 / 6. Not a regression — a bonus for
  not-yet-updated users.
- **Fixed in `22a7bbd2`**: `combat.SetPipePetID`'s revoke-on-change opened a
  one-pulse unbound window on every re-summon for 1.4.6 users. Now store-only.
- **`/mystats` + unspent-AA** aren't Zeal-version-gated at all (label 71
  predates 1.4.6; `/mystats` is pure log parsing).

## ✅ Dire Charm in Charm Pet Finder — DONE (commit `a7191196`)

The earlier "no row in quarm.db" read was wrong — it searched for the *name*
"Dire Charm". Quarm names the three per-class effect spells differently, and
they're fully specced. Per EQMacEmu `zone/aa.cpp` the Dire Charm AA
(`altadv_vars` skill_id 145, `class_type` 59 = required level, `aa_expansion`
3 = Luclin, classes bitmask 18496 = Druid+Necro+Enchanter) casts:

| Class | Spell id | quarm.db name | targettype | max charm lvl |
|---|---|---|---|---|
| Necromancer | 2759 | Undead Pact | 10 (undead) | 46 |
| Druid | 2760 | Servant of Nature | 9 (animal) | 46 |
| Enchanter | 2761 | Dominating Gaze | 5 (any) | 46 |

Body gate falls out of `RestrictionForTargetType` with no special case; resist
+ level-cap logic already read `max1`. Only the required level needed pinning
(their `classesN` columns read 254/255/bogus) — `researchLevels` generalised
to `grantedLevels` with 59 for all three. All display as "Dire Charm" via a
new `charm.DisplayName` helper. Matches eqpetfinder's
"Dire Charm (Max Level: 46, Req Lvl: 59)".
