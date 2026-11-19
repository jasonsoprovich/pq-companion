package overlay

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// hintAbilities are the special-ability codes worth calling out when sibling
// rows disagree on them — the ones a player changes tactics for. Codes follow
// the EQMacEmu numbering documented in CLAUDE.md.
var hintAbilities = map[int]string{
	1: "summons",
	2: "enrages",
	3: "rampages",
	4: "area rampages",
	5: "flurries",
}

// hintImmunities are the crowd-control immunities worth calling out when
// sibling rows disagree, as {immune, not immune} labels. Same-name rows that
// differ only here are real: the Rathe Council in poearthb alternates
// mezzable and mez-immune councilmen with identical stats and models.
var hintImmunities = []struct {
	code      int
	immune    string
	notImmune string
}{
	{13, "mez immune", "mezzable"},
	{12, "slow immune", "slowable"},
	{15, "stun immune", "stunnable"},
	{14, "charm immune", "charmable"},
	{16, "snare immune", "snareable"},
	{17, "fear immune", "fearable"},
	{31, "pacify immune", "pacifiable"},
}

var hintGenders = map[int]string{0: "male", 1: "female", 2: "neuter"}

// annotateVariants fills SpawnChance and Distinguish on a variant set so the
// overlay can label rows that otherwise read identically (same class, level
// and HP — e.g. the two A Valiant Spirit rows). Hints only describe what
// differs between the candidates; a trait shared by all of them is omitted.
// Best effort: a lookup failure leaves the variants unannotated.
func (t *NPCTracker) annotateVariants(out []TargetVariant, zoneShort string) {
	if t.db == nil || len(out) < 2 {
		return
	}
	ids := make([]int, len(out))
	for i, v := range out {
		ids[i] = v.NPC.ID
	}
	traits, err := t.db.GetVariantTraits(ids, zoneShort)
	if err != nil {
		slog.Debug("overlay: variant traits lookup failed", "err", err)
		return
	}
	for i := range out {
		if tr, ok := traits[out[i].NPC.ID]; ok && tr.SpawnChance >= 0 {
			c := tr.SpawnChance
			out[i].SpawnChance = &c
		}
		out[i].Distinguish = variantHints(i, out, traits)
	}
}

// variantHints lists what sets out[idx] apart from its siblings.
func variantHints(idx int, out []TargetVariant, traits map[int]db.VariantTraits) []string {
	me := out[idx]
	myT := traits[me.NPC.ID]
	var hints []string

	differs := func(same func(o TargetVariant) bool) bool {
		for j, o := range out {
			if j != idx && !same(o) {
				return true
			}
		}
		return false
	}

	if differs(func(o TargetVariant) bool { return traits[o.NPC.ID].Texture == myT.Texture }) {
		hints = append(hints, fmt.Sprintf("texture %d", myT.Texture))
	}
	if differs(func(o TargetVariant) bool { return traits[o.NPC.ID].Gender == myT.Gender }) {
		if g, ok := hintGenders[myT.Gender]; ok {
			hints = append(hints, g)
		}
	}
	if differs(func(o TargetVariant) bool { return math.Abs(o.NPC.Size-me.NPC.Size) < 0.5 }) {
		hints = append(hints, fmt.Sprintf("size %.0f", me.NPC.Size))
	}

	has := func(v TargetVariant, code int) bool {
		for _, a := range v.SpecialAbilities {
			if a.Code == code && a.Value != 0 {
				return true
			}
		}
		return false
	}
	for _, code := range []int{1, 2, 3, 4, 5} {
		mine := has(me, code)
		if differs(func(o TargetVariant) bool { return has(o, code) == mine }) {
			if mine {
				hints = append(hints, hintAbilities[code])
			} else {
				hints = append(hints, "no "+trimPlural(hintAbilities[code]))
			}
		}
	}
	for _, imm := range hintImmunities {
		mine := has(me, imm.code)
		if differs(func(o TargetVariant) bool { return has(o, imm.code) == mine }) {
			if mine {
				hints = append(hints, imm.immune)
			} else {
				hints = append(hints, imm.notImmune)
			}
		}
	}
	return hints
}

// trimPlural turns "summons" into "summon" for the "no summon" phrasing.
func trimPlural(s string) string {
	if len(s) > 1 && s[len(s)-1] == 's' {
		return s[:len(s)-1]
	}
	return s
}

// fillAppearance loads npc_types.texture/gender onto each candidate so
// narrowByAppearance can compare them with the live target. Best effort:
// without it candidates keep zero values and the appearance narrowing is a
// no-op for rows it can't tell apart.
func (t *NPCTracker) fillAppearance(vs []db.NPCVariant, zoneShort string) {
	if t.db == nil || len(vs) < 2 {
		return
	}
	ids := make([]int, len(vs))
	for i, v := range vs {
		ids[i] = v.NPC.ID
	}
	traits, err := t.db.GetVariantTraits(ids, zoneShort)
	if err != nil {
		slog.Debug("overlay: appearance lookup failed", "err", err)
		return
	}
	for i := range vs {
		if tr, ok := traits[vs[i].NPC.ID]; ok {
			vs[i].Texture, vs[i].Gender = tr.Texture, tr.Gender
		}
	}
}

// appearanceSizeTolerance absorbs float noise between the client's model
// height and npc_types.size.
const appearanceSizeTolerance = 0.5

// narrowByAppearance keeps the candidates whose texture, then gender, then
// size match the live target. Each step applies only if it leaves at least one
// candidate, so a client value that doesn't line up with the DB column
// (illusions, server-side scaling) can never empty the set. A size of 0 in
// the DB means "race default", so size is not compared for such rows.
func narrowByAppearance(vs []db.NPCVariant, d *zealpipe.TargetDescriptors) []db.NPCVariant {
	steps := []func(v db.NPCVariant) bool{
		func(v db.NPCVariant) bool { return v.Texture == d.Texture },
		func(v db.NPCVariant) bool { return v.Gender == d.Gender },
		func(v db.NPCVariant) bool {
			return v.NPC.Size == 0 || math.Abs(v.NPC.Size-d.Size) <= appearanceSizeTolerance
		},
	}
	for _, keep := range steps {
		if len(vs) < 2 {
			break
		}
		next := make([]db.NPCVariant, 0, len(vs))
		for _, v := range vs {
			if keep(v) {
				next = append(next, v)
			}
		}
		if len(next) > 0 {
			vs = next
		}
	}
	return vs
}
