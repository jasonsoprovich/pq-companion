package trigger

import (
	"fmt"
	"strings"
)

// Raid packs: one built-in pack per raid tier, rebuilt from the GINA packs
// curated by Xanax <Former Glory> (used with permission). Each pack is a table
// of bosses plus hand-written encounter events. Spell names, land messages and
// recasts are literals copied from quarm.db and checked against it by
// packs_raid_test.go, so a Project Quarm data update that changes a boss's
// spells fails the tests instead of shipping stale timers.

// raidCredit is appended to every raid pack's description.
const raidCredit = "Curated with Xanax <Former Glory>."

// raidMinTimerSecs is the shortest server recast that earns a boss timer.
// Anything faster is effectively "always available" and a bar for it is
// noise.
const raidMinTimerSecs = 6

// raidSpell is one spell on a boss's npc_spells list (or its melee proc).
type raidSpell struct {
	ID   int
	Name string
	// OnYou is spells_new.cast_on_you. OnOther is cast_on_other with the
	// caster-name prefix stripped, or "" when a player-castable spell logs
	// the same text (so a raid member's own cast can't trip the timer).
	OnYou   string
	OnOther string
	// Recast is the NPC AI's recast in seconds (EQMacEmu mob_ai.cpp:
	// npc_spells_entries.recast_delay when > 0, else spells_new.recast_time),
	// without the server's 0–4s random addition — the timer shows the
	// earliest the boss can cast it again. 0 for procs.
	Recast float64
	// Counter is the cure type ("Disease", "Poison", "Curse"), "" if none.
	Counter string
	Proc    bool
	// Buff marks a beneficial spell (a self-buff, or a dispel the data flags
	// as beneficial). Listed on the Who card, never timed.
	Buff bool
}

// raidBoss is one raid NPC: its Who card facts and its spells.
type raidBoss struct {
	Name    string // display name as the log writes it
	Zone    string
	Level   int
	MR      int
	CR      int
	FR      int
	Traits  string // raidTraitLabels the boss has, e.g. "Rampage, Unslowable"
	Note    string // curated notes (CH cadence etc.)
	Spells  []raidSpell
	NoWho   bool // skip the Who card (scripted or untargetable NPCs)
	Aliases []string
}

// raidTraitLabels are the npc_types.special_abilities codes worth a Who
// card mention, in display order. Abilities nearly every raid boss has
// (summon, enrage, mez/charm/stun immunity) are left out as noise.
var raidTraitLabels = []struct {
	Code  int
	Label string
}{
	{3, "Rampage"},
	{4, "AE Rampage"},
	{5, "Flurry"},
	{12, "Unslowable"},
	{20, "Immune to magic"},
}

// raidPackDef is a raid pack before it is turned into triggers.
type raidPackDef struct {
	Name        string
	Description string
	Bosses      []raidBoss
	Events      []Trigger
}

// build turns the table into a TriggerPack. Trigger order: per boss its Who
// card, then the boss's spells; spells several bosses share become one
// trigger listing every caster, placed under the first boss that casts it;
// encounter events come last.
func (d raidPackDef) build() TriggerPack {
	type shared struct {
		spell   raidSpell
		casters []BossCaster
		names   []string
	}
	byID := map[int]*shared{}
	for _, b := range d.Bosses {
		for _, s := range b.Spells {
			sh, ok := byID[s.ID]
			if !ok {
				sh = &shared{spell: s}
				byID[s.ID] = sh
			}
			// Every boss that casts the spell is a caster, whatever its own
			// recast; the trigger's duration is the longest of them.
			if !s.Proc && !s.Buff {
				sh.casters = append(sh.casters, BossCaster{Name: b.Name, DurationSecs: s.Recast})
				sh.names = append(sh.names, b.Name)
				if s.Recast > sh.spell.Recast {
					sh.spell.Recast = s.Recast
				}
			}
		}
	}

	// Cure alerts key on the on-you line: spells that share one (Wind of
	// Tashanian / Howl of Tashan) become one alert naming them all.
	cureNames := map[string][]string{}
	for _, b := range d.Bosses {
		for _, s := range b.Spells {
			if s.Counter == "" {
				continue
			}
			names := cureNames[s.OnYou]
			if !containsString(names, s.Name) {
				cureNames[s.OnYou] = append(names, s.Name)
			}
		}
	}

	var out []Trigger
	emitted := map[int]bool{}
	cureDone := map[string]bool{}
	for _, b := range d.Bosses {
		if !b.NoWho {
			out = append(out, raidWhoTrigger(d.Name, b))
		}
		for _, s := range b.Spells {
			if emitted[s.ID] {
				continue
			}
			emitted[s.ID] = true
			sh := byID[s.ID]
			if t, ok := raidTimerTrigger(d.Name, sh.spell, sh.casters, sh.names); ok {
				out = append(out, t)
			}
			if s.Counter != "" && !cureDone[s.OnYou] {
				cureDone[s.OnYou] = true
				out = append(out, raidCureTrigger(d.Name, sh.spell, cureNames[s.OnYou]))
			}
		}
	}
	for _, e := range d.Events {
		e.PackName = d.Name
		e.Enabled = true
		out = append(out, e)
	}
	return TriggerPack{PackName: d.Name, Description: d.Description, Triggers: out}
}

