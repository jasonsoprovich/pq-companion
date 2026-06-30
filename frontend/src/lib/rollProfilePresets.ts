import type { RollProfile } from '../types/rolls'

// Built-in starting points for the roll grouping profile. Users pick one as a
// base, then optionally switch to "Custom" and edit the tiers. The profile is
// global (a guild rolls one way at a time).

export interface RollProfilePreset {
  id: string
  name: string
  // One-line summary shown under the dropdown.
  description: string
  // Longer explanation + concrete examples for the collapsible info panel.
  how: string
  examples: string[]
  profile: RollProfile
}

export const ROLL_PROFILE_PRESETS: RollProfilePreset[] = [
  {
    id: 'simple',
    name: 'Simple',
    description: 'Each roll range is its own list, highest (or lowest) wins. Default.',
    how:
      'No grouping. Every distinct /random range becomes its own list and the ' +
      'highest roll wins (flip the Highest/Lowest toggle for low-roll-wins). ' +
      'Use this when your guild just calls a number and everyone rolls it.',
    examples: [
      'Caller: "roll 1000 on the robe" → everyone /random 1000 → one list, highest of those rolls wins.',
      'A separate /random 500 shows up as its own list — ranges are never mixed.',
    ],
    profile: { mode: 'simple' },
  },
  {
    id: 'pick-upgrade-alt',
    name: 'Pick · Upgrade · Alt',
    description:
      'Tiered "1xx": 11 Pick beats 22 Upgrade beats 33 Alt. Works for 2xx, 3xx… too — leading digit is the item.',
    how:
      'The last two digits choose the tier (11 = Pick, 22 = Upgrade, 33 = Alt) and ' +
      'the leading digit groups the item (1xx = item 1, 2xx = item 2…). The winner ' +
      'is the highest roll in the highest tier anyone rolled: a single Pick roll ' +
      'beats every Upgrade and Alt roll. One profile covers an unlimited number of ' +
      'items in the same raid.',
    examples: [
      'Item 1 — call "1xx": /random 111 = Pick, /random 122 = Upgrade, /random 133 = Alt.',
      'If anyone rolled 111, the highest 111 wins — 122s and 133s don\'t matter.',
      'Next item — call "2xx": 211 / 222 / 233 form a separate contest automatically.',
    ],
    profile: {
      mode: 'tiered',
      scheme: 'suffix',
      divisor: 100,
      tiers: [
        { match: 11, label: 'Pick' },
        { match: 22, label: 'Upgrade' },
        { match: 33, label: 'Alt' },
      ],
    },
  },
  {
    id: 'need-greed',
    name: 'Need · Greed',
    description: '111 Need beats 222 Greed. For one item rolled at a time (pickup groups).',
    how:
      'Specific numbers map to tiers: 111 = Need, 222 = Greed. Need always beats ' +
      'Greed regardless of the actual numbers rolled. Best for one item at a time ' +
      '(pickup groups) — finish and stop one before starting the next.',
    examples: [
      'On one item: Need rollers /random 111, Greed rollers /random 222.',
      'If anyone Needs, the highest 111 wins; otherwise the highest 222 wins.',
    ],
    profile: {
      mode: 'tiered',
      scheme: 'exact',
      tiers: [
        { match: 111, label: 'Need' },
        { match: 222, label: 'Greed' },
      ],
    },
  },
]

/** normalizeProfile fills in defaults so two equivalent profiles compare
 *  equal (e.g. an absent suffix divisor becomes 100). */
export function normalizeProfile(p: RollProfile): RollProfile {
  if (p.mode !== 'tiered') return { mode: 'simple' }
  return {
    mode: 'tiered',
    scheme: p.scheme ?? 'suffix',
    divisor: p.scheme === 'exact' ? undefined : p.divisor && p.divisor > 0 ? p.divisor : 100,
    tiers: (p.tiers ?? []).map((t) => ({ match: t.match, label: t.label })),
  }
}

export function profilesEqual(a: RollProfile, b: RollProfile): boolean {
  return JSON.stringify(normalizeProfile(a)) === JSON.stringify(normalizeProfile(b))
}

/** presetIdFor returns the id of the preset matching the given profile, or
 *  'custom' when it matches none. */
export function presetIdFor(profile: RollProfile): string {
  const hit = ROLL_PROFILE_PRESETS.find((p) => profilesEqual(p.profile, profile))
  return hit ? hit.id : 'custom'
}
