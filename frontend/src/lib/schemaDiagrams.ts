// Curated Mermaid erDiagram blocks for the Developer-mode schema viewer.
//
// quarm.db doesn't declare foreign keys, so these relationships are
// inferred from the upstream Project Quarm schema and from how the
// backend joins them. A single mega-diagram across ~60+ tables is
// unreadable; these topical views answer specific questions like
// "how do I get from a zone to its NPC drops?" in one glance.
//
// Maintenance: when the Quarm DB schema is refreshed via the data-release
// workflow, spot-check column names against quarm.db. If something
// disappears, update the affected diagram(s) here rather than try to
// keep the table relationships in sync programmatically — implicit FKs
// only exist in our heads.

export interface SchemaDiagram {
  id: string
  label: string
  description: string
  mermaid: string
}

export const SCHEMA_DIAGRAMS: SchemaDiagram[] = [
  {
    id: 'items-loot',
    label: 'Items & Loot',
    description:
      'How an item ends up on an NPC. npc_types.loottable_id picks a loottable; ' +
      'loottable_entries roll for one or more lootdrops; lootdrop_entries pick ' +
      'the actual item with per-row chance. global_loot bypasses npc_types.loottable_id ' +
      'for system-wide drops (e.g. seal of the Holy Avenger pieces).',
    mermaid: `erDiagram
  npc_types ||--o{ loottable : "loottable_id"
  loottable ||--o{ loottable_entries : "id"
  loottable_entries }o--|| lootdrop : "lootdrop_id"
  lootdrop ||--o{ lootdrop_entries : "id"
  lootdrop_entries }o--|| items : "item_id"
  global_loot }o--|| loottable : "loottable_id"

  npc_types {
    INT id PK
    TEXT name
    INT level
    INT loottable_id FK
    INT npc_spells_id FK
    INT npc_faction_id FK
  }
  loottable {
    INT id PK
    TEXT name
    INT mincash
    INT maxcash
  }
  loottable_entries {
    INT loottable_id FK
    INT lootdrop_id FK
    INT probability
    INT multiplier
  }
  lootdrop {
    INT id PK
    TEXT name
  }
  lootdrop_entries {
    INT lootdrop_id FK
    INT item_id FK
    REAL chance
    INT minlevel
    INT maxlevel
  }
  items {
    INT id PK
    TEXT Name
    INT ac
    INT hp
    INT mana
    INT itemtype
  }
  global_loot {
    INT id PK
    INT loottable_id FK
    INT min_level
    INT max_level
    TEXT zone
  }
`,
  },
  {
    id: 'npcs-spawns',
    label: 'NPCs & Spawns',
    description:
      'How an NPC gets into the world. zone defines the map; spawn2 anchors a ' +
      'spawngroup at (x,y,z); spawnentry weights which npc_types from that group ' +
      'gets picked on each cycle. Pathing comes from grid + grid_entries when ' +
      'spawn2.pathgrid is non-zero.',
    mermaid: `erDiagram
  zone ||--o{ spawn2 : "short_name = zone"
  spawn2 }o--|| spawngroup : "spawngroupID"
  spawngroup ||--o{ spawnentry : "id"
  spawnentry }o--|| npc_types : "npcID"
  spawn2 }o--o| grid : "pathgrid = id, zone"
  grid ||--o{ grid_entries : "id, zoneid"

  zone {
    INT id PK
    TEXT short_name
    TEXT long_name
    INT zoneidnumber
  }
  spawn2 {
    INT id PK
    INT spawngroupID FK
    TEXT zone
    REAL x
    REAL y
    REAL z
    INT respawntime
    INT pathgrid FK
  }
  spawngroup {
    INT id PK
    TEXT name
    INT spawn_limit
    INT delay
  }
  spawnentry {
    INT spawngroupID FK
    INT npcID FK
    INT chance
  }
  npc_types {
    INT id PK
    TEXT name
    INT level
    INT hp
    INT loottable_id
    TEXT special_abilities
  }
  grid {
    INT id PK
    INT zoneid PK
    INT type
  }
  grid_entries {
    INT gridid FK
    INT zoneid FK
    INT number
    REAL x
    REAL y
    REAL z
    INT pause
  }
`,
  },
  {
    id: 'spells',
    label: 'Spells & AAs',
    description:
      'spells_new is the master spell list (player and NPC). NPC casting tables ' +
      'link npc_types.npc_spells_id → npc_spells (set metadata) → npc_spells_entries ' +
      '(one row per spell in the set). altadv_vars is the AA master list; ' +
      'aa_actions provides per-rank reuse/spell info and aa_effects the per-rank ' +
      'effect slots.',
    mermaid: `erDiagram
  npc_types }o--|| npc_spells : "npc_spells_id"
  npc_spells ||--o{ npc_spells_entries : "id"
  npc_spells_entries }o--|| spells_new : "spellid"
  altadv_vars }o--|| spells_new : "spellid"
  altadv_vars ||--o{ aa_actions : "skill_id = aaid"
  altadv_vars ||--o{ aa_effects : "skill_id = aaid"

  spells_new {
    INT id PK
    TEXT name
    INT mana
    INT cast_time
    INT buffduration
    INT buffdurationformula
  }
  npc_spells {
    INT id PK
    TEXT name
    INT parent_list
    INT attack_proc
  }
  npc_spells_entries {
    INT id PK
    INT npc_spells_id FK
    INT spellid FK
    INT minlevel
    INT maxlevel
    INT priority
  }
  altadv_vars {
    INT skill_id PK
    TEXT name
    INT cost
    INT max_level
    INT spellid FK
    INT classes
  }
  aa_actions {
    INT aaid FK
    INT rank PK
    INT reuse_time
    INT spell_id
  }
  aa_effects {
    INT id PK
    INT aaid FK
    INT slot
    INT effectid
    INT base1
    INT base2
  }
`,
  },
  {
    id: 'keys-quests',
    label: 'Keys & Zones',
    description:
      'How keys and zone connections fit together. keyring_data is the master list ' +
      'of key items per zone+stage (PQ key progression often has 2–3 stages of a ' +
      'key). zone_points define inter-zone transitions; target_zone_id links into ' +
      'zone.zoneidnumber. items.id holds the key item itself.',
    mermaid: `erDiagram
  keyring_data }o--|| items : "key_item = id"
  keyring_data }o--|| zone : "zoneid = zoneidnumber"
  zone_points }o--|| zone : "target_zone_id = zoneidnumber"

  keyring_data {
    INT key_item FK
    TEXT key_name
    INT zoneid FK
    INT stage
  }
  items {
    INT id PK
    TEXT Name
    INT itemtype
  }
  zone {
    INT id PK
    TEXT short_name
    TEXT long_name
    INT zoneidnumber
  }
  zone_points {
    INT id PK
    TEXT zone
    INT number
    INT target_zone_id FK
    REAL target_x
    REAL target_y
    REAL target_z
  }
`,
  },
]
