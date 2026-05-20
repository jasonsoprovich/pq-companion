# Database project — manual test plan

Use this checklist after the enum migration on the `database` branch is
merged (or while running the branch locally) to verify each enum label
renders correctly. Each section names the **UI surface to inspect**,
gives **sample DB rows you can search for**, and shows **Before** (the
label the prior frontend code produced) vs **After** (the label served
by the canonical Go catalog).

**CHANGED** rows are where the migration corrects a previously wrong
or missing label — these need active verification that the new label
is the right one.
**SAME** rows confirm no regression — the label text shouldn't
change.

When you find a discrepancy, capture: page, item/NPC/zone, the label
you saw, and what you expected. I'll feed that into a follow-up commit
on the same branch.

---

## 1. Item Type — `SAME` (label data unchanged, source moved to Go)

**Where:** Items page list (the "Type" column), Item detail modal
header, Global search results, Inventory tracker per-item type pill,
NPC loot tables.

**Note:** `effectiveItemTypeLabel(item_class, item_type)` checks
`item_class` first — items with `item_class = 2` show "Book"
regardless of itemtype (e.g. `a tattered note` has itemtype 0 but
item_class 2, so it renders as "Book"). This was already the prior
behavior and isn't a migration change.

**Samples (search by item name on the Items page):**

| Item | itemtype | item_class | Before | After |
|------|----------|------------|--------|-------|
| a tattered note | 0 | 2 (book override) | Book | Book |
| Hunters Bow | 5 | 0 | Bow | Bow |
| Cloth Tunic of Truth* | 10 | 0 | Armor | Armor |
| Tundrabear Sandwich | 14 | 0 | Food | Food |
| Withered Manisi Leaf | 18 | 0 | Bandages | Bandages |
| Forlorn Arrow | 27 | 0 | Arrow | Arrow |

---

## 2. NPC Class — `SAME`

**Where:** NPCs page (the "Class" column / filter), NPC detail page,
NPC overlay panel, Global search NPC rows, Zones page NPC list.

**Samples (search NPCs page):**

| NPC | class | Before | After |
|-----|-------|--------|-------|
| any "Warrior_GM" | 20 | Warrior GM | Warrior GM |
| any "a_banker" | 40 | Banker | Banker |
| any "a_merchant" | 41 | Merchant | Merchant |
| Innoruuk | 1 (Warrior) | Warrior | Warrior |

**What to confirm:** identical labels, no class shows as "Class N".

---

## 3. NPC Race — `CHANGED` (corrects bad PC/NPC ID overlap)

**Where:** NPCs page race column, NPC detail, NPC overlay, Zones page
NPC list, Global search.

**Samples:**

| NPC | race | Before (wrong) | After (correct) |
|-----|------|----------------|-----------------|
| `aviak_rook` | 13 | Iksar | **Aviak** |
| `a_werewolf` | 14 | Vah Shir | **Werewolf** |
| `a_brownie_scout` | 15 | Froglok | **Brownie** |
| `centaur_archer` | 16 | Drakkin | **Centaur** |
| any Iksar NPC | 128 | Iksar | Iksar (unchanged) |
| any Vah Shir NPC | 130 | Vah Shir | Vah Shir (unchanged) |
| Quarm-custom monk (`a_master`) | 353 | Race 353 | **Monk Master** ‡ |
| Quarm-custom caster (`an_animist`) | 354 | Race 354 | **Akheva Caster** ‡ |
| Quarm Mistmoore vampires (`a_highborn`) | 355 | Race 355 | **Mistmoore Vampire** ‡ |
| Quarm rotting hunters | 357 | Race 357 | **Rotting Hunter** ‡ |

‡ = Quarm-specific empirical guess (no upstream canonical name). If the
real in-game category for these NPCs is different, flag it and I'll
correct the label.

**Critical to verify:** the four canonical fixes (Aviak / Werewolf /
Brownie / Centaur) — these were all silently wrong before.

