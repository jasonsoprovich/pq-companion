-- Hand translations for apply_patches.py (see its docstring for the block
-- markers). Each block mirrors one upstream file whose MySQL doesn't run on
-- SQLite as written.

-- @@ before: 2026_11_18_items_mod2_stats.sql
-- PoP "mod2" item stats. Upstream adds all eight columns in one
-- ALTER TABLE ... ADD COLUMN IF NOT EXISTS, which SQLite can't parse; the
-- file's own ALTER is skipped and its UPDATEs then run as written.
-- Re-adding a column on a replay is tolerated by run_manual.
ALTER TABLE items ADD COLUMN strikethrough INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN stunresist INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN combateffects INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN accuracy INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN avoidance INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN spellshield INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN shielding INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN damageshield INTEGER NOT NULL DEFAULT 0;

-- @@ after: 2026_11_15_Elemental_Bard_Instrument_Sources.sql
-- MANUAL_ONLY. Upstream drives this with @session variables, a
-- SELECT ... INTO guard, IF() and <=>. Same targets and inserts here; the
-- guards that check the source data become assertions (the ones that only
-- protect a rerun on the live server are covered by the NOT EXISTS checks).
-- New parent rolls: Earth trap named + Tantisala get a 20% drum
-- (27998, Drum of the Beast), Water named a 20% lute (27994, Lute of the
-- Flowing Waters); Rockskull and Grioihin keep their original roll and gain
-- a separate 20% one. Loot tables shared by several NPCs get ONE roll.
CREATE TEMP TABLE instrument_targets_20261115 (
  npc_id INTEGER PRIMARY KEY,
  loottable_id INTEGER NOT NULL,
  item_id INTEGER NOT NULL,
  probability INTEGER NOT NULL
);
INSERT INTO instrument_targets_20261115 VALUES
  (218038, 4246, 27998, 20), (218351, 3201, 27998, 20),
  (218377, 87767, 27998, 20), (218381, 97048, 27998, 20),
  (218384, 5753, 27998, 20), (218378, 3229, 27998, 20),
  (218386, 14474, 27998, 20), (218385, 6637, 27998, 20),
  (218379, 14479, 27998, 20), (218380, 87768, 27998, 20),
  (218368, 14481, 27998, 20), (218382, 4247, 27998, 20),
  (218376, 87736, 27998, 20), (216037, 6730, 27994, 20),
  (216054, 4507, 27994, 20), (216237, 4507, 27994, 20),
  (216238, 6667, 27994, 20), (216239, 5599, 27994, 20),
  (216240, 6702, 27994, 20), (216241, 6731, 27994, 20),
  (216242, 87611, 27994, 20), (216243, 5795, 27994, 20),
  (216244, 6666, 27994, 20), (216246, 6910, 27994, 20),
  (216106, 87624, 27994, 20), (216248, 6732, 27994, 20),
  (216234, 6669, 27994, 20), (216105, 6669, 27994, 20),
  (216235, 6669, 27994, 20), (216043, 3228, 27994, 20),
  (216041, 6222, 27994, 20), (216040, 6226, 27994, 20),
  (218374, 4478, 27998, 20), (216042, 4451, 27994, 20);

-- Every target NPC still uses the expected loot table.
SELECT (SELECT COUNT(*) FROM instrument_targets_20261115 x
        JOIN npc_types n ON n.id = x.npc_id AND n.loottable_id = x.loottable_id
        JOIN loottable t ON t.id = x.loottable_id) = 34;
SELECT (SELECT COUNT(*) FROM items WHERE id IN (27994, 27998)) = 2;
-- No NPC outside the list shares a target loot table.
SELECT NOT EXISTS (
  SELECT 1 FROM npc_types n
  JOIN instrument_targets_20261115 x ON x.loottable_id = n.loottable_id
  LEFT JOIN instrument_targets_20261115 a ON a.npc_id = n.id
  WHERE a.npc_id IS NULL);
-- The original Rockskull drum and Grioihin lute rolls are as upstream expects.
SELECT EXISTS (
  SELECT 1 FROM loottable_entries t
  JOIN lootdrop_entries d ON d.lootdrop_id = t.lootdrop_id
  WHERE t.loottable_id = 4478 AND t.lootdrop_id = 218374 AND d.item_id = 27998
    AND t.probability = 10 AND t.multiplier = 2 AND t.multiplier_min = 0
    AND t.droplimit = 0 AND t.mindrop = 0 AND d.chance = 100
    AND d.multiplier = 1 AND d.disabled_chance = 0);
