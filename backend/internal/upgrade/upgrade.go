// Package upgrade scores candidate gear against a character's currently-worn
// item in a slot, producing a cap-aware "is this actually an upgrade" ranking.
//
// The model is deliberately transparent and matches how players reason about
// gear: each scorable stat carries a weight, and the score of swapping the
// current slot item for a candidate is the weighted sum of the *effective*
// stat deltas. "Effective" is the key word — a stat that is already at its cap
// contributes nothing further, so an item's +20 STR is worth zero when the
// character is already at the 255 attribute cap. HP, mana, and AC are treated
// as uncapped primary value; the seven attributes respect eqstat.MaxStat; the
// five resists respect eqstat.ResistCap.
//
// Focus effects are intentionally NOT scored here — they do not stack (only the
// best focus of a type applies), so summing them into a raw stat score is
// wrong. The API surfaces focus as a separate axis instead.
//
// ATK and worn haste are effect-derived (not flat item columns) and are left
// out of phase-1 scoring; they can be layered on later as additional weights.
package upgrade

import (
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqstat"
)

// StatLine is the flat, scorable stat contribution of a single item, or the
// summed total of a whole loadout. Only stats that exist as flat item columns
// appear here.
type StatLine struct {
	HP   int `json:"hp"`
	Mana int `json:"mana"`
	AC   int `json:"ac"`
	STR  int `json:"str"`
	STA  int `json:"sta"`
	AGI  int `json:"agi"`
	DEX  int `json:"dex"`
	WIS  int `json:"wis"`
	INT  int `json:"int"`
	CHA  int `json:"cha"`
	MR   int `json:"mr"`
	FR   int `json:"fr"`
	CR   int `json:"cr"`
	DR   int `json:"dr"`
	PR   int `json:"pr"`
	// Damage/Delay are the weapon fields used to score weapon ratio (DPS) in
	// the Primary/Secondary/Range slots. Zero for non-weapons (so a shield or
	// book has ratio 0 — swapping a weapon for one is a DPS loss). See Score.
	Damage int `json:"damage"`
	Delay  int `json:"delay"`
	// Attack is the item's worn ATK bonus (SPA 2). It stacks additively; scored
	// with a soft cap (ATKSoftCap) since worn ATK past the practical ~250 target
	// has little marginal value.
	Attack int `json:"attack"`
	// ManaRegen is the item's worn mana regen (Flowing Thought). Additive but
	// item-capped at ItemFTCap (15), so it's scored cap-aware — a top caster
	// priority until capped, then worthless.
	ManaRegen int `json:"mana_regen"`
	// Haste is the item's worn melee-haste percent. Worn haste does NOT stack
	// (only the single highest item applies) and is level-capped, so it's scored
	// as a separate loadout-aware term, not summed. See Score + Context.
	Haste int `json:"haste"`
}

// Scoring caps for the two soft-capped worn stats.
const (
	// ATKSoftCap is the practical worn-attack ceiling (the BiS guides target
	// ~250); worn ATK past it scores nothing.
	ATKSoftCap = 250
	// ItemFTCap is EQ's worn mana-regen (Flowing Thought) item cap.
	ItemFTCap = 15
)

// ratio is a weapon's damage/delay — the standard EQ DPS-comparison metric
// (higher is better). Zero for anything without both a damage and a delay.
func ratio(s StatLine) float64 {
	if s.Damage <= 0 || s.Delay <= 0 {
		return 0
	}
	return float64(s.Damage) / float64(s.Delay)
}

// Weights assigns a per-stat scoring weight. Values are on an arbitrary common
// axis (defaults use an HP-equivalent scale where wHP = 1.0), so "1 AC = 5 HP"
// is simply wAC = 5 with wHP = 1. Editing is per character; DefaultWeights
// seeds sensible per-class starting points.
type Weights struct {
	HP   float64 `json:"hp"`
	Mana float64 `json:"mana"`
	AC   float64 `json:"ac"`
	STR  float64 `json:"str"`
	STA  float64 `json:"sta"`
	AGI  float64 `json:"agi"`
	DEX  float64 `json:"dex"`
	WIS  float64 `json:"wis"`
	INT  float64 `json:"int"`
	CHA  float64 `json:"cha"`
	MR   float64 `json:"mr"`
	FR   float64 `json:"fr"`
	CR   float64 `json:"cr"`
	DR   float64 `json:"dr"`
	PR   float64 `json:"pr"`
	// ATK weights the worn attack bonus (soft-capped at ATKSoftCap) — meaningful
	// for melee/tank, zero for casters.
	ATK float64 `json:"atk"`
	// ManaRegen weights worn mana regen (Flowing Thought, cap-aware at
	// ItemFTCap). A top-tier caster priority; small for tank-casters; 0 for
	// pure melee.
	ManaRegen float64 `json:"mana_regen"`
	// Haste weights worn melee-haste, scored per effective %-point gained toward
	// the level cap. Worn haste doesn't stack, so reaching the cap is a top
	// melee/tank priority and extra haste past it (or beyond the best item) is
	// worth nothing — a high weight here pushes "cap your haste first". Zero for
	// casters (melee haste doesn't help them).
	Haste float64 `json:"haste"`
	// DPS weights weapon ratio (damage/delay) in weapon slots. It's the score
	// per +1.0 of ratio, so a melee DPS class values an offhand/main-hand weapon
	// far above a shield or stat-stick, while a tank (low DPS, high AC) still
	// prefers a shield offhand. Zero for casters. Applied in Score.
	DPS float64 `json:"dps"`
	// FocusBonus is the score bump (on the same HP-equivalent axis) given to a
	// candidate that carries one of the character's priority focus effects which
	// they don't already have equipped. Applied by the API layer, not Score —
	// focus is a separate axis from raw stats. See DefaultFocusBonus.
	FocusBonus float64 `json:"focus_bonus"`
}

