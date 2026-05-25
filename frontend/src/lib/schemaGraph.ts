// Schema-graph source of truth for the Developer tab's interactive
// relationship browser. quarm.db doesn't declare foreign keys, so we
// maintain the implicit relationships here. Edges below mirror the
// joins the backend already uses (see internal/db/* and search.go).
//
// Tables array is curated, not complete — it only includes tables a
// player would explore (game content). Server-side tables like
// account, character_*, qs_player_*, guilds, chatchannels, etc. are
// intentionally omitted; they'd add visual noise without giving anyone
// useful information.
//
// When the upstream Quarm schema changes via the data-release workflow,
// spot-check this file: missing tables surface as broken edges in the
// graph (the renderer ignores edges whose endpoints aren't in `tables`).

export type SchemaCategory =
  | 'items'
  | 'npcs'
  | 'spawning'
  | 'spells'
  | 'aas'
  | 'zones'
  | 'world'

export interface SchemaTable {
  name: string
  category: SchemaCategory
  // A handful of representative columns — enough to identify joins at
  // a glance. The full column list is available in the SQL Sandbox
  // sidebar, so we don't need every column here.
  columns: Array<{ name: string; type: string; pk?: boolean; fk?: boolean }>
}

export interface SchemaEdge {
  from: string
  fromCol: string
  to: string
  toCol: string
  // Cardinality hint for the edge: 'many-one' = many `from` rows
  // reference one `to` row (the common implicit-FK case). Used by the
  // renderer to draw the appropriate marker.
  cardinality: 'one-one' | 'many-one' | 'one-many'
  // Optional note describing the join — surfaces in the hover tooltip.
  note?: string
}

