# Maps Feature — Feasibility Report & Phased Plan

Status: research complete, nothing implemented. 2026-07-28.

Origin: two Discord requests — Grokii's "Interface between the DB and Zeal
external Maps" (2026-05-24) and HughJeffner's "Vendor Database Improvements"
(2026-07-27), which converge on the same missing capability: **PQ Companion
has no concept of where anything is.**

Direction set 2026-07-28: build **our own map set**, derived primarily from
`quarm.db` and client zone geometry, bundled with the app as a `maps.db`.
Brewall's files are used as a *reference and gap-fill* source for a minority of
hand-researched annotations, with per-row provenance tracking (§7).

---

## 1. Executive summary

| Question | Answer |
|---|---|
| Can we render vector maps in-app? | **Yes** — line/point lists, trivially parseable |
| Do we have the NPC coordinates? | **Yes** — `spawn2` in `quarm.db`, 202 zones |
| Is the coordinate math known? | **Yes — empirically verified** (§3) |
| Can we drive the *in-game* Zeal map? | **Yes, and far better than expected** (§4) |
| Do we have live player position? | **Yes** — ZealPipes already streams X/Y/Z/heading |
| Can we build our own map set? | **Yes** — geometry from client files, POI from the DB (§5) |
| How big is the bundled artifact? | **~4 MB `maps.db`, 0.05 ms/zone fetch** — measured (§6) |
| Is the POI layer reconstructible? | **Mostly** — see the category audit (§7) |

Two things worth stating up front:

**Phase 1b needs no map data at all.** Driving Zeal's in-game map through
clipboard commands requires zero assets and zero licensing clearance, because
Zeal already embeds map geometry on every user's machine. It ships independently
of everything else in this document.

**The storage schema is not obvious.** The natural "one row per line segment"
table performs *worse* than the raw text files it replaces. The packed-blob
schema is ~9× smaller. Measured numbers in §6.

---

## 2. Reference data: format, coverage, size

Analysed `/Users/jasonsoprovich/Downloads/brewall-20240109/` (1707 files, 146 MB)
as the reference corpus and as a volume proxy for sizing our own output.

Format is two line types, no comments permitted:

```
L x1, y1, z1, x2, y2, z2, R, G, B      line segment
P x,  y,  z,  R, G, B, size, label     point of interest (label uses _ for space)
```

Across all files: 2,378,380 `L` lines and 33,993 `P` lines. This is **vector
data, not images** — which is also what PQDI renders (its map controls are
"Line Weight / Toggle Points / Toggle Animation", i.e. a line renderer). Vector
is the right target for us too: crisp at any zoom, and recolourable to the app's
dark theme.

Files come in layers: `<zone>.txt` (geometry), `<zone>_1.txt` (POI labels,
traps, ground spawns), `<zone>_2.txt` (extra detail). Zeal loads `_1`…`_10`.

**Zone coverage reference:** of 192 zones in `quarm.db`, the only ones absent
from the reference corpus are `air_instanced`, `arena2`, `aviak`,
`cazicthule_old`, `clz`, `erudsxing2`, `fear_instanced`, `hate_instanced`,
`hole_instanced`, `load`, `load2`, `nektropos`, `towerbone`, `towerfrost`,
`tutorial` — all instanced, loading, or dead zones. Our own set must cover the
same 181 live zones.

**Quarm-relevant subset volume:** 535 layer files, 636,952 line segments,
12,237 POI points, 41.2 MB as raw text.

---

## 3. Coordinate transform — verified, not assumed

Grokii's design doc gives the transform in PQDI's display order, which is
confusing because PQDI prints coords as `(Y, X, Z)`. Stated directly against our
own schema:

```
map_file_field1 = -( spawn2.x )
map_file_field2 = -( spawn2.y )
map_file_field3 =    spawn2.z     (unchanged; used for Z-level filtering)
```

**Negate X and Y, keep DB order, leave Z alone.** No swap is involved once you
stop going through PQDI's display convention.

Verified empirically against three independent landmarks in `qeynos2`:

| Landmark | `quarm.db` (x, y, z) | Reference file |
|---|---|---|
| Priest of Discord | `235.0, 6.0, 2.75` | `P -235.0000, -6.0000, -0.3738` |
| Aqueduct zone point | `90.0, 175.0, -39.0` | `P -90.0000, -175.0000, -40.4520` |
| Aqueduct zone point | `348.0, 194.0, -27.0` | `P -341.9558, -190.6446, -24.3454` |

(Z differs slightly — traced floor height vs. stored spawn Z. Immaterial.)

---

## 4. Zeal integration

Zeal's map command surface is much richer than the Grokii doc suggests.
From the Zeal README:

| Command | Use for us |
|---|---|
| `/map marker <y> <x> [label]` | **Pin an arbitrary NPC location.** The core "locate in game" feature |
| `/map <y> <x> <label>` | Same, shorter — matters against the clipboard cap |
| `/map line <y1> <x1> … ` | Draw a route (max 8 coordinate pairs per command) |
| `/map show_zone <shortname>` | **Preview another zone's map without travelling there** |
| `/map rsay marker <y> <x> <label>` | Broadcast a pin to the raid |
| `/map poi <search-term>` | Jump to a named POI |
| `/map loc` | Drop a marker at the player's position |
| `/map save_ini` | Persist settings — **per character**, which is Grokii's complaint |

Note the argument order is `<y> <x>`, matching in-game `/loc` output — so the
values are `spawn2.y, spawn2.x` **un-negated**, since Zeal applies the map
negation internally. This is the one assumption in this document that has not
been verified against the running game.

> **Verification gate before building Phase 1b:** in North Qeynos, run
> `/map marker 6 235 Test`. If the pin lands on the Priest of Discord, the
> convention is confirmed. If it lands mirrored, the args need negating.

Commands are short and single-line, so they fit comfortably under the
established 400-char `eqClipboard.ts` cap. One command per copy — EQ collapses
multi-line pastes to a single line (see `project_eq_clipboard_cap`), so batching
markers into one paste is not possible.

Zeal is MIT-licensed and **embeds map geometry internally** (compiled into
`zeal.asi`, not shipped as files). Two consequences:

1. Every Quarm player already has in-game map geometry — so the clipboard bridge
   drops pins onto a map the user already sees, with **zero assets from us**.
2. We **cannot** source an in-app map by reading the user's disk. An in-app map
   needs its own data. Hence §5.

---

## 5. Sourcing decision: build our own, bundle it

### 5.1 Why not simply ship Brewall's files

`eqmaps.info` publishes **no license, no copyright statement, and no
redistribution terms** — and neither does the official mirror at
`github.com/RedGuides/brewall-maps` (checked: `LICENSE`, `LICENSE.md`,
`LICENSE.txt`, `COPYING` all 404 on both branches). The community norm is
plainly "use it, credit Brewall": PQDI states "Credit for all maps goes to
Brewall", and Zeal ships the data embedded with attribution.

Norm is not permission, and wholesale redistribution is the most exposed
possible position. Hence building our own.

### 5.2 Where our data actually comes from

Two independent sources, neither of which is Brewall:

**Geometry — from client `.s3d` files.** Zone geometry is the game's, not
Brewall's, and it sits in the EQ directory every user already has. Tooling is
real and appropriately licensed:

