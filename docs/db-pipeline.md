# Database Pipeline

This document describes how the EverQuest game database ships with PQ Companion —
from an upstream Project Quarm MySQL dump to the `quarm.db` SQLite file bundled
into each release.

## Overview

```
┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│  MySQL dump      │───▶│  dbconvert CLI   │───▶│  quarm.db        │
│  (sql/*.sql)     │    │  (Go)            │    │  (SQLite)        │
└──────────────────┘    └──────────────────┘    └──────────────────┘
                                  │                       │
                                  ▼                       ▼
                         ┌──────────────────┐    ┌──────────────────┐
                         │  validation      │    │  data-latest     │
                         │  (row/FK/spot)   │    │  GitHub release  │
                         └──────────────────┘    └──────────────────┘
```

1. Upstream Project Quarm publishes periodic MySQL dumps of its live database.
2. The `dbconvert` Go CLI converts those dumps into a single SQLite file that the
   app ships with.
3. A validation suite checks the resulting database against row-count minimums,
   referential integrity, and known-value spot checks.
4. The `data-release` GitHub Actions workflow uploads the validated database to
   the `data-latest` release. The installer build and the CI test job both pull
   `quarm.db` from there.

## Why SQLite?

Shipping a MySQL server with a desktop app is not practical. SQLite is:
- embedded (no sidecar process beyond the Go backend),
- cross-platform (pure-Go driver `modernc.org/sqlite`, no CGO),
- read-only-friendly (the game database never mutates at runtime).

## Running the pipeline locally

### Prerequisites

- Go 1.22+
- A Project Quarm MySQL dump (`quarm_YYYY-MM-DD.sql`). Drop it into `sql/`.
  - Optionally include `player_tables_*.sql` and `login_tables_*.sql` from the
    same dump set.
  - Helper files (`drop_system.sql`, `example_queries.sql`) are ignored
    automatically.

### Convert from dump files

```bash
# From repo root
cd backend
go run ./cmd/dbconvert --from-dump --sql-dir ../sql --output data/quarm.db
```

Flags:

| Flag | Purpose |
|---|---|
| `--from-dump` | Read from `.sql` files on disk. |
| `--from-mysql` | Alternative: read directly from a live MySQL connection. |
| `--sql-dir <dir>` | Directory to scan for `.sql` files (default: auto-detect). |
| `--sql-files a.sql,b.sql` | Explicit file list. Overrides `--sql-dir`. |
| `--output <path>` | Where to write `quarm.db` (default: auto-detect). |
| `--validate` | Run validation after conversion (on by default). |
| `--validate-only` | Skip conversion; only validate an existing output database. |
| `--verbose` | Debug logging, including every emitted DDL statement. |

### Validate without re-converting

```bash
cd backend
go run ./cmd/dbconvert --validate-only --output data/quarm.db
```

Validation reports three severities:
- `ok` — check passed
- `warning` — non-fatal anomaly (e.g., a small number of orphan loot rows — the
  app handles these gracefully)
- `error` — likely a broken import; the CLI exits non-zero

The CLI returns exit code `1` when any check reports an error.

### Idempotency

Re-running the pipeline on the same input produces the same output rows.
- All `INSERT` statements use `INSERT OR REPLACE`, so re-running overwrites rather
  than duplicating.
- `CREATE TABLE` uses `IF NOT EXISTS`.
- Indexes use `CREATE INDEX IF NOT EXISTS`.

SQLite internal `rowid` values may differ between runs — they are never exposed
via the app's API, so this does not matter for users.

## What the conversion does

The converter lives in `backend/internal/converter/`:

| File | Responsibility |
|---|---|
| `dump.go` | Stream SQL dump files, execute `CREATE TABLE` / `INSERT`. |
| `schema.go` | Rewrite MySQL DDL to SQLite DDL (type mapping, backtick handling, index extraction). |
| `mysql_conn.go` | Alternative path: copy tables directly from a live MySQL server. |
| `validate.go` | Post-conversion row counts, referential integrity, spot checks. |

Conversion steps:
1. Open the output SQLite file and set bulk-load pragmas (`journal_mode=WAL`,
   `synchronous=NORMAL`, `foreign_keys=OFF`, 64 MB cache).
2. For each dump file, stream statements one at a time (handles multi-MB
   `INSERT` blocks without loading them fully into memory).
