# 3D Maps

Status: **feature-complete for now, awaiting a Windows test** (2026-11-04).
Everything below is on `main` and unreleased. The 2D map set and its
research are in [maps-feasibility.md](maps-feasibility.md); this document
covers the 3D view only.

| Commit | What |
|---|---|
| `4a68dae0` | 3D view on the Maps page and Live Map (earlier work, untextured) |
| `0c5ec5e6` | Textures, View menu, see-through walls, invisible faces dropped |
| `2c141fde` | Placed objects: trees, buildings, furniture |
| `b18fd4eb` | Animated (skeletal) objects, posed still |
| `97c5cefe` | Wall-opacity slider adjusts in place instead of rebuilding |
| `e7987065` | Doors from the doors table |

---

## 1. What the user gets

The 2D/3D switch sits in the map toolbar on the **Maps** page and the **Live
Map**, and **Raid Replay** has its own 3D view. All three use
`ZoneMap3D` and share the same look.

The **View** menu (top right of the 3D view), kept across restarts in
localStorage (`map3d.prefs`):

| Option | Default | Effect |
|---|---|---|
| Style | Textured | **Textured** (the zone's own textures), **Height** (5-stop blue→sand ramp), **Plain** (light floors, dark walls) |
| Wall opacity | 100% | Fades walls and ceilings; floors stay solid |
| See in through outer walls | on | Hides the back of every face, so from outside a dungeon you look in instead of at a solid box |
| Objects & doors | on | Trees, buildings, furniture, animated objects, doors |
| Hide ceilings | off | Drops downward-facing faces |

Everything is read from the user's own EQ install (`<eq_path>`) at request
time. Nothing from the client is shipped or stored.

## 2. How it works

### Backend

`GET /api/maps/zone/{zone}/mesh` → `mapgen.LoadMesh3D(eqPath, zone, doors)`
→ `Mesh3D.Encode()`, cached for the last 3 zones (`api/maps_mesh.go`).

`GET /api/maps/zone/{zone}/texture/{name}[?mask=1]` → one texture as a
256 px PNG. The cache keeps the latest zone's raw textures and the PNGs made
from them, and separately the shared archives' textures (loaded only on a
miss).

| File (`backend/internal/mapgen/`) | Role |
|---|---|
| `pfs.go` | `.s3d` (PFS) archive reader; inflates only the entries asked for |
| `wld.go` | WLD parser; meshes (0x36) with UVs, per-polygon materials, vertex pieces; object placements (0x15) |
| `mesh3d.go` | `meshBuilder` (merges WLDs, dedupes materials, groups by material, flips winding), materials (0x30/0x31 → 0x05 → 0x04 → 0x03), `Encode` |
| `objects.go` | objects.wld placements, doors, actor lookup across archives, `placeTransform` |
| `skeleton.go` | skeletal actors (0x11 → 0x10, tracks 0x13 → 0x12), posed at frame 0 |
| `texture.go` | DXT1/3/5 DDS and 8/24/32-bit BMP decode, mip choice, colour-key mask |
| `../db/doors.go` | `ZoneDoors`: the zone's doors active in the current era |

**Mesh format `PQM2`** (little-endian): magic; vertex, index, group and
material counts (u32); f32 positions (map space); f32 UVs; u32 indices;
groups (start, count, material: u32 ×3); then each material: kind u8,
opacity u8, flags u8 (bit 0 = object or door), a spare byte, a u16 length and
the texture name. All blocks before the materials are 4-byte aligned, so the
client views them without copying.

### Frontend

- `services/api.ts`: `getZoneMesh3D` (parses PQM2), `getZoneTextureUrl`.
- `hooks/useMap3DPrefs.ts`: the View menu state.
- `components/maps/ZoneMap3D.tsx`:
  - `buildLayers` splits the mesh into floor, wall and ceiling meshes (|normal z| ≥ 0.55), separately for the zone and for objects. That makes six meshes sharing one set of vertex attributes, each with a draw group per material.
  - `buildMaterials` runs only on a style or culling change.
  - `adjustLook` handles opacity and visibility in place.
  - `texture()` fetches each texture once per file and mask.

## 3. Client-file facts and decisions

These were all measured on the TAKP v22 client (`/Volumes/T7/EQ/TAKPv22`).

**Textures**
- Most ".bmp" files in TAKP zone archives are really **DXT1 DDS**, 1024 px with a full mip chain. A few are true 8-bit paletted BMPs. A survey of 12 zones found 856 DXT1 and 8 BMP files and nothing else. Serving the 256 px mip keeps a zone at roughly 17 MB of GPU memory instead of hundreds.
- **Masked** materials (foliage, fences) get their colour key as transparency: palette index 0 for BMPs, DXT1's transparent texel for DDS. Only masked materials ask for `?mask=1`, so opaque materials don't turn black where the key colour is.
- Texture names include spaces, `'` and `&` (`stained glass.bmp`, `drana'sbb.bmp`). The name whitelist allows them.
- The page's CSP blocks `<img>` from the backend, so textures are **fetched and turned into ImageBitmaps** instead of loaded as images. ImageBitmaps aren't flipped on upload, which matches EQ's top-down (DirectX) UVs.
- Untextured "solid" faces with no texture reference are dropped, following LanternExtractor's default.

**Materials** (render method & ~0x80000000, after LanternExtractor):
0/0x03/0x4B/0x53 are invisible (boundaries and collision walls) and
0x0D/0x0F/0x10 are sky, so all of those are **dropped**. Drawing the invisible
zone-boundary walls was part of the original "solid box" look.
0x05/0x09/0x0A are translucent at 50/25/75%, 0x0B/0x17 are additive, and
0x13/0x19 are masked. Everything else is solid.

