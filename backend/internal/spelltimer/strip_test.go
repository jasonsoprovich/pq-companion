package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// captureStrips wires e's strip handler into a slice.
func captureStrips(e *Engine) *[]StripEvent {
	var got []StripEvent
	e.SetStripHandler(func(ev StripEvent) { got = append(got, ev) })
	return &got
}

func seedMobDebuff(e *Engine, spell, mob string, cat Category, now time.Time, remaining time.Duration) {
	tm := &ActiveTimer{
		ID:         timerKey(spell, mob),
		SpellName:  spell,
		TargetName: mob,
		Category:   cat,
		StartsAt:   now.Add(-time.Minute),
		ExpiresAt:  now.Add(remaining),
	}
	e.timers[tm.ID] = tm
}

func TestBuffStripped(t *testing.T) {
	now := time.Now()
	tick := eqTickDuration
	tests := []struct {
		name  string
		left  time.Duration // remaining when the buff leaves the window
		after []PipeBuff    // the snapshot it leaves in
		want  bool
	}{
		{"dispelled with time left", 10 * time.Minute, []PipeBuff{{Name: "Clarity", Ticks: 50}}, true},
		{"near natural fade", tick + time.Second, []PipeBuff{{Name: "Clarity", Ticks: 50}}, false},
		{"overwritten by a new buff", 10 * time.Minute,
			[]PipeBuff{{Name: "Clarity", Ticks: 50}, {Name: "Spiritual Purity", Ticks: 300}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			got := captureStrips(e)
			seedSelfBuff(e, "Aegolism", now, time.Minute, tc.left)
			both := []PipeBuff{{Name: "Aegolism", Ticks: int(tc.left/tick) + 1}, {Name: "Clarity", Ticks: 50}}
			e.setPipeBuffSlotsAt("Osui", both, now)
			e.setPipeBuffSlotsAt("Osui", tc.after, now.Add(100*time.Millisecond))
			if len(*got) != 0 {
				t.Fatalf("strip before the fade confirmed: %+v", *got)
			}
			e.setPipeBuffSlotsAt("Osui", tc.after, now.Add(pipeFadeConfirm+200*time.Millisecond))
			if (len(*got) == 1) != tc.want {
				t.Fatalf("strips = %+v, want strip %v", *got, tc.want)
			}
			if tc.want && ((*got)[0].Kind != StripBuff || (*got)[0].Spell != "Aegolism") {
				t.Fatalf("strip = %+v", (*got)[0])
			}
		})
	}
}

func TestBuffStrippedIgnoresEmptySnapshot(t *testing.T) {
	now := time.Now()
	e := newTestEngine()
	got := captureStrips(e)
	seedSelfBuff(e, "Aegolism", now, time.Minute, 10*time.Minute)
	e.setPipeBuffSlotsAt("Osui", []PipeBuff{{Name: "Aegolism", Ticks: 100}}, now)
	e.setPipeBuffSlotsAt("Osui", nil, now.Add(time.Second))
	e.setPipeBuffSlotsAt("Osui", nil, now.Add(5*time.Second))
	if len(*got) != 0 {
		t.Fatalf("zone/camp reported as strip: %+v", *got)
	}
}

func TestDebuffStripped(t *testing.T) {
	now := time.Now()
	dispel := logparser.SpellLandedData{TargetName: "a gnoll", SpellID: 48}

	t.Run("single early fade is an overwrite", func(t *testing.T) {
		e := newTestEngine()
		got := captureStrips(e)
		seedMobDebuff(e, "Tashanian", "a gnoll", CategoryDebuff, now, 5*time.Minute)
		e.removeFadedDetrimental("Tashanian", now)
		if len(*got) != 0 {
			t.Fatalf("strips = %+v", *got)
		}
	})

	t.Run("dispel land then fade", func(t *testing.T) {
		e := newTestEngine()
		e.dispelSpells = map[int]bool{48: true}
		got := captureStrips(e)
		seedMobDebuff(e, "Tashanian", "a gnoll", CategoryDebuff, now, 5*time.Minute)
		e.noteSpellLandedForStrip(now, dispel)
		e.removeFadedDetrimental("Tashanian", now.Add(time.Second))
		if len(*got) != 1 || (*got)[0].Kind != StripDebuff || (*got)[0].Target != "a gnoll" {
			t.Fatalf("strips = %+v", *got)
		}
	})

	t.Run("fade then dispel land", func(t *testing.T) {
		e := newTestEngine()
		e.dispelSpells = map[int]bool{48: true}
		got := captureStrips(e)
		seedMobDebuff(e, "Mesmerization", "a gnoll", CategoryMez, now, time.Minute)
		e.removeFadedDetrimental("Mesmerization", now)
		e.noteSpellLandedForStrip(now.Add(2*time.Second), dispel)
		if len(*got) != 1 || (*got)[0].Spell != "Mesmerization" {
			t.Fatalf("strips = %+v", *got)
		}
	})

	t.Run("dispel on another mob", func(t *testing.T) {
		e := newTestEngine()
		e.dispelSpells = map[int]bool{48: true}
		got := captureStrips(e)
		seedMobDebuff(e, "Tashanian", "a gnoll", CategoryDebuff, now, 5*time.Minute)
		e.noteSpellLandedForStrip(now, logparser.SpellLandedData{TargetName: "an orc", SpellID: 48})
		e.removeFadedDetrimental("Tashanian", now)
		if len(*got) != 0 {
			t.Fatalf("strips = %+v", *got)
		}
	})

	t.Run("two early fades on one mob", func(t *testing.T) {
		e := newTestEngine()
		got := captureStrips(e)
		seedMobDebuff(e, "Tashanian", "a gnoll", CategoryDebuff, now, 5*time.Minute)
		seedMobDebuff(e, "Turgur's Insects", "a gnoll", CategoryDebuff, now, 3*time.Minute)
		e.removeFadedDetrimental("Tashanian", now)
		e.removeFadedDetrimental("Turgur's Insects", now.Add(500*time.Millisecond))
		if len(*got) != 2 {
			t.Fatalf("strips = %+v", *got)
		}
	})

	t.Run("two mez breaks are not a dispel", func(t *testing.T) {
		e := newTestEngine()
		got := captureStrips(e)
		seedMobDebuff(e, "Mesmerization", "a gnoll", CategoryMez, now, time.Minute)
		seedMobDebuff(e, "Tashanian", "a gnoll", CategoryDebuff, now, 5*time.Minute)
		e.removeFadedDetrimental("Mesmerization", now)
		e.removeFadedDetrimental("Tashanian", now)
		if len(*got) != 0 {
			t.Fatalf("strips = %+v", *got)
		}
	})

	t.Run("natural expiry", func(t *testing.T) {
		e := newTestEngine()
		e.dispelSpells = map[int]bool{48: true}
		got := captureStrips(e)
		seedMobDebuff(e, "Tashanian", "a gnoll", CategoryDebuff, now, 3*time.Second)
		e.noteSpellLandedForStrip(now, dispel)
		e.removeFadedDetrimental("Tashanian", now)
		if len(*got) != 0 {
			t.Fatalf("strips = %+v", *got)
		}
	})
}
