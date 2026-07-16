# PQ Companion — Features

## Phase 0 — Database Setup & Exploration ✅
- MySQL 8 Docker environment for EQEmu dump exploration
- Go CLI tool (`dbconvert`): MySQL dump → SQLite converter
  - `--from-dump` mode: parses `.sql` dump files directly, no MySQL required
  - `--from-mysql` mode: reads from a live MySQL connection
  - `--validate` / `--validate-only`: post-conversion data validation (row counts, FK integrity, spot checks)
  - Handles all MySQL→SQLite type mapping, index conversion, and data migration
  - Converts ~1.1 million rows in under 60 seconds
- Validation suite (`internal/converter/validate.go`, closes #55)
  - 14 core-table row-count checks — fails the build when a dump import drops a table
  - 10 referential-integrity checks across the loot, spawn, and NPC spell chains — warns on small orphan counts, escalates to error above 500 orphans per FK
  - Spot checks on classic-EQ records (`Cloth Cap`, `northkarana`, `Complete Healing`, `Minor Healing`) to catch partial imports that still hit row-count minimums
  - Exits non-zero on any error; unit-tested with in-memory SQLite
- `data-release` GitHub Actions workflow (`.github/workflows/data-release.yml`)
  - Manual dispatch (pick a specific dump from `sql/`) or auto-trigger on `sql/**` pushes
  - Converts, validates, uploads `quarm.db` to the `data-latest` prerelease (with `--clobber`), and archives a 30-day workflow artifact as a safety net
  - Both `ci.yml` (Go tests) and `release.yml` (Windows installer) pull `quarm.db` from that release
- Documented schema for all key tables (items, spells, NPCs, zones, loot, spawns) in `SCHEMA.md`
- Full pipeline documentation in `docs/db-pipeline.md` — local workflow, CI flow, bootstrap, idempotency guarantees, schema-diff procedure
- Go database layer (`internal/db`): typed read-only access to quarm.db
  - `Get` and `Search` functions for items, spells, NPCs, and zones
  - Paginated search results with total count
  - `ParseSpecialAbilities`: parses NPC caret-delimited special ability strings
  - All queries use parameterized statements; tested against real quarm.db

## Phase 1 — Go Backend API
- REST API: items, spells, NPCs, zones with search and filtering (`cmd/server`, `internal/api/`)
  - `GET /api/items?q=&limit=&offset=` / `GET /api/items/{id}`
  - `GET /api/spells?q=&limit=&offset=` / `GET /api/spells/{id}`
  - `GET /api/npcs?q=&limit=&offset=` / `GET /api/npcs/{id}`
  - `GET /api/zones?q=&limit=&offset=` / `GET /api/zones/{id}` / `GET /api/zones/short/{name}`
  - chi router, structured logging, 404/400 error responses, max 100 results per page
- WebSocket server for real-time event broadcasting to all connected clients (`internal/ws/`)
  - Hub pattern: register/unregister clients, buffered broadcast channel
  - `ws.Event{Type, Data}` JSON envelope — extensible for all future event types
  - Per-client read/write pumps with ping/pong keepalive (54 s interval, 60 s timeout)
  - Slow-client protection: lagging clients are dropped rather than blocking the broadcast
  - `GET /ws` endpoint integrated into chi router
  - `hub.Broadcast(event)` — call from any goroutine to push to all connected clients
  - `hub.ClientCount()` — current connection count
- YAML configuration system (`internal/config/`)
  - Config file at `~/.pq-companion/config.yaml` — created with defaults on first run
  - Fields: `eq_path` (EQ install dir), `character` (active char name), `server_addr` (listen addr)
  - `preferences`: `overlay_opacity` (0.0–1.0), `minimize_to_tray`, `parse_combat_log`
  - `config.Manager`: thread-safe `Get()` / `Update()` with automatic disk persistence
  - CLI `--addr` flag overrides `server_addr` from config when provided
  - `GET /api/config` — returns current configuration as JSON
  - `PUT /api/config` — replaces configuration and persists to disk

## Phase 2 — Electron + React Frontend

### Task 2.1 — Electron + React Project Setup ✅
- **electron-vite** build tool: unified dev/build pipeline for main, preload, and renderer processes
- **Electron 33** shell in `electron/main/index.ts`
  - `BrowserWindow` with `hiddenInset` title bar (macOS) and custom title bar (Windows)
  - `show: false` + `ready-to-show` to prevent white flash on launch
  - `nativeTheme.themeSource = 'dark'` — forces OS dark mode
  - Dev mode loads Vite dev server at `http://localhost:5173`; prod loads built `out/renderer/index.html`
  - External links opened with `shell.openExternal` (never in Electron itself)
- **Preload script** in `electron/preload/index.ts`
  - `contextBridge.exposeInMainWorld('electron', …)` — secure, typed API surface
  - Exposes: `versions` (node/chrome/electron) and `window` controls (minimize/maximize/close/isMaximized)
- **Go sidecar lifecycle** in main process
  - Production: spawns `resources/bin/pq-companion-server[.exe]` as a child process, pipes stdout/stderr to console
  - Dev: skips sidecar — backend is started separately with `go run ./cmd/server`
  - Sidecar killed gracefully on `before-quit` and `window-all-closed`
- **IPC handlers**: `window:minimize`, `window:maximize`, `window:close`, `window:is-maximized`
- **React 18** renderer in `frontend/src/`
  - Vite + TypeScript + `@vitejs/plugin-react`
  - `ElectronAPI` type declared globally in `frontend/src/types/electron.d.ts`
- **Tailwind CSS v4** via `@tailwindcss/vite` plugin
  - EQ-themed dark color palette defined in `@theme` block: `background`, `surface`, `border`, `primary` (gold), `muted`, semantic colors
  - Custom scrollbar, `user-select: none` base, `.drag-region` / `.no-drag` Electron drag utilities
- **electron-builder** config in `electron-builder.yml`
  - Mac: DMG (x64 + arm64); Windows: NSIS installer (x64)
  - Go sidecar bundled via `extraResources` into `resources/bin/`
  - GitHub Releases publish target (draft mode)
- TypeScript project references: `tsconfig.main.json`, `tsconfig.preload.json`, `tsconfig.renderer.json`

### Task 2.2 — App Layout & Navigation ✅
- **React Router v7** (`HashRouter`) wired up in `App.tsx` with nested routes under a shared `Layout`
- **`Layout` component** (`components/Layout.tsx`): full-height flex column — TitleBar + Sidebar + `<Outlet />`
- **`TitleBar` component** (`components/TitleBar.tsx`):
  - Full-width drag region (`-webkit-app-region: drag`) with EQ gold app name centered
  - macOS: 72px left inset to clear native traffic-light buttons; no custom controls
  - Windows/Linux: custom Minimize / Maximize / Close buttons (lucide-react icons) with hover states; Close highlights red on hover
  - Tracks maximized state via `window.electron.window.isMaximized()` IPC
- **`Sidebar` component** (`components/Sidebar.tsx`):
  - Fixed 192px width, surface background, border-right
  - "Database" section label at top
  - Nav links: Items (`Sword`), Spells (`Sparkles`), NPCs (`Skull`), Zones (`Map`) — all lucide-react icons
  - Active link highlighted in gold; hover state for inactive links
  - Settings link pinned at the bottom, separated by a border
  - All interactive elements marked `.no-drag` so clicks are not eaten by the drag region
- **Placeholder pages** (`pages/`): `ItemsPage`, `SpellsPage`, `NpcsPage`, `ZonesPage`, `SettingsPage` — each shows an icon + label + "coming in task X.X" note
- Root route (`/`) redirects to `/items`
- `lucide-react` added as a dependency

### Task 2.3 — Database Explorer: Items ✅
- **`types/item.ts`** — TypeScript `Item` type mirroring Go backend struct; `SearchResult<T>` generic
- **`services/api.ts`** — typed fetch client: `searchItems(q, limit, offset, baneBody?)`, `getItem(id)`
- **`lib/itemHelpers.ts`** — EverQuest bitmask/label decoders:
  - `slotsLabel` — decodes `slots` bitmask into slot names (Charm, Head, Primary, etc.)
  - `classesLabel` — decodes `classes` bitmask into class names; "All" when all bits set
  - `racesLabel` — decodes `races` bitmask into race names; "All" when all bits set
  - `itemTypeLabel` — maps `item_type` int to weapon/armor/misc label
  - `effectiveItemTypeLabel` — resolves display label using `item_class` first (Container/Book overrides) then `item_type`
  - `isLoreItem` — detects lore (unique) items via EQ's `*`-prefixed lore string convention
  - `sizeLabel`, `weightLabel`, `priceLabel` (copper → pp/gp/sp/cp)
  - `baneBodyLabel` — maps `bane_body` int to body type name (Humanoid, Undead, Dragon, etc.)
  - `BANE_BODY_OPTIONS` — sorted option list for bane body type filter dropdown
- **`pages/ItemsPage.tsx`** — split-pane layout:
  - **Left pane (288px)**: debounced search input, bane body type filter dropdown, result count, scrollable list showing name + item type + req level; selected item highlighted with gold left-border accent
  - **Detail panel (right)**: full item data in labeled sections — Combat (DMG/DLY/Range/AC), Bane Damage (Bane Damage/Bane vs Body/Bane vs Race, shown only when present), Stats (HP/Mana/STR/STA/AGI/DEX/WIS/INT/CHA), Resists (MR/CR/DR/FR/PR), Effects (Click/Proc/Worn/Focus), Restrictions (Req/Rec level, Slots, Classes, Races), Info (Weight, Size, Stack, Bag info, Price, Item ID)
  - Flags rendered as pill badges: MAGIC, LORE, NO DROP, NO RENT
  - Sections only rendered when they have non-zero values
  - Initial load fetches all items (empty query); debounced at 300ms
- **Backend `GET /api/items?bane_body=N`** — optional filter; when N > 0 restricts results to items with `banedmgbody = N`; `bane_amt`, `bane_body`, `bane_race` fields exposed on all item responses
- **Item Sources** (closes #78):
  - **Backend `GET /api/items/{id}/sources`** — returns `{ drops: [...], merchants: [...] }` with NPC `id`, `name`, and `zone_name` for each source; joins `lootdrop_entries → loottable_entries → npc_types` for drops and `merchantlist → npc_types` for merchants; zone resolved via `spawnentry → spawngroup → spawn2 → zone`; capped at 50 results per source type
  - **`types/item.ts`** — added `ItemSourceNPC` and `ItemSources` TypeScript types
  - **`services/api.ts`** — added `getItemSources(id)` fetch wrapper
  - **`pages/ItemsPage.tsx`** — "Item Sources" section in detail panel showing "Dropped by" and "Sold by" sub-groups; each NPC name is a clickable link that navigates to `/npcs?select=<id>`; zone name shown alongside NPC; section only rendered when at least one source exists

### Task 2.4 — Database Explorer: Spells ✅
- **`types/spell.ts`** — TypeScript `Spell` type mirroring Go backend struct (timing, duration, effects, class levels)
- **`services/api.ts`** — added `searchSpells(q, limit, offset)` and `getSpell(id)` typed fetch wrappers
- **`lib/spellHelpers.ts`** — EverQuest spell data decoders:
  - `castableClasses(classLevels)` — returns `{abbr, full, level}` for each class that can cast the spell (255 = cannot cast)
  - `castableClassesShort` — compact list of first 4 castable classes for list row subtitles
  - `resistLabel` — maps resist type int to name (Magic, Fire, Cold, Poison, Disease, Chromatic, etc.)
  - `targetLabel` — maps target type int to description (Self, Single, Targeted AE, PB AE, Caster Group, etc.)
  - `skillLabel` — maps skill ID to school/skill name (Abjuration, Alteration, Conjuration, Divination, Evocation, Discipline, Bard instruments, etc.); corrected ID mapping to match actual spells_new DB values
  - `msLabel` — converts milliseconds to `"2.5s"` / `"Instant"` display strings
  - `durationLabel` / `durationScales` / `ticksToTime` — converts buff_duration ticks + formula to human-readable string (1 tick = 6s); distinguishes fixed vs. level-scaling durations
  - `effectLabel` — maps spell effect IDs to readable names (160+ effects mapped)
  - `effectDescription(id, base, buffduration)` — human-readable effect descriptions: regen effects show "Increase Mana/HP by N per tick (total T)", stat buffs show "+N STR" etc.; zero-value stat slots filtered out
  - `zoneTypeLabel` — maps zone_type int to restriction string (Outdoor, Indoor, Outdoor & Underground, City); empty for unrestricted (0)
- **`pages/SpellsPage.tsx`** — split-pane layout matching Item Explorer:
  - **Left pane (288px)**: debounced search input, result count, scrollable list showing name + castable classes with levels + mana cost; selected spell highlighted with gold left-border accent; blank-name spell IDs filtered out
  - **Detail panel (right)**: spell data in labeled sections — Casting (skill school, mana, cast/recast/recovery time, duration labeled "Max Duration" for scaling spells), Targeting (target type, resist type, range, AoE range, Zone Type when restricted), Classes (full class names with required level), Effects (human-readable descriptions per slot), Messages (cast_on_you, cast_on_other, spell_fades flavor text), Taught by (clickable links to scroll items that teach this spell), Items with this effect (clickable links to click/worn/proc/focus items grouped by effect type), Info (Spell ID)
  - Flags rendered as pill badges: DISCIPLINE, NO DISPELL
  - Sections only rendered when they have relevant data

### Task 2.5 — Database Explorer: NPCs ✅
- **`types/npc.ts`** — TypeScript `NPC` type mirroring Go backend struct (combat, attributes, resists, behavior, special abilities)
- **`services/api.ts`** — added `searchNPCs(q, limit, offset)` and `getNPC(id)` typed fetch wrappers
- **`lib/npcHelpers.ts`** — EverQuest NPC data decoders:
  - `npcDisplayName(npc)` — combines name + last_name, converting EQEmu underscores to spaces
  - `className(classId)` — maps NPC class IDs 1–16 to full class names (Warrior → Berserker)
  - `raceName(raceId)` — maps race IDs to names (Human, Barbarian, Iksar, Skeleton, Dragon, etc.); display now uses `race_name` resolved via SQL JOIN to `races` table, covering all race IDs (e.g. 202 = Grimling) without a hard-coded lookup (fixes #27)
  - `bodyTypeName(bodyType)` — maps body type codes to labels (Humanoid, Undead, Magical, Invulnerable, etc.)
  - `parseSpecialAbilities(raw)` — parses caret-delimited `code,value^…` string into `{code, value, name}` objects; filters out disabled abilities (value = 0)
- **`pages/NpcsPage.tsx`** — split-pane layout matching Item/Spell Explorer:
  - **Left pane (288px)**: debounced search input, result count, scrollable list showing formatted name + level + class; selected NPC highlighted with gold left-border accent
  - **Detail panel (right)**: NPC data in labeled sections — Combat (HP/Mana/Damage range/Attacks/AC), Attributes (STR/STA/DEX/AGI/INT/WIS/CHA, omitted when all zero), Resists (MR/CR/DR/FR/PR, omitted when all zero), Special Abilities (parsed as pill badges), Behavior (Aggro Radius/Run Speed/Size), Info (NPC ID/Loot Table/Merchant/Spells/Faction IDs, Exp%, Spell/Heal Scale), Faction (primary faction name + per-faction kill hits with color-coded +/- values)
  - Flags rendered as pill badges: RAID TARGET, RARE SPAWN
  - Sections only rendered when they have non-zero values

### Task 2.6 — Database Explorer: Zones ✅
- **`types/zone.ts`** — TypeScript `Zone` type mirroring Go backend struct
- **`services/api.ts`** — added `searchZones(q, limit, offset)`, `getZone(id)`, `getNPCsByZone(shortName, limit, offset)`
- **Backend: `GetNPCsByZone`** (`internal/db/queries.go`) — follows spawn chain via UNION subquery: `spawn2→spawnentry→npc_types` (group spawns) UNION direct `spawn2.spawngroupID = npc_types.id` (solo spawns); returns paginated, deduplicated NPC list ordered by name
- **Backend: `GET /api/zones/short/{name}/npcs`** — new endpoint returning zone residents (up to 200 per page)
- **`pages/ZonesPage.tsx`** — split-pane layout matching other explorers:
  - **Left pane (288px)**: debounced search by long name, result count, list showing long name + short name + min level; selected zone highlighted with gold left-border accent
  - **Detail panel (right)**: two sections — Zone Info (Zone ID, min level, safe coordinates, note) and Residents (NPC list loaded on zone selection)
  - **NPC Resident list**: scrollable list showing NPC display name, class, level, and HP; fetched per-zone on demand; shows "Showing X of Y" when truncated; graceful empty-state for zones with no spawn data
- **Issue #30 — Zone attributes** (`outdoor`, `hotzone`, `can_levitate`, `can_bind`, `exp_mod`, `expansion`):
  - **Backend `models.go`** — added six fields to `Zone` struct
  - **Backend `queries.go`** — extended `zoneColumns` and `scanZone` to select `castoutdoor`, `hotzone`, `canlevitate`, `canbind`, `zone_exp_multiplier`, `expansion`
  - **`types/zone.ts`** — added matching fields to the TypeScript `Zone` interface
  - **`pages/ZonesPage.tsx`** — new **Quick Facts** section in the detail panel: Expansion name, XP Modifier %, Outdoor, Hotzone, Levitation, and Binding (with human-readable labels)
- **Issue #31 — Succor Point label** (`pages/ZonesPage.tsx`): renamed "Safe Point" to "Succor Point" and reformatted coordinates to `Y: ..., X: ..., Z: ...` to match EverQuest/YAQDS conventions
- **Issue #32 — Zone level range** (`models.go`, `queries.go`, `types/zone.ts`, `pages/ZonesPage.tsx`): added `npc_level_min`/`npc_level_max` fields derived via correlated subqueries (spawnentry→npc_types per zone); displayed as "Level Range: 1–66" in the Zone Info section and as "Lv 1–66" in the search list subtitle
- **Issue #63 — ZEM/XP modifier NaN% fix** (`queries.go`, `pages/ZonesPage.tsx`): added `COALESCE(z.zone_exp_multiplier, 1.0)` to the SQL query so NULL DB values default to 1.0; added NaN/undefined guard in `expModLabel` (returns `—` for non-finite values); replaced raw `Math.round` in the detail-panel header ZEM badge with `expModLabel`; wrapped the search-list ZEM badge with `isFinite()` check
- **Issue #64 — Hotzone flag field mapping verification** (`queries_test.go`): extended `TestGetZoneByShortName` with explicit assertions on `Hotzone`, `Outdoor`, and `ExpMod` fields to guard against scanZone column misalignment; verified the hotzone integer (0/1) round-trips correctly from SQLite through the Go API to the `zone.hotzone ? 'Yes' : 'No'` display in the detail panel
- **Issue #33 — Zone detail tabs** (connected zones, drops, ground spawns, foraged items, NPC spawns):
  - **Backend models** (`internal/db/models.go`): added `ZoneConnection`, `ZoneGroundSpawn`, `ZoneForageItem`, `ZoneDropItem` structs
  - **Backend queries** (`internal/db/queries.go`): added `GetZoneConnections` (zone_points→zone join, distinct by target zone), `GetZoneGroundSpawns` (ground_spawns by zoneidnumber), `GetZoneForage` (forage table by zoneidnumber), `GetZoneDrops` (aggregate loot from all NPC spawns in zone, capped at 500)
  - **Backend API** (`internal/api/zones.go`): added `getConnections`, `getGroundSpawns`, `getForage`, `getDrops` handlers
  - **Router** (`internal/api/router.go`): registered four new routes under `/api/zones/short/{name}/`
  - **Frontend types** (`types/zone.ts`): added `ZoneConnection`, `ZoneGroundSpawn`, `ZoneForageItem`, `ZoneDropItem` interfaces
  - **Frontend API** (`services/api.ts`): added `getZoneConnections`, `getZoneGroundSpawns`, `getZoneForage`, `getZoneDrops` functions
  - **`pages/ZonesPage.tsx`**: converted detail panel to tabbed layout (Overview, NPCs, Connected Zones, Drops, Ground Spawns, Forage); each tab lazy-loads its data on first view; connected zones and NPC rows are clickable links; drops and items navigate to their detail pages; respawn timers formatted as minutes/seconds

### Task 2.7 — Global Search ✅
- **`GET /api/search?q=&limit=`** — new backend endpoint; runs all four searches (items, spells, NPCs, zones) in parallel via goroutines and returns a single grouped response (`internal/api/search.go`)
- **`GlobalSearch` component** (`components/GlobalSearch.tsx`): full-screen modal overlay triggered by `Cmd+K` / `Ctrl+K` from anywhere in the app
  - Debounced search input (300ms); shows spinner while loading
  - Results grouped by category (Items, Spells, NPCs, Zones) with section headers and type icons
  - Each result shows name + contextual subtitle (item type/level, castable classes, NPC level/class, zone short name)
  - Keyboard navigation: `↑`/`↓` to move, `↵` to open, `Esc` to close; click outside to dismiss
  - Navigates to the correct explorer page (`/items`, `/spells`, `/npcs`, `/zones`) with `?select=ID` query param
- **Sidebar search hint** (`components/Sidebar.tsx`): `⌘K` shortcut pill shown above the nav links for discoverability
- **Pre-select via URL** (`?select=ID`): all four explorer pages read the `select` query param and fetch the record by ID; the `useEffect` depends on `searchParams` so it re-runs whenever the URL param changes — this ensures global search results are correctly selected even when the user is already on the target page (e.g. clicking a spell scroll from the Items page while already browsing items); param is cleared from the URL after selection (closes #5)

## Phase 3 — Zeal Integration & Backup Manager

### Task 3.1 — Zeal Export Reader ✅
- **`internal/zeal/` package** — parses and watches Zeal export files:
  - `ParseInventory(path, character)` — reads tab-delimited `<CharName>_pq.proj-Inventory.txt`; header row skipped; columns: Location, Name, ID, Count, Slots; returns `*Inventory` with `[]InventoryEntry`
  - `ParseSpellbook(path, character)` — reads `<CharName>_pq.proj-Spells.txt`; handles three formats: bare ID, `slot\tID`, or `ID\tName`; deduplicates spell IDs; returns `*Spellbook` with `[]int` spell IDs
  - `InventoryPath(eqPath, character)` / `SpellbookPath(eqPath, character)` — construct Zeal export file paths (`<CharName>_pq.proj-{Inventory,Spells}.txt`)
  - `Watcher` — polls both files every 5 seconds; re-parses on modification time change; caches latest inventory and spellbook in memory; broadcasts `zeal:inventory` and `zeal:spellbook` WebSocket events on update; gracefully skips when `eq_path` or `character` are not yet configured
- **API endpoints**:
  - `GET /api/zeal/inventory` — returns `{"inventory": {...}}` or `{"inventory": null}` if not yet available
  - `GET /api/zeal/spells` — returns `{"spellbook": {...}}` or `{"spellbook": null}`
- **Frontend — Inventory page** (`pages/InventoryPage.tsx`):
  - "Inventory" link added to sidebar under a "Zeal" section with `Package` icon
  - Header bar showing character name, item count, export timestamp, and Refresh button
  - Left pane (288px): equipped items sorted by canonical slot order (Charm → Feet), Bank items, Cursor
  - Right pane: bags (General 1–8) with sub-items indented; shows bag name when available
  - "Not configured" empty state with setup instructions and link to Settings
  - Hover "look up" button on each item navigates to `/items?select=<id>` to pre-select in Item Explorer
- **WebSocket events**: `zeal:inventory` and `zeal:spellbook` broadcast to all connected clients when export files are updated on disk
- **Tests** (`internal/zeal/reader_test.go`): 11 table-driven tests covering inventory parsing, no-header files, empty files, missing files, three spellbook formats, deduplication, path helpers, and ModTime

### Task 3.2 — Spell Checklist UI ✅
- **Backend: `GetSpellsByClass(classIndex, limit, offset)`** (`internal/db/queries.go`) — returns all spells castable by a given class (0-based: 0=Warrior … 14=Beastlord), ordered by that class's required level then spell ID; filters out empty-name spells; parameterized query (column number validated in Go before use)
- **Backend: `GET /api/spells/class/{classIndex}`** (`internal/api/spells.go`) — new endpoint; limit defaults to 500, capped at 1000; validates classIndex is 0–14
- **`services/api.ts`** — added `getSpellsByClass(classIndex, limit, offset)` typed fetch wrapper
- **`pages/SpellChecklistPage.tsx`** — full spell checklist UI:
  - **Class selector**: dropdown for all 15 EQ classes (WAR–BST); selection persisted to `localStorage`; defaults to Enchanter
  - **Filter tabs**: All / Known / Missing — instantly filters the list without re-fetching
  - **Stats bar**: shows `X / Y known` when spellbook is loaded, or `Y spells` when no export is available
  - **Spellbook status banner**: green checkmark + character name + export timestamp when Zeal spellbook is loaded; amber warning with link to Settings when no export is found
  - **Spell list**: flat scrollable list ordered by class level (ascending); each row shows — known indicator (filled circle in gold vs. empty circle in gray), spell name (clickable), level badge, mana cost
  - Clicking any row opens an inline modal popup with full spell details (casting, targeting, classes, effects, messages); modal has an "Explorer" button to navigate to `/spells?select={id}` and a close button; backdrop click also closes the modal
  - Loading and error states with retry button
  - Empty states per filter ("All spells known!", "No known spells", "No spells for this class")
- **Sidebar** (`components/Sidebar.tsx`) — "Spell Checklist" added to the Zeal nav section with `BookOpen` icon
- **`App.tsx`** — `/spell-checklist` route wired up

### Task 3.3 — Inventory Tracker (Multi-Character + Search) ✅
- **`internal/zeal/scanner.go`** — `ScanAllInventories(eqPath)`: globs `*_pq.proj-Inventory.txt`, parses each file, strips SharedBank entries from per-character inventories, and returns the SharedBank from the most-recently-modified export (deduplicated by taking the newest copy only)
- **`internal/zeal/models.go`** — `AllInventoriesResponse{Configured, Characters, SharedBank}` — `Configured` distinguishes "EQ path not set" from "no exports found yet"
- **`internal/zeal/watcher.go`** — `AllInventories()` method: uses `cfgMgr` to get EQ path, calls `ScanAllInventories`, and returns a ready-to-encode response
- **`GET /api/zeal/all-inventories`** — new endpoint; on-demand scan of all exports; returns `{configured, characters[], shared_bank[]}`
- **Frontend — Inventory Tracker page** (`pages/InventoryTrackerPage.tsx`) at `/inventory-tracker`:
  - **Character tabs**: All · one tab per discovered character (shows item count); tab selection persists within the session; selecting a tab that no longer exists after refresh resets to All
  - **Search bar**: debounce-free text filter in the header; filters by item name across the active scope (case-insensitive substring); X button to clear
  - **Sections**: Equipped (sorted by canonical slot order), Bags (grouped by bag number per character; bag name shown in sub-header when available), Bank, Shared Bank (always shown once regardless of selected character)
  - **Character badges**: shown on each item row in "All" mode when more than one character is present
  - **Empty state after search**: "No items matching …" message when all sections are empty after filtering
  - **Not-configured / no-exports states**: separate messages with setup guidance and a "Check Again" refresh button
  - Hover "look up" button on each item row navigates to `/items?select={id}`
- **Sidebar**: "Inventory" entry renamed to "Inventory Tracker" pointing at `/inventory-tracker`; old `/inventory` route kept but removed from sidebar

### Task 3.4 — Key Tracker ✅
- **`internal/keys/keys.go`** — static key definitions (no DB needed). Each `KeyDef` has an ID, name, description, ordered `[]Component{ItemID, ItemName, Notes}`, and an optional `FinalItem *Component` representing the assembled key. Item IDs are canonical; names are for display only. Ships with the following keys: Veeshan's Peak, Sleeper's Tomb, Old Sebilis, Howling Stones (Charasis), Grieg's End, Grimling Forest Shackle Pens, Katta Castellum, Arx Seru, Temple of Ssraeshza (Ring of the Shissar — 4 components, FinalItem `Ring of the Shissar` 19719), and Vex Thal (Scepter of Shadows — 13 components incl. all 10 Lucid Shards, Shadowed Scepter Frame, A Planes Rift, A Glowing Orb of Luclinite; FinalItem `The Scepter of Shadows` 22198).
- **`GET /api/keys`** — returns all key definitions as `{"keys": [...]}`. Each key may include a `final_item` field.
- **`GET /api/keys/progress`** — cross-references all character inventories (via `AllInventories`) against each key's component item IDs. Response: `{configured, keys[{key_id, characters[{character, has_export, components[{item_id, item_name, have, shared_bank}], final_item?{item_id, item_name, have, shared_bank}}]}]}`. `have` is true if the item is in that character's equipped/bag/bank slots. `shared_bank` is true when the only copy is in the Shared Bank (available to all characters, deduplicated). `final_item` is populated only when the key defines an assembled-key item, and a character holding it is treated as fully keyed.
- **`types/keys.ts`** — TypeScript types mirroring all Go response structs (`KeyDef.final_item?`, `CharacterKeyProgress.final_item?`).
- **`services/api.ts`** — `getKeys()` and `getKeysProgress()` typed fetch wrappers.
- **`pages/KeyTrackerPage.tsx`** — Key Tracker page at `/key-tracker`:
  - **Header bar**: Key Tracker title and Refresh button.
  - **Filter tabs**: All / In Progress / Complete — filters the key card list by aggregate progress across all characters. Holding the `final_item` short-circuits the per-component count and counts as "complete".
  - **Key cards**: expandable accordion cards; collapsed state shows key name and a progress bar (`X / Y components` aggregated across all characters). Complete keys render with a green border.
  - **Component table** (expanded): when the key defines a `final_item`, an "Assembled Key" header row is rendered above the component rows with distinct styling and a green badge. Component rows show a green checkmark (character has the item), `SB` gold badge (only in shared bank), faded checkmark (covered transitively by the assembled key in this character's inventory), or empty circle (missing). Component notes shown as muted subtitle text.
  - Empty states for each filter tab; not-configured state with link to Settings; no-exports state per key.
- **Sidebar**: "Key Tracker" added to the Zeal nav section with `KeyRound` icon.

### Task 3.5 — Config Backup Manager (Backend) ✅
- **`internal/backup/` package** — backup creation, storage, and restoration:
  - `models.go` — `Backup{ID, Name, Notes, CreatedAt, SizeBytes, FileCount}`; `ErrNotFound` sentinel
  - `store.go` — `Store`: opens/creates `~/.pq-companion/user.db` (first feature to use user.db); `CREATE TABLE IF NOT EXISTS backups` migration; `Insert`, `List` (newest-first), `Get`, `Delete`
  - `manager.go` — `Manager`: `NewManager` (uses `~/.pq-companion/`) and `NewManagerAt` (custom base dir for tests); `Create(name, notes)` — globs all `*.ini` files in `eq_path`, creates a deflate zip in `~/.pq-companion/backups/<id>.zip`, inserts DB record; `Delete(id)` — removes zip + record; `Restore(id)` — extracts zip back to `eq_path` with path-traversal guard; `List`/`Get` — thin wrappers over Store
  - Backup IDs are 8-byte cryptographic random hex strings
  - Errors: `eq_path` not configured, no `*.ini` files found, not-found sentinel wraps correctly through handler layer
- **API endpoints** (`internal/api/backup.go`):
  - `GET /api/backups` — list all backups newest-first
  - `POST /api/backups` — create backup; body `{"name":"…","notes":"…"}`; returns 201 + Backup JSON
  - `GET /api/backups/{id}` — get single backup
  - `DELETE /api/backups/{id}` — delete backup (zip + record); returns 204
  - `POST /api/backups/{id}/restore` — restore backup to EQ directory
- **CORS** updated to allow `POST` and `DELETE` methods (previously `GET, PUT` only)
- **Tests** (`internal/backup/backup_test.go`): 10 table-driven tests covering store open/migrate idempotency, CRUD, newest-first ordering, manager create/list, create with no eq_path, create with no ini files, delete, delete-not-found, restore, restore-not-found

### Task 3.6 — Config Backup Manager (UI) ✅
- **`types/backup.ts`** — `Backup{id, name, notes, created_at, size_bytes, file_count}` and `BackupsResponse`
- **`services/api.ts`** — added `post<T>` and `del` fetch helpers; `listBackups`, `createBackup(name, notes)`, `deleteBackup(id)`, `restoreBackup(id)`
- **`pages/BackupManagerPage.tsx`** — full backup manager UI at `/backup-manager`:
  - **Header bar**: "Config Backup Manager" title (HardDrive icon), Refresh button, "New Backup" toggle button (gold when creating)
  - **Info banner**: explains what gets backed up (`*.ini` files) and where backups are stored
  - **Create form** (inline, toggled): name input (required, auto-focused), notes input (optional), Create Backup / Cancel buttons; loading state with spinner; error display
  - **Backup cards**: archive icon, name, truncated notes, formatted date/time, file count badge, size (B/KB/MB), Restore + Delete action buttons
  - **Inline delete confirmation**: "Delete this backup permanently?" with Delete/Cancel — avoids accidental deletion
  - **Inline restore confirmation**: "Overwrite current EQ config files with this backup?" with Restore/Cancel
  - **Restored feedback**: card border turns green + "Restored" checkmark for 3 seconds after successful restore
  - **Empty state**: archive icon + "No backups yet" + "Create your first backup" button + Settings link
  - **Error states**: per-card error display for failed delete/restore operations; full-page error with Retry for load failure
- **Sidebar**: "Backup Manager" added to the Zeal nav section with `HardDrive` icon
- **`App.tsx`**: `/backup-manager` route wired up

## Phase 4 — Log Parsing & NPC Info Overlay

### Task 4.1 — Log File Tailer ✅
- **`internal/logparser/` package** — real-time EQ log file tailer and event parser:
  - `models.go` — typed `LogEvent` struct with `EventType` constants: `log:zone`, `log:combat_hit`, `log:combat_miss`, `log:spell_cast`, `log:spell_interrupt`, `log:spell_resist`, `log:spell_fade`, `log:spell_fade_from`, `log:death`; per-type data structs (`ZoneData`, `CombatHitData`, `CombatMissData`, `SpellCastData`, `SpellInterruptData`, `SpellResistData`, `SpellFadeData`, `SpellFadeFromData`, `DeathData`)
  - `parser.go` — `ParseLine(line string) (LogEvent, bool)` regex-based classifier:
    - Timestamp: `[Mon Jan _2 15:04:05 2006]` layout; handles space-padded single-digit days (ctime format)
    - Zone change: `"You have entered <ZoneName>."`
    - Spell begin casting: `"You begin casting <SpellName>."`
    - Spell interrupted: generic `"Your spell is interrupted."` and named `"Your <SpellName> spell is interrupted."`
    - Spell resist: `"Your target resisted the <SpellName> spell."`
    - Spell fade: `"Your <SpellName> spell has worn off."`
    - Spell fade from target: `"<SpellName> effect fades from <Name>."` → `EventSpellFadeFrom` with `SpellName` and `TargetName`
    - Combat hit (player→NPC): `"You <verb> <target> for <N> points of damage."` — extracts actor, verb, target, damage
    - Combat hit (NPC→player): `"<Actor> <verb>s you for <N> points of damage."` — extracts actor, conjugated verb, damage
    - Combat miss (player→NPC): `"You try to <verb> <target>, but miss!"`
    - Combat miss (NPC→player): `"<Actor> tries to <verb> you, but misses!"`
    - Player defense (dodge/parry/riposte/block): `"You <type> <actor>'s attack!"`
    - Death: `"You have been slain by <SlainBy>."`
    - Unrecognised lines return `(zero, false)` — not emitted
  - `tailer.go` — `Tailer` struct; polls the log file every 250ms:
    - File path: `<EQ_DIR>/Logs/eqlog_<CharName>_pq.proj.txt`
    - Seeks to end of file on first open (no historical replay)
    - Reads newly-appended bytes via `ReadAt` from tracked offset; handles partial lines across polls with a remainder buffer
    - Reacts to config changes: when `eq_path` or `character` changes, closes old handle and re-aims at the new path
    - Respects `preferences.parse_combat_log` config flag (stops polling when disabled)
    - Max 1 MiB read per tick to prevent blocking on large catch-up
    - Handles file truncation (re-seeks to 0) and missing file (skips silently until it appears)
    - Events dispatched to a caller-supplied `handler func(LogEvent)` outside the mutex
    - `Status()` returns `{enabled, file_path, file_exists, tailing, offset}` snapshot
  - `parser_test.go` — 28 table-driven tests covering all event types, both timestamp padding styles, unrecognised messages, and edge cases
- **`GET /api/log/status`** — returns the current tailer state: enabled, file path, file_exists, tailing, current offset
- **`cmd/server/main.go`** — tailer created and started at boot; handler logs events at debug level (Task 4.2 will wire it to `hub.Broadcast`)

### Task 4.2 — Event Broadcasting via WebSocket ✅
- **Backend wiring** (`cmd/server/main.go`) — tailer handler now calls `hub.Broadcast(ws.Event{Type: string(ev.Type), Data: ev})` for every parsed log event; all connected WebSocket clients receive log events in real time
- **`hooks/useWebSocket.ts`** — singleton WebSocket hook shared across all consumers:
  - One connection per app lifetime; auto-reconnects every 2 s on drop
  - `useWebSocket(onMessage?)` — returns `WsReadyState` ('connecting' | 'open' | 'closed'); callback is stable via ref (no need to memoize at call site)
  - Module-level `messageHandlers` / `stateHandlers` sets; connect/reconnect only while consumers are mounted
- **`types/logEvent.ts`** — TypeScript types mirroring Go structs: `LogEvent`, `LogEventType` (all event types including `log:heal`), all per-event `Data` types (`ZoneData`, `CombatHitData`, `CombatMissData`, `SpellCastData`, `SpellInterruptData`, `SpellResistData`, `SpellFadeData`, `DeathData`, `HealData`), `LogTailerStatus`
- **`services/api.ts`** — added `getLogStatus()` fetching `GET /api/log/status`
- **`pages/LogFeedPage.tsx`** — live log event feed at `/log-feed`:
  - **Header**: title, event counter (`X / 200`), WebSocket connection pill (green/orange/gray), Clear button
  - **Status bar**: tailer state inline — disabled warning with Settings link, file-not-found warning with path, or green "Tailing" with file path
  - **Event feed**: newest events at top; each row shows hh:mm:ss timestamp, color-coded type badge (blue=Zone, red=Hit, gray=Miss, purple=Cast, orange=Interrupt/Resist, teal=Fade, dark-red=Death, green=Heal), raw EQ log message in monospace; capped at 200 events
  - **Empty state**: activity icon + "Waiting for log events…" with setup instructions
- **Sidebar** — new "Parsing" section with "Log Feed" (`Activity` icon) at `/log-feed`
- **`App.tsx`** — `/log-feed` route wired up

### Task 4.3 — NPC Info Overlay (Backend) ✅

**Target Inference**
- New `overlay.NPCTracker` (`backend/internal/overlay/npc.go`) consumes parsed log events to infer the player's current combat target
- Target is set when a `log:combat_hit` or `log:combat_miss` event where `Actor == "You"` is received — the `Target` field becomes the current target
- Target is also set immediately on a `log:considered` event (EQ `/con` output) so the overlay updates before combat begins
- Target is cleared on zone change (`log:zone`), player death (`log:death`), or when a `log:kill` event names the currently-tracked target as the slain mob
- Duplicate target updates (same name) are skipped to avoid redundant DB lookups
- Zone-name guard: if a proposed target name exactly matches the current zone name it is rejected, preventing false-positive target updates from any misidentified zone-entry line (closes #71)
- Nil-DB guard added to `lookupNPC` so the tracker is usable without a live database (NPC data returns nil gracefully)

**`/con` Target Detection**
- New `EventConsidered` (`log:considered`) event type added to the log parser
- New `ConsideredData` struct carries the target name extracted from the disposition message
- Regex `reConsider` matches all classic EQ consider phrases: "scowls at you", "glares at you", "looks your way", "looks upon you", "judges you", "regards you", "warmly/kindly regards you", "considers you"
- Multi-word NPC names (e.g. "a grimling cadaverist") are correctly captured via non-greedy group before the disposition phrase
- Parser guard: after `reConsider` matches, names starting with "You" are rejected (NPC names never start with "You"; this prevents any player-action line from being misclassified as a consider event)
- Six behavioural unit tests added (`internal/overlay/npc_test.go`) covering: zone clear, zone-name guard, consider sets target, kill clears matching target, kill preserves unrelated target, death clears target

**NPC Database Lookup**
- When the target name changes, the tracker converts the log display name (spaces) to the EQ database format (underscores) and calls the new `db.GetNPCByName` query
- `GetNPCByName` performs a case-insensitive exact match against `npc_types.name` with `LIMIT 1`
- Retrieved NPC data includes all resist stats (MR, CR, DR, FR, PR), level, HP, special abilities string
- `db.ParseSpecialAbilities` is called to produce a structured `[]SpecialAbility` slice from the raw caret-delimited field

**WebSocket Broadcasting**
- On every target change (or loss), the tracker broadcasts a `overlay:npc_target` event via the shared WebSocket hub
- Payload is `overlay.TargetState`: `has_target`, `target_name`, `npc_data` (full NPC record), `special_abilities` (parsed), `current_zone`, `last_updated`
- When no target is active, `has_target: false` is broadcast with null NPC data

**REST Endpoint**
- `GET /api/overlay/npc/target` — returns the current `TargetState` snapshot for clients that poll instead of subscribing to WebSocket
- Handler in `backend/internal/api/overlay.go`; route wired in `router.go` under `/api/overlay/npc/target`

**Wiring**
- `main.go` creates the `NPCTracker` before the log tailer; the tailer's event handler calls both `hub.Broadcast` and `npcTracker.Handle` so no events are dropped

### Task 4.4 — NPC Info Overlay (Frontend) ✅

**Types & API**
- **`types/overlay.ts`** — `TargetState` interface mirroring the Go backend payload: `has_target`, `target_name`, `npc_data` (full NPC), `special_abilities` (`SpecialAbility[]` with `code`, `value`, `name`), `current_zone`, `last_updated`
- **`services/api.ts`** — added `getOverlayNPCTarget()` fetching `GET /api/overlay/npc/target` for initial-load polling

**`pages/NPCOverlayPage.tsx`** — live NPC info panel at `/npc-overlay`:
- **Header**: title with `Crosshair` icon, WebSocket connection pill
- **Status bar**: same tailer status as Log Feed — disabled warning, file-not-found, or green "Tailing"
- **No-target state**: centered crosshair icon with current zone name and instructions; shown when `has_target: false`
- **Loading state**: simple "Loading…" text while the initial REST fetch is in flight
- **NPC card** (when `has_target: true`):
  - Target name (large, bold) + current zone name + last-updated timestamp
  - RAID TARGET and RARE SPAWN flag badges (purple / amber)
  - **Identity row**: Level (gold), Class, Race, Body Type — each in a `Stat` tile
  - **Combat row**: HP (green), AC, Min DMG (red), Max DMG (red), Attack Count
  - **Resists row**: Magic, Cold, Disease, Fire, Poison
  - **Attributes row**: STR, STA, DEX, AGI, INT, WIS, CHA
  - **Special Abilities**: pill badges color-coded by severity — red for offensive (Summon, Enrage, Rampage, Flurry, Triple Attack, Immune to Melee/Magic), orange for immunities (Uncharmable, Unmezzable, Unfearable, Immune to Slow), gray for others
  - When target name is known but no DB record found: informational "no database record" notice
- **Real-time updates**: subscribes to `overlay:npc_target` WebSocket events; state updates instantly on every target change or loss without any polling
- **Initial load**: fetches current `TargetState` via REST on mount so the panel is populated even if no log event has fired since page load
- **Sidebar** — "NPC Overlay" (`Crosshair` icon) added to the Parsing nav section
- **`App.tsx`** — `/npc-overlay` route wired up

### Task 4.5 — NPC Info Overlay Window (Pop-Out) ✅

- **`electron/main/index.ts`** — `createNPCOverlay()` function creates a 360×480 transparent, frameless, always-on-top `BrowserWindow` loading `/#/npc-overlay-window`; IPC handlers `overlay:npc:open`, `overlay:npc:close`, `overlay:npc:toggle` wired up
- **`electron/preload/index.ts`** — `openNPC`, `closeNPC`, `toggleNPC` methods added to `window.electron.overlay`
- **`frontend/src/types/electron.d.ts`** — NPC overlay methods added to `ElectronAPI.overlay` type
- **`frontend/src/pages/NPCOverlayWindowPage.tsx`** — standalone overlay window page: drag-region header with `Crosshair` icon and close button, scrollable NPC content (identity, combat, resists, attributes, special abilities), no-target state; subscribes to `overlay:npc_target` WebSocket messages for real-time updates
- **`frontend/src/App.tsx`** — `/npc-overlay-window` route wired up outside the main `Layout`
- **`frontend/src/pages/NPCOverlayPage.tsx`** — "Pop out" button (`ExternalLink` icon) added to header; calls `window.electron.overlay.toggleNPC()`; only rendered inside Electron

## Phase 5 — Combat Tracking & DPS Meter

### Task 5.1 — Combat Parser ✅
- **`internal/combat/` package** — stateful combat tracker that consumes `logparser.LogEvent` values and maintains per-entity damage statistics grouped into fights:
  - `models.go` — typed structs:
    - `EntityStats` — per-combatant stats: `Name`, `TotalDamage`, `HitCount`, `MaxHit`, `DPS`
    - `FightState` — live snapshot of the active fight: `StartTime`, `Duration`, `Combatants` (outgoing damage dealers sorted by damage desc), `TotalDamage` (all outgoing), `TotalDPS`, `YouDamage`, `YouDPS`
    - `FightSummary` — immutable record of a completed fight: adds `EndTime`; same fields otherwise
    - `CombatState` — full broadcast payload: `InCombat`, `CurrentFight`, `RecentFights` (last 20), `SessionDamage` (player personal), `SessionDPS`, `LastUpdated`
    - `WSEventCombat = "overlay:combat"` — WebSocket event type constant
  - `tracker.go` — `Tracker` struct:
    - `NewTracker(hub *ws.Hub) *Tracker`
    - `Handle(ev logparser.LogEvent)` — processes `EventCombatHit` (records hit, starts/continues fight, arms inactivity timer), `EventKill` (ends fight at kill timestamp), `EventZone` and `EventDeath` (immediately ends active fight)
    - `GetState() CombatState` — thread-safe point-in-time snapshot
    - Fight boundary detection: `EventKill` ends fight immediately at log-event timestamp (accurate duration); inactivity timer fires after **6 seconds** with no new hits as fallback; uses monotonic `fightID` counter to guard stale `time.AfterFunc` callbacks
    - Per-entity tracking: `internalFight` maintains separate `outgoing` map (actors hitting non-"You" targets) and `incoming` map (actors hitting "You"); `Combatants` only reflects outgoing damage dealers
    - `TotalDamage` / `TotalDPS` = sum of all outgoing damage (all players); `YouDamage` / `YouDPS` = player personal only
    - Session aggregates: `SessionDamage` = player personal outgoing summed across completed fights; `SessionDPS` = SessionDamage / total fight time
    - Completed fights stored in a ring buffer capped at 20 entries, newest first
  - `tracker_test.go` — 9 unit tests covering: no fight initially, fight starts on first hit, hits accumulate, incoming damage excluded from Combatants, zone change ends fight, kill event ends fight at kill timestamp, session aggregates, sort order, third-party player damage tracking
- **`internal/api/combat.go`** — `combatHandler` wired to `GET /api/overlay/combat`; returns current `CombatState` as JSON
- **`internal/api/router.go`** — `NewRouter` signature extended with `*combat.Tracker`; `/api/overlay/combat` route added under `/api/overlay`
- **`cmd/server/main.go`** — `combat.NewTracker(hub)` instantiated; `combatTracker.Handle(ev)` called in the log-tailer event handler alongside the existing `npcTracker.Handle(ev)`

### Task 5.2 — DPS Overlay ✅
- **Log parser extended** (`internal/logparser/parser.go`) — added `reThirdPartyHit` regex to capture other players dealing damage: `"Playername verb target for N points of damage."` — checked after player/NPC-specific patterns to prevent false matches; guards skip if actor is `"You"` or target contains `"you"` (already handled by prior patterns); also skips if actor is a bare English article (`"a"`, `"an"`, `"the"`) to prevent multi-word NPC names (e.g. `"a fire elemental"`) from injecting a spurious `"a"` entry into the DPS table (fixes #42); added `EventKill` (`log:kill`) with `KillData{Killer, Target}` — parsed from `"You have slain X!"` and `"Playername has slain X!"` log lines (closes #40)
- **`types/combat.ts`** — TypeScript types mirroring Go structs: `EntityStats`, `FightState`, `FightSummary`, `CombatState` with all new `YouDamage`/`YouDPS` fields
- **`services/api.ts`** — added `getCombatState()` → `GET /api/overlay/combat`
- **`components/OverlayWindow.tsx`** — reusable draggable/resizable floating panel component:
  - Drag via title bar (grip icon; stops propagation on controls inside header)
  - 8-direction resize via edge and corner handles (N, S, E, W, NE, NW, SE, SW)
  - `useEffect` attaches `mousemove`/`mouseup` to `document` only during drag/resize to avoid global listener overhead
  - `minWidth`/`minHeight` props clamping; default 260×180
  - Semi-transparent themed background with `box-shadow`
  - Used by DPS overlay; designed as the base for all future overlays
- **`pages/DPSOverlayPage.tsx`** — in-app DPS overlay view (route `/dps-overlay`):
  - Floating `OverlayWindow` panel with drag/resize; hint text on background
  - **Filter toggle button** — `All` (shows every outgoing damage dealer) / `Me` (shows only `"You"`)
  - **Pop Out button** (⤢ icon) — invokes `window.electron.overlay.toggleDPS()` to open/close the standalone overlay window; only shown when running in Electron
  - Connection pill (live WS status), log-tailer status bar, combat status strip with fight duration (ticks every second via `setInterval`) and live DPS (recomputed from wall-clock start time so display updates continuously between log events)
  - Combatants table: per-row damage bar (width = % of total), name (player highlighted), % share, total damage, DPS; column headers; empty state
  - Session footer: fight count, total damage, session DPS
  - Subscribes to `overlay:combat` WebSocket events; initial state fetched via REST on mount
- **`pages/DPSOverlayWindowPage.tsx`** — compact overlay for the standalone Electron window (route `/dps-overlay-window`):
  - Transparent dark background (`rgba(10,10,12,0.88)`), 8px border-radius, no Electron frame
  - Drag via `-webkit-app-region: drag` CSS on title bar; controls use `no-drag` class
  - Filter toggle (All/Me) and × close button (calls `overlay.closeDPS()`)
  - Same combatant row layout as the in-app view; session footer
- **Electron main process** (`electron/main/index.ts`) — `createDPSOverlay()` creates a transparent, frameless, always-on-top `BrowserWindow` (420×460, min 260×180, `resizable: true`); loads `/#/dps-overlay-window`; `setAlwaysOnTop('screen-saver')` + `setVisibleOnAllWorkspaces(visibleOnFullScreen: true)` to float over fullscreen apps; IPC handlers: `overlay:dps:open`, `overlay:dps:close`, `overlay:dps:toggle`
- **Electron preload** (`electron/preload/index.ts`) — exposes `window.electron.overlay.{openDPS, closeDPS, toggleDPS}` to renderer via `contextBridge`
- **`types/electron.d.ts`** — added `overlay` field to `ElectronAPI` interface
- **`components/Sidebar.tsx`** — added `DPS Overlay` nav entry (Swords icon) in the Parsing section
- **`App.tsx`** — added `/dps-overlay` route (in Layout) and `/dps-overlay-window` route (outside Layout for standalone window)

### Task 5.3 — Combat Log History ✅
- **`pages/CombatLogPage.tsx`** — full-page fight history view (route `/combat-log`):
  - Lists all completed fights from `CombatState.recent_fights` (up to 20), newest first, numbered sequentially from session start
  - Each row: chevron toggle, fight #, start time and duration, total outgoing damage, total DPS (all players), personal DPS ("me") — color-coded orange/indigo to match DPS overlay
  - Collapsible combatant breakdown per fight: 5-column table (Name, %, Damage, DPS, Max Hit); player row highlighted in primary color; damage % relative to fight total
  - Subscribes to `overlay:combat` WebSocket events for live updates — new fight rows appear automatically when fights complete
  - Initial state fetched via `GET /api/overlay/combat` on mount
  - Log-tailer status bar (same pattern as DPS overlay) — warns when log parsing is disabled or log file not found
  - Empty state with icon + hint text when no fights completed yet
  - Session footer: fight count, total personal damage, session-average personal DPS
- **`components/Sidebar.tsx`** — added `Combat Log` nav entry (`ScrollText` icon) after DPS Overlay in the Parsing section
- **`App.tsx`** — added `/combat-log` route

### Issue #47 — Combat Log Filtering ✅
- **`internal/combat/tracker.go`** — added `Reset()` method: stops the inactivity timer, clears active fight, resets all aggregates and death records, then broadcasts the empty state
- **`internal/api/combat.go`** — added `reset` handler for `POST /api/combat/reset` (returns 204 No Content)
- **`internal/api/router.go`** — registered `POST /api/combat/reset` under `/api/combat/reset`
- **`services/api.ts`** — added `resetCombatState()` typed API call
- **`pages/CombatLogPage.tsx`** — added filter bar with:
  - Combatant name search (text input) — filters fights to those containing a matching combatant
  - Time range selector (All / Last 30m / Last 1h / Last 2h) — hides fights outside the window
  - "Me only" toggle — shows only fights where the player dealt damage (`you_damage > 0`)
  - Export CSV button — downloads visible fights with per-combatant breakdown
  - Clear button — calls `POST /api/combat/reset` to wipe all fight history and session totals
  - Filter badge in header ("N / M fights") when any filter is active
  - Contextual empty-state message distinguishing no fights vs. no matching fights

### Issue #48 — Death Tracker (Combat Log tab) ✅
- **`internal/logparser/parser.go`** — added `reDiedSimple` regex (`^You died\.$`) emitting `EventDeath` with empty `SlainBy` (complements the existing `reDeath` "slain by" pattern)
- **`internal/combat/models.go`** — added `DeathRecord` struct (`Timestamp`, `Zone`, `SlainBy`); added `Deaths []DeathRecord` and `DeathCount int` to `CombatState`
- **`internal/combat/tracker.go`** — added `currentZone string` and `deaths []DeathRecord` to `Tracker`; separated `EventZone` and `EventDeath` handling in `Handle()`: zone events update `currentZone` before ending the fight; death events append a `DeathRecord` (with timestamp, current zone, and optional killer) then end the fight; `snapshot()` copies deaths slice into state
- **`types/combat.ts`** — added `DeathRecord` interface; added `deaths` and `death_count` to `CombatState`
- **`pages/CombatLogPage.tsx`** — added `DeathLogSection` component: collapsible panel at the bottom of the Combat Log page, showing session death count with Skull icon and an expandable per-death table (time, zone, killer or "unknown cause"); shown in both normal and empty-fight states when deaths > 0

### Task 5.4 — HPS Meter ✅
- **Heal log parsing** (`internal/logparser/`):
  - `models.go` — added `EventHeal` event type constant (`log:heal`) and `HealData` struct (`Actor`, `Target`, `Amount`)
  - `parser.go` — three new regex patterns:
    - `reYouHeal` — `"You healed <target> for <N> hit points."` (player casts heal; `yourself` normalised to `You`)
    - `reHealedYou` — `"<actor> healed you for <N> hit points."` (someone heals the player)
    - `reThirdPartyHeal` — `"<actor> healed <target> for <N> hit points."` (third-party; checked last to avoid false-matching prior patterns)
- **Combat tracker extended** (`internal/combat/`):
  - `models.go` — added `HealerStats` struct (`Name`, `TotalHeal`, `HealCount`, `MaxHeal`, `HPS`); extended `FightState` and `FightSummary` with `Healers`, `TotalHeal`, `TotalHPS`, `YouHeal`, `YouHPS`; extended `CombatState` with `SessionHeal` and `SessionHPS`
  - `tracker.go` — `internalHealer` struct mirrors `internalEntity`; `internalFight.healers` map added; `Handle()` dispatches `EventHeal` to new `recordHeal()` method; `recordHeal()` only tracks heals during an active fight; `archiveFight()` and `snapshot()` compute healer stats and session HPS via `buildHealerStats()`; session heal total accumulated alongside damage
- **`types/combat.ts`** — added `HealerStats` interface; extended `FightState`, `FightSummary`, `CombatState` with all new heal fields
- **`pages/DPSOverlayPage.tsx`** — renamed conceptually to "DPS / HPS meter" (route unchanged at `/dps-overlay`):
  - **Tab bar** — DPS (orange, Swords icon) and HPS (green, HeartPulse icon) tabs; switching tabs changes the displayed data, the combat strip rate label, and the session footer values
  - **HPS panel** — `HPSRow` and `HPSPanel` components mirror DPS equivalents; green color accent; bar width proportional to healer's share of total healing
  - **Pop-out button** — per-tab: DPS tab shows `toggleDPS()`, HPS tab shows `toggleHPS()`; invokes the respective Electron IPC to open/close the standalone window
  - Session bar shows fight count + total healed + session HPS when HPS tab is active
- **`pages/HPSOverlayWindowPage.tsx`** — standalone always-on-top HPS overlay window (route `/hps-overlay-window`):
  - Same layout as `DPSOverlayWindowPage`: transparent dark background, drag-region title bar, All/Me filter toggle, × close button (calls `overlay.closeHPS()`)
  - Green color accent (`#4ade80`) throughout; title shows live current HPS in header
  - Session footer: fight count, total healed, session HPS
- **Electron main** (`electron/main/index.ts`) — `createHPSOverlay()` creates 420×460 transparent frameless always-on-top window; IPC handlers `overlay:hps:open`, `overlay:hps:close`, `overlay:hps:toggle`
- **Electron preload** (`electron/preload/index.ts`) — exposes `window.electron.overlay.{openHPS, closeHPS, toggleHPS}`
- **`types/electron.d.ts`** — added `openHPS`, `closeHPS`, `toggleHPS` to `ElectronAPI.overlay`
- **`App.tsx`** — added `/hps-overlay-window` standalone route

### Task 5.5 — Overlay Toggle Switches ✅
- **Config extended** (`internal/config/config.go`):
  - `Preferences` — added `OverlayDPSEnabled bool` (`yaml:"overlay_dps_enabled"`) and `OverlayHPSEnabled bool` (`yaml:"overlay_hps_enabled"`)
  - Defaults: `overlay_dps_enabled: true`, `overlay_hps_enabled: false`
- **`types/config.ts`** — added `overlay_dps_enabled` and `overlay_hps_enabled` to `Preferences` interface
- **`pages/SettingsPage.tsx`** — new **Overlays** settings section with two toggle switches:
  - **DPS Overlay** — enables/disables the floating DPS meter window
  - **HPS Overlay** — enables/disables the floating HPS meter window
  - Each switch persists through the existing config save flow (`PUT /api/config`); green accent used for HPS toggle thumb to distinguish it from the primary-color DPS toggle

## Phase 6 — Windows Build & Distribution

### Task 6.1 — Windows Build Pipeline ✅
- **`.github/workflows/release.yml`** — release workflow triggered on `v*` tags (and `workflow_dispatch`):
  - `build-windows` job on `windows-latest`: downloads `quarm.db` from `data-latest` release, cross-compiles Go backend with `CGO_ENABLED=0 GOOS=windows GOARCH=amd64`, runs `electron-vite build` + `electron-builder --win --publish never`, uploads NSIS `.exe` as a workflow artifact
  - `build-macos` job on `macos-latest`: same flow for `darwin/arm64`, produces a DMG artifact
  - `release` job (needs both builders): downloads artifacts, creates a draft GitHub Release with NSIS installer + DMG attached
- **`.github/workflows/ci.yml`** — CI workflow triggered on push/PR to `main`:
  - `test-backend`: runs `go test ./...` against the real SQLite backend
  - `typecheck-frontend`: runs `npm run typecheck` (all three tsconfig targets)
- **`electron-builder.yml`** — restructured `extraResources` into platform-specific sections; added `backend/data/quarm.db → bin/data/quarm.db` to both `mac` and `win` sections so the sidecar can locate the database at `resources/bin/data/quarm.db` at runtime; removed shared top-level `extraResources` block that would fail when the opposite-platform binary was absent
- **`package.json`** — added `build:backend`, `build:backend:win`, and `build:backend:mac` scripts for building the Go sidecar locally before packaging

**Data release prerequisite** — `quarm.db` is gitignored (84 MB). Before the first release build, upload it once:
```
gh release create data-latest backend/data/quarm.db \
  --prerelease --title "Game Database" \
  --notes "EQ game data — regenerate with backend/cmd/dbconvert"
```
Subsequent release builds download it automatically from that release.

### Task 6.2 — Auto-Updater ✅
- **`electron/main/index.ts`** — `setupAutoUpdater()` wires `electron-updater` into the main process:
  - Skipped in dev mode (`!app.isPackaged`)
  - `autoDownload: false` — download only triggers when user clicks Update; `autoInstallOnAppQuit: true` as fallback
  - Checks for updates 5 s after launch (gives sidecar + window time to initialise)
  - Events forwarded to the renderer via `mainWindow.webContents.send`:
    - `updater:available` → `{ version }` — new version detected, awaiting user action
    - `updater:progress` → `{ percent, transferred, total }` — download progress
    - `updater:downloaded` → `{ version }` — ready to install
    - `updater:error` → error message string
  - IPC handlers: `updater:check` (manual recheck), `updater:download` (trigger download), `updater:quit-and-install` (silent install with `isSilent=true, isForceRunAfter=true` — no UAC/path dialog, restarts to the same directory automatically)
- **`electron/preload/index.ts`** — `updater` namespace exposed via `contextBridge`:
  - `check()`, `download()`, `quitAndInstall()` — invoke IPC handlers
  - `onAvailable(cb)`, `onProgress(cb)`, `onDownloaded(cb)`, `onError(cb)` — subscribe to update events; each returns an unsubscribe function for `useEffect` cleanup
- **`frontend/src/types/electron.d.ts`** — `updater` added to `ElectronAPI` interface; includes `download()`
- **`frontend/src/components/UpdateNotification.tsx`** — bottom-of-app banner with six states:
  - `available` — "Update vX.Y.Z available" + **Update** button (user-initiated download)
  - `downloading` — gold progress bar with percentage (no user action needed)
  - `downloaded` — "Restarting in Ns" countdown (5 s) then auto-calls `quitAndInstall(true, true)` for silent install; **Restart now** button skips countdown
  - `installing` — "Installing — restarting…" with spinner (briefly shown before app exits)
  - `error` — error message + **Retry** button (re-triggers `check()`), dismissible
- **`frontend/src/components/Layout.tsx`** — `<UpdateNotification />` added below `<GlobalSearch />`
- **`.github/workflows/release.yml`** — updated for auto-updater:
  - Both `build-windows` and `build-macos` jobs changed to `--publish always`; `GH_TOKEN` is passed so `electron-builder` uploads the installer + update manifest (`latest.yml` / `latest-mac.yml`) directly to the GitHub release
  - `release` job simplified: promotes the draft release (`gh release edit --draft=false`) after both builds succeed
  - `latest.yml` and `latest-mac.yml` are now part of every release; `electron-updater` reads these to detect new versions

### Task 6.3 — Windows Code Signing ✅
- **`electron-builder.yml`** — added `signingHashAlgorithms: ['sha256']` to the `win` section; added comments documenting the two required secrets (`WIN_CSC_LINK`, `WIN_CSC_KEY_PASSWORD`) and graceful unsigned fallback
- **`.github/workflows/release.yml`** — `WIN_CSC_LINK` and `WIN_CSC_KEY_PASSWORD` secrets are now forwarded to the `electron-builder` packaging step; when both secrets are present, the installer and its NSIS stub are SHA-256 signed, suppressing Windows SmartScreen warnings; when absent the build succeeds unsigned (no CI failure)
- Electron Forge migration evaluated and rejected — `electron-builder` already supports Windows signing natively via env vars; no toolchain change needed
- To activate signing: export your PFX as base64 (`openssl base64 -in cert.pfx | tr -d '\n'`) and add `WIN_CSC_LINK` + `WIN_CSC_KEY_PASSWORD` as GitHub repository secrets under Settings → Secrets → Actions

## Phase 7 — Spell Timer Engine

### Task 7.1 — Spell Timer Engine (Backend)

**`backend/internal/spelltimer/`** — new package

- **`models.go`** — data types:
  - `Category` string type with constants: `buff`, `debuff`, `mez`, `dot`, `stun`
  - `ActiveTimer` — one live spell timer: `ID` (spell name key), `SpellName`, `SpellID`, `Category`, `CastAt`, `StartsAt` (cast_at + cast_time_ms), `ExpiresAt`, `DurationSeconds`, `RemainingSeconds`
  - `TimerState` — full broadcast payload: `Timers []ActiveTimer` sorted by remaining time ascending, `LastUpdated`
  - Constants: `WSEventTimers = "overlay:timers"`, `eqTickSeconds = 6.0`, `defaultCasterLevel = 60`

- **`duration.go`** — EQ spell duration formula engine:
  - `CalcDurationTicks(formula, base, level int) int` — implements EQEmu's `CalcBuffDuration_formula` for the 13 known formula codes (0–11, 50, 3600) used in classic-era EQ; returns tick count (multiply by 6 for seconds); formula 0 and 3600 return 0 (instant/no timer)

- **`engine.go`** — the timer engine:
  - `Engine` struct: `hub *ws.Hub`, `db *db.DB`, `mu sync.Mutex`, `timers map[string]*ActiveTimer` (keyed by spell name — one timer per spell, recasting refreshes)
  - `NewEngine(hub, db) *Engine`
  - `Start(ctx) ` — background goroutine that ticks every second: prunes expired timers (silently) and broadcasts current `TimerState`
  - `Handle(ev LogEvent)` — routes log events:
    - `EventSpellCast` → DB lookup by spell name, `CalcDurationTicks`, compute `StartsAt = CastAt + CastTime_ms`, `ExpiresAt = StartsAt + duration`; upserts timer and broadcasts
    - `EventSpellInterrupt` → removes timer by spell name if named interrupt (e.g. "Your Mesmerization spell is interrupted.")
    - `EventSpellResist` → removes timer (spell was resisted, never landed)
    - `EventSpellFade` → removes timer (personal fade: "Your X spell has worn off.")
    - `EventSpellFadeFrom` → removes timer by spell name (target fade: "X effect fades from Name.")
    - `EventZone`, `EventDeath` → clears all timers and broadcasts
  - `GetState() TimerState` — point-in-time snapshot for REST API
  - `categorize(*db.Spell) Category` — classifies spell: effect 18 → mez; effect 23 → stun; effect 0 with negative base value → dot; target type 3/6/10/41 → buff; otherwise → debuff

**`backend/internal/db/queries.go`**
- Added `GetSpellByExactName(name string) (*Spell, error)` — case-insensitive exact match on `spells_new.name`, returns nil when not found (no error)

**`backend/internal/api/timers.go`** — new handler
- `timerHandler{engine *spelltimer.Engine}` — `state` handles `GET /api/overlay/timers`

**`backend/internal/api/router.go`**
- `NewRouter` signature extended with `timerEngine *spelltimer.Engine`
- Route added: `GET /api/overlay/timers`

**`backend/cmd/server/main.go`**
- `spelltimer.NewEngine(hub, database)` created after hub, before tailer
- `go timerEngine.Start(ctx)` launched
- `timerEngine.Handle(ev)` added to the log event dispatch function

**`backend/internal/spelltimer/duration_test.go`** — 13 table-driven test cases covering all formula branches, cap behaviour, and the level-0 guard

WebSocket event `overlay:timers` is broadcast on every timer change (cast, resist, fade, zone, death) and once per second from the background ticker.

### Task 7.2 — Timer Overlay (Frontend) / Task 7.3 — Buff & Detrimental Windows

Two separate overlay windows are provided from the start — one for beneficial spells, one for detrimental spells — rather than a single combined window.

**`frontend/src/types/timer.ts`** — TypeScript types mirroring Go models
- `TimerCategory` string union: `'buff' | 'debuff' | 'mez' | 'dot' | 'stun'`
- `ActiveTimer` — mirrors Go `ActiveTimer` struct with all fields
- `TimerState` — mirrors Go `TimerState` struct

**`frontend/src/services/api.ts`**
- Added `getTimerState()` — `GET /api/overlay/timers`

**`frontend/src/pages/SpellTimerPage.tsx`** — in-app page with two floating draggable/resizable `OverlayWindow` panels:
- **Buffs panel** — shows `buff` category timers; default position top-left (24, 24); pop-out button opens standalone buff overlay window
- **Detrimental panel** — shows `debuff`, `dot`, `mez`, `stun` timers; default position top-right (344, 24); pop-out button opens standalone detrimental overlay window
- Each row: spell name, remaining time countdown, depleting progress bar; bar color shifts green → orange → red as time runs low (< 50% / < 20%)
- Detrimental rows have a color-coded left accent line and category badge (DoT, Mez, Stun, Debuff)
- Empty state: icon + "No active buffs" / "No active detrimentals"
- Shared log-status status bar on the buff panel

**`frontend/src/pages/BuffTimerWindowPage.tsx`** — standalone transparent always-on-top buff overlay
- Route: `/buff-timer-window`; Electron window: 280×380, transparent, frameless, alwaysOnTop
- Shows `buff` category timers sorted by remaining time ascending
- Drag handle header with timer count; close button

**`frontend/src/pages/DetrimTimerWindowPage.tsx`** — standalone transparent always-on-top detrimental overlay
- Route: `/detrim-timer-window`; Electron window: 300×320, transparent, frameless, alwaysOnTop
- Shows `debuff`, `dot`, `mez`, `stun` timers sorted by remaining time ascending
- Color-coded left accent lines and category badges per row

**`electron/main/index.ts`**
- `createBuffTimerOverlay()` — 280×380 transparent frameless always-on-top window; route `#/buff-timer-window`
- `createDetrimTimerOverlay()` — 300×320 transparent frameless always-on-top window; route `#/detrim-timer-window`
- IPC handlers: `overlay:bufftimer:open/close/toggle`, `overlay:detrimtimer:open/close/toggle`

**`electron/preload/index.ts`**
- Exposed new methods: `openBuffTimer`, `closeBuffTimer`, `toggleBuffTimer`, `openDetrimTimer`, `closeDetrimTimer`, `toggleDetrimTimer`

**`frontend/src/types/electron.d.ts`**
- Added six new overlay methods to `ElectronAPI.overlay`

**`frontend/src/App.tsx`**
- Routes added: `/buff-timer-window`, `/detrim-timer-window`, `/spell-timers`

**`frontend/src/components/Sidebar.tsx`**
- Added "Spell Timers" nav item (Timer icon) between DPS Overlay and Combat Log under Parsing section

## Phase 8 — Custom Trigger System

### Task 8.1 — Trigger System (Backend) ✅

**`backend/internal/trigger/models.go`**
- `Trigger` struct: ID, Name, Enabled, Pattern (regex), Actions (JSON), PackName, CreatedAt
- `Action` struct: Type (`overlay_text`), Text, DurationSecs, Color
- `TriggerFired` struct: TriggerID, TriggerName, MatchedLine, Actions, FiredAt — used as WebSocket payload and history entry
- `TriggerPack` struct for import/export
- `WSEventTriggerFired = "trigger:fired"` WebSocket event constant

**`backend/internal/trigger/store.go`**
- SQLite persistence in `~/.pq-companion/user.db` (separate connection from backup store, WAL-safe)
- `triggers` table: id, name, enabled, pattern, actions (JSON), pack_name, created_at
- Full CRUD: `Insert`, `Get`, `List`, `Update`, `Delete`, `DeleteByPack`
- Schema migration via `migrate()` using `CREATE TABLE IF NOT EXISTS`

**`backend/internal/trigger/engine.go`**
- `Engine` compiles trigger patterns on `Reload()` and matches every incoming raw log line
- `Handle(timestamp, message)` tests all enabled triggers; fires `trigger:fired` WebSocket event on match
- In-memory ring buffer history (last 200 entries); `GetHistory()` returns a copy
- Invalid regex patterns are skipped with a warning log; engine remains operational
- `NewID()` exported for use by API handlers

**`backend/internal/trigger/packs.go`**
- **Enchanter Pack**: Mez Worn Off, Mez Resisted, Charm Broke, Spell Interrupted — all with colored overlay text
- **Group Awareness Pack**: Incoming Tell, You Died, Group Member Died
- `AllPacks()` returns all built-in packs; `InstallPack(store, pack)` replaces existing triggers for a pack and assigns fresh IDs

**`backend/internal/logparser/parser.go`**
- Added `ParseRawLine(line string) (time.Time, string, bool)` — extracts timestamp and message from any valid EQ log line without classifying the event type, used by the trigger engine to match against all log lines

**`backend/internal/logparser/tailer.go`**
- `NewTailer` now accepts an optional `lineHandler func(time.Time, string)` parameter
- `parseChunk` returns `([]LogEvent, []rawLine)` — raw lines (valid EQ timestamp, any content) are fed to the trigger engine before classified events are dispatched
- `rawLine` struct carries the parsed timestamp and message text

**`backend/internal/api/triggers.go`**
- `GET /api/triggers` — list all triggers
- `POST /api/triggers` — create a trigger (name + pattern required)
- `PUT /api/triggers/{id}` — update an existing trigger
- `DELETE /api/triggers/{id}` — delete a trigger
- `GET /api/triggers/history` — recent firing history (in-memory, last 200)
- `POST /api/triggers/import` — import a JSON trigger pack (replaces existing for same pack_name)
- `GET /api/triggers/export` — export all triggers as a JSON pack
- `GET /api/triggers/packs` — list available built-in packs
- `POST /api/triggers/packs/{name}` — install a built-in pack by name
- All mutations call `engine.Reload()` to keep the engine in sync

**`backend/internal/api/router.go`**
- Added `/api/triggers` route group wired to `triggerHandler`
- `NewRouter` signature extended with `triggerStore` and `triggerEngine` parameters

**`backend/cmd/server/main.go`**
- Opens `trigger.Store` against `~/.pq-companion/user.db`
- Creates `trigger.Engine`, calls `Reload()` at startup
- Passes `triggerEngine.Handle` as the raw line handler to `logparser.NewTailer`

**Tests** (`backend/internal/trigger/engine_test.go`) — 7 table-driven tests:
- Engine fires on matching line, suppresses non-matching lines
- Disabled triggers never fire
- `Reload()` picks up enable/disable changes mid-session
- History ring buffer caps at 200 entries
- Store CRUD round-trip with action JSON serialisation
- `ErrNotFound` on get/update/delete of missing ID
- `InstallPack` replaces rather than duplicates on re-install

### Task 8.2 — Trigger Manager UI ✅

**`frontend/src/types/trigger.ts`**
- `Trigger`, `Action`, `TriggerFired`, `TriggerPack` TypeScript types mirroring Go structs

**`frontend/src/services/api.ts`**
- `listTriggers`, `createTrigger`, `updateTrigger`, `deleteTrigger` — CRUD
- `getTriggerHistory` — recent firing events
- `getBuiltinPacks`, `installBuiltinPack` — built-in pack management
- `importTriggerPack`, `exportTriggerPack` — import/export

**`frontend/src/pages/TriggersPage.tsx`** — three-tab interface:

*Triggers tab:*
- Lists all triggers with inline enable/disable toggle (PUT on change, no reload)
- Expand button shows action details (text, color swatch, duration)
- Edit (Pencil) opens inline `TriggerForm` replacing the row
- Delete (Trash) shows inline confirmation before calling API
- "New Trigger" button shows `TriggerForm` at top of list

*TriggerForm:*
- Name field, regex pattern field with live client-side validation (shows error for invalid regex)
- Enabled toggle in header
- Action list: each action has text input, numeric duration input, color picker; add/remove actions
- Create calls POST; edit calls PUT; both call `engine.Reload()` on backend

*History tab:*
- Loads initial history from `GET /api/triggers/history` (newest first)
- Subscribes to `trigger:fired` WebSocket events and prepends new entries live
- Each entry shows trigger name, action overlay text badges (colored), matched log line, timestamp

*Packs tab:*
- Import/Export section: Export All (downloads JSON), Import Pack (file picker, JSON upload)
- Built-in Packs section: shows all available packs with description, trigger count, and Install button
- Install replaces existing pack triggers; shows "Installed" confirmation with checkmark for 3 s

**`frontend/src/components/Sidebar.tsx`**
- Added "Triggers" nav item (Zap icon) to the Parsing section

**`frontend/src/App.tsx`**
- Added `/triggers` route mapped to `TriggersPage`

### Task 8.3 — Trigger Overlay ✅

**`frontend/src/pages/TriggerOverlayWindowPage.tsx`**
- Transparent, always-on-top, frameless overlay window for trigger alert display
- Subscribes to `trigger:fired` WebSocket events; only shows alerts with an `overlay_text` action
- Each alert auto-dismisses after its configured `duration_secs`; fades out in the last 500 ms
- Up to 8 alerts visible simultaneously, newest on top (older alerts pushed down)
- Alert card: large bold text in the action's configured color with matching glow shadow; truncated matched log line shown below in muted monospace
- Garbage collection timer (250 ms) prunes expired+faded entries from state
- Drag handle at top allows repositioning; close button sends `overlay:trigger:close` IPC
- Background: nearly transparent when empty (alert window doesn't block the game UI), semi-opaque when alerts are present

**`electron/main/index.ts`**
- Added `triggerOverlayWindow` variable and `createTriggerOverlay()` function
- Window: 340×360 px, transparent, frameless, always-on-top (`screen-saver` level), `skipTaskbar`, `visibleOnAllWorkspaces`
- IPC handlers: `overlay:trigger:open`, `overlay:trigger:close`, `overlay:trigger:toggle`

**`electron/preload/index.ts`**
- Added `openTrigger`, `closeTrigger`, `toggleTrigger` to the `overlay` bridge

**`frontend/src/types/electron.d.ts`**
- Added `openTrigger`, `closeTrigger`, `toggleTrigger` to `ElectronAPI.overlay`

**`frontend/src/pages/TriggersPage.tsx`**
- Added "Overlay" button (MonitorPlay icon) in the page header that calls `window.electron?.overlay?.toggleTrigger()` — present on all tabs

### Task 8.4 — Settings Tab Redesign ✅

**`frontend/src/pages/SettingsPage.tsx`**
- Added **App** section at the top: displays current app version (read via `app:version` IPC from Electron `app.getVersion()`) and a **Check for Updates** button
- Update button drives a state machine: `idle → checking → up-to-date / available → downloading → downloaded` — shows inline feedback and an "Install & Restart" button when a download is ready
- Removed **Overlays** section (DPS/HPS toggle switches) — overlay state now lives on each overlay's own controls, removing redundancy and confusion
- Kept: EverQuest Installation, Character, Preferences sections unchanged

### Issue #62 — Overlay Transparency Control ✅

**`frontend/src/hooks/useOverlayOpacity.ts`** (new)
- Custom hook that reads `preferences.overlay_opacity` from `GET /api/config` on mount and re-polls every 3 s so overlay windows pick up changes without requiring a restart

**`frontend/src/pages/SettingsPage.tsx`**
- Added **Overlays** section between Preferences and Save/Discard buttons
- `<input type="range">` slider (10–100%) controls `preferences.overlay_opacity`; live percentage readout updates beside the slider label
- Preview swatch (`rgba(10,10,12,{opacity})`) shows the resulting overlay background colour in real-time as the slider moves
- Value is persisted via the existing Save flow

**Overlay window pages** (DPS, HPS, BuffTimer, DetrimTimer, NPC, Trigger)
- Each calls `useOverlayOpacity()` and uses the returned value as the alpha channel of the root container's `backgroundColor` (`rgba(10,10,12,{opacity})`)
- `TriggerOverlayWindowPage`: drag-handle alpha and `AlertCard` background alpha scale proportionally with the configured opacity

**`electron/main/index.ts`**
- Added `ipcMain.handle('app:version', () => app.getVersion())` IPC handler

**`electron/preload/index.ts`**
- Exposed `app.getVersion` bridge to renderer

**`frontend/src/types/electron.d.ts`**
- Added `app: { getVersion: () => Promise<string> }` to `ElectronAPI`

### Issue #72 — Auto-detect active character from log file activity ✅

**`backend/internal/logparser/tailer.go`**
- Added `onCharacterChange func(string)` field to `Tailer` — called when the auto-detected active character changes
- Added `detectedCharacter string` field to track the last auto-detected character name (empty when character is set manually in config)
- Updated `NewTailer` to accept an `onCharacterChange` callback parameter
- In `tick()`, when `config.Character` is blank, the resolved character is compared against `detectedCharacter`; if it changed the callback fires and `detectedCharacter` is updated; when a manual character override is set `detectedCharacter` is cleared

**`backend/cmd/server/main.go`**
- Passes an `onCharacterChange` callback to `NewTailer` that logs the detection and broadcasts a `config:character_detected` WebSocket event with `{character: "<name>"}` payload

**`frontend/src/pages/SettingsPage.tsx`**
- Subscribes to `config:character_detected` WebSocket events via `useWebSocket`
- When the character field is blank and a character is detected, shows a muted banner below the input: "Auto-detected: **Firiona**" with a **Use This** button that copies the name into the character field
- Banner dismisses automatically when the character field is manually filled

### Issue #49 — Copy DPS Summary to Clipboard ✅

**`frontend/src/pages/CombatLogPage.tsx`**
- Added `Clipboard` / `ClipboardCheck` icon imports from lucide-react
- Added `buildFightText(fight)` — formats a fight into EQ-chat-safe lines: header `[PQ Companion] Fight: <target> (<duration>)` followed by `<name>: X.X DPS (N total)` per combatant
- Added `buildSessionText(fights, sessionDPS)` — formats a one-liner session summary with fight count and session average DPS
- `FightRow`: converted summary row from `<button>` to `<div>` with `onClick`; added a 7th grid column (24px) for a per-row clipboard icon button; button flips to `ClipboardCheck` (green) for 1.5 s after a successful copy
- `TableHeader`: added matching 7th column header (blank) to keep grid alignment
- `FilterBar`: added `onCopySession` / `sessionCopied` props; added "Copy" button (clipboard icon + label) to the right-side action group; flips to `ClipboardCheck` green for 1.5 s after copy

**`frontend/src/pages/DPSOverlayPage.tsx`**
- Added `buildFightText(fight)` helper (same format as above, operates on `FightState`)
- Added `CopyFightButton` component — clipboard icon button; disabled and faded when no active fight; toggles to green `ClipboardCheck` for 1.5 s on copy
- `CopyFightButton` placed in the DPS Meter `headerRight` between the All/Me toggle and the pop-out button; copies `combat.current_fight` data

**`frontend/src/pages/DPSOverlayWindowPage.tsx`**
- Added `buildFightText(fight)` helper for the floating overlay context
- Added `copied` state; clipboard icon button in the no-drag controls zone (between All/Me toggle and close ×); disabled and dimmed when no fight is active; green for 1.5 s on copy

### Issue #70 — Spell/Caster DPS Not Tracked ✅

**`backend/internal/logparser/parser.go`**
- Added `reTargetHitNonMelee` regex — matches `"<target> was hit by non-melee for <N> points of damage."` (the passive form EQ logs when the player's own spell damages a target); emits `EventCombatHit` with `Actor: "You"`, `Skill: "spell"`, and the target/damage extracted from the match
- Added `reNonMeleeHit` regex — matches `"<Actor> hit <Target> for <N> points of non-melee damage."` (the active form used for other players' and NPCs' spell damage, including multi-word actor names like `"A Shissar Arch Arcanist"`); emits `EventCombatHit` with `Skill: "spell"`
- Both patterns inserted in `classifyMessage` before `reNPCHitYou` and `reThirdPartyHit` so they take priority over melee patterns; non-melee hits now flow through the existing combat tracker logic and appear in DPS totals
- **`parser_test.go`** — 5 new table-driven test cases: passive player spell hit (single-word target), passive player spell hit (multi-word target), third-party caster hit, multi-word NPC spell hit (A Shissar Arch Arcanist), and NPC self-damage via spell

## Phase 9 — Audio Alerts

### Task 9.1 — Audio Engine

Extends the trigger system with two new action types — `play_sound` and `text_to_speech` — and wires up a frontend audio engine that fires them whenever a trigger matches a log line.

**`backend/internal/trigger/models.go`**
- Added `ActionPlaySound ActionType = "play_sound"` — plays a local audio file
- Added `ActionTextToSpeech ActionType = "text_to_speech"` — speaks text via TTS
- Added fields to `Action`: `SoundPath string`, `Volume float64` (0.0–1.0), `Voice string` (TTS voice name)

**`frontend/src/types/trigger.ts`**
- Extended `ActionType` union: `'overlay_text' | 'play_sound' | 'text_to_speech'`
- Added `sound_path`, `volume`, `voice` fields to `Action`

**`frontend/src/services/audio.ts`** _(new)_
- `playSound(filePath, volume)` — plays a local file via the HTML5 `Audio` constructor with `file://` URL normalisation (Windows back-slash safe); silently ignores playback errors
- `speakText(text, voice, volume)` — speaks via `window.speechSynthesis`; cancels any queued utterances before speaking to prevent pile-up; matches voice by name against `getVoices()`
- `getAvailableVoices()` — returns sorted list of available TTS voice names for the UI

**`frontend/src/hooks/useAudioEngine.ts`** _(new)_
- Subscribes to the singleton WebSocket connection
- On every `trigger:fired` event, iterates the fired actions and dispatches `play_sound` actions to `playSound()` and `text_to_speech` actions to `speakText()`
- Designed to be mounted once at the App level so audio fires regardless of active page

**`frontend/src/App.tsx`**
- Calls `useAudioEngine()` at the top of the App component — one mount, always active

**`frontend/src/pages/TriggersPage.tsx`**
- `ActionEditor` now renders a type dropdown (`overlay_text` / `play_sound` / `text_to_speech`)
- `play_sound`: sound file path input + volume slider (0–100%)
- `text_to_speech`: text input + voice dropdown (populated from `getAvailableVoices()`, fallback to free-text input) + volume slider
- All new action types default their fields (empty path/text, 0 volume = 100%, empty voice = system default)

### Task 9.2 — Timer Audio Alerts

Adds configurable audio alerts that fire whenever an active spell timer's remaining time crosses a user-defined threshold. Alerts are fully independent of the trigger system — they operate directly on `overlay:timers` WebSocket events.

**`frontend/src/types/timerAlerts.ts`** _(new)_
- `TimerAlertType` — `'play_sound' | 'text_to_speech'`
- `TimerAlertThreshold` — one configured alert: `id`, `seconds` (fire when remaining ≤ this), `type`, `sound_path`, `volume`, `tts_template` (supports `{spell}` placeholder), `voice`, `tts_volume`
- `TimerAlertConfig` — top-level config: `enabled` flag + `thresholds[]`

**`frontend/src/services/timerAlertStore.ts`** _(new)_
- `loadTimerAlertConfig()` — reads from `localStorage` key `pq-timer-alerts`; returns a built-in default (30s TTS alert) on first run
- `saveTimerAlertConfig(cfg)` — serialises config to `localStorage`; silently ignores quota errors

**`frontend/src/hooks/useTimerAlerts.ts`** _(new)_
- Subscribes to `overlay:timers` WebSocket events
- Tracks `prevRemaining: Map<timerId, number>` across renders via `useRef`
- Each update: for each timer × threshold pair, if `prev > threshold.seconds && remaining ≤ threshold.seconds` → fire `playSound()` or `speakText()` with `{spell}` interpolated
- Cleans up stale timer entries when they expire or are removed
- Reads config fresh on every tick (picks up changes instantly without requiring a remount)

**`frontend/src/components/TimerAlertsPanel.tsx`** _(new)_
- Slide-in panel (right side of SpellTimerPage, 380 px wide) for managing alert thresholds
- Global enable/disable toggle
- Per-threshold row: seconds input, type selector (`text_to_speech` / `play_sound`), and type-specific fields:
  - TTS: message template input, voice dropdown (populated from `speechSynthesis.getVoices()`), volume %
  - Sound: file path input, volume %
- Add / remove threshold buttons; changes save to localStorage immediately on every edit

**`frontend/src/pages/SpellTimerPage.tsx`**
- Added Bell icon button in the Buffs panel header that toggles `TimerAlertsPanel` open/closed; icon tinted with `--color-primary` when panel is open
- `TimerAlertsPanel` is rendered inside the page container so it overlays the timer panels without affecting the overlay window positions

**`frontend/src/App.tsx`**
- Calls `useTimerAlerts()` alongside `useAudioEngine()` at the App root — fires alerts regardless of active page

### Task 9.3 — Event Notifications

Audio alerts (TTS or sound file) for important game events parsed from the EQ log. Fires independently of the trigger system — these are always-on, low-config alerts for high-signal events.

**Supported events:**
- `log:death` — player death; supports `{slain_by}` placeholder
- `log:zone` — zone change; supports `{zone}` placeholder
- `log:spell_resist` — spell resisted by target; supports `{spell}` placeholder
- `log:spell_interrupt` — spell cast interrupted; supports `{spell}` placeholder

Combat hit/miss events are intentionally excluded — too frequent to be useful as audio alerts.

**`frontend/src/types/eventAlerts.ts`** _(new)_
- `AlertableEventType` — union of the four supported log event types
- `EventAlertRule` — per-event config: `enabled`, `type` (play_sound | text_to_speech), `sound_path`, `volume`, `tts_template`, `voice`, `tts_volume`
- `EventAlertConfig` — global `enabled` flag + array of `EventAlertRule`

**`frontend/src/services/eventAlertStore.ts`** _(new)_
- `loadEventAlertConfig()` / `saveEventAlertConfig()` — localStorage persistence under `pq-event-alerts`
- Ships with four default rules (all TTS, all enabled): death → "You have died", zone → "Entering {zone}", resist → "{spell} resisted", interrupt → "Spell interrupted"

**`frontend/src/hooks/useEventAlerts.ts`** _(new)_
- Subscribes to WebSocket messages via `useWebSocket`
- Filters to the four alertable event types; reads config fresh from localStorage on each event
- Builds per-event template variables from the typed payload (`ZoneData`, `DeathData`, etc.)
- Calls `playSound()` or `speakText()` with substituted text and normalised volume (0–100 → 0.0–1.0)

**`frontend/src/components/EventAlertsPanel.tsx`** _(new)_
- Slide-in panel (same style as `TimerAlertsPanel`): 380 px wide, anchored right, z-index 10
- Global enable/disable toggle at the top
- One `RuleRow` per supported event type, always displayed in fixed order (death, zone, resist, interrupt)
- Each row: event label + description, enabled toggle, alert type selector, TTS or sound-file fields, placeholder hint
- Missing rules (e.g. after first load with no stored config) are synthesised as disabled placeholders and merged into config on first edit
- Changes persist to localStorage immediately on every input

**`frontend/src/pages/LogFeedPage.tsx`**
- Added "Alerts" button (Bell icon) in the header toolbar; tinted primary when panel is open
- Renders `EventAlertsPanel` as an absolute-positioned overlay inside the page container when open

**`frontend/src/App.tsx`**
- Calls `useEventAlerts()` at the App root alongside `useAudioEngine()` and `useTimerAlerts()`

## Phase 10 — Character Tools

### Task 10.1 — Character Todo List
_Planned_

Simple per-character todo list for tracking arbitrary in-game goals. Keeps it minimal by design; complexity added only based on user feedback.

Design notes:
- Each todo item: ID, character name, text (free-form string), checked bool, created_at timestamp
- Items stored in user.db (`todo_items` table)
- Backend: `GET /api/todos/{character}`, `POST /api/todos/{character}`, `PATCH /api/todos/{character}/{id}` (toggle checked), `DELETE /api/todos/{character}/{id}`
- Frontend: character selector (populated from known Zeal export characters), text input + Add button, list of items with checkboxes, delete button per item, optional "hide completed" toggle
- No categories, priorities, or due dates for v1 — just text + checkbox

## v0.1.1 — File Location Fixes

- **Log file path**: Removed `Logs/` subdirectory — EQ log files are in the root of the TAKPv22 game folder (`<eq_path>/eqlog_<CharName>_pq.proj.txt`)
- **Auto log selection**: When character name is left blank in settings, the backend automatically selects the most recently modified `eqlog_*_pq.proj.txt` file in the EQ folder — no need to configure a character name during normal play. An explicit character name in settings overrides auto-selection (useful for testing/debugging).
- **Zeal export paths**: Updated inventory and spellbook file name formats from `<CharName>_pq.proj-Inventory.txt` / `<CharName>_pq.proj-Spells.txt` to `<CharName>-Inventory.txt` / `<CharName>-Spellbook.txt`, and removed the `Logs/` subdirectory reference
- **Backup location**: Backups now saved to `<eq_path>/backups/` (inside the game folder) instead of `~/.pq-companion/backups/`
- **Version bump**: 0.1.0-beta.1 → 0.1.1

## v0.6.0 — Combat History, Roll Tracker, Combat Polish

### Combat tracking
- DoT-tick and PQ-format crit hits captured by the combat parser
- Per-NPC fight tracking — each engaged NPC is its own fight record rather than one rolling encounter
- Three DPS metrics surfaced side-by-side: **Personal** (you), **Raid** (group/raid total), and **Encounter** (everything including NPCs)
- Per-combatant bar colors in the DPS overlay
- Session-break dividers in the combat strip (120-second gap rule)
- Fixes: single-word boss names (e.g. `Dracoliche`), charmed-pet attribution, Eye-of-Zomm scout naming, 0-damage rows preserved, DPS-mode toggle now updates every aggregate display

### Combat history (persistent)
- SQLite-backed history store for archived fights (`internal/combat`), pruned by retention window
- REST endpoints under `/api/combat/history` for paginated browsing and per-fight detail
- Full-page Combat History view with facet dropdowns (zone, killed-by, target), date presets, themed confirm modal
- Event-based session grouping with a per-page toggle
- FilterBar additions: pets, me-only, and DPS-mode toggles

### Roll tracker (new)
- `/random` parser feeds a roll session store with per-session IDs and remove-from-session support
- Dedicated page under **Parsing** with a session table and dashboard panel on the main view
- Timer-mode auto-stop — start a countdown and the session locks when the timer expires
- Pop-out overlay window with centered empty state

### Spell timers
- Detrimental timers for a target NPC are cleared automatically when that NPC dies (no more orphan debuff bars)

### Zones
- Curated expansion grouping in the zone list
- Graveyard pop-out information surfaced inline on zone detail

### Overlays & UI polish
- Overlay toggles flip the window to click-through; only the header stays clickable for dragging/close
- Buff, detrimental, and roll pop-out windows now center their empty states
- Unified `Trash2` clear icon across overlay windows
- Character switcher: Auto-mode behavior clarified in the picker

### Internals
- WebSocket event-type strings centralized as frontend constants
- Migration definitions tightened; dead helpers and debug logs removed in two cleanup passes

## v0.6.1 — Backend Port Resilience

- Backend server tries the preferred port first and falls back to an OS-assigned port if it's taken (`internal/server`), so the app survives a stale sidecar or unrelated process on 8080
- Sidecar lifecycle plumbs the chosen port from the Go process to the Electron main process and on to the renderer — no hardcoded port in the React client
- Settings gains a **Backend Network** section: shows the live port, a "Test connection" action, and a reset control

## v0.6.2 — Build & Settings Polish

- `dist:win` now rebuilds the Go backend as part of the Windows packaging pipeline so installers can never ship a stale sidecar binary
- Server binds explicitly to `127.0.0.1` (instead of `0.0.0.0`) so port-conflict detection is reliable on Windows
- Settings unsaved-change banner with inline Save for port edits — no more silent reverts when the user navigates away mid-edit

## v0.6.3 — Dev-Mode Robustness

- Dev mode writes the backend port to a small handshake file so the Vite-served renderer can discover it on first paint (no race against the WebSocket handshake)
- Frontend targets `127.0.0.1` explicitly instead of `localhost` to dodge IPv6/IPv4 resolution drift on Windows
- Settings page recovers gracefully from a backend-unreachable error instead of locking the UI

## v0.6.4 — Buff Math, Item Links, Search Pagination

### Character & buff math
- **Bard exemption**: bards skip AA + item duration extensions on songs (matches Quarm/EQMacEmu behavior). Unit-tested with both in-class and off-class spells
- **Spell haste cap**: SPA 127 effective value clamped to 50% even when raw items+AAs sum higher; surfaced on the character stats page
- **Clicky duration gating**: item click effects only get AA/item duration boosts when the wielder can actually cast the spell on-class (`ClassLevels[class] < 255`); off-class clickies use base duration

### Item & search UX
- Item detail pages generate Mac-era `itemlink:` URLs (Project Quarm format) instead of modern EQ ones
- Item, spell, and NPC search lists paginate with a "Show more" button instead of dumping unbounded results
- Bard trigger pack ships with a Shroud of Stealth clicky trigger out of the box

### Startup
- App detects a missing or unreadable `quarm.db` at launch and walks the user through manual repair (download from `data-latest`) instead of failing silently
- CSP allows dynamic backend ports on both `localhost` and `127.0.0.1` so the renderer can talk to whichever fallback port the sidecar landed on

## v0.7.0 — ZealPipes Integration, Players Page, Spellsets, App Backup

This is the largest release since v0.6.0 — it pulls in real-time Zeal data over Windows named pipes, adds three brand-new top-level features (Players, Spellsets, App Backup), and substantially polishes the database explorer and NPC overlay.

### ZealPipes integration (live IPC with Zeal)
Real-time data from Zeal over Windows named pipes — replaces the file-export-only model where it can, without removing the file-based fallback.

- **Schema & envelope decoder** (`internal/zealpipe`): typed envelope decoder with stream-decoding via `json.Decoder` (Zeal writes have no inter-envelope delimiter), correct handling of the double-encoded `data` field, and Windows `winio` dialer using `GENERIC_READ` only
- **Discovery & supervisor**: pipe-namespace auto-discovery, supervisor loop with reconnect, `/api/zeal/status` endpoint surfacing live runtime state
- **Detection & onboarding**: `/api/zeal/detect` detects Zeal install; setup wizard shows live detection; first-connect toast guides users into the flow
- **Settings**: dedicated **Zeal integration** section with live pipe status (WebSocket-driven, updates without reload)
- **Stage A — target labels**: forward Zeal target labels to the NPC overlay
- **Stage B — overlay polish**: live target HP bar driven by pipe; pet-owner badge on NPC overlay
- **Stage C — combat attribution**: pipe-driven target and pet attribution for the combat parser (no more guessing pet owners from name conventions)
- **Stage D — spell observability**: pipe-driven cast and buff events for the spell timer engine, including divergence logging when pipe and log disagree
- **Stage E — pipe-source triggers**: triggers can fire on pipe events, not just log lines; editor UI exposes pipe sources
- **Stage F — first-connect polish**: toast notifications, setup-wizard wiring, settings live status

### Players (new)
A `/who`-driven sightings database. Every `/who` you run captures who you saw, when, where, and at what level — and the dataset is searchable across all your characters.

- `/who` and `/guildstat` parsers feed a `players` table in user.db
- Capture: name, level, class (with title alias mapping), guild, race, zone (from `/who` summary), first-seen / last-seen timestamps
- Players page with search, sortable column headers (level, last seen, etc.), guild filter, class title alias filter, "Clear filters" reset
- Selected-player info panel pins on scroll; level history tracked over time
- Themed Clear-All confirmation dialog

### Spellsets (new)
Parse and edit Zeal `_spellsets.ini` exports.

- Backend parser exposes spellsets via API; off-class spell resolution
- Spellsets tab with a read-only viewer, then writer with save/cancel
- Fluid card grid (280px cards), vertically stacked gem slots, inline rename
- Add and remove spellset cards
- Import another character's spellsets from their `.ini` file
- Import blocks ineligible spells (wrong class, too high a level, or not in this character's spellbook)

### App Backup & Restore (new)
- Export the entire app state (settings, triggers, custom trigger packs, etc.) as a single bundle file; restore on another machine or after a reinstall
- Always-visible **App Backup & Restore** section in Settings (separate from the EQ `.ini` config backup manager)
- IPC wiring between renderer and main for export/import file dialogs

### Database explorer
- **Raw Data modal** on item, spell, NPC, and zone detail views — exposes every column for power users; flow fields render column-major to match the pqdi.cc layout
- Item-type labels realigned to the EQMacEmu enum (Quarm's item-type numbering diverges from modern EQEmu starting at 34); type 45 correctly labeled "Hand to Hand"
- Class labels for trainer GMs, bankers, and merchants in NPC detail
- VT Lucid Shard correctly assigned to Ssraeshza Temple (was Grieg's End)

### NPC info overlay
- Loot table toggle with drop rates baked in
- Tightened padding and layout in the popped-out window
- `Atk` field renamed to `Atk/Rd` with a sensible default (1) when unknown
- Dubious-faction con message ("looks at you") now parsed

### Combat & triggers
- Charm pet damage now attributed to the player when the pet name matches the current target (fixes missing damage on enchanter charm pets)
- Cooldown timers for disciplines and long-recast spells in the trigger system

### Settings refactor
- Settings reorganized into 6 category tabs with clearer backup naming
- Confirmation modals trimmed across overlays and spellsets where they were redundant

## v0.7.1 — DPS Bar Theming

- Per-class bar colors in the DPS overlay (sourced from the Settings palette) — at-a-glance class identification across your group/raid

## v0.7.2 — Spell Timer & Log Parser Stability

- **Logparser camp/login follow**: the parser now follows the in-game character across `/camp` → character-select → re-login without losing state, so spell timers and combat tracking survive a swap
- **Spell timer dedup**: zealpipe-divergence log lines are deduped across pulses — no more log-spam when the pipe and log disagree on a long buff

## v0.8.0 — Wishlist, Keyring Tracker, Live Buffs, Canonical Enum Catalog

- **Wishlist** — per-character wishlist tab with drag-reorder; star button on Items DB rows toggles entries on the active character's list
- **Keyring tracker** — per-character `/keys` snapshot ingested from the log; inventory fallback covers keys not yet added to the in-game keyring (Plane of Time, Vex Thal, Grieg's End)
- **Live Buffs in Character Stats** — buffs panel is now driven by the spell timer engine with swappable raid-buff presets per character; confirm-on-edit and remove-buff actions; v1 worn haste recomputed from item wornlevel + spell formula
- **Canonical enum catalog** — every game-data label (item types, NPC classes/races, zones, slot/race/class bitmasks, bane types, body types, expansions, special abilities, tradeskills, spell target types) now lives in a single Go catalog served via `/api/enums`. Frontend consumes the same source, eliminating drift. Includes an `enum-audit` CLI for label verification. Corrected: NPC body types, post-PoP race/class hiding, Quarm-specific SPAs 160 + 500–504, spell target types 0/9/17
- **Vex Thal zone-wide loot overlay** — zone-wide drops show on every NPC in the zone, targeted by lootdrop id with pool names
- **Trigger packs** — full discipline coverage across all melee classes; `dedup_key` field for cross-pack shared triggers; pack-grouped UI with collapsibles, filter, sort, and shared badge; Global Alerts folded into General Triggers; Gift of Brilliance added to Enchanter pack
- **Zeal version warning** when installed Zeal is below the minimum supported version (detected via crash-handler anchor)
- **NPC overlay** resolves corpse targets to the underlying NPC and humanizes loot-table headings
- **Inventory** hide-empty-bags toggle
- **Zones** show bard swarm/warp limits on the zone overview
- **Logging** mirrors backend slog to `~/.pq-companion/logs/server.log` and Electron + sidecar stdio to `electron.log` (3-session rotation)

## v0.8.1 — Wishlist UX Overhaul

- **Wishlist sections are collapsible cards** — toggle via the `+`/`−` button or by clicking the header; the layout (card order + collapse state) persists per character
- **Drag-reorder slot cards** — grip on the left of each card header reorders the section list
- **Expand all / Collapse all** toolbar control
- **All Items view** — flat list of every wishlisted item across slots with free-form cross-slot drag; toggle Category / All items from the toolbar. A single global ordering backs both views so reorders in one are reflected in the other
- Character race displays "Iksar" / "Vah Shir" / "Froglok" instead of `Race 128` and other post-Kunark race IDs

## v0.8.2 — Developer Mode, Key Tracker Rewrites, NPC Overlay Polish

- **Developer mode** — hidden Settings tab unlocked by `Ctrl+Shift+D`. Includes a guarded read-only SQL sandbox (`/api/sandbox`) with 10 curated starter queries, an interactive schema graph, and a curated Mermaid ER diagrams panel. Full-width layout with sub-tabs and results below the query editor
- **Key tracker quest rewrites** — Howling Stones uses the full Key to Charasis quest; Arx Seru uses the 4-shard Praesertum quest; Veeshan's Peak uses the full Key of Veeshan quest; Sleeper's Tomb accepts any one Velious talisman; Sebilis swaps Trakanon's Tooth for the Trakanon Idol quest. Hand of Glory (Charasis internal doors) added. Grieg's End and Grimling pens dropped from the tracker
- **Keyring** tab is first and default; live-refreshes on `/keys` and infers upgraded stages as owned. Corrected Lucid Shard zone mappings for the Vex Thal key
- **NPC overlay** — max mana shown beside HP when > 0; resists reordered to MR/CR/FR/DR/PR with EQ-convention colour-coding; multi-field `special_abilities` entries (e.g. Rampage range) now parse correctly
- **NPC detail page** — new Spells & Procs section with collapsible long cast-spell lists; Escape and outside-click dismiss ability popovers and modals
- **Zeal** — soft update notice and an `ExportOnCamp` warning when the setting is disabled
- **Settings** — EQ client version status panel (drops the unused `eqw.dll` row)
- **Backup Manager** — Open folder button in the header
- **Spell timers** — mez timer defers rendering until the spell actually lands; NPC names normalized on kill match; unmatched kills logged
- **Quarm client-status** uses `FileVersion` (not MD5) as the primary signal, so patch-day MD5 churn no longer flips users to "unknown"
- **Triggers** — positioning button passive while a session is active; positioning card reliably draggable; removed duplicate generic resist/interrupt rules from the Enchanter pack
- **Spells** — class spell list excludes disciplines and entries above level 60
- **Installer** pinned to per-user install so it stops defaulting to Program Files
- **EQ-config backups** directory moved to the user home with a migration from the legacy location
- `quarm.db` DSN now includes `immutable=1` so Program Files installs work without write access

## v0.10.0 — Lockouts Tracker, Real Stat Formulas, Respawn Timers, Multi-Monitor Overlays

- **Lockouts tracker** — new Lockouts page with live `/sll` countdowns; a `/sll` loot & legacy lockout parser feeds a per-character store exposed via the lockout API
- **Character stats from real formulas** — the `eqstat` package ports Project Quarm's player HP/mana/AC/resist formulas to Go; stats are derived on the backend from per-layer totals (with AA passive stat bonuses resolved from `aa_effects`) and the frontend renders the backend-derived stat blocks instead of approximating
- **NPC respawn timers** — new NPC death/respawn timer overlay; Quarm's fast-respawn reduction is applied to death timers
- **Multi-monitor overlays** — the trigger overlay can span all monitors, and overlay windows can be dragged across monitors via synthetic drag
- **Main window state** — persists size, position, and maximized state between launches
- **Combat tab merge** — Combat Log and History combined into one sidebar tab with sub-tabs; game-generated pet names are attributed to their owner, with pet bindings preserved on clear and multi-mob live encounters merged
- **NPC overlay disambiguation** — same-name NPCs are disambiguated by zone and player position using a captured Zeal player snapshot (`GetNPCVariantsByNameInZone`); the panel renders the variant set for ambiguous targets, and the popped-out window handles `#`-prefixed same-name rows and their special abilities
- **Wishlist on the NPC overlay** — wishlisted drops are highlighted in the loot section; a `wishlist:changed` WS event broadcasts on add/remove
- **Explorer canonical rows** — duplicate-name items and spells collapse to a single canonical row with links to the variants on the detail pages
- **Key tracker** — shows the bag/bank location of held key components
- **Locked overlays interactive on hover** (#127) — locked overlays go fully interactive on hover for scrolling and per-row actions
- Charm spell timers cleared on charm-break, kept when an unrelated mob dies, and no longer spawn phantom duplicates; detrimental timers drop on a Zeal corpse target
- Other players' clickies and NPC self-buffs no longer flood the buff overlay; combined-name pack triggers dedup against spell-landed by `SpellID`
- Character Info resist order matched to the NPC overlay (MR/CR/FR/DR/PR); Defense skill uses the correct `skill_id` for AC
- Debuff `cast_on_other` patterns broadened and bard song durations corrected across all class packs; installed packs migrated to the broadened patterns
- Overlay positions preserved across auto-updates (#126); popout windows defer show until ready-to-show to remove flicker (#125); duplicate WebSocket connections no longer race on mount (#124)
- NPC run speed % and level-scaled movement spell range fixed; run speed now shows on the popped-out overlay
- Clarified confusing "Scheduled" labels in the config backup list

## v0.11.0 — Recipes Browser, Accessibility, Stat Breakdowns

- **Recipes browser** — new Recipes page in the database section backed by tradeskill recipe queries and API; combine-station containers are named instead of labelled "(combine container)", and item detail pages gain a Tradeskills tab listing every ingredient of the recipes an item is used in
- **Favorite recipes** — a global favorite-recipes store and API to pin frequently-used combines
- **Accessibility** — app-wide zoom and a high-contrast text mode in Settings for readability, with the high-contrast preview no longer reverting before save (#130)
- **Spell acquisition sources** — a "Where to get it" button on the spell checklist surfaces where each spell can be obtained, consolidated into a single spell-acquisition view
- **Trigger capture substitution** — regex capture groups can be substituted into trigger action text (#132)
- **Character stat breakdowns** — hover popovers detail the sources of Haste, Spell Haste, Damage Shield, ATK rating, and HP/mana regen (#128), replacing the laggy native-title tooltip
- **Per-overlay lock behaviour** — each overlay's locked-mode behaviour is configurable in the Overlays tab, wired through the lock hook and persisted in config
- Buff durations ported to the EQMacEmu `CalcBuffDuration` formulas, fixing Forlorn Deeds and other spells that showed incorrect durations (#131)
- Zeal `/outputfile` exports recognized in both naming formats (#133)
- Fletching Mastery no longer offered as an AA to non-ranger classes (#134)
- Trigger overlay no longer steals game focus; Set Position is recoverable on multi-monitor setups, with corrected Escape semantics and teardown; Escape bound as a universal modal-close fallback
- Instant-clicky spell timers resolve land collisions via item lookup; self timers clear on your own death
- NPC run-speed percentage corrected to the NPC-scale player base
- Back navigation preserves search and drill-down state and steps through item selections across explorers
- Duplicate-name items collapse in spell cross-references

## v0.12.0 — CH Chain Overlays, Loot & Chat Trackers, Shopping Routes

- **CH Chain & Metronome** — a Complete Heal chain matcher and `ch_chain` timer category (backend) feed a CH Chain overlay window with cast-to-land bars, live measured cadence, and a stall indicator, plus a personal CH metronome overlay; both are first-class dashboard panels with their own Overlays toggles, and the default CH-chain pattern was broadened per user feedback
- **Loot Tracker** (#135) — a dedicated Loot Tracker page logs drops as they happen, with clickable items (detail popup) and zones (jump to the Zone browser)
- **Chat History** (#136) — the Tell Tracker for direct player-to-player tells grew into a full multi-channel Chat History with per-character tabs, chat-style conversation threads, tighter tell bubbles, and a live backfill progress meter
- **Shopping route planner** — pick spells on the checklist and the app plans an efficient vendor shopping run: a pure greedy set-cover solver, a batched `GetSpellVendorOptions` query, a `POST /api/spells/shopping-route` endpoint, distance-aware sourcing, Druid/Wizard teleports modeled as a Nexus hub, a Plane of Knowledge toggle, town exclusion, and alignment/start-zone ordering; route panel controls let you exclude spells, filter towns, and set the start zone
- **NPC caster summary** — NPC pages and the NPC overlay headline an NPC's class, key procs/highlights, and signature spells (collapsing the full list), with clickable proc/signature spells; the overlay also gains an optional Faction section with a config toggle and migration
- **Rechargeable Items** — the inventory tracker adds a Rechargeable Items section, and limited-charge clicky items show their remaining charge count
- **Log Backfill** — unified into the Logs tab and now runs in the background with a bottom progress bar, plus a 30-day log trim and wizard-driven backfill; the Logs tab adds log/Zeal status toggles, wizard diagnostics, and missing-log notices
- **Navigation settings** — a new Settings → Navigation tab hides and reorders sidebar tabs
- **Spell checklist** — added a spell-name search filter and per-spell selection for shopping runs
- **Gear layouts** — swap gear-display layouts between the Gear tab and the Inventory tab
- **Pop Out All** respects dashboard visibility toggles, only opening dashboard-visible overlays
- Skill Tracker (per-character skills tab, caps + specialize lock, Log Backfill support) is present but gated behind a `DEV_SKILLS` flag — no reliable snapshot source yet
- Ear, Ring, and Wrist slots no longer show empty for Zeal `_pq.proj` exports — format-1 equipment slot names are normalized (#137)
- Corrected Offense/weapon `skill_id`s used for the ATK rating
- Chat History no longer black-screens with no chat rows, double-loads its spinner, or shifts layout on load
- Fixed a Primal Avatar crash from an empty buff-modifier resolution and hardened the Spell Modifiers panel against bad resolutions
- Ambiguous Shissar/Brood self-lands resolve to the correct targeted timer
- Duplicate-named bosses headline the strongest matching NPC and collapse the rest
- Switching between items/spells/NPCs/zones no longer flickers the detail panel

## v0.13.0 — Gear Upgrade Finder, Quests, Trigger Rework, Custom Timers, Log Replay

- **Gear Upgrade Finder** — a per-character, per-slot upgrade scanner backed by
  a cap-aware scoring engine (255-stat-cap aware; HP/mana/AC scored uncapped),
  editable per-class weights with persistence, an all-slots overview endpoint
  showing the best pick per slot, worn ATK and cap-aware haste scoring, weapon
  DPS (ratio) scoring so melee offhands rank correctly, per-character priority
  focus effects (score bonus + badge), and a slot-scoped wishlist with stars on
  suggestions; Planes of Power gear is hidden unless `pop_enabled` is on, and
  item era is derived from quest scripts to fix gear-finder era leaks
- **Quests** — self-contained quest walkthroughs built into the app: a Quests
  section in the database explorer, a Quests tab on item detail (rewards +
  turn-ins) backed by `GET /api/items/{id}/quests`, and full quest-chain to-do
  lists for multi-step keys
- **Trigger & regex rework** — multiple regex patterns per trigger with per-row
  toggles, built-in `{c}`/`{target}` tokens with GINA-style pattern
  compatibility, custom categories with drag-and-drop reordering and pack-origin
  tagging, target-name capture into trigger-driven timers, per-pattern timer
  overrides + capture-keyed timers, assist-call triggers that also fire on kill
  calls, and seven built-in community trigger packs with shared class CC-break
  alerts
- **Custom timers** — manual countdown timers with a dedicated overlay and
  durations pulled from capture groups
- **Log replay & browse** — replay historical log segments through the live
  pipeline to test triggers/overlays, plus a read-only Browse mode for viewing
  logs out of game
- **Navigation rebuild** — collapsible sidebar sections with character pages
  nested under each character, and smoother sidebar scrolling
- **Overlay controls** — a global Position-overlays edit mode and a Manage
  overlays menu on the dashboard, a "Display only" click-through HUD lock mode,
  one-click reset to recover an off-screen/locked overlay, customizable trigger
  alert text style (color, glow, font, size) with a live preview card and
  one-click Reset Style, optional fade-out of overlay chrome with a configurable
  delay, and per-overlay pop-out buttons in the Settings lock-behaviour card
- **Player tracker** — per-player notes and a PVP flag, a sound + on-screen
  warning when a PVP-flagged player appears in `/who`, automatic tracking of
  tells and group joins, a global toggle to disable the PVP warning, and Show
  More pagination instead of a silent 500 cap
- **Combat meter** — a Combined (pooled) view across fights, a "Last 20 mobs"
  rolling-average scope, a clearer Per Fight / All Fights switch, expanded
  per-pet damage, and spell/melee crit counting
- **Inventory** — hide empty bags by default (persisted), scope the tracker to
  imported characters with an opt-in toggle, flat cross-character search
  results, and an item Characters tab showing which characters hold an item
- **NPC overlay** — a player info + timers tab when you target another character
- **Settings** — settings now autosave on change (Save/Discard buttons removed),
  a new About tab linking Discord/Ko-fi/GitHub/website, and donations moved from
  Buy Me a Coffee to Ko-fi
- **Planes of Power preview** — a backend era package and `pop_enabled`
  preference gating the level cap and content, with frontend era gating and a
  PoP preview toggle
- Native HTML5 drag-and-drop (trigger reorder/category moves, wishlist
  reordering) now works on Windows — it was silently broken by the title-bar /
  sidebar `-webkit-app-region: drag` regions
- Buff-duration modeling corrected: SPA 137/141 focus limits enforced, AA
  duration extensions applied to off-class clickies, focus SPA 134/139 limits
  use the caster's class level, and the Permanent Illusion override is honored
- CH chain matches your own shout/OOC casts, and upgrade configs pinned to an
  outdated default pattern are migrated
- Spell timer overlays (detrimental, buff, CH chain) scroll again when popped out
- Smaller fixes: spellset edit-state alignment and an unresolvable-ID re-fetch
  loop, a log-replayer crash on stop, out-of-order quest search results, and a
  black screen when switching to a character with an empty gear slot

## v0.14.0 — Threat Meter, Trader Tracker, Resist Calculator, Charm Pet Finder, Multi-Format Trigger Import

- **Threat Meter** — a personal, per-mob hate estimator built entirely from
  your own log lines: observed damage, spell instant-hate (SPA 92), standard
  hate, hate-modifying buffs (SPA 114/130), heal and miss hate, and feign-death
  resets, with a live rolling-window hate-per-second readout measured on a
  receive clock. Surfaced as a dashboard card and a pop-out overlay window
- **Trader Tracker** — infers your Bazaar sales by diffing your Trader's Satchel
  between inventory exports, captured automatically by a background poller or
  from manual `/output` exports, so you can see what sold while you were away
- **Resist Calculator** — estimates a spell's land chance against any NPC by
  porting EQMacEmu's resist check and enumerating the full roll distribution,
  with a resist-debuff section (level-scaled magnitudes), immunity and
  charm/mez/fear level-cap gating, and a searchable NPC target picker
- **Charm Pet Finder** — lists charmable NPCs per zone, class, and spell, ranked
  by DPS with land-chance odds (reusing the resist engine) and level-cap
  warnings, gating charm spells by Quarm class level
- **Multi-format trigger import** — a unified Import Triggers wizard that
  detects, previews, and commits GINA, EQNag, EQLogParser (`.tgf`), and PQ
  Companion trigger packs into a chosen category
- **Trigger anti-spam** — per-trigger refire cooldown (lockout) and a separate
  repeat-audio cooldown to tame bursts of duplicate alerts, plus a "Copy to
  Clipboard" trigger action
- **Log Feed** — right-click a line to "Play from this point", an opt-in "Raw
  lines" toggle so live search finds any line, a visible play button, and
  replay file/date/time selections that persist across navigation
- **Custom timers & overlays** — per-trigger timer bar color with a global timer
  appearance setting and a quick-add color picker, optional "keep expired
  timers" overdue reminders, NPC overlay target pin/lock during target swaps,
  overlays that restore on launch, and a fading-soon alert seeded when creating
  a trigger from a spell
- **Items & character** — weapon damage ratio on item detail, and natural
  (level/race) HP regen added to character stats
- Spell-detail effect ranges and resist magnitudes now level-scale correctly
  and respect the PoP era cap
- Fixes: main window keeps its size/position on a secondary or mixed-DPI monitor
  (Windows); mez/charm/root break triggers no longer stack multiple sounds for
  one break and trigger sounds no longer cut off mid-play; GM-only items (e.g.
  the Red Glowing Robe) are hidden from item queries and gear suggestions;
  Flowing Thought counts flavor-named worn FT effects; Log Feed browse search is
  fast and cancellable on large logs

## Unreleased — Stability, Performance & Hardening Pass

A full-app code-review pass focused on correctness, raid-time performance, and
hardening — no new features, but a broad sweep of reliability fixes.

- **Reliability** — settings are now saved atomically, so a crash or power loss
  mid-save can no longer corrupt `config.yaml` and leave the app unable to
  start; concurrent settings writes no longer revert or silently wipe each
  other's values; if the background service fails to launch (usually antivirus
  quarantine), you now get a clear recovery dialog explaining how to fix it
  instead of a blank window; a failed backup *import* now restores your original
  data instead of appearing to lose it; and deleting a character now removes all
  of its associated data (AAs, tasks, wishlist, etc.) instead of leaving orphans
- **Fewer duplicate alerts** — fixed a WebSocket reconnect edge case that could
  duplicate the live connection, which doubled meter/log updates and could fire
  trigger alerts twice (a likely contributor to "audio alerts multiply" reports)
- **Raid-time performance** — the DPS/HPS meter now coalesces its live updates to
  once a second instead of re-sending the whole state on every hit during AoE
  spam; unclassified log lines are matched far faster; per-line database writes
  no longer stall behind slow searches; and the CH metronome, roll tracker, NPC
  overlay, and DPS pop-out no longer re-render when idle
- **Database explorer accuracy** — a zone's NPC list no longer includes unrelated
  mobs from other zones; item and spell "found in" links now point at the right
  zone (e.g. Sea King → Erudin, not Erud's Crossing); and name lookups are
  index-backed (takes effect with the next game-database update)
- **UI fixes** — search boxes across Items, Spells, NPCs, Zones, Recipes,
  Players, Loot, Chat, and global search no longer flash stale results when a
  slower earlier request lands late; Combat History pagination is race-free; the
  "Searching…" state can no longer get stuck; Combat Log rows keep the right
  expanded state when two mobs are pulled in the same second; and the
  database-explorer Back/Forward buttons track correctly
- **Security hardening** — the internal audio file handler is confined to audio
  files only, all app windows now run sandboxed with locked-down navigation and
  external-link handling, the Zeal file writers reject path-traversal in
  character names, and the background service now shuts down if the app is
  force-killed instead of lingering
- **Fewer misleading errors** — API endpoints now distinguish "not found" from a
  real database error (so a locked database no longer reads as "no upgrades" or
  "no AAs"), and error messages containing Windows paths are always valid JSON

## v0.16.0 — Tradeskill Leveling Planner, Combine Success Calculator, Bandolier & Macro Editors, Tanking View

- **Tradeskill Leveling** — a new page that computes a staged plan to raise any
  tradeskill from your current skill to a target, choosing recipes by skill-up
  band and total cost. Pick "fastest" (fewest combines) or "cheapest" (least
  plat), with recursive sub-combine costing that accounts for intermediate
  items you must craft and warns when a step crosses into another tradeskill.
  Paths are derived entirely from the Quarm database (`internal/tsplan` DP
  solver, `POST /characters/{id}/tradeskill-plan`)
- **Combine success calculator** — the Recipes page now shows per-recipe success
  and failure percentages from your skill and crafting-stat gear (ported from
  EQMacEmu), including the trivial breakpoint where combines can no longer fail
  and a skill-up chance estimate. Jewelcraft, Alchemy, and Poison Mastery AAs
  and Maelin Starpyre's skill-up buff are applied automatically, with a
  gear-modifier picker and modifier-slot tooltips
- **Tradeskills tab** — characters now display their trained tradeskill values
  from the Zeal 1.4.3 quarmy export, classifying class- and race-locked
  untrained skills
- **Bandolier & Macro editors** — now public. Edit your Zeal weapon-swap
  bandolier sets (guarded to gear you own and can equip, gated by class, race,
  and level, with item hover cards and a per-character preferred storage bag)
  and your in-game social macros (a surgical `[Socials]` editor that preserves
  the rest of the file), including a new `/cancelbuff` builder that turns a
  buff-name search into cancel-by-name macro lines. Macros can be imported from
  another character
- **Tanking view** — the character Stats tab now splits avoidance vs.
  mitigation, models the mitigation softcap and melee hit chance, and folds in
  Combat Stability and Combat Agility AAs
- **Stat bars** — base attributes now show their base value plus the green AA
  bonus separately, with a tooltip explaining the source, and food/drink stats
  are applied to equipped totals
- **EQ client check** — Settings version-checks `eqw.dll` (EQW-TAKP) alongside
  the eqgame.dll / Zeal checks
- Fixes: an empty tradeskill leveling plan no longer black-screens the page;
  custom trigger alerts show the captured target, not just pack triggers; the
  regex editor accepts Go inline-flag patterns like `(?i)`; buff stat bonuses
  scale with level so buffed HP matches in-game values; distinct same-named
  items stay visible in search and gear upgrades; copied target stats drop
  crowd-control immunities; and the spell-modifiers view returns empty instead
  of erroring when no Quarmy export is present

## v0.16.1 — Piper TTS, Macro Editor Redesign, Cultural Recipe Awareness

- **Piper TTS** — trigger text-to-speech alerts can now use a
  locally-installed Piper neural voice instead of the browser's built-in
  speech synthesis. PQ Companion detects your Piper executable and voice
  model, spawns it to synthesize speech (with an optional warm-process mode
  to avoid cold-start lag on every alert), caches generated audio by
  content hash, and falls back to the browser voice if anything goes
  wrong. Piper is a user-installed external tool (like Zeal/EQW) — nothing
  is bundled. Configure it from Settings → General
- **Macro editor redesign** — the in-game macro grid now mirrors
  EverQuest's actual 2-column social layout, with drag-to-rearrange slots
  and a one-click reset back to the page 1 defaults
- **Cultural recipe awareness** — the recipe browser and Tradeskill
  Leveling planner now flag race-locked cultural recipes so a plan never
  steers you into a combine your character's race can't perform
- Fixes: Key Tracker counts keys on your key ring (not just inventory
  copies) as keyed; Gear Upgrade Finder remembers your sort order across
  tab switches and Flowing Thought AC headroom only counts a worn copy;
  the combine success calculator reports the failure floor at max skill
  instead of current skill; and Maelin Starpyre's skill-up buff only
  boosts the failure-path skill-up chance, matching its actual effect

## v0.17.0 — Item Compare, Custom Timer Windows, Wishlist Alerts

- **Item Compare** — compare items side-by-side from the item detail view,
  including a mode that compares a candidate item against whatever a
  character currently has equipped in that slot
- **Custom Timer Windows** — named custom timer overlays reached full
  dashboard parity: multiple simultaneous windows, a Timer Groups UI to
  organize triggers by window, per-trigger pin to keep a timer atop other
  overlays, and a working close (X) button on each window
- **Wishlist alerts** — a synthetic trigger fires when a wishlisted item
  appears in the combat log, with alert config hidden until the feature's
  master toggle is enabled
- **Recipe inventory status** — the recipe browser flags which components
  of a combine you already have versus what's still missing
- **Ring War signature-spell warnings** — the Raid Alerts trigger pack adds
  recast warnings for raid boss signature spells
- **Sidebar favorites** — pin frequently used nav items to the top of the
  sidebar
- **Macro move/copy** — move or copy macros between pages via an explicit
  picker (replacing cross-page drag), with a Replace option alongside Swap
  when the target slot is occupied
- **Pop out all overlays** — one-click sidebar toggle to pop out every
  dashboard-visible overlay at once
- **Overlay text alignment** — left/center/right anchor option for pinned
  overlay text alerts
- **Stat overage display** — the character stat panel shows how far gear
  pushes a stat past the level cap instead of silently capping the number
- Fixes: the installer no longer wipes the updater cache mid-update; the
  NPC overlay surfaces script-spawned NPC variants that name lookup was
  missing; NPC-only spells now appear in global search and the All Classes
  filter; respawn timers retry `#`-prefixed `npc_types` names; the log
  parser tolerates the active eqlog file being deleted or renamed while
  tailed; raid signature-spell alerts no longer stack on AE casts; the NPC
  overlay no longer guesses a raid boss's variant by pulled position;
  Group Member Died no longer misfires on 3+ word victim names; Roll
  Tracker recognizes x11-style suffix shorthand; log feed replay play/stop
  buttons no longer stick at 10x speed; replay dispatch uses real time so
  timers don't expire on arrival; the target overlay no longer gets yanked
  off a manually-selected PC by DoT combat lines; fading-soon TTS alerts
  correctly resolve capture groups; Piper TTS playback queues so
  overlapping triggers don't talk over each other; and threat credits hate
  from a superseded pending cast instead of dropping it

## v0.17.2 — Tradeskill Per-Stage Detail, Threat Meter Refinements, Copy Buttons

- **Tradeskill per-stage detail** — the Tradeskill Leveling plan now shows
  each stage's ingredients, success chance, and success% inline, and
  Custom mode is now a distinct, resettable view (rather than mutating the
  default plan) for routing around excluded recipes
- **Rogue Evade threat modeling** — the personal threat meter rescales the
  current target's hate when Hide is used mid-combat, matching Evade's
  actual effect
- **Raid threat departure reset** — a raid member's hate zeroes out when
  ambient portal text shows them leaving the zone (evac, succor, CoTH,
  circle)
- **Roll Tracker: Copy Pick Order** — copy the full roll ranking, not just
  the winner, to paste in game
- **Combat History copy button** — copy a per-fight DPS summary directly
  from combat history
- **Recipe holdings display** — the recipe browser shows owned quantity
  with an expandable breakdown across storage locations
- Fixes: Roll Tracker recognizes tier letters glued directly onto roll
  numbers; wishlist watch alerts filter out focus/clicky procs and
  correctly enumerate multi-character wishlists; hate modifiers (Subtlety
  AA, Visage) no longer shrink aggro-shed spells like Concussion; EQ
  clipboard paste is capped at a unified 400 characters across all copy
  buttons, matching the game's actual limit; and the class spell
  checklist no longer flags innate/AA-only spells as missing from a
  vendor

## v0.17.3 — Lockout Auto-Capture, Compressed Log Backups

- **Lockout auto-capture** — lockouts are now recorded the instant a kill
  notice lands, instead of waiting for a manual `/sll`; `/sll` remains
  authoritative and overwrites auto-captured rows with second-precision
  data on the next full snapshot
- **Smaller log backups** — Archive & Purge now writes a compressed
  `.bak.zip` instead of a raw text copy, and a new Export Debug Logs
  button (Settings > Advanced) bundles the app's logs into one file for
  bug reports
- Fixes: the personal threat meter now credits a successful Taunt; two-word
  attack verbs like "harm touch" no longer mis-parse into a garbled
  phantom mob name, and Harm Touch's real damage is credited to threat;
  NPC search no longer filters out real named/scripted mobs with a `#`
  prefix as placeholders; and self-authored raid chat tells are parsed
  correctly instead of being silently dropped

## Phase 11 — Project Website
_Planned_

A public-facing site for the project — feature overview, download links, screenshots, and documentation. Deferred until the app is stable and feature-complete enough to be worth promoting.

## Future Plans

The following features are tracked but not scheduled for a specific phase. They will be prioritized based on demand and feasibility once the core app is mature.

### Planes of Power Flag Tracker

Manual per-character checklist for tracking Planes of Power progression flags. Players tick off flags as they earn them; data persists in user.db.

Design notes:
- Reference: https://takp.info/flag-check/index.html — use as the source of truth for flag names, groupings, and unlock order
- Flag data is static (hardcoded in Go, similar to `internal/keys/keys.go`) since Zeal does not yet expose flag state
- One checklist per character; characters identified by name (same source as Zeal exports)
- Organized by plane/tier: Elemental Planes entry flags → God flags → Plane of Time prerequisites
- Each flag entry: name, brief description of how it's obtained, checked/unchecked state
- Backend: `GET /api/flags` (static definitions), `GET/PUT /api/flags/progress/{character}` (persisted checked state in user.db)
- Frontend: character tabs, grouped flag sections, checkboxes, progress summary per tier
- Future: wire to automatic detection if Zeal adds flag export support

### Hosted Web API

A cloud-hosted version of the backend API so external tools and the project website can query EQ game data without requiring the desktop app. Lowest priority — only relevant once the app has an established user base and the data model is stable.
