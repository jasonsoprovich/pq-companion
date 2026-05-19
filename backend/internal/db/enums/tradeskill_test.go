package enums

import "testing"

func TestTradeskillName_KnownIDs(t *testing.T) {
	cases := map[int]string{
		0:  "Common Combine",
		55: "Fishing",
		57: "Tinkering",
		58: "Research",
		61: "Tailoring",
		63: "Blacksmithing",
		69: "Pottery",
		75: "Common Combine",
	}
	for id, want := range cases {
		if got := TradeskillName(id); got != want {
			t.Errorf("TradeskillName(%d) = %q, want %q", id, got, want)
		}
	}
}

func TestTradeskillName_UnknownReturnsEmpty(t *testing.T) {
	if got := TradeskillName(9999); got != "" {
		t.Errorf("TradeskillName(9999) = %q, want empty string", got)
	}
}

func TestSnapshot_TradeskillsIncluded(t *testing.T) {
	snap := Snapshot()
	if len(snap.Tradeskills) == 0 {
		t.Fatal("Snapshot returned empty Tradeskills")
	}
	if snap.Tradeskills[61] != "Tailoring" {
		t.Errorf("Snapshot Tradeskills[61] = %q, want %q", snap.Tradeskills[61], "Tailoring")
	}
}
