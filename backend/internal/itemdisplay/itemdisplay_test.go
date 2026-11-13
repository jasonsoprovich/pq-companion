package itemdisplay

import (
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

func TestTracker(t *testing.T) {
	tests := []struct {
		name        string
		simulate    bool
		wantSeen    bool
		wantSimFlag bool
	}{
		{name: "pipe event marks Zeal as supporting items", simulate: false, wantSeen: true},
		{name: "simulated event leaves the flag alone", simulate: true, wantSeen: false, wantSimFlag: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			tr := NewTracker(ws.NewHub(), mgr)
			if tr.Last() != nil {
				t.Fatal("Last before any event: want nil")
			}

			if tc.simulate {
				tr.Simulate(13073, " Bone Chips ", "Tester")
			} else {
				tr.Handle(13073, " Bone Chips ", "Tester")
			}

			got := tr.Last()
			if got == nil || got.ItemID != 13073 || got.Name != "Bone Chips" || got.Character != "Tester" {
				t.Fatalf("Last = %+v, want item 13073 Bone Chips for Tester", got)
			}
			if got.Simulated != tc.wantSimFlag {
				t.Errorf("Simulated = %v, want %v", got.Simulated, tc.wantSimFlag)
			}
			if seen := mgr.Get().Preferences.ZealItemDisplaySeen; seen != tc.wantSeen {
				t.Errorf("ZealItemDisplaySeen = %v, want %v", seen, tc.wantSeen)
			}
		})
	}
}

func TestTrackerClear(t *testing.T) {
	mgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	tr := NewTracker(ws.NewHub(), mgr)

	tr.Clear()
	if tr.Last() != nil {
		t.Fatal("Last after Clear with no item: want nil")
	}

	tr.Handle(13073, "Bone Chips", "Tester")
	tr.Clear()
	if got := tr.Last(); got != nil {
		t.Fatalf("Last after Clear = %+v, want nil", got)
	}
	if !mgr.Get().Preferences.ZealItemDisplaySeen {
		t.Error("Clear reset ZealItemDisplaySeen; it should stay set")
	}

	tr.Handle(5013, "Rusty Short Sword", "Tester")
	if got := tr.Last(); got == nil || got.ItemID != 5013 {
		t.Fatalf("Last after a new item = %+v, want item 5013", got)
	}
}
