# PQ Companion

A desktop companion app for the [Project Quarm](https://www.projectquarm.com/) EverQuest emulated server.

Features: database explorer (items, spells, NPCs, zones), combat log parser, DPS meter, spell/buff/DoT timer overlays, NPC info overlay, spell checklist, config backup manager, and a custom trigger system.

> **Status:** Active development — Phase 0 complete (database foundation + Go data layer). Phase 1 complete: REST API, WebSocket server, and configuration system. Phase 2 in progress: Electron + React shell scaffolded, app layout and navigation complete, Item/Spell/NPC/Zone explorers live; Global Search next. See [ROADMAP.md](ROADMAP.md) for what's coming.

---

## Development Setup

### Prerequisites

- [Docker](https://www.docker.com/) — for local MySQL database exploration
- [Go 1.22+](https://go.dev/) — backend
- [Node.js 20+](https://nodejs.org/) — frontend + Electron
- A MySQL client (optional) — [DBeaver](https://dbeaver.io/) or MySQL Workbench

---

## Running the App (Dev)

Open two terminals:

```bash
# Terminal 1 — Go backend
cd backend
go run ./cmd/server

# Terminal 2 — Electron + React frontend
npm run dev   # from repo root
```

The Vite dev server starts on port 5173; Electron opens a window pointing to it. The Go backend is expected on port 8080.

### Build

```bash
npm run build          # compile all three processes to out/
npm run dist:mac       # package as macOS DMG
npm run dist:win       # package as Windows NSIS installer (requires Wine on macOS)
```

---

## Quick Start: Generate the SQLite Database

The app ships with a pre-converted `quarm.db`. To regenerate it from the MySQL dumps:

### Option A — From dump files (no Docker needed)

```bash
cd backend
go run ./cmd/dbconvert --from-dump --sql-dir ../sql --output ./data/quarm.db
```

This reads the `.sql` dump files in `sql/` and converts them directly to SQLite.
Typical runtime: **under 60 seconds** for ~1.1 million rows.

### Option B — From a live MySQL container

```bash
# Start MySQL and wait for it to finish importing
docker compose up -d
docker compose logs -f mysql   # wait for "ready for connections"

# Run the converter
cd backend
go run ./cmd/dbconvert --from-mysql --output ./data/quarm.db
```

#### dbconvert flags

| Flag | Default | Description |
|------|---------|-------------|
| `--from-dump` | — | Convert from `.sql` dump files |
| `--from-mysql` | — | Convert from live MySQL connection |
| `--sql-dir` | `./sql` | Directory containing dump files |
| `--sql-files` | — | Comma-separated list of specific `.sql` files |
| `--mysql-dsn` | `root:quarmbuddy@tcp(localhost:3306)/quarm` | MySQL DSN |
| `--output` | `backend/data/quarm.db` | Output SQLite path |
| `--verbose` | false | Verbose logging |

---

## Phase 0: MySQL Database Setup (for exploration)

The raw EQ game data is distributed as MySQL dumps from the EQEmu project. Use Docker to load them locally for ad-hoc SQL exploration.

### 1. Place dump files

```
sql/
├── quarm_<date>.sql           ← main game data (items, spells, NPCs, zones …)
├── player_tables_<date>.sql
├── login_tables_<date>.sql
└── data_tables_<date>.sql
```

### 2. Start MySQL

```bash
docker compose up -d
```

On first run, MySQL auto-executes all `.sql` files in `sql/`. Watch progress:

```bash
docker compose logs -f mysql
```

Wait until you see `ready for connections`.

### 3. Connect

| Field    | Value         |
|----------|---------------|
| Host     | `127.0.0.1`   |
| Port     | `3306`        |
| User     | `root`        |
| Password | `quarmbuddy`  |
| Database | `quarm`       |

```bash
mysql -h127.0.0.1 -uroot -pquarmbuddy quarm
```

### 4. Stop / Reset

```bash
docker compose down        # stop (data persists)
docker compose down -v     # wipe and start fresh
```

---

## Go Database Layer

The `internal/db` package provides typed, read-only access to `quarm.db`:

```go
d, _ := db.Open("backend/data/quarm.db")

// Look up by ID
item, _ := d.GetItem(1001)
spell, _ := d.GetSpell(1)
npc, _   := d.GetNPC(42)
zone, _  := d.GetZoneByShortName("qeynos")

// Paginated name search
res, _ := d.SearchItems("Sword", 20, 0)
fmt.Println(res.Total, res.Items[0].Name)

// Parse NPC special abilities
abilities := db.ParseSpecialAbilities(npc.SpecialAbilities)
// → [{Code:1 Value:1 Name:"Summon"}, {Code:18 Value:1 Name:"Unmezzable"}, ...]
```

---

## REST API Server

Start the API server (defaults to `:8080`, reads `data/quarm.db`):

```bash
cd backend
go run ./cmd/server
# or with flags:
go run ./cmd/server --addr :9000 --db /path/to/quarm.db
```

#### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/items?q=&limit=&offset=` | Search items by name |
| `GET` | `/api/items/{id}` | Get item by ID |
| `GET` | `/api/spells?q=&limit=&offset=` | Search spells by name |
| `GET` | `/api/spells/{id}` | Get spell by ID |
| `GET` | `/api/npcs?q=&limit=&offset=` | Search NPCs by name |
| `GET` | `/api/npcs/{id}` | Get NPC by ID |
| `GET` | `/api/zones?q=&limit=&offset=` | Search zones by long name |
| `GET` | `/api/zones/{id}` | Get zone by ID |
| `GET` | `/api/zones/short/{name}` | Get zone by short name |
| `GET` | `/api/config` | Get current configuration |
| `PUT` | `/api/config` | Update and persist configuration |

All search endpoints return `{"items": [...], "total": N}`. Max `limit` is 100.

---

## Configuration

On first run the server creates `~/.pq-companion/config.yaml` with defaults:

```yaml
eq_path: ""
character: ""
server_addr: :8080
preferences:
    overlay_opacity: 0.9
    minimize_to_tray: true
    parse_combat_log: true
```

Edit the file directly, or use the API:

```bash
# Read current config
curl http://localhost:8080/api/config

# Update config (full replacement)
curl -X PUT http://localhost:8080/api/config \
  -H 'Content-Type: application/json' \
  -d '{"eq_path":"/games/EverQuest","character":"Testerino","server_addr":":8080","preferences":{"overlay_opacity":0.9,"minimize_to_tray":true,"parse_combat_log":true}}'
```

The `--addr` CLI flag overrides `server_addr` from the config file when provided.

---

## WebSocket Server

Connect to `ws://localhost:8080/ws` to receive real-time events from the backend.

Messages are JSON with a type/data envelope:

```json
{"type": "zone_change", "data": {"zone": "crushbone"}}
{"type": "combat",      "data": {"actor": "You", "target": "an orc", "damage": 150}}
```

The connection is receive-only from the client side — the server broadcasts; clients do not send messages. Ping/pong keepalive runs every 54 seconds.

---

## Architecture

- **Go backend** (`backend/`): API server, log parser, timer engine, database queries, converter CLI
- **Electron shell** (`electron/`): window management, overlay windows, sidecar process lifecycle
- **React frontend** (`frontend/`): all UI, communicates with Go via REST + WebSocket
- **SQLite** (`backend/data/quarm.db`): converted EQ game data — generated once, ships with the app

See `docs/02_architecture.txt` for the full system diagram.

---

## Progress

See [PROGRESS.md](PROGRESS.md) for phase-by-phase task completion.
See [FEATURES.md](FEATURES.md) for the full feature list.
See [ROADMAP.md](ROADMAP.md) for a user-facing summary of what's built and what's coming.