---

## 4. Item Slot Bitmask — `CHANGED` (Ammo bit fix)

**Where:** Item detail modal "Slot:" row, Items page slot column.

**Samples:**

| Item | slots | Before | After |
|------|-------|--------|-------|
| Grimling Fang Dart | 0x200000 (2097152) | (empty) | **Ammo** |
| Rockhopper Talon Dart | 0x200000 | (empty) | **Ammo** |
| Forlorn Arrow | 0x200000 | (empty) | **Ammo** |
| any armor chest piece | 0x20000 | Chest | Chest |
| any ring | 0x008000 or 0x010000 | Finger | Finger |

**Critical to verify:** 446 ammo/throwing items in the dump that
previously rendered with no slot label now show "Ammo". Search for any
"Arrow", "Dart", or "Throwing" item and confirm the slot pill is
populated.

---

## 5. Item Class Bitmask — `SAME` after Quarm-era filter (see also §6b)

**Where:** Item detail modal "Classes:" row.

**Samples:**

| Item | classes mask | Before | After |
|------|-------------|--------|-------|
| The Skull of Torture | 32768 (bit 15 only) | (bit silently dropped, list was empty) | bit 15 still hidden — Berserker is not playable on Quarm |
| any all-class item | 32767 or 65535 | All | All |

**To verify:** post-PoP class bit (Berserker) is omitted from class
lists; pre-PoP classes are unchanged.

---

## 6. Item Race Bitmask — `SAME` after Quarm-era filter

**Where:** Item detail modal "Races:" row.

**Samples:**

| Item | races mask | Before | After |
|------|------------|--------|-------|
| The Skull of Torture | 56167 | Human, Barbarian, Erudite, Dark Elf, Half Elf, Troll, Ogre, Gnome, Iksar | Human, Barbarian, Erudite, Dark Elf, Half Elf, Troll, Ogre, Gnome, Iksar |
| Spell: Force of Akera | 65535 | All | All |
| Denon's Horn of Disaster | 131071 | All | All |

**To verify:** Skull of Torture races list has **no** Froglok / Drakkin
entries (Quarm is era-frozen at PoP; those races are post-PoP). The
underlying bits exist in the data but the catalog deliberately omits
them from display while the validator still accepts them as known.

**Note:** an earlier commit on this branch *did* add Froglok / Drakkin
to the display; that was rolled back per testing feedback that Quarm's
era doesn't include those PC races.

## 6b. Item Class Bitmask — `SAME` after Quarm-era filter

The Berserker bit (0x8000, GoD-era class) is also hidden from class
lists for the same reason — Berserker isn't playable on Quarm. Class
masks that set bit 15 won't show Berserker in the comma-separated
classes list.

---

## 7. Bane Body — `CHANGED` (body type 24 added)

**Where:** Item detail modal "Bane vs Body:" row.

**Samples:**

| Item | banedmgbody | Before | After |
|------|------|--------|-------|
| Scimitar of Oak | 24 | Body Type 24 | **Animation** |
| any anti-Undead weapon | 3 | Undead | Undead |
| any anti-Plant weapon | 16 | Plant | Plant |

**Note:** "Animation" is a sample-based guess (the NPCs with bodytype
24 are all `Animation1`–`Animation10`). If in-game tooltips show a
different word, flag it.

---

## 8. Bane Race — `SAME` (derived from corrected NPC race map)

**Where:** Item detail modal "Bane vs Race:" row.

**Samples:**

| Item | banedmgrace | Before | After |
|------|------|--------|-------|
| any Treant-bane weapon | 64 | Treant | Treant |
| any Goblin-bane weapon | 40 | Goblin | Goblin |
| any anti-Vah Shir weapon | 130 | Vah Shir | Vah Shir |

**To verify:** bane race labels match — the underlying data is now the
same as NPC race (which got fixed), so for items where the bane is one
of the corrected race IDs (13–16), the label updates with it.

