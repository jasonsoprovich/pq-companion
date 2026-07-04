package threat

import "github.com/jasonsoprovich/pq-companion/backend/internal/eqstat"

// Two-handed weapon item types in the EQMacEmu item_data.h ItemType enum, which
// the quarm.db items.itemtype column uses (see internal/db/enums/item_type.go).
const (
	itemType2HSlash    = 1
	itemType2HBlunt    = 4
	itemType2HPiercing = 35
)

// MeleeSwingHate returns the flat hate a single PRIMARY-hand melee swing puts on
// its target: the weapon's damage rating plus the primary-hand damage bonus.
//
// Per the EQMacEmu fork (Client::Attack: "Hate Generation is on a per swing
// basis, regardless of a hit, miss, or block, its always the same"), this value
// is added on EVERY swing and never varies with the damage actually rolled — so
// melee threat is swing-count × this value, NOT the sum of the white damage
// dealt. A double/triple attack is simply more swings.
//
// It models the primary hand only — the dominant source (main-hand swings plus
// every double/triple attack). Off-hand swings, which a log line gives no way to
// attribute to a hand, are approximated with this same value.
//
// Returns 0 when the weapon damage is non-positive (no or unknown weapon), so
// the caller can fall back to a coarser estimate.
func MeleeSwingHate(weaponDamage, weaponDelay, weaponItemType, level, class int) int {
	if weaponDamage <= 0 {
		return 0
	}
	return weaponDamage + meleeDamageBonus(weaponDelay, weaponItemType, level, class)
}

// meleeDamageBonus ports Client::GetDamageBonus — the primary-hand bonus added to
// both damage and hate for warrior-type classes at level 28+. 1H (and unarmed)
// primaries get the flat base term; two-handers add level- and delay-scaled
// terms. class is the 0-indexed eqstat value carried on character.Character.Class.
func meleeDamageBonus(delay, itemType, level, class int) int {
	if level < 28 || !isWarriorClass(class) {
		return 0
	}
	if delay <= 0 {
		delay = 1
	}
	bonus := 1 + (level-28)/3

	switch itemType {
	case itemType2HSlash, itemType2HBlunt, itemType2HPiercing:
		if delay <= 27 {
			return bonus + 1
		}
		if level > 29 {
			levelBonus := (level-30)/5 + 1
			if level > 50 {
				levelBonus++
				levelBonus2 := level - 50
				switch {
				case level > 67:
					levelBonus2 += 5
				case level > 59:
					levelBonus2 += 4
				case level > 58:
					levelBonus2 += 3
				case level > 56:
					levelBonus2 += 2
				case level > 54:
					levelBonus2++
				}
				levelBonus += levelBonus2 * delay / 40
			}
			bonus += levelBonus
		}
		if delay >= 40 {
			delayBonus := (delay-40)/3 + 1
			if delay >= 45 {
				delayBonus += 2
			} else if delay >= 43 {
				delayBonus++
			}
			bonus += delayBonus
		}
	}
	return bonus
}

// backstabSkillCap is the era backstab skill ceiling; a rogue reaches it around
// the level cap. It approximates the trained skill from level, since the
// character export doesn't carry per-skill values.
const backstabSkillCap = 252

// BackstabHate returns the flat hate a single backstab puts on its target.
//
// Per Client::DoBackstab in the EQMacEmu fork, a backstab's hate is its BASE
// damage — ((backstabSkill*0.02)+2) * weaponDamage — and it is added with a zero
// damage component, so (like a normal swing) the large rolled backstab number
// never contributes hate of its own; only this base does. weaponDamage is the
// equipped primary piercer's damage rating. Returns 0 for a non-positive weapon.
//
// The backstab skill isn't in the character export, so it's approximated as
// 5*level capped at the era ceiling (a rogue is at or near the cap by 60). The
// skill term only moves the multiplier between ~6 and ~7, so the estimate stays
// within a few percent of the server value (matches Quarmy's ~97 at level 60).
func BackstabHate(weaponDamage, level int) int {
	if weaponDamage <= 0 {
		return 0
	}
	skill := level * 5
	if skill > backstabSkillCap {
		skill = backstabSkillCap
	}
	return int((float64(skill)*0.02 + 2.0) * float64(weaponDamage))
}

// isWarriorClass mirrors Mob::IsWarriorClass: the melee and hybrid classes that
// receive the damage bonus (every class but the pure casters).
func isWarriorClass(class int) bool {
	switch class {
	case eqstat.Warrior, eqstat.Paladin, eqstat.Ranger, eqstat.ShadowKnight,
		eqstat.Monk, eqstat.Bard, eqstat.Rogue, eqstat.Beastlord:
		return true
	}
	return false
}
