package logparser

import (
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	// Reference timestamp used across cases.
	wantTS, _ := time.Parse(tsLayout, "[Mon Apr 13 06:00:00 2026]")
	wantTSSingle, _ := time.Parse(tsLayout, "[Mon Apr  3 06:00:00 2026]")

	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantType EventType
		wantData interface{}
		wantTS   time.Time
	}{
		// --- Timestamp variations ---
		{
			name:     "two-digit day",
			line:     "[Mon Apr 13 06:00:00 2026] You have entered The North Karana.",
			wantOK:   true,
			wantType: EventZone,
			wantData: ZoneData{ZoneName: "The North Karana"},
			wantTS:   wantTS,
		},
		{
			name:     "single-digit day space-padded",
			line:     "[Mon Apr  3 06:00:00 2026] You have entered The North Karana.",
			wantOK:   true,
			wantType: EventZone,
			wantData: ZoneData{ZoneName: "The North Karana"},
			wantTS:   wantTSSingle,
		},
		{
			name:   "invalid timestamp",
			line:   "not a log line",
			wantOK: false,
		},
		{
			name:   "empty line",
			line:   "",
			wantOK: false,
		},
		{
			name:   "timestamp only no message",
			line:   "[Mon Apr 13 06:00:00 2026]",
			wantOK: false,
		},

		// --- Zone change ---
		{
			name:     "zone: multi-word zone name",
			line:     "[Mon Apr 13 06:00:00 2026] You have entered The Plane of Knowledge.",
			wantOK:   true,
			wantType: EventZone,
			wantData: ZoneData{ZoneName: "The Plane of Knowledge"},
		},
		{
			name:     "zone: single-word zone name",
			line:     "[Mon Apr 13 06:00:00 2026] You have entered Crushbone.",
			wantOK:   true,
			wantType: EventZone,
			wantData: ZoneData{ZoneName: "Crushbone"},
		},

		// --- Spell cast ---
		{
			name:     "spell: begin casting",
			line:     "[Mon Apr 13 06:00:00 2026] You begin casting Mesmerization.",
			wantOK:   true,
			wantType: EventSpellCast,
			wantData: SpellCastData{SpellName: "Mesmerization"},
		},
		{
			name:     "spell: begin casting multi-word",
			line:     "[Mon Apr 13 06:00:00 2026] You begin casting Color Flux.",
			wantOK:   true,
			wantType: EventSpellCast,
			wantData: SpellCastData{SpellName: "Color Flux"},
		},

		// --- Spell interrupt ---
		{
			name:     "spell: interrupted generic",
			line:     "[Mon Apr 13 06:00:00 2026] Your spell is interrupted.",
			wantOK:   true,
			wantType: EventSpellInterrupt,
			wantData: SpellInterruptData{},
		},
		{
			name:     "spell: interrupted named",
			line:     "[Mon Apr 13 06:00:00 2026] Your Mesmerization spell is interrupted.",
			wantOK:   true,
			wantType: EventSpellInterrupt,
			wantData: SpellInterruptData{SpellName: "Mesmerization"},
		},

		// --- Spell resist ---
		{
			name:     "spell: resist",
			line:     "[Mon Apr 13 06:00:00 2026] Your target resisted the Mesmerization spell.",
			wantOK:   true,
			wantType: EventSpellResist,
			wantData: SpellResistData{SpellName: "Mesmerization"},
		},
		{
			name:     "spell: resist multi-word",
			line:     "[Mon Apr 13 06:00:00 2026] Your target resisted the Color Flux spell.",
			wantOK:   true,
			wantType: EventSpellResist,
			wantData: SpellResistData{SpellName: "Color Flux"},
		},

		// --- Spell fade ---
		{
			name:     "spell: fade",
			line:     "[Mon Apr 13 06:00:00 2026] Your Mesmerization spell has worn off.",
			wantOK:   true,
			wantType: EventSpellFade,
			wantData: SpellFadeData{SpellName: "Mesmerization"},
		},
		{
			name:     "spell: fade multi-word",
			line:     "[Mon Apr 13 06:00:00 2026] Your Color Flux spell has worn off.",
			wantOK:   true,
			wantType: EventSpellFade,
			wantData: SpellFadeData{SpellName: "Color Flux"},
		},

		// --- Combat: player hits NPC ---
		{
			name:     "combat: you slash NPC",
			line:     "[Mon Apr 13 06:00:00 2026] You slash a gnoll for 150 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "slash", Target: "a gnoll", Damage: 150},
		},
		{
			name:     "combat: you bash NPC one point",
			line:     "[Mon Apr 13 06:00:00 2026] You bash a kobold for 1 point of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "bash", Target: "a kobold", Damage: 1},
		},
		{
			name:     "combat: you hit multi-word NPC",
			line:     "[Mon Apr 13 06:00:00 2026] You pierce a young gnoll for 45 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "pierce", Target: "a young gnoll", Damage: 45},
		},

		// --- Combat: NPC hits player ---
		{
			name:     "combat: NPC hits you",
			line:     "[Mon Apr 13 06:00:00 2026] A gnoll slashes you for 50 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "A gnoll", Skill: "slashes", Target: "You", Damage: 50},
		},

		// --- Combat: misses ---
		{
			name:     "combat: you miss NPC",
			line:     "[Mon Apr 13 06:00:00 2026] You try to slash a gnoll, but miss!",
			wantOK:   true,
			wantType: EventCombatMiss,
			wantData: CombatMissData{Actor: "You", Target: "a gnoll", MissType: "miss"},
		},
		{
			name:     "combat: NPC misses you",
			line:     "[Mon Apr 13 06:00:00 2026] A gnoll tries to slash you, but misses!",
			wantOK:   true,
			wantType: EventCombatMiss,
			wantData: CombatMissData{Actor: "A gnoll", Target: "You", MissType: "miss"},
		},
		{
			name:     "combat: you dodge",
			line:     "[Mon Apr 13 06:00:00 2026] You dodge a gnoll's attack!",
			wantOK:   true,
			wantType: EventCombatMiss,
			wantData: CombatMissData{Actor: "a gnoll", Target: "You", MissType: "dodge"},
		},
		{
			name:     "combat: you parry",
			line:     "[Mon Apr 13 06:00:00 2026] You parry a gnoll's attack!",
			wantOK:   true,
			wantType: EventCombatMiss,
			wantData: CombatMissData{Actor: "a gnoll", Target: "You", MissType: "parry"},
		},

		// --- Death ---
		{
			name:     "death: slain by NPC",
			line:     "[Mon Apr 13 06:00:00 2026] You have been slain by a gnoll.",
			wantOK:   true,
			wantType: EventDeath,
			wantData: DeathData{SlainBy: "a gnoll"},
		},
		{
			name:     "death: slain by multi-word NPC",
			line:     "[Mon Apr 13 06:00:00 2026] You have been slain by a greater gnoll.",
			wantOK:   true,
			wantType: EventDeath,
			wantData: DeathData{SlainBy: "a greater gnoll"},
		},

		// --- Unrecognised messages ---
		{
			name:   "unrecognised: chat message",
			line:   "[Mon Apr 13 06:00:00 2026] Soandso says, 'Hello!'",
			wantOK: false,
		},
		{
			name:   "unrecognised: system message",
			line:   "[Mon Apr 13 06:00:00 2026] Welcome to EverQuest!",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok := ParseLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ParseLine() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if ev.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", ev.Type, tt.wantType)
			}
			if ev.Message == "" {
				t.Error("Message is empty")
			}
			if ev.Timestamp.IsZero() {
				t.Error("Timestamp is zero")
			}
			if !tt.wantTS.IsZero() && !ev.Timestamp.Equal(tt.wantTS) {
				t.Errorf("Timestamp = %v, want %v", ev.Timestamp, tt.wantTS)
			}
			if tt.wantData != nil {
				compareData(t, ev.Data, tt.wantData)
			}
		})
	}
}

