package charm

import (
	"math"
	"testing"
)

func TestScaledHP(t *testing.T) {
	tests := []struct {
		name                     string
		baseHP, baseLevel, level int
		want                     int
	}{
		// Verified against live Quarm range spawns.
		{"ravenous beast top of range", 13200, 52, 56, 14215},
		{"spectral wolf top of range", 10400, 49, 53, 11249},
		{"at base level is unchanged", 13200, 52, 52, 13200},
		{"zero base level falls back to base hp", 5000, 0, 50, 5000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScaledHP(tt.baseHP, tt.baseLevel, tt.level); got != tt.want {
				t.Errorf("ScaledHP(%d,%d,%d) = %d, want %d", tt.baseHP, tt.baseLevel, tt.level, got, tt.want)
			}
		})
	}
}

func TestScaledMaxHit(t *testing.T) {
	tests := []struct {
		name                         string
		baseMaxDmg, baseLevel, level int
		want                         int
	}{
		{"ravenous beast top of range", 149, 52, 56, 157},
		{"spectral wolf top of range", 137, 49, 53, 145},
		{"at base level is unchanged", 149, 52, 52, 149},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScaledMaxHit(tt.baseMaxDmg, tt.baseLevel, tt.level); got != tt.want {
				t.Errorf("ScaledMaxHit(%d,%d,%d) = %d, want %d", tt.baseMaxDmg, tt.baseLevel, tt.level, got, tt.want)
			}
		})
	}
}

func TestDPS(t *testing.T) {
	tests := []struct {
		name                        string
		minDmg, maxDmg, attackDelay int
		want                        float64
	}{
		// Verified against live Quarm spawns (rounded to 1 decimal for display).
		{"forager", 51, 154, 19, 53.9},
		{"herdmaster", 48, 149, 20, 49.3},
		{"bandit lookout", 46, 145, 20, 47.8},
		{"spectral wolf base", 42, 137, 21, 42.6},
		{"spectral wolf top", 42, 145, 21, 44.5},
		{"zero delay is zero dps", 50, 100, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := math.Round(DPS(tt.minDmg, tt.maxDmg, tt.attackDelay)*10) / 10
			if got != tt.want {
				t.Errorf("DPS(%d,%d,%d) = %.1f, want %.1f", tt.minDmg, tt.maxDmg, tt.attackDelay, got, tt.want)
			}
		})
	}
}

func TestBodyTypeAllowed(t *testing.T) {
	tests := []struct {
		name     string
		r        BodyRestriction
		bodytype int
		want     bool
	}{
		{"unrestricted allows undead", RestrictNone, bodyUndead, true},
		{"unrestricted allows animal", RestrictNone, bodyAnimal, true},
		{"animal allows animal", RestrictAnimal, bodyAnimal, true},
		{"animal rejects undead", RestrictAnimal, bodyUndead, false},
		{"animal rejects humanoid", RestrictAnimal, 1, false},
		{"undead allows undead", RestrictUndead, bodyUndead, true},
		{"undead allows summoned undead", RestrictUndead, bodySummonedUndead, true},
		{"undead rejects animal", RestrictUndead, bodyAnimal, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BodyTypeAllowed(tt.r, tt.bodytype); got != tt.want {
				t.Errorf("BodyTypeAllowed(%v,%d) = %v, want %v", tt.r, tt.bodytype, got, tt.want)
			}
		})
	}
}

func TestRestrictionForTargetType(t *testing.T) {
	tests := []struct {
		tt   int
		want BodyRestriction
	}{
		{5, RestrictNone},
		{9, RestrictAnimal},
		{10, RestrictUndead},
		{0, RestrictNone},
	}
	for _, tt := range tests {
		if got := RestrictionForTargetType(tt.tt); got != tt.want {
			t.Errorf("RestrictionForTargetType(%d) = %v, want %v", tt.tt, got, tt.want)
		}
	}
}
