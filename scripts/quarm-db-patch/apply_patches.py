#!/usr/bin/env python3
"""Replay Project Quarm content migrations onto quarm.db (SQLite).

quarm.db is built from periodic MySQL dumps, but Project Quarm also ships
content changes as SQL migration files between dumps. This script replays the
migrations that landed after the 2026-10-25-22:33 dump, in upstream order, so
the app's respawn/lockout/loot data matches the live server until the next
dump.

Two upstream sources (the quests repo's sql/ is applied manually on the server
and overrides EQMacEmu's values):
  - SecretsOTheP/EQMacEmu  utils/sql/git/content/  @ 1206849 (none newer
                                                     than the dump)
  - SecretsOTheP/quests    sql/                    @ 572bc06
Then the local LOCAL_FILES run last: live changes from the official patch
notes that no upstream SQL file carries.

MySQL-only statements are rewritten where mechanical (ON DUPLICATE KEY UPDATE,
FROM DUAL, LEAST) or hand-translated in a --manual file (blocks headed
"-- @@ after: <file>"). Statements on live server state (respawn_times,
character lockouts, data_buckets, launcher tables) are skipped: quarm.db holds
no live state.

Usage:
  python3 -I apply_patches.py --db quarm.db \
      --eqmacemu-dir <EQMacEmu>/utils/sql/git/content --quests-dir <quests>/sql

Drop this once a dump taken after 2026-10-27 has been converted.
"""

import argparse
import os
import re
import sqlite3
import sys

EQMACEMU_FILES = []

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

MANUAL_MARK = re.compile(r"^-- @@ after: (\S+)\s*$", re.M)


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
    if not path:
        return {}
    src = open(path).read()
    parts = MANUAL_MARK.split(src)
    return {parts[i]: parts[i + 1] for i in range(1, len(parts), 2)}


def run_file(db, path, manual, failures):
    name = os.path.basename(path)
    failed = 0
    for stmt in statements(open(path).read()):
        sql = rewrite(stmt)
        if sql is None:
            continue
        try:
            db.execute(sql)
        except sqlite3.Error as e:
            failed += 1
            if name not in manual:
                failures.append(f"{name}: {e}: {stmt[:80]!r}")
    if name in manual:
        for stmt in statements(manual.pop(name)):
            db.execute(stmt)  # hand translations must not fail
    print(f"applied {name}" + (f" ({failed} via manual)" if failed else ""))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", required=True)
    ap.add_argument("--eqmacemu-dir", required=True)
    ap.add_argument("--quests-dir", required=True)
    ap.add_argument("--manual")
    args = ap.parse_args()

    manual = load_manual(args.manual)
    db = sqlite3.connect(args.db)
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
