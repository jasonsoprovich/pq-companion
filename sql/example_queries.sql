-- PQ Companion — Example Queries
-- Run against the quarm MySQL database (or converted SQLite).
-- See SCHEMA.md for full table/column documentation.

-- ─────────────────────────────────────────────────────────────
-- ITEMS
-- ─────────────────────────────────────────────────────────────

-- 1. All items usable by Enchanters (classes bitmask: ENC = bit 13, value 8192)
SELECT id, Name, ac, hp, mana, aint, acha, reqlevel, slots
FROM items
WHERE classes & 8192 != 0
  AND nodrop = 0
ORDER BY reqlevel DESC;

-- 2. All items usable by Enchanters in the ear/neck/ring slots
--    Ears = 1+8=9, Neck = 16, Rings = 16384+32768=49152
SELECT id, Name, ac, hp, mana, aint, acha, reqlevel,
       slots & 9 AS has_ear, slots & 16 AS has_neck, slots & 49152 AS has_ring
FROM items
WHERE classes & 8192 != 0
  AND (slots & (9 | 16 | 49152)) != 0
ORDER BY aint DESC, reqlevel DESC;

-- 3. Weapons with procs usable by a specific class (Rogue = 256)
SELECT i.id, i.Name, i.damage, i.delay,
       ROUND(i.damage / (i.delay / 10.0), 2) AS ratio,
       s.name AS proc_name
FROM items i
JOIN spells_new s ON s.id = i.proceffect
WHERE i.classes & 256 != 0
  AND i.proceffect > 0
  AND i.itemtype IN (0, 1, 2, 3, 4)   -- 1H/2H slash, pierce, blunt
ORDER BY ratio DESC;

-- 4. Bags with weight reduction
SELECT id, Name, bagslots, bagsize, bagwr, weight
FROM items
WHERE itemclass = 1    -- bag
  AND bagwr > 0
ORDER BY bagwr DESC, bagslots DESC;

-- ─────────────────────────────────────────────────────────────
-- SPELLS
-- ─────────────────────────────────────────────────────────────

-- 5. All mez spells available to Enchanters (ENC = classes14)
--    Effect 18 = Mesmerize
SELECT id, name, cast_time/1000.0 AS cast_sec, mana,
       buffduration AS dur_ticks, buffduration * 6 AS dur_sec,
       resisttype, classes14 AS enc_level
FROM spells_new
WHERE classes14 < 255
  AND (effectid1 = 18 OR effectid2 = 18 OR effectid3 = 18
    OR effectid4 = 18 OR effectid5 = 18)
ORDER BY classes14;

-- 6. All spells a Wizard gets at or before level 50
--    classes12 = Wizard column (1=WAR...12=WIZ...14=ENC)
SELECT id, name, classes12 AS wiz_level, mana,
       cast_time/1000.0 AS cast_sec,
       resisttype, targettype
FROM spells_new
WHERE classes12 <= 50
ORDER BY classes12, name;

-- 7. All Enchanter DoT spells (Effect 0 = HP over time, negative base = damage)
SELECT id, name, mana, buffduration * 6 AS dur_sec,
       effect_base_value1 AS dmg_per_tick,
       ABS(effect_base_value1) * buffduration AS total_dmg,
       classes14 AS enc_level
FROM spells_new
WHERE classes14 < 255
  AND effectid1 = 0
  AND effect_base_value1 < 0
ORDER BY classes14;

-- 8. All root spells (Effect 23 = Root)
SELECT id, name, classes14 AS enc_level, classes5 AS shd_level,
       cast_time/1000.0 AS cast_sec, mana, resisttype,
       buffduration * 6 AS dur_sec
FROM spells_new
WHERE effectid1 = 23 OR effectid2 = 23 OR effectid3 = 23
ORDER BY name;

-- 9. Find all charm spells (Effect 31 = Charm) by class availability
SELECT id, name, mana,
       classes14 AS enc_level, classes7 AS mnk_level,
       targettype, resisttype
FROM spells_new
WHERE effectid1 = 31 OR effectid2 = 31 OR effectid3 = 31 OR effectid4 = 31
ORDER BY enc_level;

-- ─────────────────────────────────────────────────────────────
-- NPCS
-- ─────────────────────────────────────────────────────────────

-- 10. All NPCs that are unmezzable (special ability code 18)
SELECT id, name, level, hp, loottable_id,
       special_abilities
FROM npc_types
WHERE special_abilities LIKE '%18,%'
ORDER BY level DESC;

-- 11. All NPCs in a zone with their spawn coordinates
--     (replace 'northkarana' with any zone short_name)
SELECT n.id, n.name, n.level, n.hp,
       s2.x, s2.y, s2.z, s2.respawntime
FROM spawn2 s2
JOIN spawnentry se ON se.spawngroupID = s2.spawngroupID
JOIN npc_types n   ON n.id = se.npcID
WHERE s2.zone = 'northkarana'
  AND s2.enabled = 1
