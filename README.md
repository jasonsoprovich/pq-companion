# PQ Companion

A desktop companion app for the [Project Quarm](https://www.projectquarm.com/) EverQuest emulated server.

Features: database explorer (items, spells, NPCs, zones), combat log parser, DPS meter, spell/buff/DoT timer overlays, NPC info overlay, spell checklist, config backup manager, and a custom trigger system.

---

## Development Setup

### Prerequisites

- [Docker](https://www.docker.com/) (for local MySQL database exploration)
- [Go 1.22+](https://go.dev/) (backend)
- [Node.js 20+](https://nodejs.org/) (frontend + Electron)
- A MySQL client for exploration — [DBeaver](https://dbeaver.io/) (free) or MySQL Workbench

---

## Phase 0: MySQL Database Setup

The EverQuest game data is distributed as MySQL dumps from the EQEmu project. We load these into a local MySQL container for exploration, then convert them to SQLite for distribution.

### 1. Obtain the SQL dumps

Place the EQEmu MySQL dump files (`.sql`) in the `sql/` directory:

```
sql/
├── eqemu_items.sql
├── eqemu_spells.sql
└── ... (any .sql files)
```

Files are loaded in alphabetical order on first startup.

### 2. Start MySQL

```bash
docker compose up -d
```

On first run, MySQL will automatically execute all `.sql` files in `sql/`. This can take **several minutes** depending on dump size — watch progress with:

```bash
docker compose logs -f mysql
```

Wait until you see `ready for connections` in the logs (or the healthcheck passes).

### 3. Connect

| Field    | Value       |
|----------|-------------|
| Host     | `127.0.0.1` |
| Port     | `3306`      |
| User     | `root`      |
| Password | `quarmbuddy`|
| Database | `quarm`     |

**DBeaver:** New Connection → MySQL → fill in the table above.

**CLI:**
```bash
mysql -h127.0.0.1 -uroot -pquarmbuddy quarm
```

Verify the import:
```sql
SHOW TABLES;
SELECT COUNT(*) FROM items;
```

### 4. Stop / Reset

Stop the container (data persists in the Docker volume):
```bash
docker compose down
```

Wipe everything and start fresh (re-runs SQL imports on next `up`):
```bash
docker compose down -v
```

---

## Architecture

See `docs/02_architecture.txt` for the full system diagram.

- **Go backend** (`backend/`): API server, log parser, timer engine, database queries
- **Electron shell** (`electron/`): window management, overlay windows, sidecar lifecycle
- **React frontend** (`frontend/`): all UI, communicates with Go via REST + WebSocket
- **SQLite** (`backend/data/quarm.db`): converted EQ game data, ships with the app

---

## Progress

See [PROGRESS.md](PROGRESS.md) for phase-by-phase task completion status.

See [FEATURES.md](FEATURES.md) for a full feature list by phase.
