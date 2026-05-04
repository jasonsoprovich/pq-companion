# PQ Companion — Database Schema Reference

Source: Project Quarm EQEmu MySQL dump (quarm_2026-03-20).
Tables are read-only at runtime. All app queries target the converted `quarm.db` SQLite file.

---

## Table Index

| Table | Rows (approx) | Purpose |
|---|---|---|
| `items` | ~27,000 | All equippable and usable items |
| `spells_new` | ~4,000+ | All player and NPC spells |
| `npc_types` | ~7,000 | NPC definitions (stats, abilities, loot) |
| `zone` | ~300 | Zone definitions |
| `spawn2` | ~21,000 | Spawn points (location + timing) |
| `spawnentry` | ~21,000 | spawngroupID → npcID mapping |
| `spawngroup` | ~6,000 | Spawn group definitions |
| `lootdrop` | ~6,000 | Named loot drop tables |
| `lootdrop_entries` | ~45,000 | lootdrop → item (with chance) |
| `loottable` | ~6,000 | NPC loot table containers |
| `loottable_entries` | ~20,000 | loottable → lootdrop mapping |
| `npc_spells` | ~4,000 | NPC spell list groups |
| `npc_spells_entries` | ~30,000 | npc_spells_id → spellid mapping |
| `skill_caps` | ~32,000 | Max skill by class/level |
| `merchantlist` | — | Merchant inventories |

---

## Core Tables

### `items`

Every item in the game. Stats are stored flat (no normalization).

```sql
CREATE TABLE items (
  id            int          -- item ID (primary key)
  Name          varchar(64)  -- display name
  lore          varchar(80)  -- lore text (empty = not lore)
  classes       int          -- class bitmask (see below)
  races         int          -- race bitmask (see below)
  slots         int          -- equipment slot bitmask (see below)
  itemtype      int          -- 0=1HS, 1=2HS, 2=piercing, 3=1HB, 4=2HB, 5=archery, 7=thrown, 10=armor, 11=misc, 14=food, 15=drink, 17=bow, 19=scroll, 20=potion, 23=wind, 24=stringed, 25=brass, 26=percussion, 27=arrow, 29=bag, 31=book, ...
  itemclass     int          -- 0=common, 1=bag, 2=book
  -- Combat stats
  damage        int          -- base damage (weapons)
  delay         int          -- delay in tenths of a second
  ac            int          -- armor class
  -- Stat bonuses
  astr int, asta int, aagi int, adex int, aint int, awis int, acha int
  hp            int
  mana          int
  -- Resists
  mr int, cr int, fr int, dr int, pr int
  -- Requirements
  reqlevel      int          -- required level to equip
  reclevel      int          -- recommended level
  -- Flags
  magic         int          -- 1 = magic item
  nodrop        int          -- 1 = no drop
  norent        int          -- 1 = no rent (disappears at logout)
  stackable     int          -- 1 = stackable
  stacksize     int          -- max stack size
  -- Effects (all follow pattern: effectid, type, level, level2)
  clickeffect   int          -- spell ID for click effect
  clicktype     int          -- 0=none, 1=worn, 4=click anywhere, 5=click worn
  proceffect    int          -- spell ID for weapon proc
  worneffect    int          -- spell ID for worn (focus/innate) effect
  focuseffect   int          -- spell ID for focus effect
  scrolleffect  int          -- spell ID (for scrolls)
  -- Bags
  bagsize       int          -- 0=tiny, 1=small, 2=medium, 3=large, 4=giant
  bagslots      int          -- number of slots
  bagtype       int          -- 0=normal, 1=quiver, 2=bandolier
  bagwr         int          -- weight reduction %
  -- Misc
  price         int          -- vendor sell price in copper
  weight        int          -- weight in tenths of a stone
  icon          int          -- icon ID
  tradeskills   int          -- 1 = usable in tradeskills
  ...
)
```

#### Class Bitmask (`items.classes`)

| Bit | Value | Class |
|---|---|---|
| 0 | 1 | Warrior |
| 1 | 2 | Cleric |
| 2 | 4 | Paladin |
| 3 | 8 | Ranger |
| 4 | 16 | Shadow Knight |
| 5 | 32 | Druid |
| 6 | 64 | Monk |
| 7 | 128 | Bard |
| 8 | 256 | Rogue |
| 9 | 512 | Shaman |
| 10 | 1024 | Necromancer |
| 11 | 2048 | Wizard |
| 12 | 4096 | Magician |
| 13 | 8192 | Enchanter |
| 14 | 16384 | Beastlord |
| — | 65535 | All classes |

