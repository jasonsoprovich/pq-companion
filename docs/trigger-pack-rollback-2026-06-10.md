# Trigger pack changes 2026-06-10 — rollback guide

The 2026-06-10 batch reworked the built-in trigger packs (community packs,
shared CC break alerts, merged Enchanter spell-line triggers). This doc
records exactly what changed and how to undo it — per-user without a
release, or in code for a release — in case the merged-trigger model
doesn't land well with users.

## What changed (commits)

| Commit | Change | Risk if reverted |
|---|---|---|
| `6af14db` | Seven new community packs (Caster Alerts, Crit Alerts, Spell Breaks, Group Alerts, Raid Alerts, Tracking, Misc Alerts) **and** converted the per-class Charm/Root/Snare Broke alerts in Enchanter/Druid/Paladin/Bard/Wizard into shared dedup-keyed helpers (`sharedCharmBreak`/`sharedRootBreak`/`sharedSnareBreak`, dedup keys `charm_broke`/`root_broke`/`snare_broke`) with broader union spell lists. | Reverting wholesale also removes the seven new packs. To keep the packs but restore per-class break alerts, restore the old trigger literals from the JSON snapshots below instead. |
| `713f91d` | Engine support for merged triggers: `ExtraPattern.timer_duration_secs` / `.spell_id` per-row overrides and `Trigger.timer_key_capture` (new `user.db` column; one countdown per captured spell name). Editor UI for both. | None — purely additive and backward compatible. Triggers that don't set the new fields behave exactly as before. There is no reason to revert this even if the pack changes revert. |
| `d72b6ef` | Enchanter pack consolidation using the above: Mesmerize/Mesmerization/Dazzle → "Mez", 5 charms → "Charm", 5 pacifies → "Pacify", Root/Fetter/Greater Fetter → "Root" (pack went from 35 → 23 triggers). | This is the user-visible behavior change. Revert restores per-spell triggers. |

## What users actually experience

- **Installed packs are never mutated in place.** A user who installed the
  Enchanter pack before 2026-06-10 keeps the old per-spell triggers until
  they explicitly reinstall the pack from the Packs tab. The merged
  triggers only arrive on (re)install.
- The seven new packs are additive — a user who dislikes one just
  uninstalls it.
- The shared break alerts only dedupe on **fresh installs** (existing
  rows have no `dedup_key`). A pre-existing class pack plus a new
  "Spell Breaks" install can therefore double-fire break alerts — the fix
  is reinstalling the class pack (picks up the dedup-keyed copy) or
  disabling one row.

## How dedup and categorization work (reference)

- **Install-time dedup** happens only through `dedup_key`. Current keys:
  `disc_resistant`, `disc_fearless` (melee discs), `charm_broke`,
  `root_broke`, `snare_broke` (CC break alerts). Installing a second pack
  that ships the same key skips that trigger; uninstalling the owning pack
  promotes the copy from another still-installed pack
  (`insertPackTriggers` / promote-on-uninstall in `packs.go`).
- **Timer triggers do not overlap between class packs** — e.g. the
  Necromancer pack ships no charm/root timers and the Wizard pack no root
  timer, so Enchanter + Necromancer never collide. If a collision is ever
  introduced, two safety nets apply: class packs install with `characters`
  defaulted to that class's characters (an Enchanter-pack trigger doesn't
  fire while playing the necro), and the spelltimer engine keys timers by
  spell name, so two triggers starting "Charm" produce one timer row, not
  two.
- **Categorization:** every trigger row carries `pack_name` = the pack
  that installed it; the Triggers tab groups by that value and flags
  built-ins as pack-managed. Shared dedup triggers live under whichever
  pack installed them first (`source_pack` records the owner).

## Rollback option A — per user, no release

`docs/legacy-packs-2026-06-10/` contains the exact pre-change definitions
of every modified pack (snapshotted from commit `92290f4`, default timer
alerts included) as importable TriggerPack JSON:

- `enchanter.json` (35 triggers, per-spell mez/charm/pacify/root)
- `druid.json`, `paladin.json`, `bard.json`, `wizard.json` (per-class
  break alerts)

Importing one (Triggers → Packs tab → Import JSON, or
`POST /api/triggers/import`) **replaces all triggers in that pack_name**
with the legacy set — an instant per-user rollback. Two caveats:

1. The legacy break alerts have no `dedup_key`, so the user should
   uninstall the "Spell Breaks" pack too (or disable its break rows) to
   avoid double alerts.
2. Import re-derives the `characters` default from the pack's class, same
   as a normal install.

## Rollback option B — in code, for a release

1. `git revert d72b6ef` — restores per-spell Enchanter triggers. This is
   the only revert most complaints would need.
2. If the shared break alerts must also go: do **not** blind-revert
   `6af14db` (it would delete the seven community packs). Instead restore
   the five per-class break trigger literals from the JSON snapshots and
   drop the `shared*Break` helpers + the three break triggers from the
   Spell Breaks pack.
3. Leave `713f91d` (engine) in place either way. The `timer_key_capture`
   column in `user.db` is harmless if unused, and the migration is
   idempotent.

After a code revert ships, users on merged triggers get the old behavior
by reinstalling the affected pack — same mechanism as the rollout.