// raidTimerTrigger builds the boss recast timer for a spell, bound to the
// casting boss (Trigger.BossCasters). Returns false when no caster's recast
// is long enough to be worth a bar. Each caster carries its own recast, so
// a spell two bosses cast at different rates times correctly for each.
func raidTimerTrigger(pack string, s raidSpell, casters []BossCaster, names []string) (Trigger, bool) {
	if len(casters) == 0 || s.Recast < raidMinTimerSecs {
		return Trigger{}, false
	}
	alts := []string{lit(s.OnYou), `You resist the ` + lit(s.Name) + ` spell!`}
	if s.OnOther != "" {
		alts = append(alts, "[A-Za-z`' ]+"+lit(s.OnOther))
	}
	bc := make([]BossCaster, len(casters))
	for i, c := range casters {
		bc[i] = BossCaster{Name: c.Name}
		if c.DurationSecs != s.Recast {
			bc[i].DurationSecs = c.DurationSecs
		}
	}
	t := Trigger{
		Name:              raidSpellTriggerName(s.Name, names),
		Enabled:           true,
		Pattern:           `^(?:` + strings.Join(alts, "|") + `)$`,
		PackName:          pack,
		TimerType:         TimerTypeDetrimental,
		TimerDurationSecs: s.Recast,
		SpellID:           s.ID,
		BarColor:          "#ef4444",
		BossCasters:       bc,
		Actions:           []Action{},
	}
	if s.Recast >= 20 {
		t.TimerAlerts = []TimerAlert{{
			ID:          "raid-recast-5s",
			Seconds:     5,
			Type:        TimerAlertTypeTextToSpeech,
			TTSTemplate: s.Name + " in 5",
			TTSVolume:   100,
		}}
	}
	return t, true
}

// raidSpellTriggerName is "Grummus — Stench of Decay" for one caster and
// "Stench of Decay (Grummus, Gryme the Crypt Guardian)" for several.
func raidSpellTriggerName(spell string, casters []string) string {
	if len(casters) == 1 {
		return casters[0] + " — " + spell
	}
	return spell + " (" + strings.Join(casters, ", ") + ")"
}

var raidCounterColor = map[string]string{
	"Disease": "#84cc16",
	"Poison":  "#22c55e",
	"Curse":   "#a855f7",
}

// raidCureTrigger alerts when a curable boss debuff lands on you. names are
// every spell in the pack that logs this on-you line.
func raidCureTrigger(pack string, s raidSpell, names []string) Trigger {
	cure := strings.ToLower(s.Counter)
	label := strings.Join(names, " / ")
	return Trigger{
		Name:     label + " on you",
		Enabled:  true,
		Pattern:  `^` + lit(s.OnYou) + `$`,
		PackName: pack,
		SpellID:  s.ID,
		Actions: []Action{
			{Type: ActionOverlayText, Text: strings.ToUpper(label) + " — cure " + cure, DurationSecs: 6, Color: raidCounterColor[s.Counter]},
			{Type: ActionTextToSpeech, Text: s.Counter, Volume: 1.0},
		},
	}
}

// raidWhoTrigger shows the boss's card when you /tell it: the server answers
// "<name> is not online at this time." with the name as typed.
func raidWhoTrigger(pack string, b raidBoss) Trigger {
	names := append([]string{b.Name}, b.Aliases...)
	alts := make([]string, len(names))
	for i, n := range names {
		alts[i] = raidTellNamePattern(n)
	}
	return Trigger{
		Name:     b.Name + " — Who",
		Enabled:  true,
		Pattern:  `(?i)^(?:` + strings.Join(alts, "|") + `) is not online at this time\.$`,
		PackName: pack,
		Actions: []Action{
			{Type: ActionOverlayText, Text: raidWhoText(b), DurationSecs: 15, Color: "#e5e7eb"},
		},
	}
}

