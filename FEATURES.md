# PQ Companion — Features

## Phase 0 — Database Setup & Exploration ✅
- MySQL 8 Docker environment for EQEmu dump exploration
- Go CLI tool (`dbconvert`): MySQL dump → SQLite converter
  - `--from-dump` mode: parses `.sql` dump files directly, no MySQL required
  - `--from-mysql` mode: reads from a live MySQL connection
  - Handles all MySQL→SQLite type mapping, index conversion, and data migration
  - Converts ~1.1 million rows in under 60 seconds
- Documented schema for all key tables (items, spells, NPCs, zones, loot, spawns)
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
- **`services/api.ts`** — typed fetch client: `searchItems(q, limit, offset)`, `getItem(id)`
- **`lib/itemHelpers.ts`** — EverQuest bitmask/label decoders:
  - `slotsLabel` — decodes `slots` bitmask into slot names (Charm, Head, Primary, etc.)
  - `classesLabel` — decodes `classes` bitmask into class names; "All" when all bits set
  - `racesLabel` — decodes `races` bitmask into race names; "All" when all bits set
  - `itemTypeLabel` — maps `item_type` int to weapon/armor/misc label
  - `sizeLabel`, `weightLabel`, `priceLabel` (copper → pp/gp/sp/cp)
- **`pages/ItemsPage.tsx`** — split-pane layout:
  - **Left pane (288px)**: debounced search input, result count, scrollable list showing name + item type + req level; selected item highlighted with gold left-border accent
  - **Detail panel (right)**: full item data in labeled sections — Combat (DMG/DLY/Range/AC), Stats (HP/Mana/STR/STA/AGI/DEX/WIS/INT/CHA), Resists (MR/CR/DR/FR/PR), Effects (Click/Proc/Worn/Focus), Restrictions (Req/Rec level, Slots, Classes, Races), Info (Weight, Size, Stack, Bag info, Price, Item ID)
  - Flags rendered as pill badges: MAGIC, LORE, NO DROP, NO RENT
  - Sections only rendered when they have non-zero values
  - Initial load fetches all items (empty query); debounced at 300ms

### Task 2.4 — Database Explorer: Spells ✅
- **`types/spell.ts`** — TypeScript `Spell` type mirroring Go backend struct (timing, duration, effects, class levels)
- **`services/api.ts`** — added `searchSpells(q, limit, offset)` and `getSpell(id)` typed fetch wrappers
- **`lib/spellHelpers.ts`** — EverQuest spell data decoders:
  - `castableClasses(classLevels)` — returns `{abbr, full, level}` for each class that can cast the spell (255 = cannot cast)
  - `castableClassesShort` — compact list of first 4 castable classes for list row subtitles
  - `resistLabel` — maps resist type int to name (Magic, Fire, Cold, Poison, Disease, Chromatic, etc.)
  - `targetLabel` — maps target type int to description (Self, Single, Targeted AE, PB AE, Caster Group, etc.)
  - `skillLabel` — maps skill ID to school name (Alteration, Abjuration, Conjuration, Divination, Evocation)
  - `msLabel` — converts milliseconds to `"2.5s"` / `"Instant"` display strings
  - `durationLabel` — converts buff_duration ticks + formula to human-readable string (1 tick = 6s); distinguishes fixed vs. level-scaling durations
  - `effectLabel` — maps spell effect IDs to readable names (160+ effects mapped)
- **`pages/SpellsPage.tsx`** — split-pane layout matching Item Explorer:
  - **Left pane (288px)**: debounced search input, result count, scrollable list showing name + castable classes with levels + mana cost; selected spell highlighted with gold left-border accent; blank-name spell IDs filtered out
  - **Detail panel (right)**: spell data in labeled sections — Casting (mana, cast/recast/recovery time, duration), Targeting (target type, resist type, range, AoE range), Classes (full class names with required level), Effects (effect name + base value for each active slot), Messages (cast_on_you, cast_on_other, spell_fades flavor text), Info (Spell ID)
  - Flags rendered as pill badges: DISCIPLINE, SUSPENDABLE, NO DISPELL
  - Sections only rendered when they have relevant data