// DefaultFocusBonus is the fallback priority-focus bonus when a weight set
// doesn't specify one (e.g. profiles saved before the field existed). On the
// HP-equivalent scale, ~100 means a wanted, not-yet-equipped focus is worth
// about 100 HP of ranking — enough to float a focus item over a modest stat
// upgrade without burying large ones.
const DefaultFocusBonus = 100

// Context carries the character state needed for cap-aware scoring.
type Context struct {
	// Level drives the attribute cap (eqstat.MaxStat).
	Level int
	// CapMod is an AA stat-cap raise (SE_RaiseStatCap); 0 for the common case.
	CapMod int
	// Current is the character's full current attribute/resist totals
	// (base + all worn items + AA + buffs). It is the reference for how much
	// headroom remains under each cap. HP/mana/AC are not read from here.
	Current StatLine
	// OtherHaste is the best worn melee-haste percent the character has equipped
	// in OTHER slots (excluding the slot being evaluated). Worn haste is
	// best-of-type, so a candidate's haste only helps if it beats this, and only
	// up to the level cap (eqstat.MeleeHasteCap(Level)).
	OtherHaste int
}

// statKind classifies how a stat's effective value is computed.
type statKind int

const (
	uncapped   statKind = iota // HP, mana, AC — full value, no cap
	attrCapped                 // the seven attributes — eqstat.MaxStat
	resistCap                  // the five resists — eqstat.ResistCap
	atkCapped                  // worn ATK — soft cap ATKSoftCap
	ftCapped                   // worn mana regen — item cap ItemFTCap
)

// statDef describes one scorable stat: how to read it from a StatLine, its
// weight from Weights, and its cap behaviour.
type statDef struct {
	Key  string
	get  func(StatLine) int
	wget func(Weights) float64
	kind statKind
}

// statDefs is the canonical ordered list of scorable stats. The order is the
// display order used by the API/UI.
var statDefs = []statDef{
	{"hp", func(s StatLine) int { return s.HP }, func(w Weights) float64 { return w.HP }, uncapped},
	{"mana", func(s StatLine) int { return s.Mana }, func(w Weights) float64 { return w.Mana }, uncapped},
	{"mana_regen", func(s StatLine) int { return s.ManaRegen }, func(w Weights) float64 { return w.ManaRegen }, ftCapped},
	{"atk", func(s StatLine) int { return s.Attack }, func(w Weights) float64 { return w.ATK }, atkCapped},
	{"ac", func(s StatLine) int { return s.AC }, func(w Weights) float64 { return w.AC }, uncapped},
	{"str", func(s StatLine) int { return s.STR }, func(w Weights) float64 { return w.STR }, attrCapped},
	{"sta", func(s StatLine) int { return s.STA }, func(w Weights) float64 { return w.STA }, attrCapped},
	{"agi", func(s StatLine) int { return s.AGI }, func(w Weights) float64 { return w.AGI }, attrCapped},
	{"dex", func(s StatLine) int { return s.DEX }, func(w Weights) float64 { return w.DEX }, attrCapped},
	{"wis", func(s StatLine) int { return s.WIS }, func(w Weights) float64 { return w.WIS }, attrCapped},
	{"int", func(s StatLine) int { return s.INT }, func(w Weights) float64 { return w.INT }, attrCapped},
	{"cha", func(s StatLine) int { return s.CHA }, func(w Weights) float64 { return w.CHA }, attrCapped},
	{"mr", func(s StatLine) int { return s.MR }, func(w Weights) float64 { return w.MR }, resistCap},
	{"fr", func(s StatLine) int { return s.FR }, func(w Weights) float64 { return w.FR }, resistCap},
	{"cr", func(s StatLine) int { return s.CR }, func(w Weights) float64 { return w.CR }, resistCap},
	{"dr", func(s StatLine) int { return s.DR }, func(w Weights) float64 { return w.DR }, resistCap},
	{"pr", func(s StatLine) int { return s.PR }, func(w Weights) float64 { return w.PR }, resistCap},
}

