package spelltimer

import (
	"testing"
	"time"
)

func TestIsMountName(t *testing.T) {
	for name, want := range map[string]bool{
		"Blesko`s Mount":  true,
		"Blesko`s_Mount":  true,
		"Vilya's mount":   true,
		"a gnoll":         false,
		"Mountain giant":  false,
		"Blesko`s warder": false,
	} {
		if got := isMountName(name); got != want {
			t.Errorf("isMountName(%q) = %v, want %v", name, got, want)
		}
	}
}

// A trigger-driven detrimental aimed at a loose mount starts no timer; a
// buff on it (nobody would, but) and a debuff on a real mob still do.
func TestStartExternalSkipsDetrimentalOnMount(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	if e.StartExternalWithPolicy(false, false, "Howl of Tashan", "debuff", 60, 0, now, nil, 0, "Blesko`s Mount", "", false, "", false) {
		t.Error("debuff on a mount started a timer")
	}
	if !e.StartExternalWithPolicy(false, false, "Howl of Tashan", "debuff", 60, 0, now, nil, 0, "a gnoll", "", false, "", false) {
		t.Error("debuff on a gnoll should start a timer")
	}
}
