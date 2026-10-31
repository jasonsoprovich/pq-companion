// Visibility filters for the NPC overlay. One list of hidden keys covers both
// whole sections ("section.faction") and individual chips and special-ability
// badges inside them ("combat.atk", "ability.10"). A plain key is hidden on
// both overlay surfaces; a "dashboard:" or "popout:" prefix hides it on that
// surface only. A character can keep their own list, which replaces the
// global one. Both overlay surfaces and the Settings editor share this.

export type NPCSurface = 'dashboard' | 'popout'

const SURFACES: NPCSurface[] = ['dashboard', 'popout']

export const sectionKey = (id: string): string => `section.${id}`

// The keys hidden on one surface, with surface prefixes stripped.
export function hiddenKeysFor(list: string[], surface: NPCSurface): Set<string> {
  const out = new Set<string>()
  for (const k of list) {
    const i = k.indexOf(':')
    if (i < 0) out.add(k)
    else if (k.slice(0, i) === surface) out.add(k.slice(i + 1))
  }
  return out
}

export function isHiddenOn(list: string[], key: string, surface: NPCSurface): boolean {
  return list.includes(key) || list.includes(`${surface}:${key}`)
}

// Returns the list with key shown or hidden on the given surfaces, stored in
// its shortest form (a plain key when hidden on both).
export function setKeyVisibility(
  list: string[],
  key: string,
  surfaces: NPCSurface[],
  show: boolean,
): string[] {
  const hidden = new Map(SURFACES.map((s) => [s, isHiddenOn(list, key, s)]))
  for (const s of surfaces) hidden.set(s, !show)
  const rest = list.filter((k) => k !== key && !SURFACES.some((s) => k === `${s}:${key}`))
  const dash = hidden.get('dashboard')
  const pop = hidden.get('popout')
  if (dash && pop) rest.push(key)
  else if (dash) rest.push(`dashboard:${key}`)
  else if (pop) rest.push(`popout:${key}`)
  return rest
}

export type NPCItemSection =
  | 'identity'
  | 'combat'
  | 'behavior'
  | 'resists'
  | 'attributes'

export interface NPCItemGroup {
  section: NPCItemSection
  label: string
  items: Array<{ key: string; label: string }>
}

// Everything except special abilities, which are keyed by code and listed from
// the enum catalog (see abilityKey).
export const NPC_ITEM_GROUPS: NPCItemGroup[] = [
  {
    section: 'identity',
    label: 'Identity',
    items: [
      { key: 'identity.level', label: 'Level' },
      { key: 'identity.class', label: 'Class' },
      { key: 'identity.race', label: 'Race' },
      { key: 'identity.body', label: 'Body type' },
    ],
  },
  {
    section: 'combat',
    label: 'Combat',
    items: [
      { key: 'combat.hp', label: 'HP' },
      { key: 'combat.mana', label: 'Mana' },
      { key: 'combat.ac', label: 'AC' },
      { key: 'combat.dmg', label: 'Damage' },
      { key: 'combat.atk', label: 'Attacks per round' },
      { key: 'combat.speed', label: 'Run speed' },
      { key: 'combat.slowmit', label: 'Slow mitigation' },
      { key: 'combat.pbaoe', label: 'PBAoE XP warning' },
    ],
  },
  {
    section: 'behavior',
    label: 'Behavior',
    items: [
      { key: 'behavior.delay', label: 'Attack delay' },
      { key: 'behavior.aggro', label: 'Aggro radius' },
      { key: 'behavior.assist', label: 'Assist radius' },
    ],
  },
  {
    section: 'resists',
    label: 'Resists',
    items: [
      { key: 'resists.mr', label: 'Magic' },
      { key: 'resists.cr', label: 'Cold' },
      { key: 'resists.fr', label: 'Fire' },
      { key: 'resists.dr', label: 'Disease' },
      { key: 'resists.pr', label: 'Poison' },
    ],
  },
  {
    section: 'attributes',
    label: 'Attributes',
    items: [
      { key: 'attributes.str', label: 'STR' },
      { key: 'attributes.sta', label: 'STA' },
      { key: 'attributes.dex', label: 'DEX' },
      { key: 'attributes.agi', label: 'AGI' },
      { key: 'attributes.int', label: 'INT' },
      { key: 'attributes.wis', label: 'WIS' },
      { key: 'attributes.cha', label: 'CHA' },
    ],
  },
]

export const abilityKey = (code: number): string => `ability.${code}`

// Resolves the hidden-key list for a character: their own override when one
// exists (even an empty one), otherwise the global list.
export function resolveHiddenItems(
  global: string[] | undefined,
  byCharacter: Record<string, string[]> | undefined,
  character: string,
): string[] {
  const own = character ? byCharacter?.[character.toLowerCase()] : undefined
  return own ?? global ?? []
}
