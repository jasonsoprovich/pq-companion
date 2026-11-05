package selfstate

import (
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

var testSpells = map[string][]int{
	"invisibility":               {spaInvisibility},
	"invisibility versus undead": {spaInvisUndead},
	"illusion: dark elf":         {spaIllusion},
	"alliance":                   {spaFaction},
	"spirit of wolf":             {3},
	"feign death":                {spaFeignDeathSpa},
}

func newTest(t *testing.T) (*Tracker, *time.Time, *int) {
	t.Helper()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	tr := New(func(name string) ([]int, bool) {
		e, ok := testSpells[strings.ToLower(name)]
		return e, ok
	})
	tr.now = func() time.Time { return now }
	tr.after = func(time.Duration, func()) {}
	calls := 0
	tr.OnChange(func() { calls++ })
	return tr, &now, &calls
}

func TestPipeBuffs(t *testing.T) {
	tr, now, calls := newTest(t)
	tr.SetPipeBuffs([]string{"Spirit of Wolf"})
	if s := tr.State(); s.Reason() != "" {
		t.Fatalf("plain buff: %+v", s)
	}
	if *calls != 0 {
		t.Errorf("unrelated buff notified %d times", *calls)
	}
	tr.SetPipeBuffs([]string{"Spirit of Wolf", "Invisibility"})
	if s := tr.State(); !s.Invisible || s.Reason() != "invisible" {
		t.Errorf("invis: %+v", s)
	}
	if *calls != 1 {
		t.Errorf("gaining invis: %d notifications, want 1", *calls)
	}
	tr.SetPipeBuffs([]string{"Spirit of Wolf", "Invisibility"}) // repeat tick
	if *calls != 1 {
		t.Errorf("unchanged buffs notified again")
	}
	tr.SetPipeBuffs([]string{"Illusion: Dark Elf"})
	if s := tr.State(); !s.Invisible || !s.Illusioned {
		t.Errorf("invis just went missing (could be a partial frame), illusion on: %+v", s)
	}
	*now = now.Add(buffLossConfirm)
	if s := tr.State(); s.Invisible {
		t.Errorf("invis missing past the confirm window: %+v", s)
	}
	tr.recheck() // what the scheduled recheck does
	tr.SetPipeBuffs([]string{"Illusion: Dark Elf", "Invisibility versus Undead"})
	if s := tr.State(); !s.Invisible {
		t.Errorf("IVU counts as invisible: %+v", s)
	}
	tr.SetPipeBuffs(nil)
	*now = now.Add(buffLossConfirm)
	if s := tr.State(); s != (State{}) {
		t.Errorf("no buffs: %+v", s)
	}
	tr.recheck()
	// invis on, illusion on, invis off, IVU on, everything off.
	if *calls != 5 {
		t.Errorf("notifications = %d, want 5", *calls)
	}
}

func TestFeignDeath(t *testing.T) {
	tr, now, calls := newTest(t)
	tr.Handle(logparser.LogEvent{Type: logparser.EventFeignDeath})
	if !tr.State().Feigned || *calls != 1 {
		t.Fatalf("feign: %+v calls=%d", tr.State(), *calls)
	}
	// A DoT tick doesn't stand the player up; a melee swing does.
	tr.Handle(logparser.LogEvent{Type: logparser.EventCombatHit, Data: logparser.CombatHitData{
		Actor: "You", Target: "a gnoll", Skill: "dot", SpellName: "Heat Blood"}})
	if !tr.State().Feigned {
		t.Error("DoT tick broke feign")
	}
	tr.Handle(logparser.LogEvent{Type: logparser.EventCombatHit, Data: logparser.CombatHitData{
		Actor: "You", Target: "a gnoll", Skill: "slash"}})
	if tr.State().Feigned || *calls != 2 {
		t.Errorf("melee should end feign: %+v calls=%d", tr.State(), *calls)
	}
	// Times out when the log never shows the player standing.
	tr.Handle(logparser.LogEvent{Type: logparser.EventFeignDeath})
	*now = now.Add(feignTTL + time.Second)
	if tr.State().Feigned {
		t.Error("feign should time out")
	}
}

func TestHideLines(t *testing.T) {
	tr, _, calls := newTest(t)
	tr.HandleLine("You have hidden yourself from view.")
	if !tr.State().Hidden {
		t.Fatal("hide not seen")
	}
	tr.HandleLine("You are no longer hidden.")
	if tr.State().Hidden || *calls != 2 {
		t.Errorf("unhide: %+v calls=%d", tr.State(), *calls)
	}
	tr.HandleLine("You feel yourself starting to appear.")
	if *calls != 3 {
		t.Error("invis starting to fade should notify")
	}
	tr.HandleLine("a gnoll says, 'hello'")
	if *calls != 3 {
		t.Error("unrelated line notified")
	}
}

func TestLogSpellPulses(t *testing.T) {
	tr, _, calls := newTest(t)
	tr.Handle(logparser.LogEvent{Type: logparser.EventSpellCast, Data: logparser.SpellCastData{SpellName: "Invisibility"}})
	if *calls != 1 {
		t.Errorf("casting invis: calls=%d", *calls)
	}
	tr.Handle(logparser.LogEvent{Type: logparser.EventSpellCast, Data: logparser.SpellCastData{SpellName: "Spirit of Wolf"}})
	if *calls != 1 {
		t.Errorf("unrelated cast notified")
	}
	tr.Handle(logparser.LogEvent{Type: logparser.EventIllusionFade, Data: logparser.IllusionFadeData{}})
	if *calls != 2 {
		t.Errorf("illusion fade: calls=%d", *calls)
	}
	tr.Handle(logparser.LogEvent{Type: logparser.EventSpellCast, Data: logparser.SpellCastData{SpellName: "Alliance"}})
	if !tr.State().FactionBuff {
		t.Error("Alliance cast should flag a faction buff")
	}
	tr.Handle(logparser.LogEvent{Type: logparser.EventZone, Data: logparser.ZoneData{ZoneName: "North Karana"}})
	if tr.State().FactionBuff {
		t.Error("zoning should clear the Alliance flag")
	}
}

func TestEngagedWith(t *testing.T) {
	tr, now, _ := newTest(t)
	if tr.EngagedWith("a gnoll", 0) {
		t.Fatal("engaged with nothing")
	}
	tr.Handle(logparser.LogEvent{Type: logparser.EventCombatMiss, Data: logparser.CombatMissData{
		Actor: "a gnoll", Target: "You", MissType: "miss"}})
	if !tr.EngagedWith("A Gnoll", 0) {
		t.Error("being attacked is engagement")
	}
	*now = now.Add(engagedWindow + time.Second)
	if tr.EngagedWith("a gnoll", 0) {
		t.Error("engagement should lapse")
	}
	id := 77
	tr.SetPipeAutoAttack(true, &id)
	if !tr.EngagedWith("anything", 77) || tr.EngagedWith("anything", 78) {
		t.Error("auto attack engages the targeted spawn only")
	}
	tr.SetPipeAutoAttack(false, &id)
	if tr.EngagedWith("anything", 77) {
		t.Error("auto attack off")
	}
}

func TestResetPipe(t *testing.T) {
	tr, _, calls := newTest(t)
	tr.SetPipeBuffs([]string{"Invisibility"})
	tr.ResetPipe()
	if tr.State().Invisible || *calls != 2 {
		t.Errorf("reset: %+v calls=%d", tr.State(), *calls)
	}
}

// A frame that drops the invis slot for a moment doesn't count as invis
// dropping.
func TestPipeBuffsPartialFrame(t *testing.T) {
	tr, now, calls := newTest(t)
	tr.SetPipeBuffs([]string{"Invisibility"})
	tr.SetPipeBuffs(nil)
	*now = now.Add(200 * time.Millisecond)
	tr.SetPipeBuffs([]string{"Invisibility"})
	*now = now.Add(buffLossConfirm)
	tr.recheck()
	if !tr.State().Invisible || *calls != 1 {
		t.Errorf("state=%+v calls=%d, want still invisible, one notification", tr.State(), *calls)
	}
}