export const SCHEMA_TABLES: SchemaTable[] = [
  // ── Items ────────────────────────────────────────────────────────
  {
    name: 'items',
    category: 'items',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'Name', type: 'TEXT' },
      { name: 'ac', type: 'INT' },
      { name: 'hp', type: 'INT' },
      { name: 'mana', type: 'INT' },
      { name: 'itemtype', type: 'INT' },
      { name: 'slots', type: 'INT' },
      { name: 'classes', type: 'INT' },
      { name: 'reqlevel', type: 'INT' },
    ],
  },

  // ── Loot chain ───────────────────────────────────────────────────
  {
    name: 'loottable',
    category: 'items',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'mincash', type: 'INT' },
      { name: 'maxcash', type: 'INT' },
    ],
  },
  {
    name: 'loottable_entries',
    category: 'items',
    columns: [
      { name: 'loottable_id', type: 'INT', fk: true },
      { name: 'lootdrop_id', type: 'INT', fk: true },
      { name: 'probability', type: 'INT' },
      { name: 'multiplier', type: 'INT' },
    ],
  },
  {
    name: 'lootdrop',
    category: 'items',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
    ],
  },
  {
    name: 'lootdrop_entries',
    category: 'items',
    columns: [
      { name: 'lootdrop_id', type: 'INT', fk: true },
      { name: 'item_id', type: 'INT', fk: true },
      { name: 'chance', type: 'REAL' },
      { name: 'minlevel', type: 'INT' },
      { name: 'maxlevel', type: 'INT' },
    ],
  },
  {
    name: 'global_loot',
    category: 'items',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'loottable_id', type: 'INT', fk: true },
      { name: 'min_level', type: 'INT' },
      { name: 'max_level', type: 'INT' },
      { name: 'zone', type: 'TEXT' },
    ],
  },

  // ── NPCs ─────────────────────────────────────────────────────────
  {
    name: 'npc_types',
    category: 'npcs',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'level', type: 'INT' },
      { name: 'hp', type: 'INT' },
      { name: 'loottable_id', type: 'INT', fk: true },
      { name: 'npc_spells_id', type: 'INT', fk: true },
      { name: 'npc_spells_effects_id', type: 'INT', fk: true },
      { name: 'npc_faction_id', type: 'INT', fk: true },
      { name: 'special_abilities', type: 'TEXT' },
    ],
  },
  {
    name: 'npc_faction',
    category: 'npcs',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'primaryfaction', type: 'INT', fk: true },
    ],
  },
  {
    name: 'npc_faction_entries',
    category: 'npcs',
    columns: [
      { name: 'npc_faction_id', type: 'INT', fk: true },
      { name: 'faction_id', type: 'INT', fk: true },
      { name: 'value', type: 'INT' },
    ],
  },
  {
    name: 'faction_list',
    category: 'npcs',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'base', type: 'INT' },
    ],
  },
  {
    name: 'faction_list_mod',
    category: 'npcs',
    columns: [
      { name: 'faction_id', type: 'INT', fk: true },
      { name: 'mod', type: 'INT' },
      { name: 'mod_name', type: 'TEXT' },
    ],
  },
  {
    name: 'npc_emotes',
    category: 'npcs',
    columns: [
      { name: 'emoteid', type: 'INT' },
      { name: 'event_', type: 'INT' },
      { name: 'type', type: 'INT' },
      { name: 'text', type: 'TEXT' },
    ],
  },

  // ── Spawning ─────────────────────────────────────────────────────
  {
    name: 'spawn2',
    category: 'spawning',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'spawngroupID', type: 'INT', fk: true },
      { name: 'zone', type: 'TEXT', fk: true },
      { name: 'x', type: 'REAL' },
      { name: 'y', type: 'REAL' },
      { name: 'z', type: 'REAL' },
      { name: 'respawntime', type: 'INT' },
      { name: 'pathgrid', type: 'INT', fk: true },
    ],
  },
  {
    name: 'spawngroup',
    category: 'spawning',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'spawn_limit', type: 'INT' },
    ],
  },
  {
    name: 'spawnentry',
    category: 'spawning',
    columns: [
      { name: 'spawngroupID', type: 'INT', fk: true },
      { name: 'npcID', type: 'INT', fk: true },
      { name: 'chance', type: 'INT' },
    ],
  },
  {
    name: 'grid',
    category: 'spawning',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'zoneid', type: 'INT', pk: true },
      { name: 'type', type: 'INT' },
    ],
  },
  {
    name: 'grid_entries',
    category: 'spawning',
    columns: [
      { name: 'gridid', type: 'INT', fk: true },
      { name: 'zoneid', type: 'INT', fk: true },
      { name: 'number', type: 'INT' },
      { name: 'x', type: 'REAL' },
      { name: 'y', type: 'REAL' },
      { name: 'pause', type: 'INT' },
    ],
  },

  // ── Spells & AAs ─────────────────────────────────────────────────
  {
    name: 'spells_new',
    category: 'spells',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'mana', type: 'INT' },
      { name: 'cast_time', type: 'INT' },
      { name: 'buffduration', type: 'INT' },
      { name: 'buffdurationformula', type: 'INT' },
      { name: 'effectid1', type: 'INT' },
      { name: 'effect_base_value1', type: 'INT' },
    ],
  },
  {
    name: 'npc_spells',
    category: 'spells',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'parent_list', type: 'INT', fk: true },
      { name: 'attack_proc', type: 'INT' },
    ],
  },
  {
    name: 'npc_spells_entries',
    category: 'spells',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'npc_spells_id', type: 'INT', fk: true },
      { name: 'spellid', type: 'INT', fk: true },
      { name: 'minlevel', type: 'INT' },
      { name: 'maxlevel', type: 'INT' },
      { name: 'priority', type: 'INT' },
    ],
  },
  {
    name: 'npc_spells_effects',
    category: 'spells',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'parent_list', type: 'INT', fk: true },
    ],
  },
  {
    name: 'npc_spells_effects_entries',
    category: 'spells',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'npc_spells_effects_id', type: 'INT', fk: true },
      { name: 'spell_effect_id', type: 'INT' },
      { name: 'minlevel', type: 'INT' },
      { name: 'maxlevel', type: 'INT' },
    ],
  },
  {
    name: 'blocked_spells',
    category: 'spells',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'spellid', type: 'INT', fk: true },
      { name: 'type', type: 'INT' },
      { name: 'zoneid', type: 'INT', fk: true },
    ],
  },
  {
    name: 'altadv_vars',
    category: 'aas',
    columns: [
      { name: 'skill_id', type: 'INT', pk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'cost', type: 'INT' },
      { name: 'max_level', type: 'INT' },
      { name: 'spellid', type: 'INT', fk: true },
      { name: 'classes', type: 'INT' },
    ],
  },
  {
    name: 'aa_actions',
    category: 'aas',
    columns: [
      { name: 'aaid', type: 'INT', fk: true },
      { name: 'rank', type: 'INT', pk: true },
      { name: 'reuse_time', type: 'INT' },
      { name: 'spell_id', type: 'INT' },
    ],
  },
  {
    name: 'aa_effects',
    category: 'aas',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'aaid', type: 'INT', fk: true },
      { name: 'slot', type: 'INT' },
      { name: 'effectid', type: 'INT' },
      { name: 'base1', type: 'INT' },
    ],
  },

  // ── Zones & world ────────────────────────────────────────────────
  {
    name: 'zone',
    category: 'zones',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'short_name', type: 'TEXT' },
      { name: 'long_name', type: 'TEXT' },
      { name: 'zoneidnumber', type: 'INT' },
      { name: 'min_level', type: 'INT' },
    ],
  },
  {
    name: 'zone_points',
    category: 'zones',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'zone', type: 'TEXT', fk: true },
      { name: 'number', type: 'INT' },
      { name: 'target_zone_id', type: 'INT', fk: true },
      { name: 'target_x', type: 'REAL' },
      { name: 'target_y', type: 'REAL' },
    ],
  },
  {
    name: 'keyring_data',
    category: 'zones',
    columns: [
      { name: 'key_item', type: 'INT', fk: true },
      { name: 'key_name', type: 'TEXT' },
      { name: 'zoneid', type: 'INT', fk: true },
      { name: 'stage', type: 'INT' },
    ],
  },
  {
    name: 'doors',
    category: 'world',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'doorid', type: 'INT' },
      { name: 'zone', type: 'TEXT', fk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'lockpick', type: 'INT' },
    ],
  },
  {
    name: 'ground_spawns',
    category: 'world',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'zoneid', type: 'INT', fk: true },
      { name: 'item', type: 'INT', fk: true },
      { name: 'name', type: 'TEXT' },
      { name: 'respawn_timer', type: 'INT' },
    ],
  },
  {
    name: 'forage',
    category: 'world',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'zoneid', type: 'INT', fk: true },
      { name: 'Itemid', type: 'INT', fk: true },
      { name: 'level', type: 'INT' },
      { name: 'chance', type: 'INT' },
    ],
  },
  {
    name: 'fishing',
    category: 'world',
    columns: [
      { name: 'id', type: 'INT', pk: true },
      { name: 'zoneid', type: 'INT', fk: true },
      { name: 'Itemid', type: 'INT', fk: true },
      { name: 'skill_level', type: 'INT' },
      { name: 'chance', type: 'INT' },
    ],
  },
]

