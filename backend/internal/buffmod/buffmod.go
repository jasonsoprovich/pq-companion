// Package buffmod computes per-character spell duration / cast-time modifiers
// from equipped item focus effects and trained AAs.
//
// Item focuses are fully data-driven: items.focuseffect → spells_new SPA 128
// (Increase Spell Duration) and SPA 127 (Increase Spell Haste, i.e. cast time
// reduction), with the surrounding limit SPAs (134 max level, 137 exclude
// effect / target type, 138 spell type, 139 min level, 140 min duration ticks,
// 141 specific spell) defining which spells the focus applies to.
//
// AA focuses are NOT data-driven on Project Quarm: aa_effects is empty for
// classic-era duration AAs because EQEmu hardcodes them in C++. The aaTable
// here is the small set of duration-extending AAs we know about, indexed by
// altadv_vars.eqmacid (the AAIndex value the Zeal quarmy export emits).
package buffmod

import (
	"fmt"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// SPA codes we care about. Other SPAs in a focus spell are treated as limits.
const (
	SPACastTime         = 127 // SE_IncreaseSpellHaste
	SPADuration         = 128 // SE_IncreaseSpellDuration
	SPALimitMaxLevel    = 134
	SPALimitEffect      = 137 // SE_LimitEffect: base is an SPA code; negative = exclude, positive = require
	SPALimitSpellType   = 138 // 0 = detrimental, 1 = beneficial, 2 = any
	SPALimitMinLevel    = 139
	SPALimitMinDuration = 140 // value × 6 sec (ticks)
	SPALimitSpellID     = 141 // SE_LimitSpell: negative = exclude spell ID, positive = whitelist
)

// Spell-type filter values for SPA 138.
const (
	SpellTypeDetrimental = 0
	SpellTypeBeneficial  = 1
	SpellTypeAny         = 2
)

// SpellTypeUnset is our sentinel meaning "the focus spell did not include a
// SPA 138 limit slot, so it applies to spells of any type". Stored as -1 so
// it can't be confused with a real DB value.
const SpellTypeUnset = -1

// BardClassIdx is the 0-indexed EQ class index for Bards. Bard songs are
// excluded from SPA 128 (duration) extensions from both AAs and item focus
// effects — see project_eq_bard_duration_exempt memory. Cast-time (SPA 127)
// reductions are unaffected.
const BardClassIdx = 7

// CasterClassUnknown is the sentinel passed to Resolve when the caller does
// not have character class context (legacy call sites, tests). When set,
// class-conditional rules (bard exemption) are skipped.
const CasterClassUnknown = -1

// SpellHasteCapPercent is the in-game hard cap on SPA 127 (Increase Spell
// Haste, i.e. cast-time reduction). Items + AAs can sum to more, but the
// effective value is clamped at this limit. Melee haste (SPA 11 / 119) is
// uncapped and tracked separately.
const SpellHasteCapPercent = 50

// classCannotCast is the spells_new classes1..classes15 sentinel meaning
// "this class can never cast this spell at any level". Mirrors the
// spelltimer package constant to avoid a circular import.
const classCannotCast = 255

// Permanent Illusion (altadv_vars.eqmacid 55, Enchanter-only): EQMacEmu's
// CalcBuffDuration (zone/spells.cpp) overrides the duration of any SELF-cast
// spell containing SPA 58 (SE_Illusion) to a flat 10000 ticks (~16h40m) when
// the caster has this AA — the illusion effectively lasts until death or
// dispel. The override replaces the formula duration entirely; AA/item focus
// percentages are irrelevant at that magnitude, so callers should return
// PermanentIllusionDurationSec directly instead of running Resolve math.
const (
	PermanentIllusionEqmacID     = 55
	PermanentIllusionDurationSec = 10000 * 6
	spaIllusion                  = 58
)

// HasIllusionEffect reports whether a spell's effect slots include SPA 58
// (SE_Illusion) — the trigger condition for the Permanent Illusion override.
func HasIllusionEffect(effectIDs []int) bool {
	for _, e := range effectIDs {
		if e == spaIllusion {
			return true
		}
	}
	return false
}

// Limits is the parsed set of constraints attached to a focus modifier.
// Zero/empty fields mean "no limit on this dimension".
type Limits struct {
	MaxLevel       int   `json:"max_level,omitempty"`        // SPA 134; max caster level
	MinLevel       int   `json:"min_level,omitempty"`        // SPA 139
	SpellType      int   `json:"spell_type"`                 // SPA 138; SpellTypeUnset/0/1/2
	MinDurationSec int   `json:"min_duration_sec,omitempty"` // SPA 140 × 6
	ExcludeEffects []int `json:"exclude_effects,omitempty"`  // SPA 137 with negative base
	IncludeEffects []int `json:"include_effects,omitempty"`  // SPA 137 with positive base; spell must contain one
	IncludeSpells  []int `json:"include_spells,omitempty"`   // SPA 141 with positive base; spell ID whitelist
	ExcludeSpells  []int `json:"exclude_spells,omitempty"`   // SPA 141 with negative base
}

// Modifier is one focus contribution from either an equipped item or a trained
// AA. SourceItemID/SourceAAID are mutually exclusive — exactly one is nonzero.
type Modifier struct {
	Source         string `json:"source"` // "item" | "aa"
	SourceItemID   int    `json:"source_item_id,omitempty"`
	SourceItemName string `json:"source_item_name,omitempty"`
	SourceItemSlot string `json:"source_item_slot,omitempty"` // e.g. "Head"
	SourceAAID     int    `json:"source_aa_id,omitempty"`     // altadv_vars.eqmacid
	SourceAAName   string `json:"source_aa_name,omitempty"`
	SourceAARank   int    `json:"source_aa_rank,omitempty"`
	FocusSpellID   int    `json:"focus_spell_id,omitempty"`
	FocusSpellName string `json:"focus_spell_name,omitempty"`
	SPA            int    `json:"spa"`     // 127 (cast time) or 128 (duration)
	Percent        int    `json:"percent"` // positive = extension/reduction magnitude
	Limits         Limits `json:"limits"`
}

// aaFocus describes a hardcoded AA's per-rank focus contribution.
type aaFocus struct {
	Name      string
	SPA       int   // 128 for duration, 127 for cast time
	SpellType int   // SpellTypeBeneficial / SpellTypeDetrimental / SpellTypeAny
	PerRank   []int // PerRank[rank-1] = percent at that rank
}

// aaTable maps altadv_vars.eqmacid (Quarmy AAIndex) → focus contribution.
//
// Entries here are AAs whose duration/cast-time effect is hardcoded in EQEmu
// rather than encoded in aa_effects. Add new entries as discovered; per-rank
// percentages should match the in-game tooltip on Project Quarm.
//
// 21  = Spell Casting Reinforcement         (3 ranks: +5% / +15% / +30% beneficial)
// 113 = Spell Casting Reinforcement Mastery (1 rank:  +20% beneficial)
var aaTable = map[int]aaFocus{
	21: {
		Name:      "Spell Casting Reinforcement",
		SPA:       SPADuration,
		SpellType: SpellTypeBeneficial,
		PerRank:   []int{5, 15, 30},
	},
	113: {
		Name:      "Spell Casting Reinforcement Mastery",
		SPA:       SPADuration,
		SpellType: SpellTypeBeneficial,
		PerRank:   []int{20},
	},
}

// equipSlots is the set of inventory locations whose items can grant a focus
// to the wearer. Bag/bank contents are ignored — only worn slots count.
var equipSlots = map[string]bool{
	"Charm": true, "Ear": true, "Head": true, "Face": true, "Neck": true,
	"Shoulders": true, "Arms": true, "Back": true, "Wrist": true,
	"Range": true, "Hands": true, "Primary": true, "Secondary": true,
	"Fingers": true, "Chest": true, "Legs": true, "Feet": true, "Waist": true,
	"PowerSource": true, "Ammo": true,
}

// Result is the full picture of a character's focus modifiers.
type Result struct {
	Character    string     `json:"character"`
	Contributors []Modifier `json:"contributors"`
	// PermanentIllusion is true when the character has the Permanent
	// Illusion AA (eqmacid 55) — self-cast illusions (SPA 58) then last
	// PermanentIllusionDurationSec instead of their formula duration.
	PermanentIllusion bool `json:"permanent_illusion,omitempty"`
}

// Compute walks the character's most recent Quarmy export (equipped items +
// AAs) and returns every focus contribution they provide. Stacking rules
// (e.g. best-item-only per focus type) are intentionally not applied here —
// callers can resolve per-spell with Resolve.
func Compute(eqPath, charName string, gameDB *db.DB) (*Result, error) {
	if eqPath == "" {
		return nil, fmt.Errorf("eq_path not configured")
	}
	if charName == "" {
		return nil, fmt.Errorf("character name required")
	}
	path := zeal.FindQuarmyFile(eqPath, charName)
	if path == "" {
		return nil, fmt.Errorf("no quarmy export found for %s", charName)
	}
	q, err := zeal.ParseQuarmy(path, charName)
	if err != nil {
		return nil, fmt.Errorf("parse quarmy: %w", err)
	}

	res := &Result{Character: charName, Contributors: []Modifier{}}

	for _, entry := range q.Inventory {
		if !equipSlots[entry.Location] {
			continue
		}
		item, err := gameDB.GetItem(entry.ID)
		if err != nil || item == nil || item.FocusEffect <= 0 {
			continue
		}
		focus, err := gameDB.GetSpell(item.FocusEffect)
		if err != nil || focus == nil {
			continue
		}
		for _, m := range parseFocusSpell(focus) {
			m.Source = "item"
			m.SourceItemID = item.ID
			m.SourceItemName = item.Name
			m.SourceItemSlot = entry.Location
			res.Contributors = append(res.Contributors, m)
		}
	}

	for _, aa := range q.AAs {
		if aa.ID == PermanentIllusionEqmacID && aa.Rank > 0 {
			res.PermanentIllusion = true
		}
		entry, ok := aaTable[aa.ID]
		if !ok || aa.Rank <= 0 || aa.Rank > len(entry.PerRank) {
			continue
		}
		res.Contributors = append(res.Contributors, Modifier{
			Source:       "aa",
			SourceAAID:   aa.ID,
			SourceAAName: entry.Name,
			SourceAARank: aa.Rank,
			SPA:          entry.SPA,
			Percent:      entry.PerRank[aa.Rank-1],
			Limits:       Limits{SpellType: entry.SpellType},
		})
	}

	return res, nil
}

// parseFocusSpell extracts every Modifier (one per SPA 127/128 slot) from a
// focus spell, attaching the union of all limit SPAs in the same spell.
func parseFocusSpell(s *db.Spell) []Modifier {
	limits := Limits{SpellType: SpellTypeUnset}
	type primary struct {
		spa, base int
	}
	var primaries []primary

	for i := 0; i < 12; i++ {
		spa := s.EffectIDs[i]
		base := s.EffectBaseValues[i]
		switch spa {
		case SPADuration, SPACastTime:
			primaries = append(primaries, primary{spa, base})
		case SPALimitMaxLevel:
			limits.MaxLevel = base
		case SPALimitMinLevel:
			limits.MinLevel = base
		case SPALimitSpellType:
			limits.SpellType = base
		case SPALimitMinDuration:
			limits.MinDurationSec = base * 6
		case SPALimitEffect:
			if base < 0 {
				limits.ExcludeEffects = append(limits.ExcludeEffects, -base)
			} else if base > 0 {
				limits.IncludeEffects = append(limits.IncludeEffects, base)
			}
		case SPALimitSpellID:
			if base < 0 {
				limits.ExcludeSpells = append(limits.ExcludeSpells, -base)
			} else if base > 0 {
				limits.IncludeSpells = append(limits.IncludeSpells, base)
			}
		}
	}

	mods := make([]Modifier, 0, len(primaries))
	for _, p := range primaries {
		mods = append(mods, Modifier{
			SPA:            p.spa,
			Percent:        p.base,
			FocusSpellID:   s.ID,
			FocusSpellName: s.Name,
			Limits:         limits,
		})
	}
	return mods
}

// Match reports whether m applies to a spell with the given properties.
// casterLevel = 0 means caller does not want a level filter applied.
func (m Modifier) Match(spellID, spellLevel, durationSec, spellType int, effectIDs []int) bool {
	l := m.Limits
	if l.MaxLevel > 0 && spellLevel > l.MaxLevel {
		return false
	}
	if l.MinLevel > 0 && spellLevel < l.MinLevel {
		return false
	}
	if l.MinDurationSec > 0 && durationSec < l.MinDurationSec {
		return false
	}
	if l.SpellType >= 0 && l.SpellType != SpellTypeAny && l.SpellType != spellType {
		return false
	}
	for _, ex := range l.ExcludeEffects {
		for _, e := range effectIDs {
			if ex == e {
				return false
			}
		}
	}
	// SPA 137 with positive base: the spell must contain at least one of the
	// required effects (EQMacEmu's SE_LimitEffect include semantics — e.g.
	// Summoning Haste only hastens spells with SPA 33, Summon Pet).
	if len(l.IncludeEffects) > 0 {
		found := false
		for _, inc := range l.IncludeEffects {
			for _, e := range effectIDs {
				if inc == e {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	for _, ex := range l.ExcludeSpells {
		if spellID == ex {
			return false
		}
	}
	if len(l.IncludeSpells) > 0 {
		found := false
		for _, inc := range l.IncludeSpells {
			if spellID == inc {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Resolution is a per-spell modifier breakdown for one (spell, contributors) pair.
//
// Duration stacking on Project Quarm applies AAs first, then item focus, both
// as multiplicative factors on the base. So the total effective duration is:
//
//	extended = base × (1 + aa%/100) × (1 + item%/100)
//
// Cast time is linear and additive across sources.
type Resolution struct {
	SpellID             int        `json:"spell_id"`
	SpellName           string     `json:"spell_name"`
	SpellType           int        `json:"spell_type"`
	SpellLevel          int        `json:"spell_level"`           // level used for SPA 134/139 checks
	CasterLevel         int        `json:"caster_level"`          // level used for the duration formula
	BaseDurationSec     int        `json:"base_duration_sec"`     // formula-computed at CasterLevel
	ExtendedDurationSec int        `json:"extended_duration_sec"` // base × (1+aa/100) × (1+item/100)
	DurationAAPercent   int        `json:"duration_aa_percent"`   // sum of matching AA durations
	DurationItemPercent int        `json:"duration_item_percent"` // best matching item duration
	DurationPercent     int        `json:"duration_percent"`      // combined effective %, for display
	CastTimePercent     int        `json:"cast_time_percent"`     // total cast-time reduction %
	Applied             []Modifier `json:"applied"`               // contributors that hit
	// PermanentIllusion is set by callers (not Resolve) when the Permanent
	// Illusion AA override replaced ExtendedDurationSec — the percent fields
	// are meaningless in that case and the UI should label the duration as
	// "until death/dispel" (~16h40m).
	PermanentIllusion bool `json:"permanent_illusion,omitempty"`
}

// SpellHasteSummary returns the character's theoretical maximum spell haste
// (SPA 127) across all their contributors, clamped to SpellHasteCapPercent.
// Used for the Character stat panel — unlike Resolve, this does not apply
// per-spell limit filters because the panel reports a "best case" character-
// wide stat, not a value tied to a specific spell. Item foci use best-only
// stacking (max of item percents); AA foci sum additively on top.
func SpellHasteSummary(contributors []Modifier) int {
	bestItem := 0
	aaSum := 0
	for _, c := range contributors {
		if c.SPA != SPACastTime {
			continue
		}
		switch c.Source {
		case "item":
			if c.Percent > bestItem {
				bestItem = c.Percent
			}
		case "aa":
			aaSum += c.Percent
		}
	}
	total := bestItem + aaSum
	if total > SpellHasteCapPercent {
		return SpellHasteCapPercent
	}
	return total
}

// SpellHasteSources returns the worn-focus and AA components of the character's
// spell haste (SPA 127) separately, before the SpellHasteCapPercent clamp, so
// the stat panel can show an Equip / AA breakdown (issue #128). Item foci use
// best-only stacking; AA foci sum. The caller applies the cap to the combined
// total (plus any buff contribution).
func SpellHasteSources(contributors []Modifier) (item, aa int) {
	for _, c := range contributors {
		if c.SPA != SPACastTime {
			continue
		}
		switch c.Source {
		case "item":
			if c.Percent > item {
				item = c.Percent
			}
		case "aa":
			aa += c.Percent
		}
	}
	return item, aa
}

// SpellLevel returns the lowest non-255 class level from a spell's classes
// array — the level at which the lowest-level class first learns the spell.
// Returns 0 if all entries are 255 (NPC-only or invalid).
//
// NOTE: this is only a fallback for callers without character context.
// EQMacEmu's SE_LimitMaxLevel / SE_LimitMinLevel checks compare against
// spell.classes[casterClass] — the level at which the CASTER'S class learns
// the spell, not the minimum across classes (zone/spell_effects.cpp:
// `spell_level = spell.classes[(GetClass()%16) - 1]`). Multi-class spells
// like Celerity (ENC 39 / SHM 56 / BST 63) hit SPA 134 caps differently per
// class — Extended Enhancement II (max_level 44) extends an Enchanter's cast
// but not a Shaman's. Prefer SpellLevelForClass when the class is known.
func SpellLevel(classLevels [15]int) int {
	min := 0
	for _, lvl := range classLevels {
		if lvl <= 0 || lvl >= 255 {
			continue
		}
		if min == 0 || lvl < min {
			min = lvl
		}
	}
	return min
}

// SpellLevelForClass returns the level the caster's own class learns the
// spell at — the value EQMacEmu compares against SPA 134 (Limit: Max Level)
// and SPA 139 (Limit: Min Level) when applying focus effects. Falls back to
// SpellLevel (lowest class level) when the class is unknown or cannot cast
// the spell (off-class clickies — Resolve's off-class gate handles the item
// focus exclusion there, so the fallback only affects display).
func SpellLevelForClass(classLevels [15]int, casterClass int) int {
	if casterClass >= 0 && casterClass < len(classLevels) {
		if lvl := classLevels[casterClass]; lvl > 0 && lvl < classCannotCast {
			return lvl
		}
	}
	return SpellLevel(classLevels)
}

// Resolve computes the effective duration/cast-time % for a single spell,
// using EQEmu's stacking rule: among item focuses with the same SPA, only the
// largest matching contribution applies; AAs of that SPA stack additively on
// top of the best item.
//
// spellLevel is what gets compared against SPA 134 (Limit: Max Level) and
// SPA 139 (Limit: Min Level) — typically the spell's effective level via
// SpellLevel(). casterLevel is informational and copied through to the
// Resolution so the UI can show what level was used for the duration formula.
// effectIDs are the SPA codes 0–11 of the buff itself (used for SPA-137
// exclusion checks against e.g. Complete Heal).
//
// casterClass is the 0-indexed EQ class of the player casting / receiving the
// buff (matches character.Character.Class). Pass CasterClassUnknown when no
// character context is available — class-conditional rules are then skipped.
// Used to enforce two EQ rules:
//   - Bard exemption: bards never receive AA/item duration extensions on
//     any buff they cast or click.
//   - Off-class clicky gate: when spellClassLevels[casterClass] == 255
//     (i.e. the caster's class cannot normally cast this spell), item focus
//     duration extensions are not applied — for SPA 134 (Limit: Max Level)
//     purposes the clicky's effective level is 255, above the max-level cap
//     every era item duration focus carries (e.g. the Coldain Prayer Shawl's
//     65). AA duration extensions (Spell Casting Reinforcement) have no class
//     or level limit, so they DO still apply to off-class clickies — e.g.
//     Soul Energy from a resist ring picks up the +50% AA duration on a 60
//     druid. Player-cast spells pass the class check by definition.
//
// spellClassLevels is the spell's classes1..classes15 array (0-indexed in our
// Spell struct). All zeroes means "no data" — the off-class gate is skipped.
func Resolve(spellID int, spellName string, spellLevel, casterLevel, baseDurationSec, spellType int, effectIDs []int, contributors []Modifier, casterClass int, spellClassLevels [15]int) Resolution {
	r := Resolution{
		SpellID:         spellID,
		SpellName:       spellName,
		SpellType:       spellType,
		SpellLevel:      spellLevel,
		CasterLevel:     casterLevel,
		BaseDurationSec: baseDurationSec,
		// Applied must never be nil: a nil slice marshals to JSON null, and the
		// frontend reads r.applied.length unconditionally. When no contributor
		// applies (e.g. the off-class clicky gate fires for a Shaman buff like
		// Primal Avatar resolved by a Magician), an empty slice keeps the
		// render path safe instead of black-screening the app.
		Applied: []Modifier{},
	}

	for _, spa := range []int{SPADuration, SPACastTime} {
		var bestItem *Modifier
		var aaPct int
		var matched []Modifier
		for i := range contributors {
			c := contributors[i]
			if c.SPA != spa || !c.Match(spellID, spellLevel, baseDurationSec, spellType, effectIDs) {
				continue
			}
			matched = append(matched, c)
			switch c.Source {
			case "item":
				if bestItem == nil || c.Percent > bestItem.Percent {
					bestItem = &contributors[i]
				}
			case "aa":
				aaPct += c.Percent
			}
		}
		var pct int
		if bestItem != nil {
			pct += bestItem.Percent
		}
		pct += aaPct

		// Trim the matched list to only the contributors that *actually*
		// applied — i.e. AAs (all summed) plus the single best item.
		var applied []Modifier
		for _, c := range matched {
			if c.Source == "aa" || (bestItem != nil && c.SourceItemID == bestItem.SourceItemID && c.FocusSpellID == bestItem.FocusSpellID) {
				applied = append(applied, c)
			}
		}

		switch spa {
		case SPADuration:
			itemPct := 0
			if bestItem != nil {
				itemPct = bestItem.Percent
			}
			if casterClass == BardClassIdx {
				// Bard songs ignore SPA 128 extensions from AAs and item
				// focus effects — discard the matched contributors so the
				// breakdown UI shows no extension applied.
				r.DurationAAPercent = 0
				r.DurationItemPercent = 0
				break
			}
			if casterClass >= 0 && casterClass < len(spellClassLevels) &&
				spellClassLevels[casterClass] >= classCannotCast {
				// Off-class clicky: the player's class can't cast this spell,
				// so for SPA 134 (Limit: Max Level) purposes its effective
				// level is 255 — above the max-level cap every era item
				// duration focus carries (e.g. the Coldain Prayer Shawl's 65).
				// Item focuses therefore never extend an off-class clicky. AA
				// duration extensions have no class/level limit, so they still
				// apply (Soul Energy from a resist ring on a 60 druid:
				// 19m base × 1.5 AA = 28m30s, with no shawl contribution).
				var aaOnly []Modifier
				for _, c := range applied {
					if c.Source == "aa" {
						aaOnly = append(aaOnly, c)
					}
				}
				r.DurationAAPercent = aaPct
				r.DurationItemPercent = 0
				r.Applied = append(r.Applied, aaOnly...)
				break
			}
			r.DurationAAPercent = aaPct
			r.DurationItemPercent = itemPct
			r.Applied = append(r.Applied, applied...)
		case SPACastTime:
			if pct > SpellHasteCapPercent {
				pct = SpellHasteCapPercent
			}
			r.CastTimePercent = pct
			r.Applied = append(r.Applied, applied...)
		}
	}

	// AA duration applies first, then item focus on top of that.
	extended := baseDurationSec * (100 + r.DurationAAPercent) / 100
	extended = extended * (100 + r.DurationItemPercent) / 100
	r.ExtendedDurationSec = extended
	if baseDurationSec > 0 {
		r.DurationPercent = (extended - baseDurationSec) * 100 / baseDurationSec
	}
	return r
}
