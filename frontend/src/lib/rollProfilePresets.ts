import type { RollProfile } from '../types/rolls'

// Built-in starting points for the roll grouping profile. Users pick one as a
// base, then optionally switch to "Custom" and edit the tiers. The profile is
// global (a guild rolls one way at a time).

export interface RollProfilePreset {
  id: string
  name: string
  description: string
  profile: RollProfile
}

export const ROLL_PROFILE_PRESETS: RollProfilePreset[] = [
  {
    id: 'simple',
    name: 'Simple',
    description: 'Each roll range is its own list, highest (or lowest) wins. Default.',
    profile: { mode: 'simple' },
  },
  {
    id: 'pick-upgrade-alt',
    name: 'Pick · Upgrade · Alt',
    description:
      'Tiered "1xx": 11 Pick beats 22 Upgrade beats 33 Alt. Works for 2xx, 3xx… too — leading digit is the item.',
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