#### Slot Bitmask (`items.slots`)

| Bit | Value | Slot |
|---|---|---|
| 0 | 1 | Ear (left) |
| 1 | 2 | Head |
| 2 | 4 | Face |
| 3 | 8 | Ear (right) |
| 4 | 16 | Neck |
| 5 | 32 | Shoulders |
| 6 | 64 | Arms |
| 7 | 128 | Back |
| 8 | 256 | Wrist (left) |
| 9 | 512 | Wrist (right) |
| 10 | 1024 | Range |
| 11 | 2048 | Hands |
| 12 | 4096 | Primary |
| 13 | 8192 | Secondary |
| 14 | 16384 | Finger (left) |
| 15 | 32768 | Finger (right) |
| 16 | 65536 | Chest |
| 17 | 131072 | Legs |
| 18 | 262144 | Feet |
| 19 | 524288 | Waist |
| 20 | 1048576 | Ammo / Power Source |
| 21 | 2097152 | Charm |

---

### `spells_new`

All spells — player and NPC. Each row has 12 effect slots.

```sql
CREATE TABLE spells_new (
  id              int          -- spell ID (primary key)
  name            varchar(64)  -- spell name
  cast_on_you     varchar(120) -- message shown when spell lands on you
  cast_on_other   varchar(120) -- message shown when it lands on others
  spell_fades     varchar(120) -- message when buff wears off
  mana            int          -- mana cost
  cast_time       int          -- cast time in milliseconds
  recovery_time   int          -- recovery time in ms
  recast_time     int          -- recast delay in ms
  buffduration    int          -- duration in ticks (6 seconds each)
  buffdurationformula int      -- formula type for duration calculation
  range           int          -- range in units
  aoerange        int          -- AoE radius
  resisttype      int          -- resist type (see below)
  targettype      int          -- target type (see below)
  -- Class levels (1-65 = usable at that level, 255 = cannot use)
  classes1..classes15 int      -- WAR, CLR, PAL, RNG, SHD, DRU, MNK, BRD, ROG, SHM, NEC, WIZ, MAG, ENC, BST
  -- 12 effect slots
  effectid1..effectid12   int  -- effect type ID (see below)
  effect_base_value1..12  int  -- base value for effect
  effect_limit_value1..12 int  -- limit value
  max1..max12             int  -- max value
  -- Spell traits
  goodEffect      int          -- 1 = beneficial, 0 = detrimental
  skill           int          -- spell skill type
  nodispell       int          -- -1 = dispellable, 1 = not
  IsDiscipline    int          -- 1 = discipline (endurance cost)
  ...
)
```

#### Resist Types (`spells_new.resisttype`)

| Value | Type |
|---|---|
| 0 | Unresistable |
| 1 | Magic |
| 2 | Fire |
| 3 | Cold (Ice) |
| 4 | Poison |
| 5 | Disease |

#### Target Types (`spells_new.targettype`)

| Value | Type |
|---|---|
| 1 | Line of Sight PC |
| 2 | PC Group |
| 4 | PC AoE |
| 5 | NPC |
| 6 | Self |
| 8 | Animal |
| 11 | Undead |
| 14 | Self |
| 41 | PB AoE |

#### Key Effect IDs (`spells_new.effectid1..12`)

| Value | Effect |
|---|---|
| 0 | Current HP (heal/DoT) |
| 1 | Armor Class |
| 2 | Attack |
| 3 | Movement Speed |
| 4 | STR |
| 5 | DEX |
| 6 | AGI |
| 7 | STA |
| 8 | WIS |
| 9 | INT |
| 10 | CHA |
| 11 | Mana (direct) |
| 12 | HP (direct) |
| 13 | Fear |
| 15 | Blind |
| 18 | Mesmerize |
| 23 | Root |
| 31 | Charm |
| 35 | Add Buff Slot |
| 85 | Stun |
| 254 | Empty (no effect) |

#### Duration Formula (`spells_new.buffdurationformula`)

| Value | Formula |
|---|---|
| 0 | Instant (0 ticks) |
| 1 | `duration` ticks |
| 3 | `level / 2` ticks, capped at `duration` |
| 5 | `duration` ticks |
| 7 | `duration` ticks (most buffs) |
| 50 | Permanent |

Tick = 6 seconds. So `buffduration = 30` = 180 seconds = 3 minutes.

---

### `npc_types`

All NPC definitions.