// compareData does a type-specific comparison of the event Data field.
func compareData(t *testing.T, got, want interface{}) {
	t.Helper()
	switch w := want.(type) {
	case ZoneData:
		g, ok := got.(ZoneData)
		if !ok {
			t.Fatalf("Data type = %T, want ZoneData", got)
		}
		if g != w {
			t.Errorf("ZoneData = %+v, want %+v", g, w)
		}
	case SpellCastData:
		g, ok := got.(SpellCastData)
		if !ok {
			t.Fatalf("Data type = %T, want SpellCastData", got)
		}
		if g != w {
			t.Errorf("SpellCastData = %+v, want %+v", g, w)
		}
	case SpellInterruptData:
		g, ok := got.(SpellInterruptData)
		if !ok {
			t.Fatalf("Data type = %T, want SpellInterruptData", got)
		}
		if g != w {
			t.Errorf("SpellInterruptData = %+v, want %+v", g, w)
		}
	case SpellResistData:
		g, ok := got.(SpellResistData)
		if !ok {
			t.Fatalf("Data type = %T, want SpellResistData", got)
		}
		if g != w {
			t.Errorf("SpellResistData = %+v, want %+v", g, w)
		}
	case SpellFadeData:
		g, ok := got.(SpellFadeData)
		if !ok {
			t.Fatalf("Data type = %T, want SpellFadeData", got)
		}
		if g != w {
			t.Errorf("SpellFadeData = %+v, want %+v", g, w)
		}
	case CombatHitData:
		g, ok := got.(CombatHitData)
		if !ok {
			t.Fatalf("Data type = %T, want CombatHitData", got)
		}
		if g != w {
			t.Errorf("CombatHitData = %+v, want %+v", g, w)
		}
	case CombatMissData:
		g, ok := got.(CombatMissData)
		if !ok {
			t.Fatalf("Data type = %T, want CombatMissData", got)
		}
		if g != w {
			t.Errorf("CombatMissData = %+v, want %+v", g, w)
		}
	case DeathData:
		g, ok := got.(DeathData)
		if !ok {
			t.Fatalf("Data type = %T, want DeathData", got)
		}
		if g != w {
			t.Errorf("DeathData = %+v, want %+v", g, w)
		}
	default:
		t.Fatalf("compareData: unhandled want type %T", want)
	}
}
