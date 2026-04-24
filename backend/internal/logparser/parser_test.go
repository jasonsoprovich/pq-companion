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

		// --- Spell fade from target ---
		{
			name:     "spell: fade from target single-word spell",
			line:     "[Mon Apr 13 06:00:00 2026] Tashanian effect fades from Soandso.",
			wantOK:   true,
			wantType: EventSpellFadeFrom,
			wantData: SpellFadeFromData{SpellName: "Tashanian", TargetName: "Soandso"},
		},
		{
			name:     "spell: fade from target multi-word spell",
			line:     "[Mon Apr 13 06:00:00 2026] Color Flux effect fades from Playerone.",
			wantOK:   true,
			wantType: EventSpellFadeFrom,
			wantData: SpellFadeFromData{SpellName: "Color Flux", TargetName: "Playerone"},
		},
		{
			name:     "spell: fade from target multi-word target",
			line:     "[Mon Apr 13 06:00:00 2026] Clarity effect fades from a gnoll.",
			wantOK:   true,
			wantType: EventSpellFadeFrom,
			wantData: SpellFadeFromData{SpellName: "Clarity", TargetName: "a gnoll"},
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

		// Passive constructions starting with an auxiliary verb must not be
		// misidentified as player-hits-NPC events.
		{
			name:   "combat: passive construction 'You have been healed' not a combat hit",
			line:   "[Mon Apr 13 06:00:00 2026] You have been healed for 150 points of damage.",
			wantOK: false,
		},
		{
			name:   "combat: passive construction 'You are poisoned' not a combat hit",
			line:   "[Mon Apr 13 06:00:00 2026] You are poisoned for 5 points of damage.",
			wantOK: false,
		},

		// --- Combat: non-melee / spell damage ---
		{
			name:     "combat: player spell hits target (passive non-melee)",
			line:     "[Mon Apr 13 06:00:00 2026] a giant wasp drone was hit by non-melee for 4 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "spell", Target: "a giant wasp drone", Damage: 4},
		},
		{
			name:     "combat: player spell hits multi-word target (passive non-melee)",
			line:     "[Mon Apr 13 06:00:00 2026] a greater gnoll shaman was hit by non-melee for 150 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "spell", Target: "a greater gnoll shaman", Damage: 150},
		},
		{
			name:     "combat: other player's spell hits NPC (active non-melee)",
			line:     "[Mon Apr 13 06:00:00 2026] Takkisina hit a temple skirmisher for 18 points of non-melee damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "Takkisina", Skill: "spell", Target: "a temple skirmisher", Damage: 18},
		},
		{
			name:     "combat: multi-word NPC spell hits player (active non-melee)",
			line:     "[Mon Apr 13 06:00:00 2026] A Shissar Arch Arcanist hit Takkisina for 640 points of non-melee damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "A Shissar Arch Arcanist", Skill: "spell", Target: "Takkisina", Damage: 640},
		},
		{
			name:     "combat: NPC self-damage via spell (active non-melee)",
			line:     "[Mon Apr 13 06:00:00 2026] Gormak hit Gormak for 50 points of non-melee damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "Gormak", Skill: "spell", Target: "Gormak", Damage: 50},
		},

		// --- Combat: NPC hits player ---
		{
			name:     "combat: NPC hits you",
			line:     "[Mon Apr 13 06:00:00 2026] A gnoll slashes you for 50 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "A gnoll", Skill: "slashes", Target: "You", Damage: 50},
		},

		// --- Combat: third-party player hits NPC ---
		{
			name:     "combat: other player slashes NPC",
			line:     "[Mon Apr 13 06:00:00 2026] Playerone slashes a gnoll for 75 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "Playerone", Skill: "slashes", Target: "a gnoll", Damage: 75},
		},
		{
			name:     "combat: other player pierces multi-word NPC",
			line:     "[Mon Apr 13 06:00:00 2026] Guildmate pierces a young gnoll for 30 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "Guildmate", Skill: "pierces", Target: "a young gnoll", Damage: 30},
		},
		// A multi-word NPC acting as the source (e.g. a pet or NPC fighting another NPC)
		// must NOT be parsed — the regex would capture only "a"/"an"/"the" as the actor,
		// producing a spurious DPS entry for the bare article.
		{
			name:   "combat: multi-word NPC actor not parsed as third-party",
			line:   "[Mon Apr 13 06:00:00 2026] a fire elemental slashes a gnoll for 80 points of damage.",
			wantOK: false,
		},
		{
			name:   "combat: 'an' prefix NPC actor not parsed as third-party",
			line:   "[Mon Apr 13 06:00:00 2026] an orc warrior bashes a gnoll for 60 points of damage.",
			wantOK: false,
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

		// --- Kill ---
		{
			name:     "kill: you slay single-word mob",
			line:     "[Mon Apr 13 06:00:00 2026] You have slain a gnoll!",
			wantOK:   true,
			wantType: EventKill,
			wantData: KillData{Killer: "You", Target: "a gnoll"},
		},
		{
			name:     "kill: you slay multi-word mob",
			line:     "[Mon Apr 13 06:00:00 2026] You have slain a greater gnoll!",
			wantOK:   true,
			wantType: EventKill,
			wantData: KillData{Killer: "You", Target: "a greater gnoll"},
		},
		{
			name:     "kill: group member slays mob",
			line:     "[Mon Apr 13 06:00:00 2026] Osui has slain a gnoll!",
			wantOK:   true,
			wantType: EventKill,
			wantData: KillData{Killer: "Osui", Target: "a gnoll"},
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
		{
			name:     "death: you died (no named killer)",
			line:     "[Mon Apr 13 06:00:00 2026] You died.",
			wantOK:   true,
			wantType: EventDeath,
			wantData: DeathData{},
		},

		// --- /con considered ---
		{
			name:     "con: regards you as ally (multi-word NPC)",
			line:     "[Mon Apr 13 06:00:00 2026] a grimling cadaverist regards you as an ally.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a grimling cadaverist"},
		},
		{
			name:     "con: scowls at you",
			line:     "[Mon Apr 13 06:00:00 2026] a gnoll scowls at you, ready to attack.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a gnoll"},
		},
		{
			name:     "con: glares at you",
			line:     "[Mon Apr 13 06:00:00 2026] a young gnoll glares at you threateningly.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a young gnoll"},
		},
		{
			name:     "con: judges you indifferently",
			line:     "[Mon Apr 13 06:00:00 2026] a goblin warrior judges you indifferently, what is your business here?",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a goblin warrior"},
		},
		{
			name:     "con: warmly regards you",
			line:     "[Mon Apr 13 06:00:00 2026] a halfling guard warmly regards you as a friend.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a halfling guard"},
		},
		{
			name:     "con: considers you",
			line:     "[Mon Apr 13 06:00:00 2026] an orc pawn considers you amiably.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "an orc pawn"},
		},
		{
			name:     "con: looks upon you",
			line:     "[Mon Apr 13 06:00:00 2026] a skeleton looks upon you with suspicion.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a skeleton"},
		},
		{
			name:     "con: looks your way",
			line:     "[Mon Apr 13 06:00:00 2026] a lizardman looks your way apprehensively.",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "a lizardman"},
		},

		// --- /con false-positive guard ---
		// Lines starting with "You " must never be classified as EventConsidered.
		// Zone-entry lines reach reZone first, but the regex guard provides
		// belt-and-suspenders protection for any unclassified "You …" lines.
		{
			name:   "con: 'You' prefix line is not parsed as a consider event",
			line:   "[Mon Apr 13 06:00:00 2026] You considers you amiably.",
			wantOK: false,
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
	case SpellFadeFromData:
		g, ok := got.(SpellFadeFromData)
		if !ok {
			t.Fatalf("Data type = %T, want SpellFadeFromData", got)
		}
		if g != w {
			t.Errorf("SpellFadeFromData = %+v, want %+v", g, w)
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
	case KillData:
		g, ok := got.(KillData)
		if !ok {
			t.Fatalf("Data type = %T, want KillData", got)
		}
		if g != w {
			t.Errorf("KillData = %+v, want %+v", g, w)
		}
	case ConsideredData:
		g, ok := got.(ConsideredData)
		if !ok {
			t.Fatalf("Data type = %T, want ConsideredData", got)
		}
		if g != w {
			t.Errorf("ConsideredData = %+v, want %+v", g, w)
		}
	default:
		t.Fatalf("compareData: unhandled want type %T", want)
	}
}