```sql
CREATE TABLE npc_types (
  id                int          -- NPC type ID (primary key)
  name              text         -- NPC name (underscores = spaces in-game)
  lastname          varchar(32)  -- last name / title
  level             tinyint
  race              smallint     -- race ID
  class             tinyint      -- class ID (same as player classes)
  hp                int
  mana              int
  mindmg            int
  maxdmg            int
  attack_count      smallint     -- attacks per round (-1 = default)
  special_abilities text         -- caret-delimited (^) list of code,value pairs
  loottable_id      int          -- FK → loottable.id
  merchant_id       int          -- FK → merchantlist (0 = not a merchant)
  npc_spells_id     int          -- FK → npc_spells.id
  npc_faction_id    int          -- FK → npc_faction.id
  -- Resists
  MR int, CR int, DR int, FR int, PR int
  -- Stats
  STR int, STA int, DEX int, AGI int, _INT int, WIS int, CHA int, ATK int, AC smallint
  aggroradius       int
  see_invis         smallint     -- 1 = sees invisible
  see_invis_undead  smallint     -- 1 = sees invis vs undead
  raid_target       tinyint      -- 1 = raid boss
  rare_spawn        tinyint      -- 1 = rare/named
  ...
)
```

#### Special Abilities (`npc_types.special_abilities`)

Format: `code,value^code,value^...` (caret `^` delimited, **not** pipe).

Codes match the `SpecialAbility` namespace in the Project Quarm EQMacEmu fork
(`common/emu_constants.h`). They differ from modern EQEmu master numbering.

| Code | Ability |
|---|---|
| 1 | Summon |
| 2 | Enrage |
| 3 | Rampage |
| 4 | Area Rampage |
| 5 | Flurry |
| 6 | Triple Attack |
| 7 | Dual Wield |
| 8 | Disallow Equip |
| 9 | Bane Attack |
| 10 | Magical Attack |
| 11 | Ranged Attack |
| 12 | Immune to Slow |
| 13 | Immune to Mez |
| 14 | Immune to Charm |
| 15 | Immune to Stun |
| 16 | Immune to Snare |
| 17 | Immune to Fear |
| 18 | Immune to Dispel |
| 19 | Immune to Melee |
| 20 | Immune to Magic |
| 21 | Immune to Fleeing |
| 22 | Immune to Non-Bane Melee |
| 23 | Immune to Non-Magical Melee |
| 24 | Immune to Aggro |
| 25 | Immune to Being Aggro'd |
| 26 | Immune to Ranged Spells |
| 27 | Immune to Feign Death |
| 28 | Immune to Taunt |
| 29 | Tunnel Vision |
| 30 | Won't Heal/Buff Allies |
| 31 | Immune to Pacify |
| 32 | Leashed |
| 33 | Tethered |
| 34 | Permaroot Flee |
| 35 | Immune to Harm from Client |
| 36 | Always Flees |
| 37 | Flee Percent |
| 38 | Allows Beneficial Spells |
| 39 | Melee Disabled |
| 40 | Chase Distance |
| 41 | Allowed to Tank |
| 42 | Proximity Aggro |
| 43 | Always Calls for Help |
| 44 | Use Warrior Skills |
| 45 | Always Flee on Low Con |
| 46 | No Loitering |
| 47 | Block Handin on Bad Faction |
| 48 | PC Deathblow Corpse |
| 49 | Corpse Camper |
| 50 | Reverse Slow |
| 51 | Immune to Haste |
| 52 | Immune to Disarm |
| 53 | Immune to Riposte |
| 54 | Proximity Aggro 2 |

> See-invis / see-invis-undead are stored on dedicated `npc_types`
> columns, not in this string.

---

### `zone`

Zone definitions.

```sql
CREATE TABLE zone (
  id            int          -- internal auto-increment ID
  zoneidnumber  int          -- zone ID used in spawn2 and character data
  short_name    varchar(32)  -- short name (used in spawn2.zone, file paths)
  long_name     text         -- display name ("The North Karana")
  safe_x/y/z    float        -- safe spot coordinates
  min_level     tinyint      -- minimum level to enter (0 = no restriction)
  ...
)
```

---

## Spawn Chain

To find what NPCs spawn in a zone:

```
zone.short_name
  → spawn2.zone (filter by zone short name)
    → spawn2.spawngroupID
      → spawnentry.spawngroupID
        → spawnentry.npcID
          → npc_types.id
```

### `spawn2`

```sql
CREATE TABLE spawn2 (
  id            int          -- primary key
  spawngroupID  int          -- FK → spawngroup.id (or npc_types.id for simple spawns)
  zone          varchar(32)  -- zone short name
  x, y, z       float        -- spawn coordinates
  heading       float
  respawntime   int          -- respawn in seconds
  variance      int          -- random variance on respawn time
  enabled       tinyint      -- 1 = active
)
```

