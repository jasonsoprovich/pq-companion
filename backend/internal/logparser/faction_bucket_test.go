package logparser

import "testing"

func TestFactionBucketForLevel(t *testing.T) {
	want := map[int]FactionBucket{
		0:  "",
		1:  BucketAlly,
		2:  BucketWarmly,
		3:  BucketKindly,
		4:  BucketAmiable,
		5:  BucketIndifferent,
		6:  BucketApprehensive,
		7:  BucketDubious,
		8:  BucketThreatening,
		9:  BucketScowling,
		10: "",
		-1: "",
	}
	for level, w := range want {
		if got := FactionBucketForLevel(level); got != w {
			t.Errorf("FactionBucketForLevel(%d) = %q, want %q", level, got, w)
		}
		if w != "" && FactionLevel(w) != level {
			t.Errorf("FactionLevel(%q) = %d, want %d", w, FactionLevel(w), level)
		}
	}
}

func TestFactionLevelUnknown(t *testing.T) {
	if got := FactionLevel("glares"); got != 0 {
		t.Errorf("FactionLevel(unknown) = %d, want 0", got)
	}
}