---

## 9. Tradeskill — already `CHANGED` on `main` before this branch

Re-listed here so you can confirm it still works after the API change.

**Where:** Item detail modal Tradeskills tab.

**Samples (already validated):**

| Item | recipe.tradeskill | Before (pre-fix) | After |
|------|---|---|---|
| Coyote Pelt | 69 | Tinkering | **Pottery** |
| Leather Tunic | 61 | Brewing | **Tailoring** |
| any pottery mold | 69 | Tinkering | Pottery |

---

## 10. Special Abilities — already `SAME` on `main` (centralized in step 2)

**Where:** NPC overlay ability badges, NPC detail page.

**Samples:** any NPC with `special_abilities` field set (e.g. Innoruuk,
any boss). All 54 ability labels + the two synthetics (See Invis / See
Invis vs Undead) should render identically.

---

## 11. Zone Expansion — `CHANGED` (two sentinels added)

**Where:** Zones page expansion column / filter.

**Samples:**

| Zone | expansion | Before | After |
|------|------------|--------|-------|
| Loading Zone (`load`) | -1 | Expansion -1 | **Hidden** |
| New Loading Zone (`load2`) | -1 | Expansion -1 | **Hidden** |
| Aviak Village (`aviak`) | 99 | Expansion 99 | **Quarm Custom** |
| Marauders Mire (`erudsxing2`) | 99 | Expansion 99 | **Quarm Custom** |
| Nektropos (`nektropos`) | 99 | Expansion 99 | **Quarm Custom** |
| any classic zone (`qeynos2`) | 0 | Classic | Classic |
| any Kunark zone | 1 | Ruins of Kunark | Ruins of Kunark |

**To verify:** filter dropdown on Zones page now includes "Hidden" and
"Quarm Custom" options; previously-unlabeled zones surface their new
expansion bucket.

---

## 12. Zone Type — `SAME`

**Where:** Spell detail modal "Zone Restriction:" (spells_new.zonetype).

**Samples:** any spell with zonetype 1 = Outdoor, 2 = Indoor, etc. No
visual change expected.

---

## 13. Character Class (PC) — `SAME` (centralized)

**Where:** Onboarding wizard class dropdown, Characters page class
column and create dialog, character info pages.

**Samples:**

| Class | id | Before | After |
|-------|------|--------|-------|
| Warrior | 0 | WAR — Warrior | WAR — Warrior |
| Necromancer | 10 | NEC — Necromancer | NEC — Necromancer |
| Beastlord | 14 | BST — Beastlord | BST — Beastlord |
| (not set) | -1 | Not set / Not set / unknown | **Not set** (unified copy across both UIs) |

**Subtle change:** OnboardingWizard used to say "Not set / unknown" for
id -1; both CharactersPage and OnboardingWizard now use the same "Not
set" string from the cache helper. If you preferred the old wording,
flag it.

---

## 14. Character Race (PC) — `SAME`

**Where:** Characters page race column + create dialog, onboarding
wizard race selector (if applicable).

**Samples:**

| Race | id | Before | After |
|------|------|--------|-------|
| Human | 1 | Human | Human |
| Iksar | 13 | Iksar | Iksar |
| Vah Shir | 14 | Vah Shir | Vah Shir |

PC race IDs **stay 1–14** (this is the `RaceIndex` enum, deliberately
distinct from the corrected NPC race enum where 13 = Aviak).

---

## 15. NPC Body Type — `CHANGED` (every previous label was wrong)

**Where:** NPC detail page "Body Type:" row, NPC overlay panel, Zones
page NPC list secondary info.

The previous TS body-type map had every label off by one or more
slots vs. the EQMacEmu canonical `BodyType` enum. After migration
the labels follow EQMacEmu source verbatim, with two Quarm-display
cleanups (numbered "Summoned 2/3" / "Dragon 2/3" collapse to plain
"Summoned" / "Dragon").