### Task 2.5 — Database Explorer: NPCs ✅
- **`types/npc.ts`** — TypeScript `NPC` type mirroring Go backend struct (combat, attributes, resists, behavior, special abilities)
- **`services/api.ts`** — added `searchNPCs(q, limit, offset)` and `getNPC(id)` typed fetch wrappers
- **`lib/npcHelpers.ts`** — EverQuest NPC data decoders:
  - `npcDisplayName(npc)` — combines name + last_name, converting EQEmu underscores to spaces
  - `className(classId)` — maps NPC class IDs 1–16 to full class names (Warrior → Berserker)
  - `raceName(raceId)` — maps race IDs to names (Human, Barbarian, Iksar, Skeleton, Dragon, etc.)
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
  - `models.go` — typed `LogEvent` struct with `EventType` constants: `log:zone`, `log:combat_hit`, `log:combat_miss`, `log:spell_cast`, `log:spell_interrupt`, `log:spell_resist`, `log:spell_fade`, `log:death`; per-type data structs (`ZoneData`, `CombatHitData`, `CombatMissData`, `SpellCastData`, `SpellInterruptData`, `SpellResistData`, `SpellFadeData`, `DeathData`)
  - `parser.go` — `ParseLine(line string) (LogEvent, bool)` regex-based classifier:
    - Timestamp: `[Mon Jan _2 15:04:05 2006]` layout; handles space-padded single-digit days (ctime format)
    - Zone change: `"You have entered <ZoneName>."`
    - Spell begin casting: `"You begin casting <SpellName>."`
    - Spell interrupted: generic `"Your spell is interrupted."` and named `"Your <SpellName> spell is interrupted."`
    - Spell resist: `"Your target resisted the <SpellName> spell."`
    - Spell fade: `"Your <SpellName> spell has worn off."`
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
- **`types/logEvent.ts`** — TypeScript types mirroring Go structs: `LogEvent`, `LogEventType`, all per-event `Data` types (`ZoneData`, `CombatHitData`, `CombatMissData`, `SpellCastData`, `SpellInterruptData`, `SpellResistData`, `SpellFadeData`, `DeathData`), `LogTailerStatus`
- **`services/api.ts`** — added `getLogStatus()` fetching `GET /api/log/status`
- **`pages/LogFeedPage.tsx`** — live log event feed at `/log-feed`:
  - **Header**: title, event counter (`X / 200`), WebSocket connection pill (green/orange/gray), Clear button
  - **Status bar**: tailer state inline — disabled warning with Settings link, file-not-found warning with path, or green "Tailing" with file path
  - **Event feed**: newest events at top; each row shows hh:mm:ss timestamp, color-coded type badge (blue=Zone, red=Hit, gray=Miss, purple=Cast, orange=Interrupt/Resist, teal=Fade, dark-red=Death), raw EQ log message in monospace; capped at 200 events
  - **Empty state**: activity icon + "Waiting for log events…" with setup instructions
- **Sidebar** — new "Parsing" section with "Log Feed" (`Activity` icon) at `/log-feed`
- **`App.tsx`** — `/log-feed` route wired up

### Task 4.3 — NPC Info Overlay (Backend)
_Planned_

### Task 4.4 — NPC Info Overlay (Frontend)
_Planned_

## Phase 5 — Combat Tracking & DPS Meter
- Per-entity damage tracking (you, pet, group members, NPCs)
- DPS calculations: current fight, rolling average, session total
- Live DPS overlay: transparent always-on-top meter with group breakdown
- Combat log history: browse past fights with detailed damage breakdowns

## Phase 6 — Spell Timer Engine
- Countdown tracking for mez, stuns, DoTs, buffs
- Server-tick-aware duration calculations
- Timer overlay: color-coded bars grouped by type (mez / DoT / buff / debuff)
- Buff window enhancement: self-buff tracking with exact remaining durations

## Phase 7 — Audio Alerts
- System audio integration via Web Audio API
- Configurable alerts when timers expire (sound file or TTS)
- TTS notifications for game events (tells, death, zone messages)
- Per-trigger volume and voice settings

## Phase 8 — Custom Trigger System
- Regex-based trigger engine: match log lines → fire actions
- Actions: play sound, speak TTS, display overlay text, log to history
- Trigger Manager UI: create/edit/delete triggers, import/export packs
- Pre-built trigger packs (enchanter mez breaks, resist spam, named spawns)
- Text overlay window for trigger output display

## Phase 9 — Build & Distribution
- Windows `.exe` installer via electron-builder + GitHub Actions CI
- Auto-updater: silent background updates via electron-updater + GitHub Releases
- Optional hosted web API on Cloudflare Workers (same Go API, cloud DB)
- Project website on Cloudflare Pages

## Phase 10 — Character Tools

### Task 10.1 — Planes of Power Flag Tracker
_Planned_

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

### Task 10.2 — Character Todo List
_Planned_

Simple per-character todo list for tracking arbitrary in-game goals. Keeps it minimal by design; complexity added only based on user feedback.

Design notes:
- Each todo item: ID, character name, text (free-form string), checked bool, created_at timestamp
- Items stored in user.db (`todo_items` table)
- Backend: `GET /api/todos/{character}`, `POST /api/todos/{character}`, `PATCH /api/todos/{character}/{id}` (toggle checked), `DELETE /api/todos/{character}/{id}`
- Frontend: character selector (populated from known Zeal export characters), text input + Add button, list of items with checkboxes, delete button per item, optional "hide completed" toggle
- No categories, priorities, or due dates for v1 — just text + checkbox
