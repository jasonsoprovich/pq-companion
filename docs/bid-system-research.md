# Bid / DKP Loot-System — Research Notes

**Status: research only. Not planned, not deferred, not on any roadmap.**

This captures the feasibility work and design sketch done while discussing a
possible loot "bid" / DKP feature, so it isn't lost if the idea is ever
revisited. As of 2026-06-29 there is **no intent to build external loot-system
integration.** Recorded for reference, nothing more.

## Decision (2026-06-29)

The app will **not** integrate with external DKP / EPGP / plat-ledger / roster
or point systems. Guilds run dozens of bespoke loot systems and referencing
external sources to decide winners is explicitly out of scope.

**Loot winners are determined solely by roll numbers in the EQ logs.** The
tiered roll system already shipped (see the roll tracker: pick/upgrade/alt and
need/greed contests) covers the roll-based loot determination guilds asked
for — winner = highest roll in the highest-priority tier, all derived from
`/random` lines, no external lookup.

Anything revisited here in the future must remain trackable from log rolls
alone, unless that constraint is deliberately revisited.

## Where the idea came from

User-forwarded requests floated tying rolls into existing loot/bid/DKP
systems — a "bid tracker" for different loot frameworks, customizable per
guild. On reflection the integration half was cut: too many bespoke systems,
and deciding winners shouldn't depend on data the app can't see in a log.

## Feasibility findings (the hard constraints)

These are the realities any future bid feature would have to live with:

1. **One log = the loot master.** The app reads a single character's log
   (`eqlog_<CharName>_pq.proj.txt`). Player bids are only visible when they
   arrive as **tells to the app user** (`X tells you, '...'`) or in a **channel
   the user is in**. This is inherently a loot-master tool, not an
   everyone-sees-all-bids system.

2. **Bids are free-form chat.** There is no structured "bid" log line. Parsing
   `/tell master 100` or `100 Robe` is heuristic and guild-specific, so
   **manual entry / correction would have to be a first-class feature**, not a
   fallback.

3. **Point balances are never in the logs.** DKP/EPGP balances live in an
   external site/sheet/bot. The app cannot know them from logs. It could only
   *import and track against* a roster — it could never be the authoritative
   points DB without becoming a full system of record.

4. **Item association is the other messy half.** A bare-number bid has to be
   tied to an item. Real item links in Quarm/Mac raid chat are **plain text
   with no structural delimiter** (confirmed while building the roll tracker's
   item auto-suggest), so item naming is best-effort + manual.

## Design sketch (if ever built)

Preserved as-is from the planning discussion. Not endorsed, just recorded.

### Safe rollout
Would ship **dev-gated** behind a `bid_tracker_enabled` preview toggle in the
Developer tab (off by default), mirroring Trader Tracker / Resist Calc /
Charm Pet Finder — invisible to the general user base until proven.

### Architecture (parallel to the roll tracker)
- Backend `internal/bidtracker`: `BidSession` / `Bid` model, chat capture via
  `HandleLine` (reusing `chat.ParseChat` + the roll tracker's item matcher),
  WS broadcast, REST API.
- user.db store (`OpenStore` + `migrate`, like `internal/trader`) for an
  optional roster (name→points) and bid history.
- Config holds a `BidProfile` (the per-guild knobs), like the roll profiles.
- Frontend: a gated `/bid-tracker` page + dashboard card + pop-out overlay,
  reusing the contest / copy / label UI patterns from the roll tracker.

### Data model
```
BidProfile (config — the customization knobs)
  captureSources: [tells-to-me, channel:<name>, raid, guild, say]
  minBid, minIncrement: int
  winnerRule: highest                  // + tie rule (split / reroll / earliest)
  accounting: none | deduct-bid | deduct-fixed(cost)
  rosterRequired: bool

BidSession   { itemName, open, rulesSnapshot, bids[] }
Bid          { bidder, amount, ts, source, valid, note }
Roster       { name -> points }   (user.db; import/edit/export)
```

### Loot-master workflow
Open a bid on an item (name it manually or auto from the announce line) →
bids stream in from tells/channel/manual → live leaderboard with validation
(min/increment, affordability if a roster is loaded) → close → winner =
highest valid bid → Copy result line → optional point deduction → export the
updated roster to sync back externally.

### Phasing (if it ever happened)
- **3a — MVP:** bid sessions, capture (tells-to-me + manual + one optional
  channel), customizable rules, live leaderboard, Copy result. **No point
  accounting** — already useful for plat auctions and "highest bid, points
  tracked elsewhere."
- **3b — DKP layer:** roster import/edit/export, balance display, over-budget
  flagging, configurable deduction on win.
- **3c:** attendance/kill *earning* — app as fuller system of record. Largest
  scope; most guilds earn externally.

## Bottom line

The roll-based loot work guilds actually needed is shipped. External loot-system
integration is intentionally not pursued. If it ever is, start from the
constraints in this doc — especially the single-log / loot-master reality.