**Samples (search NPCs by name and open the detail page):**

| NPC | bodytype | Before (wrong) | After (correct) |
|-----|----------|----------------|-----------------|
| any Warrior NPC | 1 | Biped | **Humanoid** |
| any wolf / bear / tiger | 21 | Untargetable | **Animal** |
| any spider / beetle | 22 | (none, was unmapped) | **Insect** |
| any treant / plant | 25 | Swarmcreature | **Plant** |
| any dragon | 26 | (none, was unmapped) | **Dragon** |
| #Lord Inquisitor Seru | 16 | Type 16 | **Seru** |
| a flame lordling | 28 | Humanoid 2 | **Summoned** |
| #Draz Nurakk | 18 | (none, was unmapped) | **Draz Nurakk** |
| Rizlona | 32 | (none, was unmapped) | **Dragon** |
| #Lord Yelinak | 30 | (none, was unmapped) | **Velious Dragon** |
| any Plane of War creature | 19 | (none, was unmapped) | **Zek** |
| #Khati Sha NPCs | 15 | (none, was unmapped) | **Khati Sha** |
| Muramite NPCs | 34 | No Corpse | **Muramite** |

**Critical to verify:** the most common body type (1 = Humanoid)
covers ~10,600 NPCs that previously rendered as "Biped". Spot-check
any group of humanoid NPCs in any city or dungeon and confirm the
body type pill now reads "Humanoid".

## 16. Hidden placeholder NPCs — new filter (not an enum, related cleanup)

The NPC search and zone NPC list now exclude `npc_types` rows that
EQEmu uses for invisible game objects rather than real NPCs:

- name is empty, just `#`, or just `_`
- race = 127 (Invisible Man)

**Samples that should no longer appear:**

| Before-filter NPC (id) | name | race | What it actually is |
|-----------|--------|---------------------|-----|
| 2173 | `_` | 127 (Invisible Man) | dev placeholder |
| 129098 | `_` | 127 (Invisible Man) | dev placeholder |

**Where this changes the UI:**

- NPCs page search results no longer return unnamed/placeholder NPCs.
- Zone NPC lists exclude these rows.
- Zone min/max level summaries are now accurate. Example: Freeport
  East was previously showing `Level 1-99` because of a placeholder
  level-99 invisible-man row; it now correctly shows `Level 1-61`.

**To verify:** load a few zones (Freeport East, Plane of Hate, anything
city-scoped) and confirm the level range matches the highest *visible*
NPC, not a hidden trigger.

---

## 17. Spell Effect (SPA) labels — `CHANGED` (many previous labels wrong)

**Where:** Spell detail panel — the "Effects:" block (one row per spell
slot) and the focus/limit descriptions on items with click effects.

The prior frontend SPA map was modern-EQEmu-flavored, while the Quarm
dump uses EQMacEmu numbering. Several common SPA codes shift between
the two enums, so spells that use those codes rendered with the wrong
label (and a few common codes silently rendered as "Effect N").

**Sample spells to spot-check (search by name in the Spells page,
expand the Effects section):**

