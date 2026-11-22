#!/usr/bin/env python3
"""Replay Project Quarm content migrations onto quarm.db (SQLite).

quarm.db is built from periodic MySQL dumps, but Project Quarm also ships
content changes as SQL migration files between dumps. This script replays the
migrations that landed after the 2026-10-25-22:33 dump, in upstream order, so
the app's item/loot/respawn/lockout data matches the live server until the
next dump.

Two upstream sources (the quests repo's sql/ is applied manually on the server
and overrides EQMacEmu's values):
  - SecretsOTheP/EQMacEmu  utils/sql/git/{required,content}/  @ fc75fcf
  - SecretsOTheP/quests    sql/                               @ 05a3ec0
Then the local LOCAL_FILES run last: live changes from the official patch
notes that no upstream SQL file carries.

MySQL-only statements are rewritten where mechanical (ON DUPLICATE KEY UPDATE,
FROM DUAL, LEAST) or hand-translated in a --manual file. Blocks headed
"-- @@ before: <file>" run ahead of that file (schema changes its statements
depend on); "-- @@ after: <file>" blocks run after it and cover the statements
that failed. Files in MANUAL_ONLY skip their upstream statements entirely and
run only the hand translation (MySQL session variables and temp-table guards
don't translate statement by statement). In a manual block a SELECT is an
assertion: its first column must be true or the whole replay rolls back.
Statements on live server state (respawn_times, character lockouts,
data_buckets, launcher tables) are skipped: quarm.db holds no live state.

Usage:
  python3 -I apply_patches.py --db quarm.db \
      --eqmacemu-dir <EQMacEmu>/utils/sql/git --quests-dir <quests>/sql \
      --manual manual-2026-11-22.sql

Replaying onto an already-patched quarm.db is a no-op (every file is guarded
or idempotent). Drop this once a dump taken after 2026-11-22 is converted.
"""

import argparse
import os
import re
import sqlite3
import sys

# Paths are relative to EQMacEmu's utils/sql/git. Already in the 10-25 dump:
# content/2026_10_21_Justice_Fruit_Habanero_Pepper_Ground_Spawns.sql.
EQMACEMU_FILES = [
    "required/2026_10_27_Plane_Of_Earth_Traps_See_Improved_Hide.sql",
    "content/2026_11_15_Elemental_Bard_Instrument_Sources.sql",
    "content/2026_11_17_Bo_Staff_of_Transcendence_Magic.sql",
    "required/2026_11_18_items_mod2_stats.sql",
    "content/2026_11_20_Wakening_Lands_Port_Scrolls_PoP.sql",
]

# Run only from their hand translation in the --manual file.
MANUAL_ONLY = {"2026_11_15_Elemental_Bard_Instrument_Sources.sql"}

# 2026_10_24_Torment_Avatar_Placeholder_Blood.sql is already in the dump.
QUESTS_FILES = [
    "2026_10_22_Item_Appearance_Fixes.sql",
    "2026_10_24_Elemental_Essence_Parent_Chance.sql",
    "2026_10_25_Obsidianwood_Sap_Drop_Chance.sql",
]

# Not replayed: launcher/PvP/rule/timer-cleanup files touch server state only.

LOCAL_FILES = []

STATE_TABLES = ("respawn_times", "character_loot_lockouts",
                "character_legacy_items", "data_buckets", "launcher_zones")

MANUAL_MARK = re.compile(r"^-- @@ (before|after): (\S+)\s*$", re.M)


def strip_comments(src):
    src = re.sub(r"/\*.*?\*/", "", src, flags=re.S)
    return "\n".join(l for l in src.splitlines()
                     if not l.strip().startswith("--"))


def statements(src):
    buf = ""
    for line in strip_comments(src).splitlines(True):
        buf += line
        if sqlite3.complete_statement(buf):
            stmt = buf.strip()
            buf = ""
            if stmt:
                yield stmt


