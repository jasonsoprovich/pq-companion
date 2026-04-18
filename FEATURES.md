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
  - **Detail panel (right)**: spell data in labeled sections — Casting (skill school, mana, cast/recast/recovery time, duration labeled "Max Duration" for scaling spells), Targeting (target type, resist type, range, AoE range, Zone Type when restricted), Classes (full class names with required level), Effects (human-readable descriptions per slot), Messages (cast_on_you, cast_on_other, spell_fades flavor text), Info (Spell ID)
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
  - **Detail panel (right)**: NPC data in labeled sections — Combat (HP/Mana/Damage range/Attacks/AC), Attributes (STR/STA/DEX/AGI/INT/WIS/CHA, omitted when all zero), Resists (MR/CR/DR/FR/PR, omitted when all zero), Special Abilities (parsed as pill badges), Behavior (Aggro Radius/Run Speed/Size), Info (NPC ID/Loot Table/Merchant/Spells/Faction IDs, Exp%, Spell/Heal Scale)
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

### Task 2.7 — Global Search ✅
- **`GET /api/search?q=&limit=`** — new backend endpoint; runs all four searches (items, spells, NPCs, zones) in parallel via goroutines and returns a single grouped response (`internal/api/search.go`)
- **`GlobalSearch` component** (`components/GlobalSearch.tsx`): full-screen modal overlay triggered by `Cmd+K` / `Ctrl+K` from anywhere in the app
  - Debounced search input (300ms); shows spinner while loading
  - Results grouped by category (Items, Spells, NPCs, Zones) with section headers and type icons
  - Each result shows name + contextual subtitle (item type/level, castable classes, NPC level/class, zone short name)
  - Keyboard navigation: `↑`/`↓` to move, `↵` to open, `Esc` to close; click outside to dismiss
  - Navigates to the correct explorer page (`/items`, `/spells`, `/npcs`, `/zones`) with `?select=ID` query param
- **Sidebar search hint** (`components/Sidebar.tsx`): `⌘K` shortcut pill shown above the nav links for discoverability
- **Pre-select via URL** (`?select=ID`): all four explorer pages now read the `select` query param on mount, fetch the record by ID, and pre-populate the detail panel; param is cleared from the URL after selection

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
  - Clicking any row navigates to `/spells?select={id}` to open that spell in the Spell Explorer detail panel
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
- **`internal/keys/keys.go`** — static key definitions (no DB needed). Each `KeyDef` has an ID, name, description, and ordered `[]Component{ItemID, ItemName, Notes}`. Item IDs are canonical; names are for display only. Ships with 6 keys: Veeshan's Peak, Old Sebilis, Howling Stones (Charasis), Grieg's End, Grimling Forest Shackle Pens, and Katta Castellum.
- **`GET /api/keys`** — returns all key definitions as `{"keys": [...]}`.
- **`GET /api/keys/progress`** — cross-references all character inventories (via `AllInventories`) against each key's component item IDs. Response: `{configured, keys[{key_id, characters[{character, has_export, components[{item_id, item_name, have, shared_bank}]}]}]}`. `have` is true if the item is in that character's equipped/bag/bank slots. `shared_bank` is true when the only copy is in the Shared Bank (available to all characters, deduplicated).
- **`types/keys.ts`** — TypeScript types mirroring all Go response structs.
- **`services/api.ts`** — added `getKeys()` and `getKeysProgress()` typed fetch wrappers.
- **`pages/KeyTrackerPage.tsx`** — Key Tracker page at `/key-tracker`:
  - **Header bar**: Key Tracker title and Refresh button.
  - **Filter tabs**: All / In Progress / Complete — filters the key card list by aggregate progress across all characters.
  - **Key cards**: expandable accordion cards; collapsed state shows key name and a progress bar (`X / Y components` aggregated across all characters). Complete keys render with a green border.
  - **Component table** (expanded): rows = components, columns = one per character with a Zeal export. Each cell shows a green checkmark (character has the item), `SB` gold badge (only in shared bank), or an empty circle (missing). Component notes shown as muted subtitle text.
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

**`/con` Target Detection**
- New `EventConsidered` (`log:considered`) event type added to the log parser
- New `ConsideredData` struct carries the target name extracted from the disposition message
- Regex `reConsider` matches all classic EQ consider phrases: "scowls at you", "glares at you", "looks your way", "looks upon you", "judges you", "regards you", "warmly/kindly regards you", "considers you"
- Multi-word NPC names (e.g. "a grimling cadaverist") are correctly captured via non-greedy group before the disposition phrase

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
  - `autoDownload: true`, `autoInstallOnAppQuit: true`
  - Checks for updates 5 s after launch (gives sidecar + window time to initialise)
  - Events forwarded to the renderer via `mainWindow.webContents.send`:
    - `updater:available` → `{ version }` — new version detected, download started
    - `updater:progress` → `{ percent, transferred, total }` — download progress
    - `updater:downloaded` → `{ version }` — ready to install
    - `updater:error` → error message string
  - IPC handlers: `updater:check` (manual recheck), `updater:quit-and-install`
- **`electron/preload/index.ts`** — `updater` namespace exposed via `contextBridge`:
  - `check()`, `quitAndInstall()` — invoke IPC handlers
  - `onAvailable(cb)`, `onProgress(cb)`, `onDownloaded(cb)`, `onError(cb)` — subscribe to update events; each returns an unsubscribe function for `useEffect` cleanup
- **`frontend/src/types/electron.d.ts`** — `updater` added to `ElectronAPI` interface
- **`frontend/src/components/UpdateNotification.tsx`** — bottom-of-app banner with four states:
  - `available` — "Update vX.Y.Z available — downloading in the background…" (dismissible)
  - `downloading` — gold progress bar with percentage
  - `downloaded` — "vX.Y.Z ready — restart to install" + **Restart** button
  - `error` — silent fallback message (dismissible); does not interrupt the user
- **`frontend/src/components/Layout.tsx`** — `<UpdateNotification />` added below `<GlobalSearch />`
- **`.github/workflows/release.yml`** — updated for auto-updater:
  - Both `build-windows` and `build-macos` jobs changed to `--publish always`; `GH_TOKEN` is passed so `electron-builder` uploads the installer + update manifest (`latest.yml` / `latest-mac.yml`) directly to the GitHub release
  - `release` job simplified: promotes the draft release (`gh release edit --draft=false`) after both builds succeed
  - `latest.yml` and `latest-mac.yml` are now part of every release; `electron-updater` reads these to detect new versions

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