| Project | License | Role |
|---|---|---|
| [LanternExtractor](https://github.com/LanternEQ/LanternExtractor) | **MIT** | Extracts collision meshes from `.s3d`; WLD reader is portable to Go |
| [EQEmu/zone-utilities](https://github.com/EQEmu/zone-utilities) (`azone`) | GPL-2.0 | Build-time tool only — output is not encumbered |
| [EQEmu/maps](https://github.com/EQEmu/maps) | **NOASSERTION** — no LICENSE file | Not a usable shortcut |

**POI data — from `quarm.db`.** Substantially reconstructible, and in several
categories *more* authoritative than hand-placed markers. Full audit in §7.

### 5.3 Bundled, not generated on the user's machine

An earlier draft recommended shipping a generator that builds maps from the
user's own client files at setup, on the theory that a purely local transform
distributes nothing. That was over-cautious and is **rejected**:

- Precedent is overwhelming — Brewall has distributed this class of derivative
  publicly for 26 years; EQEmu ships a 1.1 GB repo of collision geometry pulled
  straight from client files; Project Quarm is itself a far larger IP question
  than its maps.
- Machine-generated floor plans are close to pure fact/function — thin copyright.
- **Decisive practical objection: we cannot QA output we never generate.**
  181 zones × every user's variant install, on hardware we cannot reproduce,
  with per-zone parse failures arriving as support tickets. Bundled data is
  testable, versionable, and patchable in a point release.

Generation therefore happens **once, offline, by us**, as a build-time pipeline.
The output is a `maps.db` artifact bundled with the app. User-side generation
stays documented as a fallback only.

### 5.4 Still worth asking Brewall

Even though he is no longer the primary source, a permission/blessing
conversation is still worth having, because §7 gap-fill uses his research and
because a yes removes all ambiguity. He is reachable at:

1. **RedGuides** — [account](https://www.redguides.com/community/members/brewall.237155/),
   [resource thread](https://www.redguides.com/community/threads/brewalls-everquest-maps.39316/).
   Active and responsive; asking publicly is also good optics.
2. **Twitter/X `@BrewallEQ`** — the only contact he names himself.
3. **Email `brian.harris.tx@gmail.com`** — *inferred, not published.* Derived
   from the WordPress author-archive slug `/author/brian-harris-txgmail-com/`.
   Likely valid, but CMS-leaked rather than published; fallback only.

Whatever the outcome, credit him in the UI and the About page.

---

## 5b. Spike results — pipeline validated end to end (2026-07-28)

Ran against a real TAKPv22 client (`/Volumes/T7/EQ/TAKPv22`, 10 GB, 661 `.s3d`).
Working Python prototype in `scripts/mapspike/` — ~120 lines total. **Every
structural assumption in §5.2 held, with two corrections.**

### 5b.1 Archive and WLD format — confirmed

| Layer | Result |
|---|---|
| `.s3d` PFS archive | Parses. Header `PFS ` / v`0x00020000` |
| **Name-table CRC** | **`0xFFFFFFFF`, not the `0x61580AC9` that PC-client tooling assumes** — Mac/Trilogy divergence |
| `unrest.s3d` contents | 76 files: 73 `.bmp` + `unrest.wld` (3.5 MB), `objects.wld`, `lights.wld` |
| WLD version | `0x00015500` (Trilogy). Uses **`0x36` mesh fragments** — the well-documented kind, not legacy `0x2C` |
| `unrest.wld` fragments | 7,578: 4,058 × `0x22` BSP region, 3,198 × `0x36` mesh, 1 × `0x21` BSP tree |
| `objects.wld` | 164 × `0x15` placements (object instances) |
| Mesh extraction | unrest: 48,631 verts / 26,291 tris. Polygon flag `0x10` = non-collidable, filter it |

### 5b.2 Coordinate transform — corrected

```
game_x = mesh_Y
game_y = mesh_X      ← pure axis swap, NO negation
game_z = mesh_Z

map_f1 = -game_x = -mesh_Y
map_f2 = -game_y = -mesh_X
```

Validated by matching `objects.wld` placements against `doors` + `spawn2` across
all 8 axis permutations, on three zones:

| Zone | Best orientation, within 10u | Runner-up |
|---|---|---|
| unrest | `(mY, mX)` — 82/164 | 70/164 |
| qeynos2 | `(mY, mX)` — 81/480 | 50/480 |
| gfaydark | `(mY, mX)` — 200/2829 | 28/2829 |

> **Methodology warning.** An earlier pass tested only 4 of 8 orientations and
> compared bounding boxes, which agreed on a *mirrored* transform — both test
> zones are near-symmetric on that axis, so bounds cannot detect a sign flip.
> The error was only caught by rendering the map and eyeballing it against
> Brewall's. **Always validate cartography visually, not just numerically.**

### 5b.3 Extraction technique depends on zone type

Two distinct problems, established by rendering:

**Interiors / structures → walkable-surface boundary extraction.** Keep faces
whose normal is within ~55° of vertical, weld vertices by position, take edges
belonging to exactly one floor triangle, then join into polylines and decimate
near-collinear points.

*Vertex welding is not optional.* Meshes arrive as ~3,200 separate `0x36`
fragments with independent index spaces, so coincident vertices never share
indices and every triangle looks like a boundary. Welding at 0.25u cut unrest
from 18,005 raw boundary segments to 7,796, and polyline decimation to 5,013.

Result on unrest: clean, correctly oriented, and **more legible than Brewall's**
— the house interior and the outer walkway read clearly. 5,013 segments vs his
1,815. See `scripts/mapspike/v3_unrest_ours.png` vs `v2_unrest_brewall.png`.

**Outdoor terrain → elevation contours.** Boundary extraction *fails by design*
here: terrain is one continuous mesh, so no interior edge is ever unshared and
the result is an empty rectangle plus the zone rim. gfaydark produced only the
perimeter and Kelethin's platforms.

Slicing the terrain at fixed Z intervals (triangle/plane intersection) instead
produces a topographic map: 14,140 segments at 30u intervals across 31 levels,
showing real relief. See `scripts/mapspike/ct_gfaydark.png`. **Brewall's
gfaydark has no elevation information at all** — this is the clearest concrete
example of beating the existing map sets rather than matching them.

Production pipeline therefore emits **both** layers per zone and lets the
renderer toggle them: structures as boundary polylines, terrain as contours.

### 5b.4 Deep multi-level zones — Akheva and Dragon Necropolis

Both parsed and extracted. Two further corrections came out of this round.

**The name-table CRC is not a constant.** `akheva.s3d` uses `0x61580AC9` — the
*PC-client* value — while `unrest`/`qeynos2`/`gfaydark` use `0xFFFFFFFF`. Both
conventions ship in the same TAKPv22 install, so §5b.1's "Mac uses `0xFFFFFFFF`"
is too simple. The parser must identify the name table **structurally** — the
entry that decodes as a name list of exactly `len(entries) - 1` names — not by
matching either constant. Fixed in `scripts/mapspike/wld.py`.

**Dragon Necropolis is `necropolis`, zone 123** — not `dragonn` / 309 as
Grokii's design doc states. Treat that doc's zone table as unverified.

| Zone | Triangles | Our segments | Brewall | Z span | Level structure |
|---|---|---|---|---|---|
| akheva | 91,633 | 16,260 | 768 | 402u | **Continuous** — no clean bands |
| necropolis | 22,878 | 5,178 | 1,365 | 730u | **Discrete** — clear histogram peaks |

This split is the important result, and it settles the §8.3 design:

- **necropolis** has clear Z-histogram peaks (`-260..-240`, `-200..-180`,
  `0..20`, `280..320`), so offline level detection works and a discrete floor
  picker is viable.
- **akheva** has a flat, continuous Z distribution — ramps and terraces, not
  floors. Automatic level detection would produce arbitrary cuts.
  **Auto-Z fade is therefore not optional**; for zones like this it is the only
  workable depth mode.

Z-banding itself is validated: rendering akheva's `Z 10..90` band alone produces
a clean, readable plan (`scripts/mapspike/band_akheva_10_90.png`) where the
flattened all-levels view is cluttered.

**Design insight from the akheva comparison.** Brewall's akheva (768 segments)
is not a floor plan at all — it is a single simplified *silhouette* of the
walkable area, flattened, with vertical structure discarded entirely. For
at-a-glance navigation it is genuinely more readable than our detailed
extraction. Our version wins on depth and detail; his wins on instant legibility.

The obvious conclusion — generate a silhouette layer and make it the default
view everywhere — turns out to be wrong. See §5b.5.

### 5b.5 Silhouette layer — built, and it only works on half the zones

Implemented in `scripts/mapspike/silhouette.py`: rasterize walkable faces into
an occupancy grid (PIL `ImageDraw.polygon`), morphological close, marching
squares over edge midpoints, chain into polylines, Douglas-Peucker simplify.
All five zones in ~6 seconds total.

| Zone | Grid occupancy | Silhouette segs | Brewall | Verdict |
|---|---|---|---|---|
| akheva | 48.5% | 190 | 768 | **Useful** — clean corridor network |
| necropolis | 46.6% | 1,605 | 1,365 | **Useful** — organic cave boundaries |
| qeynos2 | 74.4% | 261 | 2,137 | Degenerate |
| unrest | 73.4% | 35 | 1,815 | Degenerate |
| gfaydark | 81.1% | 682 | 2,730 | Degenerate |

**The result contradicts the §5b.4 recommendation.** A walkable silhouette is
only meaningful where walkable area is *sparse* — corridors carved through
solid rock. In open-ground zones the walkable region simply **is** the zone
footprint, so the silhouette collapses to the outer perimeter and carries no
information: unrest reduces to a 35-segment blob, gfaydark to a bare rectangle.

Brewall is not applying one technique either. His akheva is a corridor
silhouette; his unrest is a detailed floor plan with walls and the hedge maze.
He picks per zone, and so must we.

Grid occupancy looked like a clean discriminator on these five zones — 46–49%
where silhouette works, 73–81% where it degenerates. **That turned out to be an
artifact of the small sample; see §5b.6.**

Tuning notes for the port: the morphological close is nearly irrelevant
(1–3% occupancy change from 0 to 8 units) — it bridges triangle seams and
nothing more. Douglas-Peucker at ε=1.5 does the real work, collapsing
marching-squares staircases into straight runs (akheva: 9,848 → 190). Pad the
raster by more than the dilation radius or contours clip at the image border
and never close.

### 5b.6 Full-corpus classification — all 178 zones

`scripts/mapspike/classify.py` run across every Quarm zone with client files.
Output: `zone_classification.csv` (gitignored — regenerate locally).

**178 zones parsed, zero failures.** The parser handles the entire client, not
just the sample. That is the strongest single result of the spike.

**Correction to §5b.5: occupancy is not bimodal.** Across 178 zones it is
roughly uniform from 0.1 to 1.0 with no natural gap anywhere. The "clean
separation, no overlap" claim came from a five-zone sample and does not hold.

**Boundary density is the reliable primary signal.** It *is* strongly bimodal:

```
bnd_density   0.00-0.30  ###################################  71 zones
              0.30-0.60  #########                            18 zones   <- valley
              0.60-1.10  ###########################████████  87 zones
```

The sparse valley at 0.30–0.60 is a genuine natural boundary. Revised rule,
occupancy demoted to a secondary split *within* the high-density cluster:

```
if   bnd_density < 0.45          -> contours     (continuous terrain)
elif occupancy   < 0.60          -> silhouette   (sparse corridors / caves)
else                             -> boundary     (discrete floor slabs)
```

Scores 5/5 against the visually-verified zones. Resulting distribution:

| Technique | Zones | Share |
|---|---|---|
| contours | 80 | 45% |
| silhouette | 76 | 43% |
| boundary | 22 | 12% |

45% terrain is plausible for classic EQ. Spot-checks agree on the obvious
cases — `tox`, `sro`, `steamfont`, `stonebrunt`, `timorous`, `trakanon`,
`twilight`, `umbral`, `wakening`, `warslikswood`, `westwastes` all → contours;
`sebilis`, `guktop`, `najena`, `charasis`, `soltemple`, `permafrost`,
`thurgadinb`, `veeshan`, `vexthal` all → silhouette; `unrest`, `qeynos2`,
`mistmoore`, `sseru` → boundary.

**160 of 178 zones (90%) classify confidently.** The 18 in the valley need
visual review before the build is trusted — the classifier is a strong first
pass, not a fully automatic decision:

| Zone | occ | bnd | Rule says | Note |
|---|---|---|---|---|
| gfaydark | 0.81 | 0.32 | contours | verified correct |
| paludal | 0.30 | 0.34 | contours | |
| netherbian | 0.24 | 0.34 | contours | |
| warrens | 0.26 | 0.35 | contours | **likely wrong** — indoor dungeon, low occ suggests silhouette |
| echo | 0.15 | 0.37 | contours | |
| thedeep | 0.31 | 0.38 | contours | |
| powater | 0.16 | 0.38 | contours | |
| arena | 0.78 | 0.38 | contours | **suspect** — a built structure |
| ssratemple | 0.69 | 0.42 | contours | **suspect** — large indoor temple |
| airplane | 0.99 | 0.48 | boundary | |
| potimeb | 0.47 | 0.49 | silhouette | |
| poearthb | 0.43 | 0.49 | silhouette | |
| droga | 0.15 | 0.53 | silhouette | |
| befallen | 0.48 | 0.55 | silhouette | |
| cshome | 0.63 | 0.56 | boundary | |
| frozenshadow | 0.37 | 0.57 | silhouette | |
| potimea | 0.78 | 0.58 | boundary | |
| crushbone | 0.52 | 0.59 | silhouette | |

Plan: render all 18 both ways during Phase 2 and pin the technique per zone in
a small override table, rather than trying to find a threshold that classifies
every zone correctly. A hand-checked override list of ~18 entries is cheaper and
more honest than an over-fitted heuristic.

### 5b.7 Go pipeline — full corpus build (2026-07-29)

`backend/cmd/mapgen` now builds `maps.db` end to end. First full run:

```
178 zones, 0 failed, 1,127,344 segments, 2m21s
   contours     77
   silhouette   78
   boundary     23
wrote data/maps.db (6.3 MB)
```

**Zero failures across the whole client**, matching the Python prototype.

**Size: 6.3 MB, not the 4.0 MB projected in §6.1.** The projection assumed
Brewall-equivalent density; our pipeline emits ~1.13M segments against his
637k, so per-segment cost is as predicted and the delta is purely extra detail.
Fetch is 0.74 ms per zone including decompression — slower than the 0.05 ms
measured in Python because that figure excluded unpacking, but far below
anything a user notices.

Two bugs surfaced only because the Go output was compared against the
prototype's numbers:

- **Contours were shipping unsimplified.** Boundary and silhouette were both
  simplified; contours went straight from triangle-plane intersection to
  output. Adding Douglas-Peucker at ε=0.5 cut the corpus from 1.94M segments to
  1.13M and the file from 10.9 MB to 6.3 MB, with sub-pixel error.
- **Douglas-Peucker had a slice-aliasing bug.** The two recursive halves are
  sub-slices of one backing array, and the base case returns the input slice
  itself, so appending into the left half overwrote points the right half still
  referenced. The tell was impossible output: a *larger* epsilon produced
  *more* segments. Both halves are now copied into a fresh slice, and a
  monotonicity test guards it.

Neither would have been caught by "does it compile and produce a map" — the
maps looked fine in both cases.

### 5b.8 Valley-zone review — the override list was the wrong fix

All 18 valley zones rendered three ways with `mapgen -compare` and inspected.
The finding was not "three overrides were right" but that most of the valley was
one misdiagnosed pattern.

**The three existing overrides were all correct**, and for the reason expected:

| Zone | Contours | Silhouette | Boundary | Verdict |
|---|---|---|---|---|
| arena | scattered fragments | bare rectangle | ring, alcoves, tunnel | boundary ✓ |
| ssratemple | a few wall rings | blob outline | full temple floor plan | boundary ✓ |
| warrens | 298 fragments | clean corridors | detailed corridors | not contours ✓ |

**But five more zones were misclassified the same way.** `echo`, `powater`,
`netherbian`, `paludal` and `thedeep` all took the terrain branch on boundary
density alone, and on every one contours came out fragmentary or dashed while
the silhouette was clean. They share a signature: **occupancy at or below 0.30**
— walkable area covering less than a third of the footprint, which no open
terrain zone does.

That is a rule gap, not five more names for the override table. Added a sparse
pre-check ahead of the density test:

```
if   occupancy < 0.35 && bnd_density >= 0.30  -> silhouette   (corridor / cave network)
elif bnd_density < 0.45                       -> contours     (continuous terrain)
elif occupancy   < 0.60                       -> silhouette
else                                          -> boundary
```

The `bnd_density >= 0.30` floor is doing real work. `fungusgrove` is the one
low-occupancy zone whose contours render continuous and genuinely informative,
and its density (0.24) sits well below the others (0.34–0.38) — a more
continuous mesh, not a corridor network. The floor keeps it on contours without
needing an entry in the table.

Corpus effect: exactly five zones flipped contours → silhouette, `fungusgrove`
held, and **`warrens` no longer needs an override** — the rule now classifies it
directly. Nothing else moved.

| | before | after |
|---|---|---|
| contours | 77 | 72 |
| silhouette | 78 | 83 |
| boundary | 23 | 23 |
| overrides | 3 | **2** |

Rendering was the only way to find this. Every one of these zones produced
plausible numbers and a map that looked like *something*.

### 5b.9 POI generation — built

`mapgen` now emits `map_poi` alongside the geometry. 4,723 POIs across eight
categories, all `source="db:*"`:

| Category | Rows | Source |
|---|---|---|
| vendor | 1,958 | `spawn2` + `npc_types.merchant_id > 0` |
| door | 916 | `doors`, keyed or leading somewhere |
| zone_line | 492 | `zone_points` |
| ground_spawn | 467 | `ground_spawns` |
| raid_target | 351 | `npc_types.raid_target = 1` |
| tradeskill | 284 | `object`, named via the existing bagtype enum |
| succor | 172 | `zone.safe_x/y/z` |
| trap | 83 | `traps` |

Three decisions worth recording:

**No generic "named NPC" category.** The obvious heuristic — any name without a
leading article — matches 9,018 NPCs, overwhelmingly guards and merchants. That
would bury the map. `raid_target` is the server's own designation and gives 351
pins that are actually worth finding. A curated notable-NPC layer can come later
from `source='community'`.

**Doors filtered to keyed-or-destination.** All 8,205 would be noise; most are
ordinary interior doors. 916 are locked or lead somewhere.

**Tradeskill container names reuse `internal/db/enums.ContainerTypeName`**
rather than a second copy of the bagtype table that could drift from it.

Validation: **99.9% of POIs fall inside their zone's geometry bounds**, and the
Priest of Discord lands at map `(-235, -6)` from game `(235, 6)` — the same
anchor the in-game `/map marker` check confirmed. That cross-check matters
because POIs reach map space from `quarm.db` game coordinates by a different
route than geometry does from client mesh coordinates; if the two diverged every
pin would be silently misplaced with nothing looking wrong. It is now a test.

`map_poi` writes with `INSERT OR IGNORE` against its UNIQUE key, because several
sources legitimately produce duplicate pins on one spot — a spawngroup listing
the same NPC twice, or a zone line recorded from both sides.

Phase 2 is complete: geometry, three extractors, classifier, POIs and the
`maps.db` writer. Final artifact is 6.9 MB.

### 5b.10 Effect on the risk register

The §11 "WLD/`.s3d` Go port is the long pole" risk drops from **High to
Medium**. A complete working parser — PFS, WLD, `0x36` mesh, `0x15` placements,
welding, boundary extraction, contouring — is ~120 lines of Python and relies on
no exotic fragment types. The Go port is a translation exercise, not research.

---

## 6. Storage: `maps.db` schema — measured, not estimated

### 6.1 The obvious schema is a trap

Both schemas were built against the full 636,952-segment corpus and measured:

| Storage | Size |
|---|---|
| Raw text files (baseline) | 41.2 MB |
| **Schema A** — one row per line segment | **34.7 MB** |
| **Schema B** — packed blob per (zone, layer) | **4.0 MB** |

Row-per-segment barely beats plain text: SQLite's per-row overhead swamps a
payload of nine small integers. It only pays off if spatial queries are needed,
and they are not — rendering always wants *every* segment in a zone layer.

### 6.2 Recommended schema

```sql
CREATE TABLE map_layer (
  zone    TEXT NOT NULL,      -- zone short_name
  layer   INTEGER NOT NULL,   -- 0 = geometry, 1..n = overlays
  nlines  INTEGER NOT NULL,
  lines   BLOB NOT NULL,      -- packed <6h3B> per segment (x1,y1,z1,x2,y2,z2,r,g,b)
  pois    BLOB,               -- packed <3h3BB> + label bytes
  PRIMARY KEY (zone, layer)
);

CREATE TABLE map_zone (        -- render metadata, one row per zone
  zone     TEXT PRIMARY KEY,
  min_x, min_y, max_x, max_y  INTEGER,   -- bounds, for fit-to-zone
  levels   BLOB                          -- detected Z bands (see §8.3)
);
```

Measured: **0.05 ms** to fetch and zlib-decompress a zone layer. One primary-key
lookup, no join. The blob can be handed to the frontend as an `ArrayBuffer`
without ever becoming JSON.

**Precision.** Coordinates span `-11147 .. 23621`, so `int16` fits at 1-unit
resolution. 19.6% of source coords carry sub-unit precision, giving worst-case
0.5-unit error — roughly a quarter pixel at typical zoom. Invisible.

### 6.3 Packaging

`maps.db` must **not** live in `quarm.db`. Per project convention `quarm.db` is
regenerated from MySQL dumps by `data-release.yml` and never hand-edited — map
data is our own derived asset on a different release cadence and would be wiped
on the next data release.

Ship it as a sibling; one line in `electron-builder.yml`:

```yaml
extraResources:
  - from: backend/data/quarm.db
    to: bin/data/quarm.db
  - from: backend/data/maps.db      # new
    to: bin/data/maps.db
```

Opened read-only alongside `quarm.db` in `backend/internal/db`.

---

## 7. POI taxonomy and provenance

### 7.1 Category audit — what the DB already gives us

Categorised from the reference corpus' `_1` POI layer, each mapped to our schema:

| POI type | Reference count | Our source | Rows |
|---|---|---|---|
| `to_<zone>` zone lines | 1,670 | `zone_points` | 500 |
| `GS:` ground spawns | 697 | `ground_spawns` | 514 |
| `TRAP` markers | 387 | `traps` (authoritative) | 83 |
| Forge / Oven / Kiln / Pottery | ~340 | `object` (typed + icon) | 284 |
| `Succor` | 247 | `zone.safe_x/y/z` | every zone |
| Named NPCs / camps | ~1,400 | `spawn2` + `npc_types` | all |
| Doors, locked doors | — | `doors` | 8,205 |
| **Patrol routes** | **none** | **`grid_entries`** | **711,517** |

Reference counts span all 568 zones including post-PoP; ours cover Quarm's 192.
Per-zone parity is therefore better than the raw numbers suggest. On several
categories we are strictly more authoritative — `traps` is the server's actual
trap definition table rather than hand-placed guesses, and `object` gives typed
tradeskill containers rather than eyeballed labels.

`grid_entries` is a category no existing map set has at all.

### 7.2 What the DB does *not* have

Experiential knowledge that exists in no table, and which is exactly what the
user asked to be captured:

- `Fake_Wall`, `TRAP: Fake Floor`, illusory geometry
- Player-vernacular camp names ("Seafury camp", "the pit")
- Safe-AFK spots, pull spots, common CR paths
- Judgement about which geometry matters and which is noise

These are the minority gap-fill cases where Brewall's research is genuinely
irreplaceable, and they must be represented in our data for the feature to be
complete.

### 7.3 Provenance is a schema requirement, not a footnote

Because a minority of POI rows are Brewall-derived while the majority are
DB-derived, **every POI row carries its source**:

```sql
CREATE TABLE map_poi (
  id       INTEGER PRIMARY KEY,
  zone     TEXT NOT NULL,
  x, y, z  INTEGER NOT NULL,
  category TEXT NOT NULL,    -- trap | ground_spawn | zone_line | vendor | door
                             -- tradeskill | succor | named | hazard | camp | note
  label    TEXT NOT NULL,
  source   TEXT NOT NULL,    -- 'db:traps' | 'db:spawn2' | 'geometry'
                             -- 'brewall' | 'community' | 'manual'
  ref_id   INTEGER,          -- FK into the originating quarm.db table where applicable
  UNIQUE (zone, x, y, category, label)
);
```

The `source` column is doing real work:

- **Auditable attribution** — we can state exactly how much is Brewall-derived
  rather than guessing.
- **Reversibility** — if permission is declined, `DELETE FROM map_poi WHERE
  source='brewall'` strips the dependency and the map still stands on its
  DB-derived majority.
- **Regeneration safety** — `db:*` rows are rebuilt from scratch on each
  `quarm.db` data release; `brewall`/`community`/`manual` rows are preserved.
  Without this distinction a data release would silently destroy the hand
  research.
- **Community contribution path** — `source='community'` is how player-submitted
  camp names and hazards grow the set over time, which is the long-term route to
  surpassing hand-curated maps rather than merely matching them.

> **Honest note.** Systematically extracting Brewall's annotation set is still
> using his research, even as a minority of rows. Individual facts are not
> copyrightable and the volume here is small, but this is precisely why §5.4
> matters: with his blessing the question disappears entirely. Attribute him in
> the UI regardless.

---

## 8. UI surfaces

Three places maps appear, per the 2026-07-28 direction.

### 8.1 Zones tab — zone map

- Full-zone map with all POI layers available.
- Layer toggles (§8.4) and depth control (§8.3).
- Spawn list cross-linked to the map: hover a row → highlight the pin; click a
  pin → scroll to the row.
- "Show this zone in game" → `/map show_zone <shortname>` to clipboard.

### 8.2 NPC tab — locate this NPC

Answers HughJeffner's request end-to-end.

- Map centred on the NPC's spawn point(s), pin highlighted, rest of the zone
  dimmed for context.
- Multiple `spawn2` rows → spawn-point picker; show all pins at once with an
  index.
- **"Copy Zeal marker"** → `/map marker <y> <x> <name>` to clipboard, per §4.
- **"Share with raid"** → `/map rsay marker …`.
- Patrol path drawn from `grid_entries` when the NPC roams, so the pin is
  honestly labelled as a spawn point rather than a live position.
- Reached from the item page: "purchased from" → vendor → this view. That closes
  the original vendor-location loop.

### 8.3 Maps tab — full standalone viewer

The dedicated exploration surface.

- Zone picker with search; recent/favourite zones.
- Pan, zoom, fit-to-zone, reset. Zoom to cursor.
- **Depth / Z levels**, two complementary modes mirroring Zeal:
  - *Auto-Z fade* — geometry within ±N units of the focus height at full
    opacity, everything else faded. Port Zeal's `autoz` model; N adjustable.
  - *Discrete floor picker* — levels detected offline by Z-histogram clustering
    and stored in `map_zone.levels`, so the UI just offers a list.
- **Label density** — off / summary / all, matching Zeal's vocabulary so the two
  feel like one product.
- POI layer toggles (§8.4).
- Click a POI → detail popover → deep-link to the NPC/item page, plus
  "copy Zeal marker". Use the existing `lib/overlayNav` pattern for any
  overlay→main-window navigation (`project_entity_linking_overlay_nav`).
- Distance/ruler tool between two points.
- Live player position when ZealPipes is connected (§9).

### 8.4 POI layer toggles

One toggle per `map_poi.category`, persisted per user:

`traps` · `ground spawns` · `zone lines` · `vendors` · `doors` ·
`tradeskill containers` · `succor` · `named / rare spawns` · `hazards` ·
`camps` · `patrol routes`

Defaults on: zone lines, vendors, named spawns, traps. Everything else off, to
avoid the clutter that makes dense maps unreadable.

---

## 9. What makes this better than PQDI, not just equal to it

1. **Live player position.** `backend/internal/zealpipe/events.go` already
   streams `Location{X,Y,Z}`, `Heading`, and `Zone`. A "you are here" arrow with
   facing — or a transparent always-on-top map overlay — is something neither
   PQDI nor Zeal's small in-game window offers.
2. **A two-way in-game bridge.** PQDI can show a coordinate; we can put a pin on
   the player's actual in-game map and share it with their raid.
3. **DB-driven overlays.** Nobody generates `map_files/` content from the live
   game database.
4. **Patrol routes.** 711,517 waypoints in `grid_entries`, which no existing map
   set draws.
5. **Cleaner cartography.** Naive edge-dumping gives wireframe noise — that is
   what makes auto-generated maps look bad. Walkable-surface extraction (keep
   faces whose normal points up, trace the boundary) and Z-slicing into contours
   instead yield architectural-style floor plans, with consistent line weights
   and real theming. This is where "nicer looking than Brewall" actually comes
   from, and it is only achievable by *not* copying him.

---

## 10. Phased plan

### Phase 1a — Table UX fixes (no maps involved)
Answers the first half of HughJeffner's request with no dependencies.
- Zebra striping + sortable column headers on item "purchased from" / "drops
  from" tables; sortable by zone; fix wide-window readability.
- **Vendor** tab on the NPC page: what they sell, base price, greedy flag.

### Phase 1b — Zeal bridge (no map assets, no licensing dependency)
- "Copy Zeal marker" on NPC pages → `/map marker <y> <x> <name>`.
- Spawn-point picker for multi-`spawn2` NPCs.
- "Show zone in game" → `/map show_zone <shortname>`.
- "Share with raid" → `/map rsay marker …`.
- Reuse `eqClipboard.ts`; one command per copy.
- **Zeal map config sync** — Grokii's ask: push a map config block to *all*
  characters' `UI_<name>_pq.ini` at once, since `/map save_ini` is per-character.
  `eqconfig` and the backup manager already read/write these files.

*Gate: verify the `/map marker` argument convention in game (§4).*

### Phase 2 — Map pipeline + `maps.db`
Offline, build-time. The largest single chunk of work.
- Port a WLD/`.s3d` reader to Go from LanternExtractor (MIT). No Go library
  exists — this is the bulk of the effort, but the format is well documented.
- Walkable-surface extraction + Z-slice contouring → line segments.
- Z-histogram level detection → `map_zone.levels`.
- POI generation from `quarm.db` per §7.1, with `source='db:*'`.
- Emit `maps.db` per §6.2; add to `extraResources`.
- Validation harness: render every zone, diff against the reference corpus for
  gross errors (missing zones, inverted axes, empty layers).

### Phase 3 — Renderer + the three UI surfaces
- Go: `backend/internal/api/maps.go`, serving packed blobs.
- React: canvas or SVG renderer with pan/zoom, auto-Z fade, discrete levels,
  label density, layer toggles.
- Zones tab (§8.1), NPC tab (§8.2), Maps tab (§8.3).

### Phases 1a–3 — status

Done. 1b shipped without the Zeal map-config sync: that config turned out to
live in `zeal.ini [Zeal]`, which is already global, so Grokii's per-character
ask was unnecessary (`project_zeal_map_settings_are_global`).

Phase 3 deliberately substituted the **Z-fade window** for the `map_zone.levels`
floor picker, per the §11 fallback. Zones like Akheva have continuous ramps
rather than discrete storeys, so any floor list is arbitrary cuts. The `levels`
column was never added and is not wanted. See §13 for the readability iteration
that followed (outline mode, height tinting, real Z bounds).

### Phase 3c — Release plumbing ✅ Done, except the first upload

Not a feature. `maps.db` is gitignored and exists only on the machine that ran
`cmd/mapgen`, and unlike `quarm.db` nothing gates on its presence:

- `release.yml` downloads `quarm.db` from the `data-latest` release and
  **hard-fails** if it is missing. It says nothing about `maps.db`, so that path
  would build and publish a mapless installer with no error — and the app
  degrades silently, hiding the Map tab rather than complaining.
- The local `npm run dist:win` path (the real one — `/newrelease` publishes
  locally, `project_newrelease_ci_publishes`) happens to work because the file
  is sitting there, but nothing verifies it is present or current.

Work:
- ✅ `scripts/verify-maps-db.cjs`, wired into `dist`/`dist:win`: fails the build
  if `maps.db` is absent, under 1 MB (a truncated file passes an existence check
  and fails at runtime), or older than `internal/mapgen/`.
- ✅ `release.yml` downloads `maps.db` from `data-latest` with the same hard-fail
  as `quarm.db`; `data-release.yml` documents why it clobbers `quarm.db` by name
  rather than by wildcard.
- ⬜ **The first upload still has to happen** — it is the one step that cannot be
  done from a machine without an EQ client, and it publishes a ~14 MB asset:

      gh release upload data-latest backend/data/maps.db --clobber

  Only needed before `release.yml` is ever used. The local `/newrelease` path
  works without it, because the file is already on the machine that generates it.

Auto-update needs no work — verified by reading the path rather than assuming:
`extraResources` land in the install dir's `resources/`, electron-updater runs
the full NSIS installer, and `installer.nsh`'s cleanup is gated on
`${isUpdated}` so it touches user data only on a real uninstall, never app
files. An updating user gets the new `maps.db` the same way they get the new
`.exe`.

### Phase 4 — Annotation layer (4a/4b/4c done; research corpus awaiting content)

The gap-fill pass, reframed. Original plan was to import Brewall's annotations
under `source='brewall'`. Two things changed:

1. **Far more is derivable than assumed.** The trap pass already produced 819
   markers from `spawn2` invisible NPCs — annotations *no existing map set
   carries*, including Brewall's. The `doors` table has more of the same waiting:
   `keyitem`/`altkeyitem`, `lockpick`, `triggerdoor`/`triggertype`, `islift`,
   `dest_*`. Deriving beats importing — it is verifiable, regenerable, and
   carries detail a hand annotation cannot (the actual *name* of the key).
2. **The facts are not the drawing.** "There is a floor trap here" and "this wall
   is fake" are observations about the game, published across Brewall, PQDI and
   several map packs. The map *drawing* is Brewall's work; the facts are not. So
   these are safe to record — the line to stay on is not bulk-lifting his
   annotation file as a dataset, but entering facts verified against the game or
   multiple public sources. Same standing as the traps already shipped.

- **4a — Derive from `doors`. ✅ Done.** Three new categories out of 8,205 door
  rows: `locked` (173 — key doors name the actual key and link to its item page;
  pick-only doors state the lockpicking skill), `teleport` (387 in-zone ports,
  which no static map set records), `switch` (29 levers and lifts). `door` itself
  narrows to 356 cross-zone exits from 885.

  The one trap here was `triggerdoor`, which looks like the signal for "hidden
  lever" and is not: 667 rows carry it, but it only means "opening this opens
  that", and Paineel's `PADOOR101`/`PADOOR102` alone account for 372 of them as
  ordinary double doors wired to their own other half. Only ~15 are named like
  actual controls. Pinning all 667 would have buried the real levers in
  door-pair noise, so switches are name-filtered plus `islift`.
- **4b — Hand-researched set. Pipeline done, corpus empty.**
  `internal/mapgen/annotations.json` is the authoring format — game coordinates,
  three categories (`wall`, `hazard`, `note`), compiled in with
  `source='research'`. Zone names are checked against `quarm.db`, because a typo
  would otherwise silently drop rows somebody researched by hand, which is the
  worst possible failure for this data. Every row **requires** an `evidence`
  string and the loader errors without one, so the standard — verified in game,
  or corroborated by two independent public sources — is enforced rather than
  merely documented.

  Before writing it I checked whether any of this was derivable after all: no
  door name in `quarm.db` contains WALL, SECRET, HIDDEN or ILLUS, and
  `zone.underworld` is a plane rather than a point. So fake walls genuinely are
  not in the data, which is exactly why they are worth recording.

  **The corpus itself is empty on purpose.** Populating it is research, and
  inventing coordinates from memory would produce confident-looking markers that
  send people into walls that are not there — worse than a blank map.
- **4c — User annotations + submission path. Done.** Markers the user places
  themselves, in `user.db` (never `maps.db`, which every app update replaces
  wholesale). Placed by clicking the map, edited and deleted through the same POI
  inspector every other pin uses, and drawn through the normal pin pipeline so
  layer toggles, depth fade, labels and search all work on them for free.

  The submission path is the export: it emits the **exact shape**
  `annotations.json` reads, so the distance between "I marked this on my map" and
  "every player gets this marked" is a pull request rather than a conversion. Two
  properties make that safe, both tested:
  - Coordinates negate back to game space. The two halves live in different
    packages with no shared code, which is where a sign error hides — and it
    would hide quietly, since a mirrored submission still parses and still draws.
  - `evidence` exports **blank**, and the loader rejects blank. A raw export
    cannot be merged without a human writing down where the fact came from.

The provenance guarantee is already built and is what makes all of this safe:
regeneration rewrites `db:*` rows only (§7.3), so research and community rows
survive a data release untouched.

### Phase 5 — Live position (done)

"You are here" on our maps. `ZealPipes` already sends what is needed —
`Player{Location{X,Y,Z}, Heading}` per tick (`internal/zealpipe/events.go`) — and
`ZoneMap` already draws a cased heading arrow from a `playerPos` prop that
nothing currently passes. The work is wiring, not new capability.

- **5a — In-app. Done.** `internal/playerpos` broadcasts `player:position` on the
  WebSocket; `usePlayerPosition` consumes it. The arrow draws only when the
  position's zone matches the map on screen — a position from elsewhere would
  land at plausible coordinates on the wrong map, which is worse than nothing.
  "Follow me" recentres the view, off by default since it takes pan away from
  you. Rather than auto-switching zones, the Maps page offers a "You are in
  &lt;zone&gt;" button when the player is elsewhere: switching the zone out from
  under someone reading a map is not an improvement.

  Two things decided the design:
  - **The heartbeat is load-bearing.** Broadcasting only on movement makes
    standing still indistinguishable from a dead pipe, and the renderer's
    staleness timeout would blank the arrow exactly when a player stops to fight
    something. The tracker re-sends every 2s regardless; the renderer trusts a
    position for 6s.
  - **Staleness lives in the renderer**, because the pipe-connected gate has none
    of its own (`project_npc_overlay_pipe_stall_no_fallback`), and a frozen arrow
    is worse than no arrow — it still looks authoritative.
- **5b — Auto-depth. Done.** The Z window centres on the player's own height,
  ±max(50, z_span/12). This is the payoff of the §13.3 work: standing in a
  Necropolis tunnel, the map shows your level with no input at all.

  Manual always wins and never silently — dragging a thumb takes over, the AUTO
  badge dismisses it, and Reset returns to whatever the default is for the
  situation (the followed window with a live position, all levels without one).
  Making the badge dismissible turned out to require Reset being enabled in that
  state too, or dismissing auto was a one-way door.
- **5c — Map overlay window. Done.** Transparent, always-on-top, frameless, on
  the existing overlay infrastructure — dashboard panel *and* popped-out window
  per `feedback_overlay_dashboard_pattern`, registered in `OVERLAY_DEFS`,
  `dashboardLayout`, Settings and the popout-selection paths like every other
  overlay. Defaults to `clickthrough` locked mode: it follows the player, so
  there is nothing to scroll or clear and no reason for the map body to capture
  the mouse mid-fight.

  Both overlay surfaces render `chromeless`, which strips the technique badge,
  Reset view, height legend and depth control. At 384px square that chrome costs
  a third of the width and clips at the edges, and it is redundant on a surface
  that follows the player automatically — there is nothing to reset and no window
  to set by hand. Full controls stay on the Live Map tab.

### Phase 5d — Live Map tab (added 2026-07-30)

Not in the original plan; asked for once live position existed, and it earns its
own nav entry for a reason that is not cosmetic.

The Zones tab and the Maps page are **browsing** surfaces, and a browsing surface
must not move under you — so they deliberately do *not* follow the player between
zones. The Maps page offers a "You are in \<zone\>" button and leaves the choice
to you. On a live map, following is the entire point.

So `/live-map` (under Parsing, not Database — it is a pipe-driven real-time
surface like Log Feed): zone tracks the game, follow-me and auto-depth default
on, and POI search is enabled so "where is the vendor / the key door / the traps"
is one query away from a pin. Search results feed the existing POI inspector
rather than growing a second path to the same actions.

`useLiveZone` makes the zone **sticky**: once seen it is held even after the
position goes away, because zoning takes several seconds during which the pipe is
silent, and a live map that blanked itself on every zone would spend those
seconds showing nothing and then rebuild from scratch. A Live/Last known badge
says which state it is in, so a stale map never passes for a live one.

Constraint: Windows + Zeal only, and the pipe has no staleness detection
(`project_npc_overlay_pipe_stall_no_fallback`), so a dead pipe must read as "no
position" rather than freezing the arrow somewhere misleading.

### Phase 6 — Export our POIs to the in-game map (built 2026-07-30)

Grokii's original ask, fully realised, and a different thing from the existing
"Pin in game" button — worth being precise, because they look similar:

| | Pin in game (shipped) | Phase 6 |
|---|---|---|
| Scope | one marker | every marker we have for the zone |
| Effort | copy, alt-tab, paste, repeat | write files once |
| Lifetime | until you clear the map | persists, toggleable in game |
| Limit | one command per paste (`project_eq_clipboard_cap`) | none |

So: Akheva's 129 trap markers become 129 pins on the in-game map instead of 129
copy-paste cycles.

- **6a/6b — Done.** `internal/mapexport` writes P-line files into
  `<EQPath>/map_files`, exposed as Settings → In-Game Map Markers with per-category
  toggles. Defaults to the categories no existing map set carries (traps, locked
  doors, teleports, switches, plus the user's own markers); vendors, zone lines
  and raid targets are available but off, since packs already mark those. The UI
  prompts for `/map data_mode both` after writing, without which the export
  appears to do nothing.

  Measured on the real corpus: 1,398 markers across 94 zones.

  **The safety property is the design.** Ownership is tracked by content hash in
  `~/.pq-companion/map_export.json` — outside `map_files`, because the format
  permits no comments, so a marker line would fail the parse and a base file that
  fails to parse disables external data for the whole zone. From that:
  - With a foreign pack present, we append at the first free contiguous slot.
    Verified against a fixture with a foreign base + `_1`: we took `_2` and left
    both of theirs byte-identical.
  - Remove deletes only files whose contents still match what we wrote. Verified
    by overwriting one of ours before removing: 93 deleted, that one kept, the
    foreign files untouched.
  - A corrupt manifest authorises nothing, rather than falling back to
    path-matching and deleting somebody's map pack.
- **6c — Patrol routes** from `grid_entries` as line data, still to do. Something
  no static map pack has, since patrol paths are server data.

### 6.1 The clobber question — answered (2026-07-30)

Resolved from Zeal's own source and README rather than by experiment, since it is
MIT and public. The answer changes the design, so it was worth doing first.

**Slots:** `map_files/<zone>.txt` plus `_1` through `_10` — eleven files per zone.

**They must be contiguous**, which is the part that matters and the part I would
have got wrong by assuming:

> "The previous optional file must exist (ie: `map_files/commons_2.txt` must
> exist) before it will check for the following one"

So "write to a high slot to stay out of Brewall's way" does not work: dropping
`commons_5.txt` next to a pack that ends at `_2` means Zeal never reads it, and
it fails *silently*. The correct strategy is to **append at the first free slot
in the sequence** — scan for the highest contiguous `_N` and write `_N+1`. That
clobbers nothing and is guaranteed to load.

**Format**, from `add_map_data_from_file`:

```
L x0, y0, z0, x1, y1, z1, r, g, b
P x,  y,  z,  r, g, b, dummy, label
```

Same format as the reference corpus (§2), and the same map-space coordinates our
`map_poi` already stores (§3) — so writing is direct, with no transform.

**Constraints to respect:**
- Label buffer is 64 bytes; truncate at 63. Our longest collapsed trap labels run
  ~35 characters, so this bites only on the longest vendor names.
- RGB is cast to `uint8_t`, so clamp to 0-255.
- `data_mode` must be `both` (adds to Zeal's internal geometry) or `external`
  (replaces it). `both` is what we want.
- Files reload on zone change, so no manual reload command is needed.

**Still unverified:** the source mentions `kMaxNonAllyTriangles = 500` around the
vertex buffer. Whether that caps drawn map lines is unclear from reading alone,
and Akheva would ship 129 P-lines plus whatever geometry — so a large file needs
an in-game check before this is trusted.

**The base file is mandatory.** Checked against the loading loop, because the
README's wording left it open and the answer changes the design:

```cpp
std::string filename = "map_files/" + short_name + ".txt";
if (!add_map_data_from_file(filename, *new_map)) {
  map_data_cache[zone_id] = nullptr;
  return internal_map;          // never checks _1 at all
}
```

So we cannot politely take `_1` and leave `<zone>.txt` free for a future Brewall
install. On an install with no `map_files/` — which is what a plain Quarm + Zeal
setup looks like, confirmed against a real one — writing only `_1` would be read
by nothing.

Two consequences:

1. **Slot strategy depends on what is already there.** With an existing pack,
   append at the first free contiguous slot and clobber nothing. With no pack, we
   must own `<zone>.txt`, so write a recognisable marker line into it — if a
   Brewall installer later overwrites it, we need to be able to tell "replaced"
   from "the user deleted it".
2. **A malformed base file disables external maps for that zone entirely**, since
   a failed parse returns `internal_map` and skips every numbered file. Whatever
   we write has to be valid, and validated before it lands.

**We do not need to ship geometry.** In `data_mode both` Zeal draws its internal
geometry *and* the external file, so the base file can contain only our `P`
markers — traps, locked doors, teleports — with no `L` lines at all. That makes
the export small, avoids drawing a second copy of the zone outline over Zeal's
own, and sidesteps any line-count limit. Geometry export stays possible later for
`data_mode external`, but it is not what makes this feature useful.

**Still required before writing a byte into anyone's EQ directory:** back up
`map_files/` through the existing config backup manager, and record which files
we wrote so removing them is exact rather than a guess.

---

## 11. Risks

| Risk | Severity | Mitigation |
|---|---|---|
| WLD/`.s3d` Go port is the long pole | ~~High~~ **Medium** | **Retired by the §5b spike** — working parser is ~120 lines, no exotic fragments. Translation, not research |
| Auto-generated geometry looks worse than hand-curated | ~~High~~ **Low** | **Retired by §5b** — unrest more legible than Brewall's, gfaydark contours add elevation he lacks, akheva/necropolis both extract cleanly under Z-banding. Add a simplified-silhouette layer for at-a-glance parity (§5b.4) |
| Zones with continuous vertical structure (akheva) defeat discrete floor pickers | Medium | Auto-Z fade is mandatory, not optional (§5b.4). Detect per zone offline whether histogram peaks are clean enough for a floor picker; fall back to fade |
| Coordinate errors that bounds-checking cannot detect | Medium | Symmetric zones hide sign flips (§5b.2). Every zone gets a visual diff against the reference corpus in the validation harness |
| `quarm.db` data release wipes hand-researched POIs | **High** | `map_poi.source` — regenerate `db:*` rows only (§7.3) |
| Brewall gap-fill attribution / permission | Medium | Ask (§5.4); provenance column makes the dependency strippable |
| `/map marker` arg order or sign wrong | Low | 30-second in-game test (§4) |
| Installer size +4 MB | Low | Measured; packed blobs already compressed |
| Multi-level zones render as tangle | Medium | Auto-Z fade + offline level detection (§8.3) |
| `spawn2` is a spawn *point*, not a live location | Medium | Label pins as spawn points; draw patrol paths |
| One spawn point → many NPCs via `spawngroup` | Low | Already handled elsewhere; show the group |
| Multi-line clipboard paste collapses in EQ | Low | Known (`project_eq_clipboard_cap`); one command per copy |
| Client geometry version drift (Mac/TAKP vs PC) | Medium | Generate from a TAKP-era client; validate against reference corpus |

---

## 12. Recommendation

1. Ship **Phase 1a** immediately — small, self-contained, real complaint, no
   dependencies.
2. Run the `/map marker` verification, then ship **Phase 1b**. That delivers
   "locate this NPC in game" — the headline ask from both requests — without a
   single byte of map data.
3. **Prototype one zone end-to-end before committing to Phase 2.** Extract
   `unrest` geometry, contour it, render it, and compare against the reference.
   That single spike de-risks the largest item in §11 and answers the only
   question that really matters: does our output actually look better?
4. Open the Brewall conversation in parallel (§5.4) — it is free to start and
   only gets more useful.
5. Phases 3–6 in order.

## 13. Outline mode — the readability pass (2026-07-29)

First real user reaction to the shipped renderer: some zones read well (Sanctus
Seru, North Qeynos) and others read as an "MRI echolocation depth map" that is
interesting but not usable for pathing (Fungus Grove, Maiden's Eye, Akheva).
Brewall's equivalents are less detailed and far easier to read.

### 13.1 Diagnosis

Two distinct causes, and neither is data quality:

1. **Three techniques means three visual languages.** The classifier picks
   whichever extractor describes a zone's shape best, which is right for
   fidelity and wrong for consistency. Contour zones look like a topographic
   survey, silhouette zones like a stack of tracings, boundary zones like a
   floor plan. Moving between zones feels like moving between three apps.
2. **Every Z level is drawn at once.** The detailed layers band at ~40 units
   (one storey) and composite all of them. Fungus Grove is 14 bands
   superimposed. Brewall draws essentially one plan with a couple of annotated
   upper levels, which is why his stays legible at full Z range.

Brewall's advantage is **abstraction, not information**. He drew what a player
needs — the walls that constrain movement, at the level you walk on — and left
out everything else. Nothing about that requires his data.

### 13.2 The outline layer (layer 2)

A third layer per zone, in one visual language for every zone in the game:

- **Never contours.** Elevation hatching is the single largest source of noise;
  those lines are not walls and cannot be walked along.
- **Coarse Z bands** — 160 units, max 4, against the detailed layer's 40/14.
- **Harder simplification** — RDP ε=4.0 vs 0.5–2.0, which removes the
  marching-squares ripple that made our lines look furry next to a drawn map.
- **Small-chain culling** — any chain whose bounding-box diagonal is under 24
  world units is dropped. These are the confetti (single ledges, mesh slivers,
  one-cell islands) a cartographer would not draw.
- **One neutral colour** for every zone, since in this mode every line means the
  same thing: an edge you cannot walk through.

Technique split is by occupancy alone — does a flat footprint of the walkable
area mean anything? Below 0.50, yes (caves, dungeons): coarse silhouette. Above,
no (open terrain, built interiors): boundary extraction.

Measured over the corpus: +495k segments, `maps.db` 10.2 → 13.5 MB. Per zone,
outline is far smaller than the layer it replaces on screen — Fungus Grove
5,315 → 2,487, Necropolis 15,080 → 1,743, Akheva 6,423 → 1,300.

Outline is the **default**; Detailed is one toggle away and unchanged, because
the dense view is genuinely liked and does carry more information. Layers are
fetched on demand, so the common path downloads only layer 2.

### 13.3 Depth control: a real Z range

Reworked from focus±thickness to an explicit floor/ceiling pair on a dual-thumb
track, matching PQDI and the in-game slider. It reads directly as "show me
between these heights" and can express an asymmetric window, which a
centre-and-width control cannot.

This surfaced a **latent bug that made the control useless in most zones**:
`map_zone` stored only `z_span` (a width), so the slider synthesised its range
as symmetric about zero. Real Z ranges are nowhere near symmetric — Fungus Grove
is −495..66, Necropolis −309..369, South Qeynos −308..84. In Fungus Grove the
old slider offered −298..298, so most of the zone was unreachable by the control
and its default focus of 0 sat above the zone's ceiling. Fixed by storing
`min_z`/`max_z`, computed from the drawn segments across every layer rather than
from the raw mesh — the slider filters what is on screen, so its range must
match what is on screen.

Note the interaction with 13.2: depth filtering is much finer in Detailed mode
(up to 14 bands) than Outline (4). Outline trades depth granularity for
legibility, which is the right trade for its purpose but worth knowing.

### 13.4 Height colouring

Brewall colours lines by level — the tunnels beneath Fungus Grove and the temple
above them are different colours, which is why two lines crossing on his map are
obviously not at the same place. Confirmed from the PQDI screenshots: the purple
network appears and disappears as the Z slider moves while the black lines stay.

We can derive this rather than hand-author it, since every segment carries its
height. Three decisions that mattered:

1. **Sequential ramp, never a rainbow.** Elevation is ordered data and a hue
   cycle destroys the ordering — nothing about magenta says "above green". Cool
   → neutral → warm, desaturated so the POI pins stay dominant.
2. **Anchored on the modal height, not min/max.** Stretching the ramp between
   the extremes sounds more principled and looks far worse: Fungus Grove spans
   −495..66 but nearly all of it sits near the top, so a linear stretch painted
   most of the zone one hot colour and spent the cool half of the ramp on a
   handful of segments — a thermal camera, not a map. Anchoring the neutral stop
   to where the zone actually is makes colour mean "unusually high or low *for
   this zone*", which is both more useful and what the hand-drawn maps say.
3. **Dim by distance from the main level.** Colour alone gave every level equal
   visual weight, so Necropolis — whose cavern shell sits well above its
   tunnels — came out mostly orange with the tunnels lost inside it. Fading with
   distance from the modal band makes prominence track usefulness: the floor
   you are most likely standing on is brightest, everything else is context.

The legend is not decoration — a ramp with no key is just decoration. Its stops
are drawn at the width of the height range that actually maps to them, so it
describes the anchored scale rather than implying a plain min-max gradient.

Cost is flat: opacity bucket and height band are both derived from the same Z, so
one pass assigns each segment to its pair. Total work is unchanged from before
colour existed rather than one extra full scan per ramp step.

Side benefit: in Detailed mode this turns contour zones into genuine hypsometric
topographic maps, which is what contour lines want to be.

### 13.5 On borrowing from Brewall

Still no reply to the permission request (§5.4), so nothing of his is used. The
outline results suggest that for *geometry* we do not need to: the shapes come
out matching his topology because both derive from the same client meshes. What
he has that we cannot derive is **judgement** — which wall matters, where a
fake wall is, which route is the safe one. That is the gap worth asking about,
and it is annotation, not geometry.

## References
- [Zeal — CoastalRedwood/Zeal](https://github.com/CoastalRedwood/Zeal) (MIT)
- [Brewall EQ Maps](https://www.eqmaps.info/eq-map-files/) · [RedGuides mirror](https://github.com/RedGuides/brewall-maps)
- [PQDI](https://www.pqdi.cc/) — credits Brewall for all maps
- [LanternExtractor](https://github.com/LanternEQ/LanternExtractor) (MIT)
- [EQEmu zone-utilities](https://github.com/EQEmu/zone-utilities) (GPL-2.0)
