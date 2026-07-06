# Tradeskill Leveling Optimizer — Feasibility & Implementation Plan

Status: **RESEARCHED + PLANNED, not built** (2026-07-06)
Gated to: **Luclin era** (dev flag). PoP/PoK admitted automatically later via DB regen — no code change.

## Goal

Help a user efficiently level a chosen tradeskill from their current skill to a
target by generating an optimized, staged plan of *which recipe to make in which
skill range*. Mirrors the spell **shopping-route calculator** in architecture and
UX (pure solver package → staged plan → frozen-plan / re-plan page).

Decisions locked with the user:
- **Objective: both modes with a toggle** — "fastest" (fewest combines) and
  "cheapest plat" (vendor cost where known). Cheapest is *partial* by design
  (farmed/dropped components have no DB cost) and must be labelled as such.
- **Path source: pure DB-derived** — the optimizer picks the best recipe at each
  breakpoint from `quarm.db` live. No curated recipe lists. This is inherently
  era-safe because `quarm.db` only contains in-era recipes.

## Why this is feasible (what we already have)

The two formulas the feature depends on are **already ported** in
`backend/internal/tradeskill`:
- `Chance(rawSkill, trivial, skillModPct, aaFailReducePct, nofail)` → combine
  success% (`chance.go:119`, EQMacEmu formula).
- `EstimateSkillUp(current, trivial, cap, tradeStat, difficulty, skillMod,
  aaReduce, skillupBonusPct, nofail)` → attempts-to-next / attempts-to-target
  (`skillup.go:78`, EQMacEmu `CheckIncreaseTradeskill` port). Governing-stat
  resolution (`TradeStat`) and difficulty from `skill_difficulty` already wired.

So this feature is a **selection + planning layer** over an existing engine, not
new game-mechanics math.

We also have a proven blueprint: `internal/shoproute` (pure solver, no DB/HTTP) +
`ShoppingRoutePanel.tsx` (staged plan, frozen-plan/"Re-plan" UX, dev-flag-gated
nav). The tradeskill planner is the same shape — an ordered `[]Stage` instead of
`[]Stop`.

## The core finding (drives the whole design)