**Geometry**
- After the map-space conversion and the view's y flip, the client's faces wind **clockwise**. Area-weighted, floors in outdoor zones point down, for example 2 against 52 in gfaydark. The builder swaps B/C so floors face up.
- With winding correct, **front-face culling** ("See in through outer walls") is what removes a dungeon's outer shell when seen from outside. Hole showed this clearly: a solid lump with it off, and the city visible with it on.

**Objects**
- `objects.wld` (inside `<zone>.s3d`) places actors by name. The models are in `<zone>_obj.s3d`, then `<zone>_2_obj.s3d` (Velious), then, only for models still missing, the shared `global_obj.s3d` and `gequip.s3d`. `gequip.s3d` is 13 MB, so it's read lazily and only the needed actors are parsed.
- A 0x15 placement has the actor name, position, three rotations (1/512 turn; slot 0 is yaw on nearly every object, slot 1 is a rare tilt, slot 2 is never used) and scale (slot 1).
- **Yaw sign is +1** in raw WLD space, the opposite of LanternExtractor, whose negation comes from Unity's left-handed axes. Totals across ten zones didn't decide it, but **wall-mounted models** did: Kael's torches touch a wall at 603 vertices with +1 against 18 with −1, and torches, sconces and bookcases agree. Some models (light poles, a bear-head trophy) prefer the other sign, probably because they face the other way in their own files.
- Some zones **park unused objects at z = −32768** (gfaydark). Placements outside the zone's bounds + 500 units are skipped, or they stretch the camera framing to nothing.
- Gfaydark grows to about 185k triangles (111k of them objects) and a 6 MB payload. That's fine on localhost.

**Animated objects**
- 466 actors in the client are skeletal (torches, braziers, street lights, the Temple of Life emblem): 196 hang a mesh on each bone and 270 use a skinned mesh with vertex pieces. All tracks use the int16 frame format.
- They're posed at **frame 0**: each bone is translate · rotate · scale, chained from the root (bone 0). Checked with wireframes of the bazaar street light and the Akheva sconce, which assemble correctly.
- **No FLAG or BANNER actor anywhere in the client is skeletal.** Flags are static models and were already drawn with ordinary objects.

**Doors**
- They come from `quarm.db` `doors`, filtered with `activeContentClause` (the same expansion and content-flag window as spawns). Plane of Mischief, for example, keeps 9 of its 30 POMDOOR201 rows.
- Game (x, y) is raw (y, x). Size is a percentage, and incline is treated like object tilt.
- **Heading sign is +1**, the same as object yaw. This was checked on plan views of doors at headings 128 and 384 in four zones: with +1 every slab closes its doorway, while −1 swings it 180° onto the wall beside it.
- Every door resolved in freportw (71), kael (83, including 53 crates from gequip) and qeynos2 (70).

**Performance**
- The wall-opacity slider used to rebuild every material on each step, about 800 ms per step software-rendered. It now changes a uniform, about 150 ms per step in the same test, and that remainder is drawing the frame on the CPU.
- Mesh build is about 180–200 ms for freportw and kael, including the gequip fallback.

## 4. How it was verified

- **Unit tests**: PQM2 layout, material kinds, bitmap names, DDS mip choice and DXT1 mask, 8-bit BMP decode, placement and door transforms, actor components, skeleton parsing, track pose (`mapgen/*_test.go`), and `ZoneDoors` era filtering (`db/doors_test.go`).
- **Client-gated tests** (skip without the T7 drive): every material's texture decodes in gfaydark, akheva and qeynos2; floors face up in outdoor zones; objects are present; no parked objects (`mapgen/mapgen_test.go`).
- **Headless renders**:
  - Setup: isolated backend (`HOME=<scratch>`), renderer-only Vite, and playwright-core with SwiftShader WebGL. See the memory note on CDP verification.
  - Zones: hole, unrest, gfaydark, qeynos2 and bazaar, in all three styles and with wall fading.
- **Not verified**: anything on Windows, a real GPU, Drunder (`potactics`) or any PoP zone (the T7 client has none), and the Live Map and Raid Replay 3D views after these changes.

## 5. Known limitations

- **Unverified in practice**: object yaw has only been checked by measurement, and the tilt axis is unconfirmed (it affects a few rocks and doors).
- **Missing from the scene**: doors are always drawn closed (`invert_state` is ignored), doors with no model (invisible triggers) aren't drawn, particle effects (torch flames) are skipped, and animated objects don't move.
- **Possible loading problems**: `.tga` textures (seen in some PoP zones' materials) fall back to plain grey, and the mesh is fetched twice per zone in dev, because of a React double effect before the cache fills.
- **UI leftover**: the toolbar's "Height colours" button still shows in 3D, and it's unknown whether it does anything there.

## 6. Possible improvements

Roughly in order of value:

1. **Animate** skeletal objects (play the tracks) and particle flames. That would mean sending tracks, not just baked frame-0 poses.
2. **Open-state doors**: honour `invert_state`, and maybe show lifts at both stops.
3. **Instancing** for repeated objects (gfaydark draws the same pine about 2,800 times). It would cut payload and draw cost on slow machines.
4. **Baked vertex lighting**: zone meshes carry per-vertex colours (skipped today) that would give the textured style the game's own light and dark.
5. **Higher-resolution textures** as an option (512 px), for close-up views on capable GPUs.
6. **Water surface** styling: animated or tinted translucent water instead of the flat material.
7. **TGA decode**, if PoP zones turn out to need it.
8. **Camera presets**: top-down, "fly to marker", or "fly to door/zone line".

## 7. Where to pick up

- Windows test checklist: Obsidian vault `docs/pq-companion/3D Maps Windows Test.md`.
- Release: unreleased. Add the 3D map work to `FEATURES.md` and `README.md` in `/newrelease`.
