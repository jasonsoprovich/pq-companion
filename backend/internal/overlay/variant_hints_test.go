package overlay

import (
	"reflect"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// Two Rathe Councilman rows identical but for mez immunity get labels that
// say which is which; an immunity both share isn't mentioned.
func TestVariantHints_Immunities(t *testing.T) {
	abil := func(codes ...int) []db.SpecialAbility {
		out := make([]db.SpecialAbility, len(codes))
		for i, c := range codes {
			out[i] = db.SpecialAbility{Code: c, Value: 1}
		}
		return out
	}
	out := []TargetVariant{
		{NPC: db.NPC{ID: 222003, Size: 15}, SpecialAbilities: abil(14, 31)},
		{NPC: db.NPC{ID: 222039, Size: 15}, SpecialAbilities: abil(13, 14, 31)},
	}
	traits := map[int]db.VariantTraits{222003: {}, 222039: {}}
	if got := variantHints(0, out, traits); !reflect.DeepEqual(got, []string{"mezzable"}) {
		t.Errorf("mezzable row: got %v", got)
	}
	if got := variantHints(1, out, traits); !reflect.DeepEqual(got, []string{"mez immune"}) {
		t.Errorf("mez-immune row: got %v", got)
	}
}