| Spell | SPA in use | Before (wrong) | After (correct) |
|-------|-----------|----------------|-----------------|
| Shadow Step | 42 | Berserker Strength | **Shadow Step** |
| True North | 56 | Effect 56 | **True North** |
| Levitate | 57 | True North | **Levitate** |
| Pacify | 30 | Effect 30 | **Pacify** |
| Animate Dead | 71 | Effect 71 | **Animate Dead** |
| Wake of Karana | 93 | Effect 93 | **Stop Rain** |
| Scale of Wolf / Spirit of Scale | 94 | Stop Rain | **Negate if Combat** |
| Song of Sustenance | 115 | Cannibalize | **Hunger** |
| Flame Wind / Solar Storm | 116 | Crit Melee | **Curse Counter** |
| Magical Monologue | 117 | Crit Direct Damage | **Magic Weapon** |
| Amplification / Syncopation | 118 | Crippling Blow | **Amplification** |
| Trueshot Discipline | 184 + 301 | Hit Chance, Effect 301 | **Hit Chance, Archery Damage Modifier** |
| Power Kick / Savage Kick | 164 | Effect 164 | **Kick Damage Bonus** ‡ |
| Power Bash / Savage Bash | 165 | Effect 165 | **Bash Damage Bonus** ‡ |
| Maelin's Magical Concoction | 500 / 501 / 503 / 504 | Effect 500–504 | **Quarm SPA 500/501/503/504** ‡ |
| Swiftness / Fleetness / Nimbleness | 160 | Make Drunk | Make Drunk ‡ (carried over; needs in-game verify) |

‡ = Quarm-specific or undocumented in EQMacEmu source — best-effort
label that may need refinement once you check the in-game tooltip.

**Also changed (label-only swaps from EQMac canonical):**

- 41 SE_Destroy ("Destroy") — was labeled "Shadow Step"
- 58 SE_Illusion ("Illusion") — was labeled "Levitate"
- 95 SE_Sacrifice ("Sacrifice") — was missing
- 110 SE_IncreaseArchery ("Increase Archery") — was missing
- 123 SE_Screech ("Screech") — was labeled "Reflect Spell"
- 161 ↔ 162 — "Magic Rune" and "Rune" labels were swapped
- Various focus/limit cite name tweaks (e.g. SE_Hunger 115)

**Critical to verify:** Open spell detail on at least one spell from
each row of the "wrong label" table above and confirm the new label
matches in-game expectation. For Quarm-specific codes (164, 165,
500–504), open the in-game spell tooltip and report back if the
listed best-effort label is wrong.

## 18. Spell Target / Resist / Skill — `SAME` (centralized, no label change)

**Where:** Spell detail panel header — "Target:", "Resist:", "Skill:" rows.

Labels for these three columns moved from `frontend/src/lib/spellHelpers.ts`
into the canonical Go catalog. The text itself is unchanged from the
last fix (target type 9 = "Animal" etc.).

**Quick check:**
- A Druid charm-animal spell (`Beguile Animals`, id 141) → Target: **Animal**
- Any fire DD → Resist: **Fire**
- Any wizard nuke → Skill: **Evocation**

No visual changes expected. If you see "Unknown (n)" anywhere in
Target/Resist/Skill rows, capture the spell id.

---

## End-to-end smoke

After spot-checking the rows above, also do this lap:

1. **App boot** — launch the app cold. There should be a ~10–50ms
   pause on first paint while `/api/enums` resolves; after that, the
   main window loads. If labels render as `Tradeskill 75` /
   `Ability 99` / `Race 327` stubs anywhere, the catalog fetch failed
   silently — capture the network panel for `/api/enums`.

2. **All four overlays** — DPS, buff timers, detrim timers, NPC info
   panel. Each opens a separate window and also blocks on
   `loadEnums()`. Confirm labels appear after the cache loads.

3. **Search global Cmd/Ctrl+K** — exercise item / NPC / spell / zone
   results to confirm labels render across mixed result types.

4. **Inventory tracker** — load a character's bags and confirm item
   types (Armor / Weapon / Container / etc.) all show, including any
   ammo/throwing items (`Forlorn Arrow`, darts).

5. **Validator before shipping** — from `backend/`, run:
   ```
   go run ./cmd/enum-audit
   ```
   It must print "All clean." If any future Quarm DB drop adds a new
   code the catalog doesn't recognize, it'll be flagged here.

---

## Reporting issues

For each issue found, copy this template into your feedback:

```
Enum: <which one>
Page: <where you saw it>
Sample: <item/NPC/zone name + id if you can grab it>
Got: <the label that rendered>
Expected: <what it should say>
```
