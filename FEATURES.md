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

## Phase 4 — Log Parsing & NPC Info Overlay
- Real-time EQ log file tailer (reads new lines as they appear)
- Combat, spell, zone, and chat event parsing from log format
- Event broadcast to all WebSocket clients
- NPC info overlay: transparent always-on-top window showing current target's stats and special abilities

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
