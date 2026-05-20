# Enum Audit — Step 1 of the Database Project

This is a **read-only survey** of every place in the PQ Companion codebase
that interprets a raw numeric value from `quarm.db` (or from log lines that
reflect DB state) as a human label, a behavior switch, or a bitmask
decomposition. It is the input to a later centralization refactor in
`backend/internal/db/enums/` (see `docs/db-pipeline.md` for the full plan).

## Why this exists

The Project Quarm SQL dump is assembled from three upstream conventions:

- **EQMacEmu** (`github.com/EQMacEmu/Server`) — Mac-classic-client fork,
  closest to Quarm's frozen era. Canonical for skills, item types, special
  abilities, and most pre-PoP enums.
- **EQEmu** (`github.com/EQEmu/Server`) — modern fork, newer enum values
  and more complete tables. Used as a backstop for values that exist in
  the dump but aren't documented in the Mac fork.
- **Quarm-specific tweaks** — ZEM, hotzones, custom content
  (`item.id >= 100000`), AC softcap behavior, custom AA grants, etc.

Precedence order when interpreting a code: **Quarm dump wins on values
that exist in it; for label/shape questions, EQMacEmu first, then EQEmu.**

## How to use this doc

- **New bug surfaces?** Find the relevant enum row below — if the
  "Source/citation" column says "uncited" or "guessed," the bug is likely
  there and the fix is to cite an authoritative source.
- **New `quarm.db` dump drops?** Once we have the planned validator
  (`backend/cmd/enum-audit/`), run it; any unknown values it reports map
  directly back to rows in this table.

---

## Findings

| Enum | File:line | Coverage | Source/citation | Quarm-specific |
| --- | --- | --- | --- | --- |
| **Spell SPA (Effect ID)** | `frontend/src/lib/spellHelpers.ts:141` | Complete (0–220+) | EQEmu `spdat.h` `SE_*` — **cited in file** | No |
| **Spell Resist Type** | `frontend/src/lib/spellHelpers.ts:31` | Complete (0–9) | EQEmu — uncited | No |
| **Spell Target Type** | `frontend/src/lib/spellHelpers.ts:50` | Covers every code observed in the live Quarm dump; gaps remain at 7, 19, 21–23, 26–35, 37–39, 44–49 (unused) | EQEmu `common/spdat.h` `ST_*` — **cited in file** | No |
| **Spell Skill / School** | `frontend/src/lib/spellHelpers.ts:83` | Partial (13 entries: 4, 5, 12, 14, 15, 18, 24, 33, 41, 49, 52, 54, 70) | EQEmu — uncited | No |
| **Spell Duration Formula** | `backend/internal/spelltimer/duration.go:12` | Complete (0–11, 50, 3600) | EQEmu `CalcBuffDuration_formula` — **cited with per-formula comments** | Partial — formula 8 noted as Quarm override |
| **Spell SPA Category (mez/stun/DoT)** | `backend/internal/spelltimer/engine.go:1137` | Limited (18, 23, 0) | EQEmu — uncited | No |
| **Spell Limit SPA Codes (focus)** | `backend/internal/buffmod/buffmod.go:24` | Limited (127 cast-time, 128 duration, 134–141 limits) | EQEmu — uncited | No |
| **Spell Type Filter (beneficial/detrimental)** | `frontend/src/lib/spellHelpers.ts:473`, `backend/internal/buffmod/buffmod.go:35` | Complete (0, 1, 2) | EQEmu — uncited | No |
| **Item Type** | `frontend/src/lib/itemHelpers.ts:97` | Complete (0–45, 52) | EQMacEmu `common/item_data.h` — **cited in file** | No (EQMacEmu baseline, with documented offset vs. modern EQEmu) |
| **Item Slot Bitmask** | `frontend/src/lib/itemHelpers.ts:18` | Complete (21 slots, 0x000001–0x800000) | EQMacEmu — uncited | No |
| **Item Class Bitmask** | `frontend/src/lib/itemHelpers.ts:55` | Complete (15 classes, array-indexed) | EQMacEmu — uncited | No |
| **Item Race Bitmask** | `frontend/src/lib/itemHelpers.ts:74` | Complete (14 races, array-indexed) | EQMacEmu — uncited | No |
| **Bane Body Type** | `frontend/src/lib/itemHelpers.ts:183` | Partial (1–8, 10, 12–14, 16–19, 25–26, 28) | EQMacEmu — uncited | No |
| **Bane Race Type** | `frontend/src/lib/itemHelpers.ts:218` | Very complete (100+ entries) | EQMacEmu — uncited; "verified by sampling actual items" | No |
| **Tradeskill Type** | `frontend/src/components/ItemDetailModal.tsx:54`, `frontend/src/pages/ItemsPage.tsx:727` | Complete (0, 55–69, 75) — **duplicated across two files** | EQMacEmu `common/skills.h` — uncited (just corrected on `main`); 75 alias-mapped to "Common Combine" empirically | Yes — 75 is a Quarm-only convention (nofail combines) |
| **NPC Class ID** | `frontend/src/lib/npcHelpers.ts:21` | Complete (0–16, 20–35, 40–41) | EQMacEmu `class.h` — **cited in file** | No |
| **NPC Race ID** | `frontend/src/lib/npcHelpers.ts:65` | Very complete (1–16, 42–130) | EQMacEmu — uncited; "verified by sampling" | No |
| **NPC Body Type** | `frontend/src/lib/npcHelpers.ts:171` | Partial (0–12, 14, 21, 23, 25, 28, 33, 34, 66, 67, 100) | EQMacEmu — uncited | No |
| **NPC Special Ability Code** | `frontend/src/lib/npcHelpers.ts:220`, `backend/internal/db/special_abilities.go:14` | Complete (1–54 + synthetic 1001–1002) — **duplicated across two files** | EQMacEmu `common/emu_constants.h` `SpecialAbility` — **cited with explicit "differs from modern EQEmu" warning** | Yes — Quarm fork uses EQMacEmu numbering, not modern |
| **AA Focus (hardcoded duration AAs)** | `backend/internal/buffmod/buffmod.go:114` | **Very limited** — only AA 21 and AA 113 | EQMacEmu — uncited; "EQEmu hardcodes in C++, `aa_effects` is empty for classic-era duration AAs" | Yes — empirically discovered; explicitly marked "add new entries as discovered" |
| **Zone Expansion** | `frontend/src/pages/ZonesPage.tsx:20` | Complete (0–14) | EQ standard — uncited | No |
| **Zone Type** | `frontend/src/lib/spellHelpers.ts:367` | Complete (1–4) | EQ standard — uncited | No |
| **Character Class (UI)** | `frontend/src/pages/CharactersPage.tsx:14`, `frontend/src/components/OnboardingWizard.tsx:26` | Complete (0–14, -1 sentinel) | EQ standard — uncited | No |
| **Character Race (UI)** | `frontend/src/pages/CharactersPage.tsx:33` | Complete (1–15, -1 sentinel) | EQ standard — uncited | No |
| **AC Softcap Factor (per class)** | `frontend/src/pages/CharacterProgressPage.tsx:434` | Complete (14 classes) | **Reverse-fit to quarmy.com** — explicitly empirical, not from upstream source | Yes — Quarm tuning |
| **Trigger Action Type** | `backend/internal/trigger/models.go:16` | Complete | Internal — PQ Companion native | n/a (app-only) |
| **Pipe Condition Kind** | `backend/internal/trigger/models.go:98` | Complete | Internal — PQ Companion native | n/a (app-only) |
| **Log Event Type** | `backend/internal/logparser/models.go:12` | Complete | Internal — PQ Companion native | n/a (app-only) |

