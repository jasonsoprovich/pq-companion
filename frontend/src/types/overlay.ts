import type { NPC, NPCCasterSummary } from './npc'

// Caster-summary types live in ./npc (shared by the overlays and the DB page);
// re-exported here so existing overlay imports keep resolving.
export type {
  NPCCasterSummary,
  CasterHighlight,
  NamedSpell,
  ClassListSummary,
} from './npc'

export interface SpecialAbility {
  code: number
  value: number
  name: string
}

// TargetVariant carries one alternative interpretation when the targeted
// name maps to multiple npc_types rows the backend couldn't reduce to one
// (typically Quarm RNG-pair NPCs that share a spawngroup, e.g. ssratemple's
// shissar revenant necro/SK split). Present in TargetState.variants when
// ambiguity is real; absent or single-element otherwise.
export interface TargetVariant {
  npc: NPC
  special_abilities: SpecialAbility[]
  caster_summary?: NPCCasterSummary
}

export interface TargetState {
  has_target: boolean
  target_name?: string
  npc_data?: NPC
  special_abilities?: SpecialAbility[]
  caster_summary?: NPCCasterSummary
  // variants is populated (length >= 2) when the target name is ambiguous —
  // e.g. two shissar revenant rows that share a spawngroup. Renderers should
  // surface all variants (class label, loot, abilities) instead of pretending
  // npc_data is the only answer. Single-variant lookups leave this empty.
  variants?: TargetVariant[]
  current_zone?: string
  // hp_percent is 0-100 when fed by the Zeal pipe, or -1 when unknown
  // (Zeal not running or no value yet for the current target).
  hp_percent: number
  pet_owner?: string
  // is_corpse is true when the target name ended in "'s corpse" — the lookup
  // strips the suffix to find the underlying NPC, but the HP bar should pin
  // to 0% regardless of what Zeal reports.
  is_corpse?: boolean
  last_updated: string
}