ORDER BY n.level DESC;

-- 12. Named/rare NPCs across all zones with their loottable
SELECT n.id, n.name, n.level, n.hp,
       s2.zone, n.loottable_id,
       n.special_abilities
FROM npc_types n
JOIN spawnentry se ON se.npcID = n.id
JOIN spawn2 s2     ON s2.spawngroupID = se.spawngroupID
WHERE n.rare_spawn = 1
ORDER BY s2.zone, n.level DESC;

-- 13. Full loot table for a specific NPC (by npc_types.id)
--     Replace 12345 with actual NPC id
SELECT n.name AS npc, lt.name AS loottable,
       ld.name AS lootdrop, i.id AS item_id,
       i.Name AS item_name, lde.chance,
       lte.probability AS table_prob, lte.multiplier
FROM npc_types n
JOIN loottable lt       ON lt.id = n.loottable_id
JOIN loottable_entries lte ON lte.loottable_id = lt.id
JOIN lootdrop ld        ON ld.id = lte.lootdrop_id
JOIN lootdrop_entries lde ON lde.lootdrop_id = ld.id
JOIN items i            ON i.id = lde.item_id
WHERE n.id = 12345
ORDER BY lde.chance DESC;

-- 14. NPCs that can summon AND are unmezzable (dangerous combo)
SELECT id, name, level, hp, special_abilities
FROM npc_types
WHERE special_abilities LIKE '%1,%'    -- summon
  AND special_abilities LIKE '%18,%'   -- unmezzable
ORDER BY level DESC;

-- 15. What spells does a specific NPC cast?
--     Replace 12345 with actual npc_types.id
SELECT n.name AS npc, s.id AS spell_id, s.name AS spell_name,
       s.cast_time/1000.0 AS cast_sec, s.mana,
       nse.priority, nse.recast_delay, nse.minlevel, nse.maxlevel
FROM npc_types n
JOIN npc_spells ns      ON ns.id = n.npc_spells_id
JOIN npc_spells_entries nse ON nse.npc_spells_id = ns.id
JOIN spells_new s       ON s.id = nse.spellid
WHERE n.id = 12345
ORDER BY nse.priority DESC;

-- ─────────────────────────────────────────────────────────────
-- ZONES
-- ─────────────────────────────────────────────────────────────

-- 16. All zones with their safe spot coordinates
SELECT zoneidnumber, short_name, long_name,
       safe_x, safe_y, safe_z, min_level
FROM zone
ORDER BY long_name;

-- 17. NPC count per zone (useful for finding populated zones)
SELECT s2.zone, z.long_name, COUNT(DISTINCT se.npcID) AS unique_npcs
FROM spawn2 s2
JOIN spawnentry se ON se.spawngroupID = s2.spawngroupID
LEFT JOIN zone z   ON z.short_name = s2.zone
WHERE s2.enabled = 1
GROUP BY s2.zone, z.long_name
ORDER BY unique_npcs DESC;

-- ─────────────────────────────────────────────────────────────
-- SKILLS
-- ─────────────────────────────────────────────────────────────

-- 18. Max skill caps for Enchanters at every level (class_id = 14)
SELECT level, skill_id, cap
FROM skill_caps
WHERE class_id = 14
ORDER BY skill_id, level;

-- ─────────────────────────────────────────────────────────────
-- USEFUL CROSS-QUERIES
-- ─────────────────────────────────────────────────────────────

-- 19. Items dropped by NPCs in a specific zone that Enchanters can use
SELECT DISTINCT i.id, i.Name, i.ac, i.hp, i.mana, i.aint, i.acha,
                i.reqlevel, lde.chance AS drop_chance, n.name AS from_npc
FROM spawn2 s2
JOIN spawnentry se     ON se.spawngroupID = s2.spawngroupID
JOIN npc_types n       ON n.id = se.npcID
JOIN loottable lt      ON lt.id = n.loottable_id
JOIN loottable_entries lte ON lte.loottable_id = lt.id
JOIN lootdrop_entries lde  ON lde.lootdrop_id = lte.lootdrop_id
JOIN items i           ON i.id = lde.item_id
WHERE s2.zone = 'northkarana'
  AND s2.enabled = 1
  AND i.classes & 8192 != 0   -- enchanter usable
ORDER BY i.aint DESC, lde.chance DESC;

-- 20. All Enchanter spells and whether Enchanters can find them in this zone
--     (items with scrolleffect matching the spell)
SELECT sn.id, sn.name AS spell_name, sn.classes14 AS enc_level,
       i.Name AS scroll_name, i.id AS scroll_id
FROM spells_new sn
LEFT JOIN items i ON i.scrolleffect = sn.id AND i.classes & 8192 != 0
WHERE sn.classes14 < 255
ORDER BY sn.classes14, sn.name;