**Do not transcribe the online guides' recipe paths.** The only guides with real
step-by-step paths — TAKP/Darchon and bonzz — are **PoP/PoK-contaminated**:
- TAKP/Darchon: despite being "Mac-era," Al'Kabor *had* PoP. Its sourcing assumes
  Plane of Knowledge trader buildings, Celestial Essence from PoK NPCs (Darius
  Gandril, Loran Thu'Leth), and PoP raid-drop components (Shadowscream, etc.).
- bonzz.com: modern, through Empires of Kunark, cap 300+. Almost none era-valid.
- P99: era-appropriate baseline in spirit, but publishes **no paths** — only
  formulas and caps.

Use the guides for **methodology, not data**:
- The `[skill_low – skill_high] → recipe → trivial` breakpoint grammar. You switch
  recipe when you hit its trivial, because **skill-ups stop at trivial**.
- The "stay ~25 above current skill" sweet-spot band (EQ Traders g12).
- The cost objectives (cheapest-plat vs fewest-combines) and vendor-vs-farm split.

Derive the **actual paths from `quarm.db`**, which is the era authority — a
DB-derived path physically cannot leak PoP content on a Luclin DB.

## Mechanic recap (already implemented, for reference)

Two *separate* rolls — conflating them is the classic builder mistake:
- **Combine success** (produces item vs. destroys ingredients): trivial ≥ 68 →
  `skill − 0.75·trivial + 51.5`; trivial ≤ 67 → `skill − trivial + 66`; clamp
  [5, 95]. Pinned at 95% once skill ≥ trivial.
- **Skill-up** (+1 skill on this combine): a distinct roll that only fires while
  `skill < trivial` and `skill < cap`. Chance falls as skill rises toward trivial
  and gets rare above ~190. Failed combines can still skill you up. This is
  exactly what `EstimateSkillUp` models.

## Data feasibility — easy vs. hard

**From `quarm.db` alone (all tables our queries already touch):**
- Trivial, components (item ids + counts), yield/`successcount`, container/
  `objecttype`, tradeskill id — `tradeskill_recipe` / `tradeskill_recipe_entries`.
- **Sub-combine DAG**: a component that is itself a recipe *product* is derivable
  by joining component item-ids back to recipe outputs. No guide needed.
- Skill caps (`skill_caps`), skill difficulty (`skill_difficulty`).
- Vendor buy price **for items an in-era vendor sells** (`merchantlist` / `items.price`).

**NOT in the DB (human-curated only — the honest gap):**
- Plat cost of **farmed/dropped** components (no "cost" exists for a drop).
- Vendor-bought vs. farmed vs. sub-combine *judgment*.
- Bottlenecks like "needs an Enchanter for Clarify Mana."

⇒ **Fastest mode is 100% auto-derivable and era-safe.** **Cheapest mode is
partial** — real for vendor-stocked components, "farmed / unknown cost" otherwise.
The UI must label cheapest as partial, never imply a complete plat figure.

## Algorithm (`internal/tsplan`, new pure package) — BUILT

Implemented as an **optimal shortest-path DP over skill levels**, not the greedy
first sketched below (DP is barely more code and is exact; each transition
"grind recipe X from skill s to min(trivial, cap, target)" has additive cost, so
Dijkstra/DP applies cleanly). Reuses a newly-extracted
`tradeskill.SkillUpChanceAt` (per-attempt CheckIncreaseTradeskill probability) so
the mechanic stays single-sourced with `EstimateSkillUp`. A `SwitchPenalty` param
(objective units) curbs fragmentation; no-farming mode drops unknown-cost recipes;
cap-exceeded and unreachable-target both degrade to a partial plan + warning.
`Solve(recipes []RecipeCandidate, Params) Plan`. Table-driven tests cover chain
ordering, cheapest-vs-fastest divergence, farming toggle, cap clamp, unreachable,
switch-penalty consolidation, and stage notes.

Original greedy sketch (superseded by the DP above):

```
plan(tradeskill, startSkill, targetSkill, cap, char stats, objective, allowFarming):
  skill = startSkill
  stages = []
  loop until skill >= targetSkill (or no candidate):
    candidates = recipes where skill < trivial <= <reasonable ceiling>
                 AND in-era (DB-scoped) AND container/skill constraints satisfiable
    for each candidate: score via EstimateSkillUp at current skill
      - fastest  → maximize expected skill-ups per combine (min total combines)
      - cheapest → minimize (combines * per-combine vendor cost); farmed = ∞/flagged
                   unless allowFarming, then farmed treated as 0-plat but time-costly
    pick best candidate; compute combines to advance skill to min(its trivial, target)
    append Stage{range, recipe, trivial, estCombines, estCost|null, container, subCombineDeps, warnings}
    skill = min(candidate.trivial, target)
  return stages + global warnings
```

Notes:
- Ceiling on candidate trivial keeps success rate sane (guide "~25 over" band) —
  tunable; don't pick a wildly-high-trivial recipe you can't make.
- Sub-combine deps computed but **Phase 2** does full recursive cost/skill-gap
  resolution; Phase 1 just flags "this step needs crafted component X (recipe Y)."
- Container/forge constraint from `objecttype` surfaced as a stage note.

## Architecture (mirrors shoproute)

| Layer | shoproute analog | New for tradeskill |
|---|---|---|
| Pure solver | `internal/shoproute` | `internal/tsplan` — `Plan()`, `Stage`, objective enum. Reuses `tradeskill.EstimateSkillUp` for scoring. Zero DB/HTTP deps. Table-driven tests. |
| DB queries | `GetSpellVendorOptions` etc. | `internal/db`: `RecipesForTradeskill(tsID)` returning recipes + components joined to vendor prices; helper flagging which components are craftable (DAG edges). |
| API | `POST /api/spells/shopping-route` | **[BUILT — commit 6cf615b]** `POST /api/characters/{id}/tradeskill-plan` (character-scoped, not `/tradeskills/plan` — reuses skillup-estimate's stat wiring). Body `{tradeskill, target_skill?, start_skill?, objective, allow_farming?, ...}`; start defaults to Zeal export, target to class cap. Auto-resolves cap/stat/difficulty/mastery-AA. `TrivialCeiling` default 40. |
| Frontend | `ShoppingRoutePanel.tsx` | **[BUILT — commit 90b424c]** `TradeskillLevelingPage.tsx` (Database nav). Pick tradeskill + character (auto-fills current skill + cap from Zeal export), objective toggle (Fastest / Cheapest), farming + Maelin toggles. Staged table: range · recipe (links `/recipes?select=`) · trivial · combines · per-combine cost (pp/gp/sp/cp). Summary totals; warning / partial-cost / mastery-AA notes. Debounced re-fetch on input change. |
| Types | `types/spell.ts` | **[BUILT]** `types/tradeskill.ts` (`TradeskillLevelingPlan`, `LevelingStage`) + `getTradeskillLevelingPlan` in `api.ts`. |
| Gating | `flag: 'pop_flags_enabled'` in `sidebarNav.tsx` | **NONE (decided 2026-07-06).** Feature is read-only (no file/DB writes), so it ships public — a plain Database nav item, no dev flag. Owner does manual smoke before release. |

## Era gating

- Ship behind dev flag `tradeskill_planner_enabled` (default off), exact
  `pop_flags_enabled` pattern.
- Era-correctness is **automatic**: paths come from `quarm.db`, which is
  Luclin-scoped today. When PoP launches, the `data-release` regen adds PoK
  recipes and `pop_enabled` logic admits them — **no planner code change needed**.
- Nothing in `internal/tsplan` hardcodes era assumptions; it only reads what the
  DB serves.

## Phasing

1. **Phase 1 — DB-derived planner, both objective modes. ✅ COMPLETE (2026-07-06).**
   `internal/tsplan` (5d24f4b) + `RecipesForTradeskill` (9b68c18) +
   `POST /characters/{id}/tradeskill-plan` (6cf615b) + `TradeskillLevelingPage`
   (90b424c). Fastest fully auto; Cheapest = vendor-cost-where-known, farmed
   items flagged. Sub-combines *flagged* (not yet recursively costed). Shipped
   public (no dev flag). Pending owner Windows smoke before release.
2. **Phase 2 — sub-combine DAG resolution.** Recursive cost + skill-requirement;
   warn when a step needs another tradeskill (e.g. Tailoring needing Brewing
   intermediates like Paeala Bark Tannin). Fold sub-combine combines into totals.
3. **Phase 3 — curated cost/sourcing overlay (optional).** Hand-maintained JSON
   (like `quest_sources.json`) adding "buy from NPC X in zone Y / farm rate" so
   "cheapest plat" becomes complete rather than partial.

## Open questions for build time

- Candidate-trivial ceiling: fixed "+25" band, or tune per tradeskill?
- Cheapest mode with `allow_farming` off: hide farmed-dependent recipes entirely,
  or show them greyed with a "requires farming" note?
- Where to mount: new top-level "Tradeskills" nav section, or a second tab on the
  existing Recipes page? (Leaning: sibling page under the Database section, near
  Recipes.)

## Sources

- TAKP / Darchon's Tradeskill Guide — paths + cost + sub-combines (PoP/PoK-dependent).
- Project 1999 / Tradeskills — success + skill-up formulas, caps; no paths.
- EQ Traders Corner g12 — trivial concept, "within ~25 levels," recipe-window color.
- bonzz.com master index — same range→recipe→trivial grammar; modern/out-of-era.
