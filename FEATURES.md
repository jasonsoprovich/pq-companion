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

### Coming in Phase 2
- **Spell Explorer** (Task 2.4): search by name/class/level, duration and resist type display
- **NPC Explorer** (Task 2.5): search by name/zone, special ability parsing, loot table view
- **Zone Explorer** (Task 2.6): browse zones, list resident NPCs
- **Global Search** (Task 2.7): cross-database search via `Ctrl+K` / `Cmd+K`

## Phase 3 — Zeal Integration & Backup Manager
- Parse Zeal inventory export files (on logout) to track carried items
- Spell checklist: compare spellbook export against all available class spells by level
- INI/config file watcher: automatic versioned backups when EQ config files change
- Backup timeline UI: browse and restore any previous config version

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