SELECT EXISTS (
  SELECT 1 FROM loottable_entries t
  WHERE t.loottable_id = 4451 AND t.lootdrop_id = 8503 AND t.probability = 50
    AND t.multiplier = 2 AND t.multiplier_min = 1 AND t.droplimit = 1
    AND t.mindrop = 1);
SELECT (SELECT COUNT(*) FROM lootdrop_entries WHERE lootdrop_id = 8503) = 3
   AND (SELECT COUNT(*) FROM lootdrop_entries
        WHERE lootdrop_id = 8503 AND multiplier = 1 AND disabled_chance = 0
          AND ((item_id = 27994 AND chance = 20)
            OR (item_id IN (10945, 16605) AND chance = 40))) = 3;
-- Only the original drop tables (plus ours, on a replay) carry the target
-- instrument on these loot tables.
SELECT NOT EXISTS (
  SELECT 1 FROM instrument_targets_20261115 x
  JOIN loottable_entries t ON t.loottable_id = x.loottable_id
  JOIN lootdrop_entries d ON d.lootdrop_id = t.lootdrop_id
                         AND d.item_id = x.item_id
  JOIN lootdrop ld ON ld.id = t.lootdrop_id
  WHERE ld.name NOT IN ('quarm_20261115_additional_earth_drum',
                        'quarm_20261115_additional_water_lute')
    AND NOT (x.npc_id = 218374 AND t.lootdrop_id = 218374)
    AND NOT (x.npc_id = 216042 AND t.lootdrop_id = 8503));

INSERT INTO lootdrop (name)
SELECT 'quarm_20261115_additional_earth_drum'
WHERE NOT EXISTS (SELECT 1 FROM lootdrop
                  WHERE name = 'quarm_20261115_additional_earth_drum');
INSERT INTO lootdrop (name)
SELECT 'quarm_20261115_additional_water_lute'
WHERE NOT EXISTS (SELECT 1 FROM lootdrop
                  WHERE name = 'quarm_20261115_additional_water_lute');

INSERT INTO lootdrop_entries
  (lootdrop_id, item_id, item_charges, equip_item, chance, multiplier,
   disabled_chance)
SELECT ld.id, CASE ld.name WHEN 'quarm_20261115_additional_earth_drum'
                           THEN 27998 ELSE 27994 END, 1, 0, 100, 1, 0
FROM lootdrop ld
WHERE ld.name IN ('quarm_20261115_additional_earth_drum',
                  'quarm_20261115_additional_water_lute')
  AND NOT EXISTS (SELECT 1 FROM lootdrop_entries e WHERE e.lootdrop_id = ld.id);

INSERT INTO loottable_entries
  (loottable_id, lootdrop_id, multiplier, probability, droplimit, mindrop,
   multiplier_min)
SELECT DISTINCT x.loottable_id, ld.id, 1, x.probability, 1, 1, 0
FROM instrument_targets_20261115 x
JOIN lootdrop ld ON ld.name = CASE x.item_id
       WHEN 27998 THEN 'quarm_20261115_additional_earth_drum'
       ELSE 'quarm_20261115_additional_water_lute' END
WHERE NOT EXISTS (SELECT 1 FROM loottable_entries t
                  WHERE t.loottable_id = x.loottable_id
                    AND t.lootdrop_id = ld.id);

-- Expected: 34 NPC rows, backed by 31 distinct new parent entries.
SELECT (SELECT COUNT(*) FROM instrument_targets_20261115 x
        JOIN npc_types n ON n.id = x.npc_id
        JOIN loottable_entries t ON t.loottable_id = n.loottable_id
        JOIN lootdrop ld ON ld.id = t.lootdrop_id AND ld.name LIKE 'quarm_20261115_%'
        JOIN lootdrop_entries d ON d.lootdrop_id = ld.id AND d.item_id = x.item_id
        WHERE t.probability = 20) = 34;
SELECT (SELECT COUNT(*) FROM loottable_entries t
        JOIN lootdrop ld ON ld.id = t.lootdrop_id
        WHERE ld.name LIKE 'quarm_20261115_%') = 31;
DROP TABLE temp.instrument_targets_20261115;
