package spelltimer

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Someone else's land whose text a whole spell line shares resolves to the
// line's highest-level rank when every rank lasts the same, and is still
// dropped when the ranks' durations differ.
func TestResolveLandedSpellName_SameDurationFamily(t *testing.T) {
	e := charmEngine(t)
	tests := []struct {
		name  string
		cands []logparser.SpellLandedCandidate
		want  string
	}{
		{"tash line", []logparser.SpellLandedCandidate{
			{SpellID: 676, SpellName: "Tashan"}, {SpellID: 677, SpellName: "Tashani"},
			{SpellID: 678, SpellName: "Tashania"}, {SpellID: 1699, SpellName: "Wind of Tashani"},
			{SpellID: 1702, SpellName: "Tashanian"}, {SpellID: 1704, SpellName: "Wind of Tashanian"},
			{SpellID: 3342, SpellName: "Howl of Tashan"},
		}, "Howl of Tashan"},
		{"ro line", []logparser.SpellLandedCandidate{
			{SpellID: 1436, SpellName: "Fixation of Ro"},
			{SpellID: 2518, SpellName: "Ro's Smoldering Disjunction"},
			{SpellID: 3440, SpellName: "Ro's Illumination"},
		}, "Ro's Illumination"},
		{"malo ranks differ", []logparser.SpellLandedCandidate{
			{SpellID: 112, SpellName: "Malosi"}, {SpellID: 1577, SpellName: "Malosini"},
			{SpellID: 1578, SpellName: "Malo"}, {SpellID: 1772, SpellName: "Mala"},
			{SpellID: 3387, SpellName: "Malosinia"}, {SpellID: 3395, SpellName: "Malos"},
		}, ""},
		{"yawns slows differ", []logparser.SpellLandedCandidate{
			{SpellID: 270, SpellName: "Drowsy"}, {SpellID: 1588, SpellName: "Turgur's Insects"},
			{SpellID: 1589, SpellName: "Tigir's Insects"}, {SpellID: 1960, SpellName: "Energy Sap"},
		}, ""},
	}
	for _, tt := range tests {
		data := logparser.SpellLandedData{
			Kind: logparser.SpellLandedKindOther, TargetName: "a Diaku Armsman", Candidates: tt.cands,
		}
		if got := e.resolveLandedSpellName(data); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