---

## Notes & flags

### Citations are uneven
The most safety-critical maps (Spell SPA, NPC Special Ability, Item Type,
Spell Duration Formula, NPC Class) cite an upstream source in code
comments. **Everything else is uncited** — the values are likely right
(most pass casual inspection) but there's no breadcrumb back to authority
when the next dump introduces a value we don't recognize.

### Duplicated maps
Two enums are defined twice in the codebase, which is how the recent
tradeskill bug was able to be half-fixed silently:

- **Tradeskill Type** — `ItemDetailModal.tsx` + `ItemsPage.tsx` (just
  re-synced; vulnerable to drift again).
- **NPC Special Ability** — `npcHelpers.ts` + `special_abilities.go`
  (currently in sync; vulnerable).

These are the strongest case for centralizing into `backend/internal/db/enums/`
with a generated TS shim.

### Known incomplete maps (will silently fall back)
- `EFFECT_LABELS` target-type — many gaps; spells with rare target types
  will render as "Unknown."
- `SKILL_LABELS` (spell school) — only 13 of ~70+ possible values.
- `aaTable` (hardcoded duration AAs) — only 2 entries; **explicitly
  flagged for expansion**. Any classic-era duration AA outside these two
  will silently give 0% bonus.

### Empirically derived (treat as best-guess, not canonical)
- `AC_FACTOR_BY_CLASS` — reverse-fit to match quarmy.com display ±10%.
- Bane race / NPC race / NPC body / tradeskill 75 — "verified by sampling"
  the dump, no source code citation.

### Quarm-specific behaviors confirmed
- NPC special abilities (EQMacEmu numbering, not modern EQEmu).
- Item types (EQMacEmu offset documented in memory + code).
- Tradeskill 75 (Quarm nofail-combine convention; not in either upstream
  enum).
- Spell duration formula 8 (Quarm override vs. EQEmu canonical).
- AC softcap formula (Quarm-tuned numbers).
- Hardcoded duration AAs 21/113 (EQEmu C++ side, mirrored manually here).

### DB columns we read but never interpret
Worth checking whether these matter for any in-app feature:

- Spell `goodEffect` flag — read in `spelltimer/engine.go` for
  categorization but never surfaced.
- NPC deity restrictions — no mapping found in repo; if the column
  carries data, we're ignoring it.
- Item type values 6, 13, 28–30, 41–44 — gaps in `ITEM_TYPES`; may be
  unused or absent in the Quarm dump.

### Numeric ranges we expected but didn't find
- No code branches on `item.id >= 100000` for Quarm custom content. If
  custom items need different display treatment, that gate isn't wired up
  yet.

---

## Project status

1. ~~Discovery pass~~ — this doc.
2. ~~Centralize into `backend/internal/db/enums/`~~ — pattern proven on
   the two duplicated maps (tradeskill, special abilities). All other
   enums still live as TS in `frontend/src/`; migrate them as bugs
   surface or after a new dump triggers a validator hit.
3. ~~Validator: `backend/cmd/enum-audit/`~~ — `cd backend && go run
   ./cmd/enum-audit`. Exit 0 = clean, 1 = unknown codes found, 2 = DB
   open failed. Run after every Quarm DB refresh.
4. Triage remaining rows — start with **uncited** + **partial** +
   **duplicated**; defer empirically-derived rows until they cause a
   bug. The validator only protects enums already in
   `backend/internal/db/enums/`, so migrating the rest is what
   actually expands its coverage.