### `spawnentry`

```sql
CREATE TABLE spawnentry (
  spawngroupID  int    -- FK → spawngroup.id
  npcID         int    -- FK → npc_types.id
  chance        smallint  -- % chance this NPC spawns from this group
)
```

### `spawngroup`

```sql
CREATE TABLE spawngroup (
  id            int          -- primary key
  name          varchar(50)  -- usually "zone_npcname_N"
  spawn_limit   tinyint      -- max concurrent spawns (0 = unlimited)
  rand_spawns   int          -- number to randomly pick from group
)
```

---

## Loot Chain

To find what items an NPC drops:

```
npc_types.loottable_id
  → loottable.id
    → loottable_entries.loottable_id
      → loottable_entries.lootdrop_id
        → lootdrop_entries.lootdrop_id
          → lootdrop_entries.item_id, lootdrop_entries.chance
            → items.id
```

### `loottable`

```sql
CREATE TABLE loottable (
  id       int          -- primary key
  name     varchar(255) -- descriptive name
  mincash  int          -- min coin drop (copper)
  maxcash  int          -- max coin drop (copper)
)
```

### `loottable_entries`

```sql
CREATE TABLE loottable_entries (
  loottable_id  int
  lootdrop_id   int
  multiplier    tinyint   -- how many times to roll this drop table
  probability   tinyint   -- % chance to roll this table at all (0-100)
  droplimit     tinyint   -- max items from this table (0 = no limit)
  mindrop       tinyint   -- guaranteed minimum drops
)
```

### `lootdrop`

```sql
CREATE TABLE lootdrop (
  id    int          -- primary key
  name  varchar(255) -- descriptive name
)
```

### `lootdrop_entries`

```sql
CREATE TABLE lootdrop_entries (
  lootdrop_id   int
  item_id       int    -- FK → items.id
  item_charges  smallint
  equip_item    tinyint  -- 1 = NPC equips this item
  chance        float    -- % drop chance (0.0 - 100.0)
  minlevel      tinyint  -- min player level to receive (0 = any)
  maxlevel      tinyint  -- max player level (255 = any)
  multiplier    tinyint  -- number of items to drop
)
```

---

## NPC Spell Chain

To find what spells an NPC casts:

```
npc_types.npc_spells_id
  → npc_spells.id
    → npc_spells_entries.npc_spells_id
      → npc_spells_entries.spellid
        → spells_new.id
```

### `npc_spells`

```sql
CREATE TABLE npc_spells (
  id    int       -- primary key
  name  tinytext  -- descriptive name (e.g., "Default Enchanter List")
)
```

### `npc_spells_entries`

```sql
CREATE TABLE npc_spells_entries (
  npc_spells_id  int
  spellid        smallint  -- FK → spells_new.id
  type           smallint  -- spell category (1=heal, 2=nuke, 3=root, etc.)
  minlevel       tinyint   -- min NPC level to use this spell
  maxlevel       tinyint
  priority       smallint  -- higher = cast more often
  recast_delay   int       -- seconds between uses (-1 = use spell's default)
)
```

---

## Skill Data

### `skill_caps`

Max skill value for a given class at a given level.

```sql
CREATE TABLE skill_caps (
  skill_id   tinyint   -- skill type ID
  class_id   tinyint   -- class ID (1=WAR, 2=CLR, ... 14=ENC, 15=BST)
  level      tinyint   -- character level
  cap        mediumint -- max skill at this level
)
```

---

## Player Tables (player_tables dump)

These are in the same database but populated by the live server. **Read-only for the app** — we only use them if doing character-side features like spell checklist.

Key tables:
- `character_data` — character stats, zone, level, class
- `account` — account info
- `character_spells` — spellbook contents (spellid + slot)
- `character_inventory` — equipped/carried items

---

## Important Notes

1. **`special_abilities` delimiter** is `^` (caret), not `|` (pipe). Each entry is `code,value`.
2. **`spells_new.classes1-15`** store the *minimum level* to scribe/use, with `255` meaning the class cannot use the spell.
3. **`items.classes`** is a bitmask; use `classes & 8192 != 0` to test Enchanter usability.
4. **`items.slots`** is a bitmask; `slots & 4096 != 0` = primary slot.
5. **`spawn2.spawngroupID`** sometimes points directly to an `npc_types.id` for solo-spawn NPCs (spawngroupID == npcID in those cases).
6. All currency in `loottable` is in copper (1 pp = 1000 cp).
