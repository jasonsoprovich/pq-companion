package enums

import "testing"

func TestSpecialAbilityName_KnownCodes(t *testing.T) {
	cases := map[int]string{
		1:                       "Summon",
		18:                      "Immune to Dispel",
		54:                      "Proximity Aggro 2",
		SyntheticSeeInvis:       "See Invis",
		SyntheticSeeInvisUndead: "See Invis vs Undead",
	}
	for code, want := range cases {
		if got := SpecialAbilityName(code); got != want {
			t.Errorf("SpecialAbilityName(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestSpecialAbilityName_UnknownReturnsEmpty(t *testing.T) {
	if got := SpecialAbilityName(9999); got != "" {
		t.Errorf("SpecialAbilityName(9999) = %q, want empty string", got)
	}
}

func TestSpecialAbilities_AllCanonicalCodesPresent(t *testing.T) {
	for code := 1; code <= 54; code++ {
		if _, ok := specialAbilities[code]; !ok {
			t.Errorf("SpecialAbility code %d missing from map (EQMacEmu codes 1–54 must all be present)", code)
		}
	}
}

func TestSpecialAbilities_DescriptionsNonEmpty(t *testing.T) {
	for code, meta := range specialAbilities {
		if meta.Name == "" {
			t.Errorf("SpecialAbility code %d has empty Name", code)
		}
		if meta.Description == "" {
			t.Errorf("SpecialAbility code %d (%s) has empty Description", code, meta.Name)
		}
	}
}

func TestSnapshot_SpecialAbilitiesIncluded(t *testing.T) {
	snap := Snapshot()
	if len(snap.SpecialAbilities) == 0 {
		t.Fatal("Snapshot returned empty SpecialAbilities")
	}
	if snap.SpecialAbilities[1].Name != "Summon" {
		t.Errorf("Snapshot SpecialAbilities[1].Name = %q, want %q", snap.SpecialAbilities[1].Name, "Summon")
	}
}