// StatDelta is the per-stat effective comparison between a candidate item and
// the item currently in the slot.
type StatDelta struct {
	Stat string `json:"stat"`
	// Cand and Current are the raw stat values on each item.
	Cand    int `json:"cand"`
	Current int `json:"current"`
	// Effective is the cap-aware delta that actually feeds the score — it can
	// be smaller than Cand-Current when the stat is at/over its cap.
	Effective int     `json:"effective"`
	Weight    float64 `json:"weight"`
	Weighted  float64 `json:"weighted"`
	// Capped flags a stat whose effective value was clipped by a cap, so the
	// UI can explain why a "better" number scored less than expected.
	Capped bool `json:"capped"`
}

// Result is the scored comparison of one candidate against the current item.
type Result struct {
	Score  float64     `json:"score"`
	Deltas []StatDelta `json:"deltas"`
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// effective returns how much of itemVal does useful work, given the loadout's
// value for this stat excluding the slot being compared (base).
func (d statDef) effective(ctx Context, base, itemVal int) (eff int, capped bool) {
	switch d.kind {
	case attrCapped:
		cap := eqstat.MaxStat(ctx.Level, ctx.CapMod)
		eff = clamp(base+itemVal, 0, cap) - clamp(base, 0, cap)
	case resistCap:
		cap := eqstat.ResistCap
		eff = clamp(base+itemVal, 0, cap) - clamp(base, 0, cap)
	case atkCapped:
		eff = clamp(base+itemVal, 0, ATKSoftCap) - clamp(base, 0, ATKSoftCap)
	case ftCapped:
		eff = clamp(base+itemVal, 0, ItemFTCap) - clamp(base, 0, ItemFTCap)
	default: // uncapped
		return itemVal, false
	}
	return eff, eff != itemVal
}

// Score computes the weighted marginal value of replacing the slot's current
// item (slotCur) with a candidate (cand). A positive score means the candidate
// is a net upgrade under the given weights. Per-stat deltas are returned for
// display; stats with zero weight and zero effect are omitted.
func Score(ctx Context, w Weights, slotCur, cand StatLine) Result {
	res := Result{Deltas: make([]StatDelta, 0, len(statDefs))}
	for _, d := range statDefs {
		curVal := d.get(slotCur)
		candVal := d.get(cand)
		weight := d.wget(w)

		// base = this stat across the whole loadout EXCEPT the slot item, so
		// the candidate is compared apples-to-apples against what it replaces.
		base := d.get(ctx.Current) - curVal

		effCur, _ := d.effective(ctx, base, curVal)
		effCand, candCapped := d.effective(ctx, base, candVal)
		eff := effCand - effCur
		if eff == 0 && curVal == candVal {
			continue // nothing to show for this stat
		}
		weighted := weight * float64(eff)
		res.Score += weighted
		res.Deltas = append(res.Deltas, StatDelta{
			Stat:      d.Key,
			Cand:      candVal,
			Current:   curVal,
			Effective: eff,
			Weight:    weight,
			Weighted:  weighted,
			Capped:    candCapped && candVal > curVal,
		})
	}

	// Worn melee haste — best-of-type and level-capped, so it's scored on the
	// EFFECTIVE percent gained: the candidate only helps if it raises the
	// character's best haste (vs other slots) toward the cap. Reaching the cap
	// is a top melee priority; past it, or below your existing best, a haste
	// item adds nothing (and swapping away your only haste source is a loss).
	if w.Haste != 0 {
		cap := eqstat.MeleeHasteCap(ctx.Level)
		curEff := clamp(maxInt(ctx.OtherHaste, slotCur.Haste), 0, cap)
		candEff := clamp(maxInt(ctx.OtherHaste, cand.Haste), 0, cap)
		eff := candEff - curEff
		if eff != 0 {
			weighted := w.Haste * float64(eff)
			res.Score += weighted
			res.Deltas = append(res.Deltas, StatDelta{
				Stat:      "haste",
				Cand:      cand.Haste,
				Current:   slotCur.Haste,
				Effective: eff,
				Weight:    w.Haste,
				Weighted:  weighted,
				Capped:    cand.Haste > slotCur.Haste && candEff == cap && maxInt(ctx.OtherHaste, cand.Haste) > cap,
			})
		}
	}

	// Weapon ratio (DPS) — only meaningful in weapon slots, where it's the
	// dominant value for a melee class. Scored as a delta vs the worn weapon, so
	// swapping a weapon for a shield/stat-stick (ratio 0) is a DPS loss. Ratios
	// are carried at x100 in the int delta fields for display (e.g. 185 = 1.85);
	// Weighted uses the true ratio delta so it matches the score contribution.
	if w.DPS != 0 {
		curR, candR := ratio(slotCur), ratio(cand)
		if curR != 0 || candR != 0 {
			deltaR := candR - curR
			weighted := w.DPS * deltaR
			res.Score += weighted
			res.Deltas = append(res.Deltas, StatDelta{
				Stat:      "dps",
				Cand:      int(candR*100 + 0.5),
				Current:   int(curR*100 + 0.5),
				Effective: int(deltaR*100 + 0.5*sign(deltaR)),
				Weight:    w.DPS,
				Weighted:  weighted,
			})
		}
	}
	return res
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