// raidTellNamePattern accepts the boss name as people type it in /tell:
// words joined by underscores or spaces, each word shortened to as little as
// its first three letters (the last word to its first letter), matching the
// curated GINA patterns ("aramin_the_spider_g"). A one-word name must be
// typed in full.
func raidTellNamePattern(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '_' || r == '-' })
	if len(words) == 1 {
		return raidTellWord(words[0], len(words[0])) // whole name, or "/tell g x" would match
	}
	parts := make([]string, len(words))
	for i, w := range words {
		keep := 3
		if i == len(words)-1 {
			keep = 1
		}
		parts[i] = raidTellWord(w, keep)
	}
	return strings.Join(parts, `[_ -]?`)
}

// raidTellWord matches one word of a boss name: its first keep letters
// required, the rest optional, and punctuation ("Falto," "Aerin`Dar")
// optional wherever it appears, since players rarely type it.
func raidTellWord(w string, keep int) string {
	var req, opt strings.Builder
	letters := 0
	for _, r := range w {
		var tok string
		if r == ',' || r == '`' || r == '\'' {
			tok = "[`',]?"
		} else {
			tok = lit(string(r))
			letters++
		}
		if letters <= keep && (letters < keep || tok != "[`',]?") {
			req.WriteString(tok)
		} else {
			opt.WriteString(tok)
		}
	}
	if opt.Len() == 0 {
		return req.String()
	}
	return req.String() + `(?:` + opt.String() + `)?`
}

// raidWhoText is the Who card body.
func raidWhoText(b raidBoss) string {
	parts := []string{
		fmt.Sprintf("%s · Lv%d · MR %d / CR %d / FR %d", b.Name, b.Level, b.MR, b.CR, b.FR),
	}
	if b.Note != "" {
		parts = append(parts, b.Note)
	}
	if b.Traits != "" {
		parts = append(parts, b.Traits)
	}
	var casts, procs []string
	for _, s := range b.Spells {
		if s.Proc {
			procs = append(procs, s.Name)
		} else {
			casts = append(casts, s.Name)
		}
	}
	if len(casts) > 0 {
		parts = append(parts, "Casts: "+strings.Join(casts, ", "))
	}
	if len(procs) > 0 {
		parts = append(parts, "Procs: "+strings.Join(procs, ", "))
	}
	return strings.Join(parts, " · ")
}

// raidAlert is a one-shot encounter event: overlay plus optional TTS.
func raidAlert(name, pattern, overlay, tts string) Trigger {
	t := Trigger{
		Name:    name,
		Pattern: pattern,
		Actions: []Action{{Type: ActionOverlayText, Text: overlay, DurationSecs: 6, Color: "#f59e0b"}},
	}
	if tts != "" {
		t.Actions = append(t.Actions, Action{Type: ActionTextToSpeech, Text: tts, Volume: 1.0})
	}
	return t
}

// raidEventTimer is a scripted encounter countdown with a 10s warning when
// long enough to need one.
func raidEventTimer(name, pattern string, secs float64, extras ...ExtraPattern) Trigger {
	t := Trigger{
		Name:              name,
		Pattern:           pattern,
		TimerType:         TimerTypeCustom,
		TimerDurationSecs: secs,
		ExtraPatterns:     extras,
		BarColor:          "#f59e0b",
		Actions:           []Action{},
	}
	if secs >= 30 {
		t.TimerAlerts = []TimerAlert{{
			ID:          "raid-event-10s",
			Seconds:     10,
			Type:        TimerAlertTypeTextToSpeech,
			TTSTemplate: name + " in 10",
			TTSVolume:   100,
		}}
	}
	return t
}

// raidPackDefs lists every raid pack table, oldest content first.
func raidPackDefs() []raidPackDef {
	return []raidPackDef{
		raidPoPTier1(),
		raidPoPTier2(),
		raidPoPTier3(),
		raidPoPFire(),
		raidPoPAir(),
		raidPoPWater(),
	}
}

// raidPacks returns every built-in raid pack.
func raidPacks() []TriggerPack {
	defs := raidPackDefs()
	out := make([]TriggerPack, len(defs))
	for i, d := range defs {
		out[i] = d.build()
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
