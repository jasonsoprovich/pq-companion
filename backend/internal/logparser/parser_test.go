package logparser

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	// Reference timestamp used across cases.
	wantTS, _ := time.ParseInLocation(tsLayout, "[Mon Apr 13 06:00:00 2026]", time.Local)
	wantTSSingle, _ := time.ParseInLocation(tsLayout, "[Mon Apr  3 06:00:00 2026]", time.Local)

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

		// --- Spell did not take hold ---
		{
			name:     "spell: did not take hold (self)",
			line:     "[Mon Apr 13 06:00:00 2026] Your spell did not take hold.",
			wantOK:   true,
			wantType: EventSpellDidNotTakeHold,
			wantData: SpellDidNotTakeHoldData{},
		},
		{
			name:     "spell: did not take hold (on target)",
			line:     "[Mon Apr 13 06:00:00 2026] Your spell did not take hold on your target.",
			wantOK:   true,
			wantType: EventSpellDidNotTakeHold,
			wantData: SpellDidNotTakeHoldData{},
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

		// --- Combat: DoT tick (PQ "from your" form) ---
		// Project Quarm only logs the local player's own DoT ticks; ticks
		// from other casters are server-side and never appear in this log.
		// The spell name is always present, so attribution is unambiguous.
		{
			name:     "combat: DoT tick from your spell",
			line:     "[Mon Apr 13 06:00:00 2026] Pli Thall Xakra has taken 48 damage from your Asphyxiate.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "dot", Target: "Pli Thall Xakra", Damage: 48, SpellName: "Asphyxiate"},
		},
		{
			name:     "combat: DoT tick multi-word spell",
			line:     "[Mon Apr 13 06:00:00 2026] a goblin has taken 12 damage from your Disease Cloud.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "You", Skill: "dot", Target: "a goblin", Damage: 12, SpellName: "Disease Cloud"},
		},

		// --- Combat: critical hit announcement (PQ standalone form) ---
		// Emitted on its own line immediately before the matching damage
		// line. Surfaced as a distinct event so the tracker can correlate.
		{
			name:     "combat: PQ-format critical hit",
			line:     "[Mon Apr 13 06:00:00 2026] Sandrian Scores a critical hit!(62)",
			wantOK:   true,
			wantType: EventCritHit,
			wantData: CritHitData{Actor: "Sandrian", Damage: 62},
		},

		// --- Charmed pet: "tells you, 'Attacking X Master.'" ---
		// The canonical EQ charmed-pet attack tell, sent only to the
		// charmer. We extract the pet name; the owner is always the active
		// character (consumers fill that in).
		{
			name:     "charmed pet tell: simple NPC pet",
			line:     "[Mon Apr 13 06:00:00 2026] a fetid fiend tells you, 'Attacking a spinechiller spider Master.'",
			wantOK:   true,
			wantType: EventCharmedPet,
			wantData: CharmedPetData{Pet: "a fetid fiend"},
		},
		{
			name:     "charmed pet tell: proper-noun BST warder",
			line:     "[Mon Apr 13 06:00:00 2026] Rygan Anisher tells you, 'Attacking A Centi Dator Master.'",
			wantOK:   true,
			wantType: EventCharmedPet,
			wantData: CharmedPetData{Pet: "Rygan Anisher"},
		},

		// --- Charm-broken ---
		{
			name:     "charm: your charm spell wore off",
			line:     "[Mon Apr 13 06:00:00 2026] Your charm spell has worn off.",
			wantOK:   true,
			wantType: EventCharmBroken,
			wantData: nil,
		},

		// --- Verified-player chat lines ---
		// Single-capitalised-word speaker + tell channel. Used by the
		// combat tracker to learn which names are players so single-word
		// boss names like "Zlandicar" can be correctly identified as NPCs
		// during third-party damage routing.
		{
			name:     "verified player: tells the guild",
			line:     "[Mon Apr 13 06:00:00 2026] Sandrian tells the guild, 'I have the invite going'",
			wantOK:   true,
			wantType: EventVerifiedPlayer,
			wantData: VerifiedPlayerData{Name: "Sandrian"},
		},
		{
			name:     "verified player: tells the raid",
			line:     "[Mon Apr 13 06:00:00 2026] Takkisina tells the raid,  'ASSIST ME | Zlandicar  |'",
			wantOK:   true,
			wantType: EventVerifiedPlayer,
			wantData: VerifiedPlayerData{Name: "Takkisina"},
		},
		{
			name:     "verified player: plain tells you (must beat spell-landed)",
			line:     "[Mon Apr 13 06:00:00 2026] Maykill tells you, 'May I get kei and vog wmp'",
			wantOK:   true,
			wantType: EventVerifiedPlayer,
			wantData: VerifiedPlayerData{Name: "Maykill"},
		},
		// Multi-word "tells you" should NOT be a verified player — it's a
		// charmed pet's "Attacking X Master." or a similar non-player
		// utterance, already caught earlier in the dispatcher.
		{
			name:     "verified player: charm tell does not classify as player",
			line:     "[Mon Apr 13 06:00:00 2026] a fetid fiend tells you, 'Attacking a goblin Master.'",
			wantOK:   true,
			wantType: EventCharmedPet,
			wantData: CharmedPetData{Pet: "a fetid fiend"},
		},
		{
			name:     "combat: PQ-format critical hit large damage",
			line:     "[Mon Apr 13 06:00:00 2026] Muadib Scores a critical hit!(2014)",
			wantOK:   true,
			wantType: EventCritHit,
			wantData: CritHitData{Actor: "Muadib", Damage: 2014},
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
		// Multi-word NPC actors must be captured intact. The verb-anchored
		// regex covers article-prefixed mobs ("an orc warrior") as well as
		// proper-noun multi-word NPCs ("Sambata Tribal Member", "Enchanted
		// Golem"). The combat tracker filters these out of the DPS view via
		// looksLikeNPC downstream — but the parser's job is to record the
		// actor accurately, not to second-guess.
		{
			name:     "combat: multi-word NPC actor (article-prefixed)",
			line:     "[Mon Apr 13 06:00:00 2026] a fire elemental slashes a gnoll for 80 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "a fire elemental", Skill: "slashes", Target: "a gnoll", Damage: 80},
		},
		{
			name:     "combat: 'an' prefix multi-word NPC actor",
			line:     "[Mon Apr 13 06:00:00 2026] an orc warrior bashes a gnoll for 60 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "an orc warrior", Skill: "bashes", Target: "a gnoll", Damage: 60},
		},
		{
			name:     "combat: proper-noun multi-word NPC actor",
			line:     "[Mon Apr 13 06:00:00 2026] Sambata Tribal Member hits Nealuwenya for 24 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "Sambata Tribal Member", Skill: "hits", Target: "Nealuwenya", Damage: 24},
		},
		{
			name:     "combat: NPC hits player with YOU all-caps (Project Quarm format)",
			line:     "[Mon Apr 13 06:00:00 2026] A wolf bites YOU for 10 points of damage.",
			wantOK:   true,
			wantType: EventCombatHit,
			wantData: CombatHitData{Actor: "A wolf", Skill: "bites", Target: "You", Damage: 10},
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
		{
			name:     "kill: passive form (PQ/EQMac), single-word target",
			line:     "[Mon Apr 13 06:00:00 2026] Takkisina has been slain by Eom Va Liako Xakra!",
			wantOK:   true,
			wantType: EventKill,
			wantData: KillData{Killer: "Eom Va Liako Xakra", Target: "Takkisina"},
		},
		{
			name:     "kill: passive form, article-prefixed target",
			line:     "[Mon Apr 13 06:00:00 2026] a lightcrawler has been slain by Ineka!",
			wantOK:   true,
			wantType: EventKill,
			wantData: KillData{Killer: "Ineka", Target: "a lightcrawler"},
		},
		{
			name:     "kill: passive form, possessive-pet killer",
			line:     "[Mon Apr 13 06:00:00 2026] a fungi shroom has been slain by Gygr`s warder!",
			wantOK:   true,
			wantType: EventKill,
			wantData: KillData{Killer: "Gygr`s warder", Target: "a fungi shroom"},
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
		{
			name:     "con: looks at you (dubious)",
			line:     "[Mon Apr 13 06:00:00 2026] an Icepaw cleric looks at you dubiously -- what would you like your tombstone to say?",
			wantOK:   true,
			wantType: EventConsidered,
			wantData: ConsideredData{TargetName: "an Icepaw cleric"},
		},

		// --- /who entries ---
		{
			name:     "who: named with race and guild",
			line:     "[Mon Apr 13 06:00:00 2026] [60 Necromancer] Foo (Iksar) <Drowsy Disciples>",
			wantOK:   true,
			wantType: EventWhoEntry,
			wantData: WhoEntryData{Name: "Foo", Level: 60, Class: "Necromancer", Race: "Iksar", Guild: "Drowsy Disciples"},
		},
		{
			name:     "who: named with multi-word class and LFG flag",
			line:     "[Mon Apr 13 06:00:00 2026] [55 Shadow Knight] Bar LFG",
			wantOK:   true,
			wantType: EventWhoEntry,
			wantData: WhoEntryData{Name: "Bar", Level: 55, Class: "Shadow Knight", LFG: true},
		},
		{
			name:     "who: anonymous",
			line:     "[Mon Apr 13 06:00:00 2026] [ANONYMOUS] Baz",
			wantOK:   true,
			wantType: EventWhoEntry,
			wantData: WhoEntryData{Name: "Baz", Anonymous: true},
		},
		{
			name:     "who: anon shorthand with AFK",
			line:     "[Mon Apr 13 06:00:00 2026] [ANON] Qux AFK",
			wantOK:   true,
			wantType: EventWhoEntry,
			wantData: WhoEntryData{Name: "Qux", Anonymous: true, AFK: true},
		},
		{
			name:     "who: race only, no guild",
			line:     "[Mon Apr 13 06:00:00 2026] [42 Druid] Zee (Wood Elf)",
			wantOK:   true,
			wantType: EventWhoEntry,
			wantData: WhoEntryData{Name: "Zee", Level: 42, Class: "Druid", Race: "Wood Elf"},
		},
		{
			name:   "who: not actually a who line — bracket without level",
			line:   "[Mon Apr 13 06:00:00 2026] [Some bracket text] Foo says hello",
			wantOK: false,
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

		// --- Pet owner binding ---
		{
			name:     "pet owner: charm bind names player",
			line:     "[Mon Apr 13 06:00:00 2026] Kebartik says 'My leader is Kildrey.'",
			wantOK:   true,
			wantType: EventPetOwner,
			wantData: PetOwnerData{Pet: "Kebartik", Owner: "Kildrey"},
		},
		{
			name:     "pet owner: summoned warder names player",
			line:     "[Mon Apr 13 06:00:00 2026] Grimrose`s warder says 'My leader is Grimrose.'",
			wantOK:   true,
			wantType: EventPetOwner,
			wantData: PetOwnerData{Pet: "Grimrose`s warder", Owner: "Grimrose"},
		},

		// --- /random dice rolls ---
		{
			name:     "roll: announce line names the roller",
			line:     "[Mon Apr 13 06:00:00 2026] **A Magic Die is rolled by Tabbie.",
			wantOK:   true,
			wantType: EventRollAnnounce,
			wantData: RollAnnounceData{Roller: "Tabbie"},
		},
		{
			name:     "roll: result line carries range and value",
			line:     "[Mon Apr 13 06:00:00 2026] **It could have been any number from 0 to 222, but this time it turned up a 69.",
			wantOK:   true,
			wantType: EventRollResult,
			wantData: RollResultData{Min: 0, Max: 222, Value: 69},
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
	case SpellDidNotTakeHoldData:
		if _, ok := got.(SpellDidNotTakeHoldData); !ok {
			t.Fatalf("Data type = %T, want SpellDidNotTakeHoldData", got)
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
	case PetOwnerData:
		g, ok := got.(PetOwnerData)
		if !ok {
			t.Fatalf("Data type = %T, want PetOwnerData", got)
		}
		if g != w {
			t.Errorf("PetOwnerData = %+v, want %+v", g, w)
		}
	case CritHitData:
		g, ok := got.(CritHitData)
		if !ok {
			t.Fatalf("Data type = %T, want CritHitData", got)
		}
		if g != w {
			t.Errorf("CritHitData = %+v, want %+v", g, w)
		}
	case CharmedPetData:
		g, ok := got.(CharmedPetData)
		if !ok {
			t.Fatalf("Data type = %T, want CharmedPetData", got)
		}
		if g != w {
			t.Errorf("CharmedPetData = %+v, want %+v", g, w)
		}
	case VerifiedPlayerData:
		g, ok := got.(VerifiedPlayerData)
		if !ok {
			t.Fatalf("Data type = %T, want VerifiedPlayerData", got)
		}
		if g != w {
			t.Errorf("VerifiedPlayerData = %+v, want %+v", g, w)
		}
	case RollAnnounceData:
		g, ok := got.(RollAnnounceData)
		if !ok {
			t.Fatalf("Data type = %T, want RollAnnounceData", got)
		}
		if g != w {
			t.Errorf("RollAnnounceData = %+v, want %+v", g, w)
		}
	case RollResultData:
		g, ok := got.(RollResultData)
		if !ok {
			t.Fatalf("Data type = %T, want RollResultData", got)
		}
		if g != w {
			t.Errorf("RollResultData = %+v, want %+v", g, w)
		}
	case WhoEntryData:
		g, ok := got.(WhoEntryData)
		if !ok {
			t.Fatalf("Data type = %T, want WhoEntryData", got)
		}
		if g != w {
			t.Errorf("WhoEntryData = %+v, want %+v", g, w)
		}
	default:
		t.Fatalf("compareData: unhandled want type %T", want)
	}
}

// TestRealOsuiLogPhase1Coverage streams the real Osui (Enchanter) log file
// through ParseLine and asserts the Phase 1 patterns are picked up at the
// counts a manual grep produced. Skipped when the gitignored testdata
// fixture is not present (e.g. CI). Counts are intentionally exact so a
// regression in the regexes (e.g. an over-eager guard rejecting valid
// lines) fails loudly.
func TestRealOsuiLogPhase1Coverage(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "eqlog_Osui_pq.proj.txt")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("testdata fixture %s not present", path)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	const (
		// Counts established by `grep -c ... testdata/eqlog_Osui_pq.proj.txt`.
		// Updated 2026-05-11 after the testdata fixture was extended with
		// the Sun May 10 raid session (Zlandicar, Plane of Fear, etc.).
		wantDoTTicks   = 138
		wantCritEvents = 6546
	)

	dotCount, critCount := 0, 0
	scanner := bufio.NewScanner(f)
	// Some log lines (long /tells, raid messages) exceed bufio's default
	// 64 KiB buffer; bump to 1 MiB so they don't truncate.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		ev, ok := ParseLine(scanner.Text())
		if !ok {
			continue
		}
		switch ev.Type {
		case EventCombatHit:
			if d, ok := ev.Data.(CombatHitData); ok && d.Skill == "dot" {
				dotCount++
			}
		case EventCritHit:
			critCount++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if dotCount != wantDoTTicks {
		t.Errorf("DoT tick events: got %d, want %d", dotCount, wantDoTTicks)
	}
	if critCount != wantCritEvents {
		t.Errorf("EventCritHit events: got %d, want %d", critCount, wantCritEvents)
	}
}
