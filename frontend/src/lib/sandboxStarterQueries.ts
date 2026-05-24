// Starter queries surfaced from the SQL sandbox "Examples" picker.
//
// Each query is a working SELECT against quarm.db (read-only) with inline
// comments calling out the moving parts the user is most likely to tweak.
// Keep one purpose per query — these are jumping-off points, not a guide
// to every possible combination.

export interface StarterQuery {
  id: string
  label: string
  description: string
  sql: string
}

export const STARTER_QUERIES: StarterQuery[] = [
  {
    id: 'items-by-stat',
    label: 'Items: top by stat',
    description: 'Highest-AC chest pieces visible to most casters.',
    sql: `-- Top-AC chest items.
-- Adjust "ac" to mana, hp, astr, asta, … for other stats.
-- items.slots is a bitfield; 0x20000 (=131072) is the Chest slot.
--   Charm=0x1     Ear=0x2/0x10  Head=0x4    Face=0x8     Neck=0x20
--   Shoulder=0x40 Arms=0x80     Back=0x100  Wrist=0x200/0x400
--   Range=0x800   Hands=0x1000  Primary=0x2000 Secondary=0x4000
--   Finger=0x8000/0x10000 Chest=0x20000 Legs=0x40000
--   Feet=0x80000  Waist=0x100000 Ammo=0x200000
SELECT id, Name, ac, hp, mana, reqlevel
FROM items
WHERE (slots & 131072) != 0
ORDER BY ac DESC
LIMIT 50;`,
  },
  {
    id: 'items-by-class',
    label: 'Items: by class',
    description: 'Items usable by a given class (Enchanter = 0x2000).',
    sql: `-- items.classes is a bitfield, one bit per playable class.
-- WAR=0x1   CLR=0x2    PAL=0x4   RNG=0x8    SHD=0x10
-- DRU=0x20  MNK=0x40   BRD=0x80  ROG=0x100  SHM=0x200
-- NEC=0x400 WIZ=0x800  MAG=0x1000 ENC=0x2000 BST=0x4000
-- The query below filters to enchanter-usable items.
SELECT id, Name, ac, hp, mana, reqlevel
FROM items
WHERE (classes & 0x2000) != 0
  AND reqlevel BETWEEN 50 AND 60
ORDER BY mana DESC
LIMIT 100;`,
  },
  {
    id: 'npcs-in-zone',
    label: 'NPCs in a zone',
    description: 'All NPCs that can spawn in a given zone short-name.',
    sql: `-- Replace 'qcat' with your zone short_name (e.g. 'permafrost',
-- 'kael', 'tower'). Uses the same spawn chain the NPC overlay uses.
SELECT DISTINCT n.id, n.name, n.level, n.hp, n.class
FROM zone z
JOIN spawn2 s ON s.zone = z.short_name
JOIN spawnentry se ON se.spawngroupID = s.spawngroupID
JOIN npc_types n ON n.id = se.npcID
WHERE z.short_name = 'qcat'
ORDER BY n.level DESC, n.name;`,
  },
  {
    id: 'npc-loot',
    label: 'NPC: what does it drop?',
    description: 'Resolve an NPC name to every possible item drop.',
    sql: `-- Walks npc_types -> loottable -> lootdrop -> items.
-- Drops with chance < 1.0 mean "rolled probabilistically"; column
-- multiplier in loottable_entries decides how many times each drop is
-- rolled per kill.
SELECT i.id AS item_id, i.Name AS item_name,
       le.chance AS drop_chance, lt.probability AS pool_chance
FROM npc_types n
JOIN loottable t ON t.id = n.loottable_id
JOIN loottable_entries lt ON lt.loottable_id = t.id
JOIN lootdrop ld ON ld.id = lt.lootdrop_id
JOIN lootdrop_entries le ON le.lootdrop_id = ld.id
JOIN items i ON i.id = le.item_id
WHERE n.name LIKE '%King_Tormax%'
ORDER BY i.Name;`,
  },
  {
    id: 'spell-class-level',
    label: 'Spells: by class & level',
    description: 'Mage spells learnable at level 60 or below.',
    sql: `-- spells_new.classes1 = Warrior, classes2 = Cleric, …, classes13 = Magician.
-- Class N column stores the level at which that class learns the spell;
-- 255 means "not castable by this class".
SELECT id, name, mana, cast_time, classes13 AS mag_level
FROM spells_new
WHERE classes13 BETWEEN 1 AND 60
ORDER BY classes13, name;`,
  },
  {
    id: 'spell-by-spa',
    label: 'Spells: by effect (SPA)',
    description: 'Find spells with a specific effect/SPA code.',
    sql: `-- spells_new has 12 effect slots (effectid1..12). SPA = "spell
-- particle attribute" code. Examples: 0 = damage/heal, 11 = melee
-- haste, 0x9 (9) = pacify, 22 = stun, 26 = root, 33 = summon item.
-- Adjust the literal below for the SPA you want.
SELECT id, name, effectid1, effect_base_value1
FROM spells_new
WHERE effectid1 = 11
ORDER BY effect_base_value1 DESC
LIMIT 100;`,
  },
  {
    id: 'zone-key-components',
    label: 'Keys: components for a zone',
    description: 'Items that count as keys for a zone (resolves stages).',
    sql: `-- keyring_data lists every item that satisfies a zone key.
-- Many PQ keys have multiple "stages" (assembly steps); stage 1 is the
-- earliest piece. Joins to items so the user sees the actual item name.
SELECT k.zoneid, z.short_name, z.long_name,
       k.stage, k.key_name, i.id AS item_id, i.Name AS item_name
FROM keyring_data k
LEFT JOIN zone z ON z.zoneidnumber = k.zoneid
LEFT JOIN items i ON i.id = k.key_item
ORDER BY z.short_name, k.stage;`,
  },
  {
    id: 'forage-by-zone',
    label: 'Forage table by zone',
    description: 'Items found by Druid/Ranger Forage in a zone.',
    sql: `-- Druid/Ranger forage table. Join to zone for the readable zone
-- name and to items for the item name. chance is a weight, not a
-- percent — higher = more common within the zone's forage pool.
SELECT z.short_name, z.long_name, f.level AS skill_req,
       f.chance, i.id AS item_id, i.Name AS item_name
FROM forage f
JOIN zone z ON z.zoneidnumber = f.zoneid
JOIN items i ON i.id = f.Itemid
WHERE z.short_name = 'lfaydark'
ORDER BY f.chance DESC;`,
  },
  {
    id: 'zone-connections',
    label: 'Zone connections',
    description: 'What zones connect to/from a given zone.',
    sql: `-- zone_points lists every inter-zone transition (zone lines, books,
-- portals). Each row is one direction; the same logical connection
-- often has a paired row going the other way. target_zone_id matches
-- zone.zoneidnumber.
SELECT zp.zone AS from_zone, z.short_name AS to_zone, z.long_name,
       zp.target_x, zp.target_y, zp.target_z
FROM zone_points zp
JOIN zone z ON z.zoneidnumber = zp.target_zone_id
WHERE zp.zone = 'qeynos'
ORDER BY z.short_name;`,
  },
  {
    id: 'aa-by-class',
    label: 'AAs: by class',
    description: 'Alternate abilities available to a class bitmask.',
    sql: `-- altadv_vars.classes is a bitfield identical to items/spells.
-- Filter to a single class: e.g. Cleric = 2. Costs scale with rank;
-- max_level is the highest rank that can be purchased.
SELECT skill_id, name, cost, max_level, spellid, aa_expansion
FROM altadv_vars
WHERE (classes & 2) != 0
ORDER BY aa_expansion, name;`,
  },
]
