# Issue #123 — Log Parser Tightening from Zeal Divergence Logs

Investigation plan for triaging the `zealpipe-divergence` observability logs
added in Stage D. Deferred until after current bug-fix / small-feature work
clears; this doc captures everything needed to pick the task back up.

## Background

Three `slog.Info` calls in `backend/internal/spelltimer/engine.go` fire when
the log-parser-driven spell timer state disagrees with what the Zeal pipe
reports the client is doing:

| Line | Message | Meaning |
| --- | --- | --- |
| `engine.go:313` | `zealpipe-divergence: log cast != pipe cast` | Log parser caught a different spell name than the client reports being cast |
| `engine.go:480` | `zealpipe-divergence: timers think buff is active, pipe does not` | Engine has a buff timer the client doesn't show in any slot |
| `engine.go:484` | `zealpipe-divergence: pipe has buff slot, timers don't` | Client has a buff slot the engine never registered a timer for |

Commit `e48b3c2` deduped these across pulses so they don't spam, but no real
play-session samples have been collected yet. The task is fundamentally
gated on **data**.

## What I need from the user to investigate

A divergence log sample from real play. Two acceptable shapes:

### Option A — full slog output filtered (preferred)

Run the app on Windows through a normal play session (ideally one that
includes: zoning, buffs landing/fading, cast interrupts, mez, multiple
combat encounters, group-buff acceptance). Then ship me the filtered log:

```powershell
# In PowerShell, while the app is running, slog writes to stdout/stderr of
# the Go sidecar. The Electron shell captures this — find the log file.
# Typical location on Windows:
#   %APPDATA%\pq-companion\logs\sidecar-*.log
# (confirm the exact path by checking electron/src/main/sidecar.ts)
Select-String -Path "$env:APPDATA\pq-companion\logs\*.log" `
  -Pattern "zealpipe-divergence" | Out-File divergence.txt
```

Then attach `divergence.txt` to a comment on #123 or paste it here.

### Option B — raw eqlog + concurrent slog

If filtering is awkward, just zip:
- The eqlog file for the character: `<EQ_DIR>/Logs/eqlog_<Char>_pq.proj.txt`
  for the session window
- The full sidecar log for the same window

I'll filter and correlate.

### What "enough data" looks like

- **Minimum useful:** ~30 minutes of active play with at least one combat
  encounter where buffs are involved (group buffs, self-buffs, debuffs on
  mobs). Expect a few dozen divergence lines.
- **Ideal:** A varied 1–2 hour session covering zoning, raid buff stacks
  (Aego/KEI/Shaman buffs), DoTs, mez chains, and at least one death-camp
  cycle so the "buffs cleared on death" path is observed.
- **Character class matters:** classes that cast a lot (Enchanter, Wizard,
  Cleric, Shaman) will surface more divergences than melee. If the user has
  testdata-character options, prefer Osui (Enchanter) — already documented
  as the canonical test character.

## What I'll do with the data

1. **Bucket the divergence lines by category** (one of the three messages
   above) and by `spell_name` / `slot` if present in the structured fields.
2. **For each bucket:**
   - Pull the surrounding eqlog lines (±10s) around each divergence
     timestamp.
   - Identify the underlying parser cause:
     - *False positive*: the eqlog really is ambiguous, or the pipe is
       reporting transient state (e.g. between-tick race, cancel cast).
     - *Real miss*: a regex or event-handler bug — the parser should have
       fired but didn't.
     - *Real over-fire*: parser fired on a line that shouldn't have started
       or extended a timer.
3. **Decide per bucket:**
   - Fix the regex/handler (commit with test fixture using the offending
     eqlog snippet under `backend/internal/logparser/testdata/`).
   - Suppress the divergence log for known-benign cases (don't paper over;
     leave a comment naming the cause).
   - Leave as-is if rare and unactionable.
4. **Output the triage as either:**
   - A single PR with parser fixes + suppression rules, OR
   - New narrowly-scoped issues if any one category is too large to do in
     one pass.

## Pipe-truth auto-pruning — the deferred decision

The Stage D commit explicitly deferred this question: when the engine has a
buff timer the pipe doesn't see, should we **auto-prune** the engine state
to match the pipe? Stage D chose to log and leave alone because pruning a
real timer on a transient pipe glitch would be worse than leaving a stale
one.

The data triage above is what justifies (or rejects) the behavior change:

- If `timers think buff is active, pipe does not` divergences turn out to
  be **mostly stale engine state** (e.g. buff was actually dispelled, log
  parser missed the wear-off line) → auto-prune is correct and we should
  ship it.
- If they're **mostly transient pipe state** (e.g. pipe momentarily empty
  during a zone) → auto-prune is wrong; tighten the parser instead.

Don't make this call without the data.

## Files to touch when implementing

- `backend/internal/spelltimer/engine.go` — divergence sites (`:313`,
  `:480`, `:484`); possibly the prune hook depending on Step 4 above.
- `backend/internal/logparser/` — regex and event handler fixes.
- `backend/internal/logparser/testdata/` — new fixture lines for any
  parser fix; existing fixtures already exercise Osui (Enchanter) and
  Nariana (Wizard).
- `backend/internal/spelltimer/engine_test.go` — table-driven tests for
  any auto-prune behavior.

## Out of scope for this task

- Adding any new pipe sources beyond what Stage D already wired.
- Re-litigating the engine architecture (timer keying, composite keys,
  bard song handling) — only touch parser-side fixes unless data points at
  the engine.
- Building UI for divergence visibility — this is a backend triage task.

## Status when picked back up

Check the issue comments for any user-supplied divergence samples first.
If samples are attached, start at Step 1 above. If not, prompt the user
for them per the "What I need" section.