def rewrite(stmt):
    """Mechanical MySQL -> SQLite rewrites. Returns None to skip."""
    up = stmt.upper()
    if up.startswith(("START TRANSACTION", "COMMIT", "SET @", "SELECT",
                      "DROP TEMPORARY", "ALTER TABLE")):
        return None
    if any(re.search(r"\b%s\b" % t, stmt) for t in STATE_TABLES):
        return None
    if "ON DUPLICATE KEY UPDATE" in up:
        stmt = re.sub(r"\s+ON DUPLICATE KEY UPDATE.*;\s*$", ";", stmt,
                      flags=re.S | re.I)
        stmt = re.sub(r"^INSERT INTO", "INSERT OR IGNORE INTO", stmt, flags=re.I)
    stmt = re.sub(r"\bFROM DUAL\b", "", stmt, flags=re.I)
    stmt = re.sub(r"\bLEAST\(", "MIN(", stmt)
    return stmt


def load_manual(path):
    """Return {(when, file): sql} for the --manual file's blocks."""
    if not path:
        return {}
    parts = MANUAL_MARK.split(open(path).read())
    return {(parts[i], parts[i + 1]): parts[i + 2]
            for i in range(1, len(parts), 3)}


def run_manual(db, name, sql, failures):
    """Run a hand-translated block. These must not fail: a SELECT is an
    assertion, and the only tolerated error is re-adding a column that an
    earlier replay already added (SQLite has no ADD COLUMN IF NOT EXISTS)."""
    for stmt in statements(sql):
        try:
            cur = db.execute(stmt)
        except sqlite3.OperationalError as e:
            if (stmt.upper().startswith("ALTER TABLE")
                    and "duplicate column name" in str(e)):
                continue
            raise
        if stmt.upper().startswith("SELECT"):
            row = cur.fetchone()
            if not row or not row[0]:
                failures.append(f"{name}: assertion failed: {stmt[:100]!r}")


def run_file(db, path, manual, failures):
    name = os.path.basename(path)
    if ("before", name) in manual:
        run_manual(db, name, manual.pop(("before", name)), failures)
    failed = 0
    if name not in MANUAL_ONLY:
        for stmt in statements(open(path).read()):
            sql = rewrite(stmt)
            if sql is None:
                continue
            try:
                db.execute(sql)
            except sqlite3.Error as e:
                failed += 1
                if ("after", name) not in manual:
                    failures.append(f"{name}: {e}: {stmt[:80]!r}")
    elif ("after", name) not in manual:
        failures.append(f"{name}: MANUAL_ONLY but no '@@ after' block")
    if ("after", name) in manual:
        run_manual(db, name, manual.pop(("after", name)), failures)
    print(f"applied {name}" + (f" ({failed} via manual)" if failed else "")
          + (" (manual only)" if name in MANUAL_ONLY else ""))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqmacemu-dir", required=True)
    ap.add_argument("--quests-dir", required=True)
    ap.add_argument("--manual")
    args = ap.parse_args()

    manual = load_manual(args.manual)
    # One explicit transaction, so the manual ALTER TABLEs roll back with
    # everything else (the default mode only opens one before DML).
    db = sqlite3.connect(args.db, isolation_level=None)
    db.execute("BEGIN")
    failures = []
    for f in EQMACEMU_FILES:
        run_file(db, os.path.join(args.eqmacemu_dir, f), manual, failures)
    for f in QUESTS_FILES:
        run_file(db, os.path.join(args.quests_dir, f), manual, failures)
    here = os.path.dirname(os.path.abspath(__file__))
    for f in LOCAL_FILES:
        run_file(db, os.path.join(here, f), manual, failures)
    if manual:
        failures.append(f"unused manual blocks: {sorted(manual)}")
    if failures:
        db.rollback()
        print("\n".join(["FAILED:"] + failures), file=sys.stderr)
        sys.exit(1)
    db.commit()
    db.execute("ANALYZE")
    db.commit()
    db.close()


if __name__ == "__main__":
    main()
