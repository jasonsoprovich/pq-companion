# Code Review Findings — 2026-07-01

Full-app review pass (Go backend hot paths, API/DB layer, React frontend,
Electron shell + build). Check items off as they're fixed; each entry has
enough detail to fix without re-reviewing. Severity reflects real-world
impact for this app; confidence is how sure the reviewer was after reading
the code (certain / likely / possible).

**Areas verified clean (no action needed, don't re-audit):** ws hub writer
discipline & slow-client handling, trigger engine (compile-once regexes,
bounded history), spelltimer engine, SQL parameterization everywhere,
rows.Close/rows.Err/tx-rollback hygiene, strconv validation, startup
critical path (nothing heavy before BACKEND_PORT), zip handling (no
zip-slip), migrations idempotency, frontend listener/interval cleanup
(40+ sites checked), useWebSocket subscribe/unsubscribe, memory caps
(log feed 200, alerts 8, recent_fights 20), audio engine activePlaybacks
GC fix, contextIsolation/nodeIntegration on all 13 windows, IPC
single-registration + sender resolution, sidecar shutdown on normal quit
paths, port selection, auto-update wiring, electron-builder file globs.

---

## Priority 1 — correctness bugs with real user impact

### [x] 1.1 Config save is non-atomic — crash mid-write bricks the install
- **Where:** `backend/internal/config/config.go:1021-1030` (`save()`)
- **Severity/confidence:** HIGH / certain
- **Problem:** `os.WriteFile(m.path, ...)` truncates `config.yaml` in place
  before writing. This runs on every settings change, roll-tracker
  preference persist, and character auto-detect during gameplay. A crash /
  power loss / disk-full mid-write leaves truncated YAML; on next launch
  `config.Load()` fails and `cmd/server/main.go:67-71` does `os.Exit(1)` —
  app won't start until the user hand-deletes the file.
- **Fix:** Write to a temp file in the same dir, then `os.Rename` over the
  target (same pattern `appbackup.Export` already uses).

### [x] 1.2 Config lost-update races across three uncoordinated writers
- **Where:** `config/config.go:1006-1018` (`Update`),
  `cmd/server/main.go:918-937` (roll tracker persist),
  `cmd/server/main.go:1027-1040` (character auto-detect),
  `api/config.go:136-155` (`PUT /api/config`)
- **Severity/confidence:** MEDIUM / certain
- **Problem:** Manager only has `Get()` (copy) + `Update(c)` (wholesale
  replace). Concurrent writers silently revert each other's fields. Worse:
  the PUT handler decodes into a **fresh** `config.Config`, so any field an
  older/stale frontend omits decodes to the zero value and gets persisted —
  silently wiping that setting.
- **Fix:** Add `Modify(func(*Config))` that runs under the manager lock for
  backend callers; in the PUT handler, decode over `h.mgr.Get()` instead of
  a zero struct.

### [ ] 1.3 Sidecar spawn failure / early crash → permanent black screen, no diagnostics
- **Where:** `electron/main/index.ts:680-728` (spawn, no `'error'`
  listener; `'exit'` handler at :723 only logs), `:535-540`
  (`getBackendPort()` promise never settles), `logger.ts:67-69`
  (uncaughtException catch-all swallows the error event)
- **Severity/confidence:** MEDIUM / certain
- **Problem:** If `pq-companion-server.exe` fails to launch (AV quarantine
  is the realistic case — the quarm.db recovery dialog at :614-617 already
  anticipates it), `BACKEND_PORT` never arrives, every pending
  `backend:port` IPC hangs, user gets a permanent black screen with zero
  indication why.
- **Fix:** Add a `'error'` listener on the spawn; on `'error'` or early
  `'exit'` before a port was resolved, show a dialog (reuse the quarm.db
  recovery UX) instead of hanging.

### [x] 1.4 Singleton WebSocket can duplicate itself — everything fires twice for the rest of the session
- **Where:** `frontend/src/hooks/useWebSocket.ts:84-95` (`onclose`), guard
  at `:33-37`
- **Severity/confidence:** MEDIUM / certain mechanism, possible occurrence
- **Problem:** `onerror` calls `ws.close()` (socket → CLOSING). The
  `connect()` guard only short-circuits on OPEN/CONNECTING, so a consumer
  mount during the CLOSING window creates socket S2. When S1's queued
  `onclose` fires it unconditionally nulls `socket` and schedules a
  reconnect → S3 while S2 is still alive. All feed the shared
  `messageHandlers` set: doubled meter updates, doubled log feed, double
  trigger fires (lands outside the 750 ms audio dedup — plausible
  remaining contributor to the "audio alerts multiply" report).
- **Fix:** `if (socket !== ws) return;` at the top of `onclose`/`onopen`;
  treat CLOSING as "connection pending" in the connect guard.

### [x] 1.5 Raid-threat taunt offsets deleted under non-canonical mob key
- **Where:** `backend/internal/raidthreat/assembler.go:368-374`
  (`EventKill` handler)
- **Severity/confidence:** MEDIUM / certain mismatch, likely occurrence
- **Problem:** `applyTaunt` (:278) canonicalizes the mob name before
  storing, but the kill handler does `delete(a.taunt, data.Target)` with
  the **raw** name (the next line canonicalizes for `dismissed`,
  confirming the oversight). Kill lines arrive with capitalized leading
  articles ("A lightcrawler has been slain…"), so the canonical key never
  gets removed — dead mobs stay pinned on the raid-threat overlay with the
  tank's taunt row, and the map accretes per kill.
- **Fix:** One line:
  `delete(a.taunt, logparser.CanonicalNPCName(data.Target))`.

### [x] 1.6 Deleting a character orphans 8 child tables (foreign_keys never enabled)
- **Where:** `backend/internal/character/store.go:178-181` (`Delete`); no
  user.db DSN sets `_pragma=foreign_keys(1)` (verified by grep)
- **Severity/confidence:** MEDIUM / certain
- **Problem:** All `ON DELETE CASCADE` clauses in the schema are inert.
  Deleting a character leaves AAs, raid buffs, tasks, subtasks, wishlist,
  slot layouts, and upgrade weights orphaned forever (AUTOINCREMENT ids so
  no cross-character bleed — silent permanent bloat).
- **Fix:** Explicit child deletes in a transaction inside `Store.Delete`
  (safer than enabling FK enforcement globally on the shared file).

---

## Priority 2 — raid-time performance (hot path)

### [x] 2.1 Combat meter re-marshals entire state (incl. 20 archived fights) per damage/heal line
- **Note:** Fixed via the 1 Hz coalescing ticker (dominant cost was the
  per-hit frequency: merge + RecentFights embed + marshal, hundreds/sec →
  1/sec). Deliberately did NOT split RecentFields/deaths into a leaner
  payload — at 1 Hz that embed is negligible and a lean-payload protocol
  split would need a matching frontend "absent = unchanged" change (risk not
  worth it). Transitions (fight start/end, death, reset) stay immediate.
- **Where:** `backend/internal/combat/tracker.go:1022-1025` (recordHit),
  `:1067-1070` (recordHeal), `:1544-1552` (snapshot);
  `cmd/server/main.go:949-950` also broadcasts each parsed event separately
- **Severity/confidence:** MEDIUM / certain mechanism, likely CPU cost
- **Problem:** Every hit line rebuilds the merged fight (O(fights ×
  entities) map merge + sort), embeds `RecentFights` (up to 20 full
  `FightSummary` values) plus a fresh copy of every death record, then
  JSON-marshals + broadcasts the whole thing. During raid AoE spam this is
  hundreds of KB/s of marshaling for data that only changes on
  archive/death. Also: with the 256-slot hub channel, this spam is the one
  scenario where a `trigger:fired` event could get dropped.
- **Fix:** Broadcast `RecentFights`/death records only on archive/death
  events; throttle per-hit combat broadcasts to the existing 1 Hz ticker.

### [x] 2.2 Cast-index fallback runs ~1,600 regexes linearly per unclassified line
- **Where:** `backend/internal/logparser/castindex.go:141-154`, invoked
  from `classifyMessage` (parser.go:813-820); with raw-feed enabled,
  `dispatchLine` (main.go:990-991) classifies unmatched lines a second time
- **Severity/confidence:** MEDIUM / certain, likely measurable on raid logs
- **Problem:** `otherByPattern` holds one anchored regex per distinct
  `cast_on_other` string (1,628 in current quarm.db). Every emote, NPC say,
  system message, etc. walks the full slice — largest constant per-line
  cost in the pipeline, doubled when raw feed is on.
- **Fix:** Each regex is `^(name)<literal suffix>$` — do a
  `strings.HasSuffix` pre-filter before running each regex, or bucket by
  final word / last N bytes. Eliminates ~all of the scan.

### [x] 2.3 Per-line SQLite writes share MaxOpenConns(1) with slow HTTP reads — dispatch chain can stall
- **Where:** `backend/internal/chat/store.go:56` + `chat/consumer.go`
  (Insert in `HandleLine`); `players/store.go:114` +
  `players/consumer.go:107,126,159-182`; unindexed `LIKE '%…%'` at
  `chat/store.go:246`, `players/store.go:627`
- **Severity/confidence:** MEDIUM / certain serialization, likely visible
- **Problem:** Every matched chat line does an `Exec` on the parse
  goroutine (tells add a second tx via `RecordTell → TouchInteraction`).
  The same single-connection store serves HTTP; a Chat History `LIKE` scan
  holds the only connection, the parse goroutine blocks in `database/sql`
  pool wait (busy_timeout never even consulted), and because
  `dispatchLine` (main.go:985-1020) is one serial chain, triggers / spell
  timers / DPS meter all stall behind it.
- **Fix:** `SetMaxOpenConns(2+)` on these stores (WAL supports concurrent
  readers), or queue chat/player writes onto a worker goroutine.

### [x] 2.4 Roll tracker: Active sessions never evicted in manual mode + full-state broadcast per roll
- **Where:** `backend/internal/rolltracker/tracker.go:383, 394, 448-458`
  (eviction skips Active), `:595-619` (`stateLocked` deep-copies
  everything per roll)
- **Severity/confidence:** MEDIUM / certain mechanism, likely impact
- **Problem:** In default manual mode, `staleAfter` (5 min) only prevents
  new rolls joining a stale session — a fresh permanently-Active session is
  created per lull, `maxSessions` (20) never bites, and per-roll broadcast
  work grows linearly all night.
- **Fix:** Auto-flip `Active=false` once a session passes `staleAfter`
  (it can already never accept another roll).

### [ ] 2.5 Roll tracker re-runs the chat regex battery on every raw line
- **Where:** `rolltracker/tracker.go:161-174` (`SetItemMatcher`, wired
  unconditionally at main.go:943)
- **Severity/confidence:** LOW / certain duplication, possible measurable
- **Problem:** `chat.ParseChat(msg)` (~16 anchored regexes) runs a second
  time per line in the same dispatch chain where the chat consumer already
  ran it.
- **Fix:** Share the chat consumer's parse result, or gate the matcher on
  roll-tracker relevance.

---

## Priority 3 — frontend correctness cluster

### [x] 3.1 Stale-response races on every debounced search
- **Where (no guard):** `components/GlobalSearch.tsx:116-130` (also
  repopulates after query cleared), `components/ItemSearchModal.tsx:54-61`,
  `components/SpellSearchPicker.tsx:34-39`, `pages/ItemsPage.tsx:430-440`,
  `SpellsPage.tsx:70-83`, `NpcsPage.tsx:59-69`, `ZonesPage.tsx:68-78`,
  `RecipesPage.tsx:73-94` (loadMore also appends stale page for old query),
  `PlayersPage.tsx:475-511`, `LootTrackerPage.tsx:87-99`,
  `ChatHistoryPage.tsx:124-162`
- **Severity/confidence:** MEDIUM / likely
- **Problem:** Older slow response overwrites newer results. Query cost is
  non-monotonic (broad LIKE scans slower than narrow) and `api.ts`'s
  connection-retry loop (:83-103, 5×250 ms) can delay an early request past
  later ones.
- **Fix:** `pages/QuestsPage.tsx:93-108` already implements the correct
  `seqRef` sequence-guard pattern — copy it to every site above.

### [x] 3.2 ItemSearchModal "Searching…" gets permanently stuck
- **Where:** `components/ItemSearchModal.tsx:47-64` (loading set
  synchronously, only reset in debounced `.finally`); open-reset effect at
  `:38-44` doesn't reset `loading`
- **Severity/confidence:** MEDIUM / certain (trivially reproducible)
- **Problem:** Backspace below 2 chars (or close the modal) within the
  200 ms debounce window → cleanup clears the timeout, `<2` branch returns
  without resetting loading → "Searching…" forever; reopening doesn't
  recover.
- **Fix:** `setLoading(false)` in the `<2` branch and the open-reset
  effect (or move `setLoading(true)` inside the timeout).

### [x] 3.3 Combat History pagination: stale fetch overwrites newer page/filter
- **Where:** `pages/CombatHistoryPage.tsx:1136-1158` (`fetchPage`)
- **Severity/confidence:** MEDIUM / certain mechanism, likely occurrence
- **Problem:** No sequence guard; double-Next or Apply-during-fetch can
  leave older response resolving last — page-1 rows shown while paginator
  says page 2; stale `.finally` ends loading while newer request pending.
- **Fix:** Same `seqRef` pattern as 3.1.

### [ ] 3.4 Combat Log React keys collide for fights starting in the same second
- **Where:** `pages/CombatLogPage.tsx:1118` (`key={row.fight.start_time}`)
- **Severity/confidence:** MEDIUM / likely
- **Problem:** 1-second log timestamp resolution + per-NPC fights → two
  mobs pulled in the same second share a key. `FightRow` holds local state
  (`expanded`, `copied`), so as fights age out of the 20-cap, one fight's
  expanded breakdown can attach to a different fight's data.
- **Fix:** Key on `start_time + primary_target` (or add a backend fight id).

### [ ] 3.5 useHistoryNav: double entry on mount + REPLACE counted as PUSH → Back/Forward drift
- **Where:** `hooks/useHistoryNav.ts:8-28`
- **Severity/confidence:** MEDIUM / certain (double push), likely (drift)
- **Problem:** useRef initializer seeds the stack with the initial
  location, then the mount effect pushes it again → `[X, X]`; `canGoBack`
  true at boot, first Back is a no-op that lights Forward. Redirect routes
  (`/combat` → `/combat/log`, index-route replaces) record two stack
  entries vs one real history entry — drift accumulates.
- **Fix:** Use `useNavigationType()`; skip the POP on mount and skip
  REPLACE entries.

---

## Priority 4 — API/DB correctness (secondary)

### [x] 4.1 GetNPCsByZone solo-spawn heuristic returns provably wrong NPCs
- **Where:** `backend/internal/db/queries.go:2038-2047` (second UNION arm)
- **Severity/confidence:** MEDIUM / certain (data-verified)
- **Problem:** Treats `spawn2.spawngroupID` as an `npc_types.id`; keyspaces
  collide. Verified live: Crushbone's NPC list includes 5 Plane of
  Tactics mobs (e.g. `A_Chaos_Boar`, id 214287) whose ids equal Crushbone
  spawngroup ids. The `EXISTS` guard only checks *some* NPC has the id.
- **Fix:** Stronger guard (e.g. spawngroup has no `spawnentry` rows) or
  remove the arm.

### [x] 4.2 Independent MIN() aggregates pair values from different rows
- **Where:** `db/queries.go:461-497` (`GetItemSources`), `:1826-1828`
  (`GetSpellVendorOptions`)
- **Severity/confidence:** MEDIUM / certain (data-verified)
- **Problem:** Under `GROUP BY n.id`, `MIN(z.long_name)` and
  `MIN(s2.zone)` come from different zones for multi-zone NPCs. Verified:
  `Sea_King` shows label "Erud's Crossing" but nav short-name `erudnext`
  (Erudin) — UI link goes to the wrong zone. Same shape gives the
  spell-vendor route optimizer phantom `(MIN(x), MIN(y))` coordinates.
- **Fix:** Correlate the aggregates (pick one row per NPC — e.g. window
  function or a min-by-zoneid join) instead of independent MINs.

### [x] 4.3 Bulk trigger edits: DB failures → 400, partial writes left live without engine.Reload()
- **Where:** `api/triggers.go:1045-1048` (bulk), `:532-570` (import commit)
- **Severity/confidence:** MEDIUM / certain
- **Problem:** `BulkApplyActions`/`BulkConvertTTSToSound` update one at a
  time and bail mid-loop; handler maps all errors (incl. real SQLite
  failures) to 400 and returns **before** `h.engine.Reload()` — DB and
  running engine out of sync, partial `BulkResult` discarded.
  `importCommit` same shape: mid-loop failure 500s with k triggers
  persisted and no reload; a retry duplicates them under new IDs.
- **Fix:** Wrap in a transaction (or at minimum always reload on error
  paths); return 500 for store errors.

### [x] 4.4 Exact-name NPC/item lookups full-scan; N+1 amplified by lockouts endpoint
- **Note:** Indexes added to the converter's finalize pass; they take effect
  on the NEXT quarm.db regeneration / data release (the currently-bundled db
  won't have them until then). Verified on a copy: both lookups now SEARCH via
  the NOCASE index instead of SCAN.
- **Where:** `db/queries.go:653-746` (`GetNPCByName`, `GetNPCIDByName`,
  `GetItemIDByName` + variants); `api/lockouts.go:82-85`
- **Severity/confidence:** MEDIUM / certain (EXPLAIN-verified)
- **Problem:** `Name = ? COLLATE NOCASE` can't use the BINARY-collated
  `items__name_idx`; `npc_types.name` has no index at all.
  `GET /api/lockouts/{char}` does one such lookup per lockout entry; NPC
  overlay variant lookup hits it per target change.
- **Fix:** Add NOCASE indexes on `npc_types(name)` and `items(Name)` in
  the converter's hot-join index pass (see quarm.db regen memory — indexes
  auto-apply during conversion).

### [x] 4.5 Players store txs read-then-write → SQLITE_BUSY_SNAPSHOT bypasses busy_timeout (done as 2.3 prerequisite)
- **Where:** `players/store.go` `Upsert` (:327-345), `TouchInteraction`
  (:258-275), `UpdateGuild` (:430-445), `BackfillUpsert` (:495-512)
- **Severity/confidence:** MEDIUM / likely
- **Problem:** Deferred tx whose first statement is a read, then writes in
  the same tx. Under WAL, upgrading read→write after another connection
  commits returns SQLITE_BUSY immediately (busy handler not consulted).
  A `/who` burst hitting players while chat/loot commit → intermittent
  "database is locked". (All other multi-statement txs in the codebase
  write first, so they're covered.)
- **Fix:** `_txlock=immediate` on this store's DSN, or issue a write
  before the read.

### [x] 4.6 Hand-built JSON error bodies invalid when values contain quotes/backslashes
- Also fixed the same anti-pattern found in `api/log.go` (3) and
  `api/keys.go` (1) — literal-string form, so they only had the text/plain
  Content-Type issue, but converted for consistency. Zero `http.Error` JSON
  bodies remain in internal/api.
- **Where:** `api/zeal.go:228, 236, 240, 297, 367-402, 509, 641-671, 731`;
  `api/backup.go:62, 77, 91, 105, 119, 136`
- **Severity/confidence:** MEDIUM / certain
- **Problem:** Patterns like `http.Error(w, \`{"error":"\`+err.Error()+…)`:
  Windows paths (`C:\Users\…`) produce invalid JSON escapes, `%q` emits
  unescaped inner quotes; frontend `res.json()` throws on exactly the
  error paths meant to explain the failure. `http.Error` also sets
  Content-Type to text/plain.
- **Fix:** Use the existing `writeError` helper at every site.

### [x] 4.7 Gear upgrade finder renders DB failures as "no upgrades"
- **Where:** `api/characters_upgrades.go:272-274, 346-348`
- **Severity/confidence:** MEDIUM / certain
- **Problem:** `if err != nil { cands = nil }` → 200 with empty
  candidates; a locked/corrupt quarm.db is indistinguishable from
  "already best in slot"; error not even logged.
- **Fix:** Return 500 (and log) on DB error.

### [x] 4.8 Data race: enrichEntries mutates Zeal watcher's shared cached inventory
- **Where:** `api/zeal.go:39-77, 144-148`
- **Severity/confidence:** MEDIUM race / benign symptoms likely
- **Problem:** `watcher.Inventory()`/`Quarmy()` return the cached pointer
  (not a copy); handler writes `entries[i].Icon`/`MaxCharges` after the
  lock is released. Concurrent GETs write the same slice elements
  unsynchronized — trips `-race` immediately.
- **Fix:** Enrich into a copy.

### [x] 4.9 N+1 query storms: charm pets + spell statDeltas
- **Done:** statDeltas — added db.GetSpells (IN batch), propagate real DB
  errors as 500 (was the actual bug: swallowed errors → wrong buff totals).
- **Deferred:** charm.go SummarizeNPCCaster-per-candidate — dev-gated
  (charm_pet_finder_enabled) and latency-only, no correctness bug. Batching
  it is a larger refactor; left for a later pass.
- **Where:** `api/charm.go:251` (SummarizeNPCCaster per candidate →
  hundreds of queries in caster-heavy zones; dev-gated, latency only);
  `api/spells.go:126-129` (up to 200 individual `GetSpell` calls where one
  `IN (...)` batch would do — the `ItemIcons` batch pattern exists; also
  `if err != nil || sp == nil { continue }` treats real DB errors as
  "spell doesn't exist" → 200 with silently wrong buff totals)
- **Severity/confidence:** MEDIUM / certain
- **Fix:** Batch with `IN (...)`; propagate real errors in statDeltas.

### [x] 4.10 Misleading status codes (pattern, several handlers)
- **Where / what:**
  - `api/wishlist.go:149-152`: any `GetItem` error → 400 "item not found"
    (split `sql.ErrNoRows` from 500, as `items.go` does)
  - `api/characters.go:330-334`: `spellModifiers` → 404 "spell not found"
    for any DB error
  - `api/replay.go:200-203`: all `Replayer.Start` errors → 409 (inverted
    ranges should be 400, file-open failures 500)
  - `api/zeal.go:665-668`: any `os.Stat` failure → 400 "log in once…";
    gate on `os.IsNotExist`
  - `api/characters.go:188-212`: `aas` swallows `ListAvailableAAs` errors
    → 200 with empty AA catalog, unlogged
- **Severity/confidence:** LOW / certain
- **Fix:** Distinguish not-found from DB error at each site.

---

## Priority 5 — Electron hardening & lifecycle

### [ ] 5.1 pq-audio:// protocol handler = unrestricted arbitrary-file-read primitive
- **Where:** `electron/main/index.ts:2543-2569` (handler), `:50-61`
  (registered standard + secure + supportFetchAPI + bypassCSP)
- **Severity/confidence:** MEDIUM / likely (requires renderer compromise)
- **Problem:** `readFile()`s whatever path is in the request URL — no
  extension allowlist, no directory confinement; reachable from all 13
  windows. A compromised renderer can pull user.db, SSH keys, cookies.
- **Fix:** At minimum gate on `audioMimeType()` returning a known audio
  type (reject octet-stream); better, confine to configured sound dirs.

### [ ] 5.2 Navigation/window.open hardening gaps + unvalidated shell.openExternal
- **Where:** `electron/main/index.ts:880-883` (main window
  setWindowOpenHandler → `shell.openExternal(url)` with no scheme check);
  11 popout overlays + trigger overlay have **no** setWindowOpenHandler
  (default creates a child window inheriting the preload); no window
  registers `will-navigate`
- **Severity/confidence:** MEDIUM / likely (defense-in-depth layer)
- **Fix:** Gate openExternal to `http:`/`https:`; add a deny-all
  setWindowOpenHandler to every overlay; add a `will-navigate` guard
  allowing only the app's own origin/file URL.

### [ ] 5.3 Orphaned sidecar on hard crash of Electron main
- **Where:** `electron/main/index.ts:672-771` (cleanup only on quit
  paths); Go server has no parent-death watchdog
- **Severity/confidence:** LOW-MEDIUM / likely
- **Problem:** Native crash / Task Manager kill of Electron leaves
  `pq-companion-server.exe` alive — tailing logs, holding user.db, and
  holding a lock on the installed exe that wedges the next NSIS update.
- **Fix:** Cheapest robust fix: Go server treats stdin EOF as "parent
  died, exit" (stdio is already piped).

### [x] 5.4 closeLogger() runs at the start of the quit sequence — shutdown is unlogged
- **Where:** `electron/main/index.ts:23` (before-quit closeLogger
  registered first) vs `:2632-2643` (real teardown handler)
- **Severity/confidence:** LOW / certain
- **Problem:** Logger fd closes, quit is prevented, app runs up to 5 more
  seconds of teardown (taskkill, snapshot writes) with `appendLine()`
  silently no-oping — exactly the phase where update-wedge evidence would
  be written.
- **Fix:** Move `closeLogger()` to `will-quit`/`quit` (or register last).

### [ ] 5.5 window:drag:start leaks one 'closed' listener per drag
- **Where:** `electron/main/index.ts:1653`
- **Severity/confidence:** LOW / certain
- **Problem:** `win.once('closed', …)` per drag, never removed →
  MaxListenersExceededWarning noise after ~10 drags, closures accumulate.
- **Fix:** Register the `once('closed')` a single time at window creation,
  or remove it in `stopDrag`.

### [ ] 5.6 Persist-on-close of window bounds is a dead no-op
- **Where:** `electron/main/index.ts:331-345, 436-441, 478-499`
- **Severity/confidence:** LOW / certain
- **Problem:** 'close' handlers route through the 500 ms debounce; timer
  fires after destroy and bails on `isDestroyed()` (and reads bounds at
  timer time anyway). Move-then-close-within-500ms loses the final
  position.
- **Fix:** In the 'close' handler, capture bounds synchronously and write
  immediately.

### [ ] 5.7 sandbox: false on all 13 windows for no reason
- **Where:** `electron/main/index.ts:849` + every `create*Overlay`
- **Severity/confidence:** LOW / certain
- **Problem:** Preload uses only contextBridge/ipcRenderer (works fine
  sandboxed); sandbox off means a Chromium renderer exploit yields a
  full-privilege process.
- **Fix:** Remove the line (sandbox is default-on since Electron 20);
  needs a Windows smoke test.

### [ ] 5.8 Wildcard CORS + no auth on local API; macros PUT allows path traversal
- **Where:** `api/router.go:48-59`; `api/zeal.go` macros PUT (`character`
  field reaches `filepath.Join(eqPath, character+"_pq.proj.ini")`
  unsanitized — `..\` escapes the EQ dir)
- **Severity/confidence:** LOW-MEDIUM / possible (accepted-risk class)
- **Problem:** Any web page that discovers the port can call
  state-changing endpoints.
- **Fix:** Standard Electron-sidecar hardening: per-launch bearer token
  passed via env, checked in middleware. At minimum sanitize the
  `character` field (reject path separators / `..`).

### [ ] 5.9 Renderer-only libraries shipped twice (installer bloat)
- **Where:** `package.json:20-31`
- **Severity/confidence:** LOW / certain
- **Problem:** `react-router-dom`, `lucide-react`, `@dnd-kit/*`,
  `@xyflow/react`, `dagre`, `@types/dagre` are prod dependencies; Vite
  bundles them into out/renderer AND electron-builder packs their
  node_modules into the asar. Only `electron-updater` needs to be prod.
- **Fix:** Move all but `electron-updater` to devDependencies; verify
  packaged build still boots.

### [ ] 5.10 Dev-only: stale server-port file latches onto a dead port
- **Where:** `electron/main/index.ts:648-670`; Go writes
  `~/.pq-companion/server-port` on bind, never deletes on exit
- **Severity/confidence:** LOW / certain (dev-only annoyance)
- **Fix:** mtime freshness check or health-probe the port before accepting.

---

## Priority 6 — smaller / hygiene

### [ ] 6.1 Stale time.AfterFunc fire races (threat / hate-mod / fight timeout)
- **Where:** `threat/tracker.go:941-948` (`armExpiryLocked` → `endMob`:
  stale fire deletes an actively-refreshed mob's hate), `:600-610`
  (`expireMod`: recast buff deleted for its whole refreshed duration);
  `combat/tracker.go:864-918` (`fightTimerExpired`: fightID guard catches
  recreated fights but not the same fight with fresh activity — splits one
  encounter into two parses)
- **Severity/confidence:** LOW / certain race, rare in practice
  (microsecond window at each timeout boundary)
- **Fix:** Re-check staleness (`time.Since(lastTouched) >= timeout` or a
  generation counter) inside the callback under the lock before deleting.

### [x] 6.2 Trigger overlay GC tick re-renders 4×/sec even when empty
- **Where:** `pages/TriggerOverlayWindowPage.tsx:380-391`
- **Severity/confidence:** LOW / certain
- **Problem:** `setAlerts(prev => prev.filter(...))` always returns a new
  array — ~14k no-op renders/hour in a hidden-but-alive window.
- **Fix:** Return `prev` unchanged when nothing was removed.

### [ ] 6.3 CH Metronome ticks at 10 Hz unconditionally; Roll Tracker at 1 Hz with no sessions
- **Where:** `pages/CHMetronomeOverlayWindowPage.tsx:247-250`,
  `components/overlays/CHMetronomePanel.tsx:219-222`;
  `pages/RollTrackerWindowPage.tsx:412-416`,
  `components/overlays/RollTrackerPanel.tsx:428`
- **Severity/confidence:** LOW / certain
- **Fix:** Gate the ticker on an active anchor / active session (the
  DPS/HPS overlays gate correctly on `in_combat` — reference pattern
  in-repo).

### [ ] 6.4 NPC overlay re-renders full stats/loot tree every second while any spell timer exists
- **Where:** `pages/NPCOverlayWindowPage.tsx:484` +
  `hooks/useTargetTimers.ts:29-32`
- **Severity/confidence:** LOW / likely
- **Fix:** Move `useTargetTimers` into a child component rendered only for
  the timers view.

### [ ] 6.5 DPS popout frozenFight effect doubles render work per combat message
- **Where:** `pages/DPSOverlayWindowPage.tsx:251-262`
- **Severity/confidence:** LOW / certain
- **Fix:** Keep the in-combat capture in a ref; only setState on the
  combat→idle transition.

### [ ] 6.6 audio.ts dedup map grows unbounded
- **Where:** `services/audio.ts:19, 103-109` (`lastFiredAt`)
- **Severity/confidence:** LOW / certain growth, small impact
- **Problem:** Retains every unique `tts:${text}` / `sound:${path}` key
  forever; TTS text is capture-substituted (per-sender, per-mob), so long
  sessions accumulate one entry per distinct utterance — in the main
  window AND each audio-playing overlay.
- **Fix:** Sweep on insert (sibling dedup in `useTriggerClipboard.ts:40-44`
  already prunes at 256).

### [ ] 6.7 Combat Log relative time-range filter never ages out while idle
- **Where:** `pages/CombatLogPage.tsx:815-819` + `:994`
- **Severity/confidence:** LOW / certain
- **Problem:** `cutoff = Date.now() - …` captured in a useMemo keyed only
  on `[allFights, filters]`; with "Last 30m" and no new combat, stale
  fights stay visible indefinitely.
- **Fix:** Add a coarse (e.g. 60 s) tick to the memo deps, or compute
  cutoff at render.

### [ ] 6.8 Mount-order race: initial REST snapshot can overwrite a fresher WS broadcast
- **Where:** `components/overlays/DPSPanel.tsx:445-448` (same in
  HPSPanel, ThreatPanel, CombatLogPage:978, DPSOverlayWindowPage:236,
  ThreatOverlayWindowPage:47); Roll Tracker mutations
  (`pages/RollTrackerPage.tsx:466-512` — `stopRollSession(id).then(setState)`
  can clobber a newer `overlay:rolls` broadcast)
- **Severity/confidence:** LOW / possible (usually self-heals next
  broadcast)
- **Fix:** "Skip REST result once any WS message applied" ref.

### [ ] 6.9 ChatHistoryPage debounced reload timer not cleared on unmount
- **Where:** `pages/ChatHistoryPage.tsx:168-174`
- **Severity/confidence:** LOW / certain, minor (one wasted fetch)
- **Fix:** Clear the timer in the effect cleanup.

### [x] 6.10 SpellSearchPicker search failure silently swallowed (done alongside 3.1)
- **Where:** `components/SpellSearchPicker.tsx:34-39`
- **Severity/confidence:** LOW / certain
- **Problem:** No `.catch` → unhandled rejection; previous query's results
  stay on screen presented as the new query's answer.
- **Fix:** `.catch` → clear results / show error state.

### [ ] 6.11 players.Consumer invokes onPVP callback while holding c.mu
- **Where:** `players/consumer.go:187-195` (current callback at
  main.go:678-703 is verified non-blocking — nothing deadlocks today)
- **Severity/confidence:** LOW / possible future hazard
- **Fix:** Copy-then-invoke after unlock (pattern chat/rolltracker
  consumers already use).

### [ ] 6.12 Tailer can't detect log-file replacement at same path
- **Where:** `logparser/tailer.go:284-303` (stats the open handle;
  truncation caught, delete+recreate not)
- **Severity/confidence:** LOW / possible (largely mitigated on Windows —
  delete of an open file fails)
- **Fix:** Periodic path-based re-stat comparing inode/identity.

### [ ] 6.13 ApplyPackUpdate non-transactional: row vs baseline can desync
- **Where:** `trigger/packupdate.go:411-578` (failure between
  `store.Update` and `UpsertPackBaseline`)
- **Severity/confidence:** LOW / possible
- **Problem:** Affected fields then read as "user customized" forever —
  preserve-mode merges silently stop following future pack changes.
- **Fix:** Wrap update + baseline upsert in one transaction.

### [x] 6.14 Failed backup creation leaves orphaned partial zip
- **Where:** `backup/manager.go:122-126`
- **Severity/confidence:** LOW / certain
- **Problem:** No DB row → invisible to List/Prune/Delete, yet
  `appbackup.listBackupZips` globs the dir so orphans get exported. (The
  DB-insert failure path right below already does `os.Remove(zipPath)`.)
- **Fix:** Mirror the `os.Remove` on the creation-failure path.

### [ ] 6.15 ApplyPendingImport partial failure presents as total data loss
- **Where:** `appbackup/manager.go:280-311` + `main.go:91-96`
- **Severity/confidence:** LOW / possible (AV lock on Windows is the
  realistic case)
- **Problem:** If the staged-DB rename fails after live user.db was set
  aside, startup logs and continues — every store creates a fresh empty
  user.db and nothing tells the user their data sits in
  `user.db.<ts>.preimport`.
- **Fix:** On rename failure, restore the preimport file (or surface a
  startup error the frontend can display).

### [ ] 6.16 Silent truncation in lazy index builders
- **Where:** `db/variants.go:184-199`, `db/pop_index.go:96-103, 163-180`,
  `db/quest_search.go:49-58`
- **Severity/confidence:** LOW / likely
- **Problem:** Ignored Scan errors / missing `rows.Err()`, no logging.
  Notably pop_index.go is also the **generator** path (`cmd/pop-index`) —
  a mid-iteration I/O error would bake an incomplete pop_gated.json into a
  release with no signal.
- **Fix:** Check `rows.Err()`, log scan failures; error out in the
  generator path.

### [ ] 6.17 LIKE escaping handles % but not \ or _
- **Where:** `db/queries.go:187, 830, 1594, 2121`, `db/recipes.go:93`
- **Severity/confidence:** LOW / certain
- **Problem:** Trailing `\` in a search term escapes the appended `%`
  (silent zero results); `_` is an unintended single-char wildcard.
- **Fix:** Reuse the correct `escapeLike` from `db/bandolier.go:104-107`.

### [x] 6.18 SetMaxOpenConns(4) without SetMaxIdleConns(4) on quarm.db pool
- **Where:** `db/db.go:83`
- **Severity/confidence:** LOW / possible
- **Problem:** Default idle is 2 — under load conns 3-4 close/reopen cold,
  defeating the warm 64 MiB page cache the pool exists for.
- **Fix:** One line: `SetMaxIdleConns(4)`.

### [ ] 6.19 Redundant wide queries in upgrades overview
- **Where:** `api/characters_upgrades.go:735-754`
  (`equippedItemsForSlot` re-fetches each worn item via `GetItem` — each
  nesting a `GetSpell` — though `resolveWornItems` exists and the `worn`
  map is available at every call site)
- **Severity/confidence:** LOW / certain (~25-40 redundant queries per
  overview request)
- **Fix:** Pass the `worn` map through.

### [ ] 6.20 No graceful shutdown path (design note)
- **Where:** `cmd/server/main.go:1156` (`http.Serve` blocks forever; no
  signal handler; sidecar dies by taskkill; all `defer Close()` never run)
- **Severity/confidence:** LOW / certain behavior, likely acceptable
- **Note:** WAL SQLite tolerates abrupt kill, so this is acceptable — but
  it's load-bearing that all writes remain small autocommit/short-tx.
  Revisit if long transactions are ever introduced.

---

## Suggested batches

1. **One-liners / near-one-liners:** 1.1 (atomic save), 1.5 (taunt
   canonicalize), 1.4 (WS guard), 3.2 (stuck loading), 5.4 (closeLogger
   order), 6.18 (idle conns), 6.14 (orphan zip), 6.2 (GC tick bail-out).
2. **Config safety:** 1.2 (Modify + decode-over-current).
3. **Search races:** 3.1 + 3.3 (copy QuestsPage seqRef everywhere).
4. **Hot-path perf:** 2.1 (broadcast diet), 2.2 (suffix pre-filter),
   2.3 (pool/worker), 2.4 (roll eviction).
5. **DB correctness:** 1.6, 4.1, 4.2, 4.3, 4.4 (converter indexes —
   lands with next data release), 4.5-4.10.
6. **Electron hardening pass:** 5.1, 5.2, 5.7, 5.8 together (one smoke
   test), then 5.3, 5.5, 5.6, 5.9.
7. **Hygiene sweep:** remaining Priority 6 items opportunistically.
