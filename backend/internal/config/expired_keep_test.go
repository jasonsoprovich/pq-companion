package config

import (
	"testing"
	"time"
)

func TestSpellTimerSettings_ExpiredKeep(t *testing.T) {
	s := SpellTimerSettings{
		BuffExpiredKeepMins:   30,
		DetrimExpiredKeepMins: 5,
		ExpiredKeepOverrides:  map[string]int{"Quiescence": 1, "Malosinia": 0},
	}
	tests := []struct {
		name        string
		settings    SpellTimerSettings
		spell       string
		detrimental bool
		want        time.Duration
	}{
		{"buff cap", s, "Talisman of Kragg", false, 30 * time.Minute},
		{"detrimental cap", s, "Cripple", true, 5 * time.Minute},
		{"override beats buff cap", s, "Quiescence", false, time.Minute},
		{"override is case-insensitive", s, "quiescence", false, time.Minute},
		{"zero override drops on expiry", s, "Malosinia", true, 0},
		{"unset caps use default", SpellTimerSettings{}, "Cripple", true,
			DefaultExpiredKeepMins * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.ExpiredKeep(tt.spell, tt.detrimental); got != tt.want {
				t.Errorf("ExpiredKeep(%q, %v) = %v, want %v", tt.spell, tt.detrimental, got, tt.want)
			}
		})
	}
}
