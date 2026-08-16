# Item Recharging — Feasibility Report

Status: research complete, nothing implemented. 2026-08-16.

Origin: repeat Discord request (Airythia, Osui, Sandrian) for a recharge-cost
display on items with charges. Osui had looked into this before and got stuck
on the cost formula; this is a second pass with a public-source check and
first-principles reverse engineering of the three examples on hand.

## 1. Executive summary

| Question | Answer |
|---|---|
| Is recharging in `quarm.db`? | **No.** No column, no price table, nothing charge-cost-shaped. |
| Is it in EQMacEmu's public server source? | **No.** Zero hits for "recharge" anywhere in `jdewitt/Server` (core zone/common/world code). It is not a stock game-engine mechanic. |
| Can we compute a recharge price from item data? | **No** — see §3. None of price, spell mana, spell level, or scroll price reproduce the three known examples, even approximately. |
| Can we tell whether an item is recharge-*eligible* at all? | **Yes, heuristically** — see §4. High confidence, not certain. |
| Is the classic "vendor sell-stacking trick" the mechanism here? | **No** — ruled out, see §3.4. |

Bottom line: **price is not reverse-engineerable from what we have.** Eligibility
("can this be recharged, or does it just poof") is a cheap, shippable win today.

## 2. What we know from Discord

```
Puppet Strings   10 charges, value 20pp        — sell 19pp 4sp 8cp — recharge 780pp 9gp 0sp 4cp
Larrikan's Mask  10 charges, value 1gp         — sell 9sp 5cp      — recharge 1pp 2gp 8sp 0cp
Golem Metal Wand  5 charges, value 2pp 8gp     — sell 2pp 6sp 6sp 7cp — recharge 582pp 6gp 6sp 6cp
```

Confirmed against `items` table (`price`, `maxcharges`): all three "value"
figures match `items.price` exactly (Puppet Strings id 11643 price=20000cp,
Larrikan's Mask id 2736 price=100cp, Golem Metal Wand id 14313 price=2800cp).
The reported vendor sell offers are consistently **~95% of `price`** for this
character across all three items — so `items.price` is the right base value,
and this player's CHA/faction sell rate is simply high and roughly constant
per-item. That part checks out cleanly.

Recharge does not.

## 3. Why the price isn't derivable

Ratio of recharge cost to `items.price`:

| Item | Recharge ÷ price |
|---|---|
| Puppet Strings | **39.0×** |
| Larrikan's Mask | **12.8×** |
| Golem Metal Wand | **208.1×** |

No consistent multiplier. Also tried and ruled out:

- **Click spell's mana cost** (Allure 245, Superior Camouflage 40, Pillage
  Enchantment 70) — ratios of recharge-cost-per-mana don't track the
  price-ratio ordering either.
- **Click spell's min. class level** (49, 19, 44) — no correlation.
- **Click spell's scroll price**, on the theory that recharging = re-buying N
  casts worth of the spell — direction is inconsistent: Puppet Strings'
  recharge-per-charge is *far above* its scroll price, Larrikan's Mask's is
  *far below* its scroll price.

### 3.4 Ruled out: the P1999 "vendor sell-stacking" trick

