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

**Samples (search by item name on the Items page):**

| Item | itemtype | Before | After |
|------|----------|--------|-------|
| a tattered note | 0 | 1H Slashing | 1H Slashing |
| Hunters Bow | 5 | Bow | Bow |
| Cloth Tunic of Truth* | 10 | Armor | Armor |
| Tundrabear Sandwich | 14 | Food | Food |
| Withered Manisi Leaf | 18 | Bandages | Bandages |
| Forlorn Arrow | 27 | Arrow | Arrow |

**What to confirm:** every item type pill across these pages still
renders identically.

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

## 5. Item Class Bitmask — `CHANGED` (Berserker bit added)

**Where:** Item detail modal "Classes:" row.

**Samples:**

| Item | classes mask | Before | After |
|------|-------------|--------|-------|
| The Skull of Torture | 56167 | (bit 15 silently dropped, listed classes excluded Berserker) | Berserker present in the comma-separated list |
| any all-class item | 32767 or 65535 | All | All |

**To verify:** any item whose `classes` mask sets bit 0x8000 used to
silently drop Berserker from the rendered list. Search "Skull of
Torture" and confirm Berserker is included.

---

## 6. Item Race Bitmask — `CHANGED` (Froglok + Drakkin bits)

**Where:** Item detail modal "Races:" row.

**Samples:**

| Item | races mask | Before | After |
|------|------------|--------|-------|
| The Skull of Torture | 56167 | races list dropped bits 14/15 | now includes **Froglok** and **Drakkin** |
| Spell: Force of Akera | 65535 | All | All |
| Denon's Horn of Disaster | 131071 | All | All |

**To verify:** any item with `races` mask that has bits 14 or 15 set
(but isn't the "all" sentinel ≥ 65535) now lists Froglok / Drakkin.

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