export const SCHEMA_EDGES: SchemaEdge[] = [
  // Loot chain
  { from: 'npc_types', fromCol: 'loottable_id', to: 'loottable', toCol: 'id', cardinality: 'many-one', note: 'NPC drops are configured by loottable_id' },
  { from: 'global_loot', fromCol: 'loottable_id', to: 'loottable', toCol: 'id', cardinality: 'many-one', note: 'System-wide drops (bypass npc_types.loottable_id)' },
  { from: 'loottable_entries', fromCol: 'loottable_id', to: 'loottable', toCol: 'id', cardinality: 'many-one' },
  { from: 'loottable_entries', fromCol: 'lootdrop_id', to: 'lootdrop', toCol: 'id', cardinality: 'many-one', note: 'A loottable picks one or more lootdrops' },
  { from: 'lootdrop_entries', fromCol: 'lootdrop_id', to: 'lootdrop', toCol: 'id', cardinality: 'many-one' },
  { from: 'lootdrop_entries', fromCol: 'item_id', to: 'items', toCol: 'id', cardinality: 'many-one', note: 'Actual item dropped' },

  // NPC spells
  { from: 'npc_types', fromCol: 'npc_spells_id', to: 'npc_spells', toCol: 'id', cardinality: 'many-one', note: 'Set of spells this NPC can cast' },
  { from: 'npc_types', fromCol: 'npc_spells_effects_id', to: 'npc_spells_effects', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_spells', fromCol: 'parent_list', to: 'npc_spells', toCol: 'id', cardinality: 'many-one', note: 'Inherits spells from a parent list' },
  { from: 'npc_spells_entries', fromCol: 'npc_spells_id', to: 'npc_spells', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_spells_entries', fromCol: 'spellid', to: 'spells_new', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_spells_effects_entries', fromCol: 'npc_spells_effects_id', to: 'npc_spells_effects', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_spells_effects', fromCol: 'parent_list', to: 'npc_spells_effects', toCol: 'id', cardinality: 'many-one' },

  // NPC factions
  { from: 'npc_types', fromCol: 'npc_faction_id', to: 'npc_faction', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_faction', fromCol: 'primaryfaction', to: 'faction_list', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_faction_entries', fromCol: 'npc_faction_id', to: 'npc_faction', toCol: 'id', cardinality: 'many-one' },
  { from: 'npc_faction_entries', fromCol: 'faction_id', to: 'faction_list', toCol: 'id', cardinality: 'many-one' },
  { from: 'faction_list_mod', fromCol: 'faction_id', to: 'faction_list', toCol: 'id', cardinality: 'many-one' },

  // Spawning
  { from: 'spawn2', fromCol: 'spawngroupID', to: 'spawngroup', toCol: 'id', cardinality: 'many-one' },
  { from: 'spawn2', fromCol: 'zone', to: 'zone', toCol: 'short_name', cardinality: 'many-one', note: 'Spawn location resolves to zone.short_name' },
  { from: 'spawn2', fromCol: 'pathgrid', to: 'grid', toCol: 'id', cardinality: 'many-one', note: 'Pathing grid (also joined on zoneid)' },
  { from: 'spawnentry', fromCol: 'spawngroupID', to: 'spawngroup', toCol: 'id', cardinality: 'many-one' },
  { from: 'spawnentry', fromCol: 'npcID', to: 'npc_types', toCol: 'id', cardinality: 'many-one', note: 'Which NPC this spawnentry picks' },
  { from: 'grid_entries', fromCol: 'gridid', to: 'grid', toCol: 'id', cardinality: 'many-one' },

  // AAs
  { from: 'altadv_vars', fromCol: 'spellid', to: 'spells_new', toCol: 'id', cardinality: 'many-one', note: 'AA fires this spell when activated' },
  { from: 'aa_actions', fromCol: 'aaid', to: 'altadv_vars', toCol: 'skill_id', cardinality: 'many-one' },
  { from: 'aa_effects', fromCol: 'aaid', to: 'altadv_vars', toCol: 'skill_id', cardinality: 'many-one' },

  // Zones & world
  { from: 'zone_points', fromCol: 'zone', to: 'zone', toCol: 'short_name', cardinality: 'many-one', note: 'Origin zone of the transition' },
  { from: 'zone_points', fromCol: 'target_zone_id', to: 'zone', toCol: 'zoneidnumber', cardinality: 'many-one', note: 'Target zone of the transition' },
  { from: 'keyring_data', fromCol: 'key_item', to: 'items', toCol: 'id', cardinality: 'many-one' },
  { from: 'keyring_data', fromCol: 'zoneid', to: 'zone', toCol: 'zoneidnumber', cardinality: 'many-one' },
  { from: 'doors', fromCol: 'zone', to: 'zone', toCol: 'short_name', cardinality: 'many-one' },
  { from: 'ground_spawns', fromCol: 'zoneid', to: 'zone', toCol: 'zoneidnumber', cardinality: 'many-one' },
  { from: 'ground_spawns', fromCol: 'item', to: 'items', toCol: 'id', cardinality: 'many-one' },
  { from: 'forage', fromCol: 'zoneid', to: 'zone', toCol: 'zoneidnumber', cardinality: 'many-one' },
  { from: 'forage', fromCol: 'Itemid', to: 'items', toCol: 'id', cardinality: 'many-one' },
  { from: 'fishing', fromCol: 'zoneid', to: 'zone', toCol: 'zoneidnumber', cardinality: 'many-one' },
  { from: 'fishing', fromCol: 'Itemid', to: 'items', toCol: 'id', cardinality: 'many-one' },
  { from: 'blocked_spells', fromCol: 'spellid', to: 'spells_new', toCol: 'id', cardinality: 'many-one' },
  { from: 'blocked_spells', fromCol: 'zoneid', to: 'zone', toCol: 'zoneidnumber', cardinality: 'many-one' },
]

// Colour palette per category — referenced by the graph node renderer so
// the user can visually group related tables. Kept here so the graph and
// any future legend stay in sync.
export const CATEGORY_COLORS: Record<SchemaCategory, { bg: string; border: string; label: string }> = {
  items:    { bg: '#3b2f1a', border: '#f59e0b', label: 'Items & Loot' },
  npcs:     { bg: '#2a1f2a', border: '#c084fc', label: 'NPCs & Factions' },
  spawning: { bg: '#1a2a2a', border: '#22d3ee', label: 'Spawning' },
  spells:   { bg: '#1a2535', border: '#60a5fa', label: 'Spells' },
  aas:      { bg: '#22183a', border: '#a78bfa', label: 'AAs' },
  zones:    { bg: '#1f2a1f', border: '#4ade80', label: 'Zones' },
  world:    { bg: '#2a221a', border: '#fb923c', label: 'World content' },
}
