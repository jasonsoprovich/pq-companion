# Log Backfill from archived logs — implementation plan

**Created 2026-09-09.** From a Discord thread on the Character Info → Recap
"AA Points" stat reading `+0` / undercounting.

The immediate cause of that report was a parser bug (Quarm's client prints
`ability point(s).` literally; `reAAGain` only matched `point`/`points`) —
fixed in `f7889d5e`. But it surfaced a real limitation: **backfill only
ever reads the current live log file**, so anything Archive & Trim has
already moved out is unrecoverable. Grokii's ~27 AA dings over 3 days were
in a `.bak.zip` archive; his Recap could never see them.

This plan adds a scope choice to Log Backfill:

- **Current log only** — today's behavior, the default.
- **Current + all archived logs** — slower, recovers milestones from every
  `.bak.zip` / `.bak.txt` next to the live log.

---

## What the archives look like

`logparser.BackupAndPurge` (`backend/internal/logparser/cleanup.go`) writes,
in the same directory as the live log (`EQPath`):

- `eqlog_<Char>_pq.proj.<YYYY-MM-DD>.bak.zip` — a single `zip.Deflate`
  entry named `eqlog_<Char>_pq.proj.txt`, size-verified against the
  original after write.
- `eqlog_<Char>_pq.proj.<YYYY-MM-DD>.bak.txt` — uncompressed, from an
  earlier version of the feature. One example lives in `testdata/`
  (`eqlog_Osui_pq.proj.2026-05-05.bak.txt`).

Each archive is a **full** copy of the log as of its date. After archiving,
the live log keeps only the last `purgeKeepDays` (30) days. So each archive
overlaps the next file (archive or live) by ~30 days — dedup has to absorb
that. It already can: every backfill handler is `INSERT OR IGNORE` on a
natural key and timestamp-aware (see `backfill/engine.go` package doc).

---

## Backend

### 1. Archive discovery — `logparser.DiscoverArchives`

New function in `backend/internal/logparser/` (alongside `discover.go`):

```go
type ArchiveFile struct {
	Path       string
	ArchivedAt time.Time // parsed from the YYYY-MM-DD in the name; mtime fallback
	Compressed bool      // .bak.zip vs .bak.txt
	Bytes      int64     // uncompressed size — sum of zip entry UncompressedSize64, or file size
}

func DiscoverArchives(eqPath, character string) []ArchiveFile
```

- Globs `eqlog_<Char>_pq.proj.*.bak.zip` and `...*.bak.txt` in `eqPath`.
- Parses the `YYYY-MM-DD` token for ordering; falls back to file mtime when
  it won't parse.
- Returns sorted **oldest first**.
- `Bytes` feeds the progress bar's `total`.

### 2. Multi-file replay — `Registry.RunMulti`

In `backend/internal/backfill/engine.go`:

```go
func (r *Registry) RunMulti(paths []string, character string, keys []string,
	progress func(done, total int64)) (map[string]int, error)
```

- Build the handler set **once**. Stream each path through it in list
  order. Call `Finalize()` only after the last path. This makes
  `[archive1, archive2, …, liveLog]` behave exactly like one concatenated
  log — every handler's "first/last seen in window" logic is preserved.
- `total` = Σ `ArchiveFile.Bytes` + live-log size. `done` accumulates
  across files, so `BackfillProgressBar` and its ETA keep working with no
  change.
- Per-file reader:
  - `.txt` (archive or live) → `os.Open` (live log still via `openShared`).
  - `.zip` → `archive/zip`; pick the entry named
    `eqlog_<Char>_pq.proj.txt`, else the first entry whose name ends
    `.txt`; stream its `io.ReadCloser` through the existing
    `bufio.Scanner` loop (same `ParseRawLine` + `ParseLine` dispatch).
- A corrupt / unreadable archive is `slog.Warn`-ed and skipped, not fatal;
  the count of skipped files is returned so the API can surface it.
- Keep the current `Run` as a one-line wrapper:
  `return r.RunMulti([]string{logPath}, character, keys, progress)`.

### 3. API — `backend/internal/api/backfill.go`

- `POST /api/backfill` body gains:

  ```jsonc
  { "character": "...", "sections": ["..."], "scope": "current" | "all" }
  ```

  `scope` defaults to `"current"`. On `"all"`:
  `DiscoverArchives(eqPath, character)` (oldest first) + live-log path
  appended → `RunMulti`.

- `GET /api/backfill` per-character info gains `archive_count`,
  `archive_bytes`, `archive_oldest` (RFC3339 or empty) so the UI can label
  the option, e.g. *"4 archived logs · ~180 MB · back to 2026-03-01"*.

---

## Frontend

- `frontend/src/components/settings/BackfillPanel.tsx`: a two-option radio
  under the tracker list —
  - `( ) Current log only`
  - `( ) Current + archives (N files, slower)`

  Hidden / disabled when `archive_count === 0`.

- Thread `scope` through: `startBackfill(chars, sections, scope)` in
  `BackfillContext.tsx` → `runBackfill(character, sections, scope)` in
  `services/api.ts`.

- `ConfirmModal` copy: note it will read N extra files per character and
  may take several minutes.

- `BackfillProgressBar.tsx`: no change (already byte-based).

---

## Correctness notes

- **Dedup** — recap events `UNIQUE(character, at, kind, detail, value)`;
  `character_active_days` `PRIMARY KEY(character, date)`; loot / chat /
  lockout / faction / players handlers are all documented dedup-safe and
  timestamp-aware. Re-scanning the ~30-day overlap yields zero new inserts,
  so `Inserted()` counts stay honest.
- **Ordering** — UI reads that aggregate events already `ORDER BY at`
  (e.g. `progress.BuildRecap`), so replay insertion order is irrelevant to
  them. Processing oldest-first additionally keeps the in-memory
  "latest seen" handlers (faction backfill's `latest` map, players) moving
  forward only.

---

## Edge cases

- No archives → `"all"` is identical to `"current"`.
- A character with only archives and no live log won't appear in the
  Backfill character list (it's globbed from `*_pq.proj.txt`). Acceptable;
  optional follow-up: also surface archive-only characters.
- Mixed `.bak.txt` + `.bak.zip` for one character → both included, ordered
  by date.
- Same-day duplicate archive name → can't happen; `BackupAndPurge`
  overwrites in place for a given day.
- Zip whose entry name doesn't match the expected basename → fall back to
  first `.txt` entry.
- Very old archive predating a tracker's data model → handlers already
  tolerate arbitrary old timestamps.

---

## Effort / risk

Moderate. One new file (`DiscoverArchives` + a small test), ~60 lines in
`engine.go`, small API + UI changes. Low risk: additive, default behavior
unchanged, all writes already idempotent.

Test additions:

- `RunMulti` idempotency across an overlapping `[archive, live]` pair
  (re-run inserts nothing the second time).
- Zip-entry streaming, using a fixture `.bak.zip` built from an existing
  `testdata` log.
- Legacy `.bak.txt` archive is picked up by `DiscoverArchives`.

---

## Open question

Skip the ~30-day overlap window between each archive and the next file for
speed, or just scan everything and lean on `INSERT OR IGNORE`? Lean toward
scanning everything (simpler, obviously correct) unless real-world archive
sets get big enough that the redundant passes noticeably drag out the run.