3. For `CREATE TABLE`: rewrite column types (`mediumint(8) unsigned` → `INTEGER`,
   `varchar(64)` → `TEXT`, etc.), convert backtick identifiers to double quotes,
   extract `KEY` / `UNIQUE KEY` definitions into standalone `CREATE INDEX`
   statements, prefix index names with the table name (SQLite shares the
   table/index namespace).
4. For `INSERT`: parse values manually (custom tokenizer handles MySQL string
   escapes like `\'`, `\\`, `\n`, `\Z`), batch into multi-row inserts sized to
   stay under SQLite's 32 766-parameter limit, wrap each table in a single
   transaction.
5. Validate.

Unsupported MySQL features (skipped intentionally):
- `SET`, `LOCK TABLES`, `UNLOCK TABLES`, `ALTER TABLE` — discarded.
- `ENGINE=`, `DEFAULT CHARSET=`, `COLLATE=` column modifiers — stripped.
- `AUTO_INCREMENT` — dropped. SQLite's `INTEGER PRIMARY KEY` is equivalent.
- Stored procedures / views / triggers — not present in the PQ dumps.

## Validation suite

Defined in `backend/internal/converter/validate.go`. Three categories:

**Row-count checks (errors)**
Minimum row counts for 14 core tables. A dump that loses a table silently trips
the check.

**Referential integrity checks (warnings → errors)**
For every documented foreign key (loot chain, spawn chain, NPC spell chain),
count orphan child rows. Reported as a warning by default; if the orphan count
exceeds the per-check threshold (500 by default) the check escalates to an
error — at that point the more plausible explanation is a partial import
rather than dump cruft.

**Spot checks (errors)**
Well-known records that must exist: `items.id=1001` ("Cloth Cap"), `zone.short_name='northkarana'`,
`spells_new.id=13` ("Complete Healing"). These catch partial imports where the
row-count minimums still pass.

## CI release workflow

`.github/workflows/data-release.yml` rebuilds `quarm.db` and uploads it to the
`data-latest` release. It runs on:
- `workflow_dispatch` — manual rebuild. Optional input: specific dump file to
  convert (under `sql/`). Optional release notes.
- Push to `main` under `sql/**` — auto-rebuild when a new dump lands.

The workflow:
1. Checks out the repo.
2. Runs `dbconvert --from-dump` with validation enabled (the job fails if
   validation reports errors).
3. Uploads `quarm.db` as a workflow artifact (30-day retention — a safety net
   in case the release upload fails).
4. Creates or updates the `data-latest` prerelease and uploads `quarm.db` with
   `--clobber`.

Once `data-latest` contains a fresh `quarm.db`:
- `.github/workflows/ci.yml` downloads it to run Go tests against real data.
- `.github/workflows/release.yml` downloads it when packaging a new Windows
  installer.

## Bootstrap: first-time setup

For a brand-new clone or a new machine, `quarm.db` is not in the repo (too
large — ~84 MB). Fetch it from the release:

```bash
gh release download data-latest \
  --pattern "quarm.db" \
  --dir backend/data \
  --repo jasonsoprovich/pq-companion
```

If `data-latest` does not yet exist, run the pipeline locally (above) and
upload the result:

```bash
gh release create data-latest backend/data/quarm.db \
  --prerelease \
  --title "Game Database" \
  --notes "Initial upload"
```

## Tracking schema changes between dumps

Dumps occasionally change schema (new columns, renamed tables). To diff two
dumps:

```bash
diff <(grep '^CREATE TABLE' sql/quarm_OLD.sql | sort) \
     <(grep '^CREATE TABLE' sql/quarm_NEW.sql | sort)
```

If the converter hits a new column type it cannot map, it falls back to
`TEXT`. Add the mapping to `mysqlTypeMap` in
`backend/internal/converter/schema.go` when introducing an unknown type.

When a rename breaks one of the queries in `backend/internal/db/queries.go`,
the CI Go tests catch it — all queries run against the real database in CI.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| `exec create: ... near "AUTO_INCREMENT"` | New MySQL feature the rewriter does not strip. Add handling in `convertColumnDef`. |
| `validation failed: row count X` | The dump is missing rows — often a truncated file. Redownload and retry. |
| `validation failed: spot check` | The conversion imported the row count but not the actual data — e.g., a silent `INSERT` parser failure. Run with `--verbose` and inspect logs. |
| `data-release` workflow can't find dump | The workflow converts files under `sql/`. Commit the dump there (or adjust the input path). |