Classic EQ has a well-documented player technique (not an NPC service): sell a
full-charge copy of an item to a merchant, then sell a drained copy right
after — the merchant's inventory slot only remembers the first (full) charge
count, so buying it back "recharges" it. [P1999's guide](https://wiki.project1999.com/Guide_to_Recharging_Items)
gives the cost as `2×buy − 2×sell`, rounded up for CHA/faction slop. Plugging
in our numbers: Puppet Strings → 2×20000−2×19048 = **1,904cp**, not the
reported 780,904cp. Off by 400×. This is not what Quarm's "recharge" quote is.

Airythia's numbers describe a **distinct, much more expensive NPC-quoted
price** — almost certainly a bespoke Quarm server feature (an NPC dialog
option), not a core EQMacEmu/live-EQ mechanic. Server logic for it lives in
Project Quarm's private fork, which we have no access to.

### 3.5 Confounds even a bigger dataset won't fix without more fields

Three data points can't isolate a formula with this many free variables, and
none of the three reports capture:

- **Charges missing at quote time** — recharge cost almost certainly scales
  with charges restored, and we don't know how depleted each item was.
- **Same-session repeat discount** — Osui recalls a "halves on repeat
  recharge without zoning" effect, unconfirmed and unquantified.
- **CHA / faction** — Sandrian confirmed Puppet Strings recharge price
  already differs by character. Vendor *sell* price varies with these too
  (consistent with the ~95% figure above), so it's plausible recharge price
  does as well, on top of item-specific pricing.
- **Which NPC / zone** — different item types are plausibly recharged by
  different specialist NPCs (wand-vendor, illusion/mask-vendor, etc.), each
  of which may have independently hand-set pricing rather than sharing one
  server-wide formula. If true, no single formula exists to find — it'd be a
  per-NPC (possibly per-item) lookup table, authored by hand in Quarm's
  content, same as any other quest reward price.

## 4. What *is* feasible now: eligibility, not price

Osui's other question — "is there a simple way to tell if a charged item can
be recharged, or does it just poof?" — has a workable answer.

`items.itemtype = 21` is **Potion** in the EQMacEmu type enum
(`backend/internal/db/enums/item_type.go`). Potions are EQ's single-use
consumable category — they are drunk to depletion and never recharged, by
design, on live/emu servers generally. Every other charged-clicky itemtype
(Armor, Miscellaneous/trinkets, instruments, etc.) is the category actual
recharge NPCs service. Puppet Strings (itemtype 10), Larrikan's Mask
(itemtype 10), and Golem Metal Wand (itemtype 11) — all confirmed
recharge-able — are consistent with this.

This is a **heuristic**, not a flag Quarm's data actually exposes, so it can
misfire on individual items. But it's cheap, directionally strong, and
immediately useful.

**Bonus finding**: the existing Inventory Tracker "Rechargeable" section
(`InventoryTrackerPage.tsx:364`, backed by `RechargeableMaxCharges` in
`backend/internal/db/queries.go:210`) currently flags *any* item with
`maxcharges > 1 AND clickeffect > 0` — no itemtype filter. That's currently
**238 of 481** matching items (49%) that are Potions, mislabeled as
"Rechargeable" today. Worth fixing regardless of what happens with pricing.

## 5. Proposed plan

**Phase 1 — ship now (cheap, high-confidence):**
1. Fix `RechargeableMaxCharges` (or its caller) to exclude `itemtype = 21`
   Potions — removes the existing false-positive bug in Inventory Tracker.
2. Surface the same eligibility heuristic (`maxcharges > 1 AND clickeffect > 0
   AND itemtype != 21`) as a "Rechargeable" badge on the Items page /
   `ItemDetailModal`, not just Inventory Tracker — answers Airythia's original
   ask ("see it any place you can look at the item").
3. No price shown yet — badge only.

**Phase 2 — price, gated on more data:**
Reply in Discord asking for a structured set of reports, specifically:
item name, **charges remaining right before the recharge quote**, the
recharge NPC's name/zone, whether it was the first recharge that session or a
repeat (and how many), and the character's CHA. Without charges-before and
NPC identity controlled, no amount of additional "value / sell / recharge"
triples like the current three will resolve anything.

**Phase 3 — display, only if Phase 2 yields a pattern:**
If a real per-NPC/per-item formula falls out, treat it like the discipline
timer data ([[project_discipline_timer_data_source]]) — an externally-sourced
value layered onto `quarm.db` query-side, never hand-edited into the dump. If
it *doesn't* resolve to a formula (more likely, given §3.5), the fallback is
a manually-maintained, crowd-sourced per-item price table with a "community
reported, may vary by faction/CHA" disclaimer — same pattern as
[[project_quarm_data_corrections]].
