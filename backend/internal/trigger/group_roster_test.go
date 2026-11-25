package trigger

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// fakeClock is a settable clock for roster windows.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func newTestRoster() (*GroupRoster, *fakeClock) {
	c := &fakeClock{t: time.Date(2026, 11, 24, 20, 0, 0, 0, time.UTC)}
	r := NewGroupRoster()
	r.now = c.now
	return r, c
}

func TestGroupRoster_Check(t *testing.T) {
	r, c := newTestRoster()

	if _, known := r.Check("Vortikai", "Soandso"); known {
		t.Fatal("no pipe yet: roster should be unknown")
	}

	// Pipe live, no group frames: solo, known.
	r.ObservePipe("Vortikai")
	if member, known := r.Check("Vortikai", "Soandso"); member || !known {
		t.Errorf("solo: got member=%v known=%v, want false/true", member, known)
	}

	r.ObserveGroup("Vortikai", []string{"Vortikai", "Soandso", "Healbot"})
	for _, n := range []string{"Soandso", "soandso", "Healbot"} {
		if member, known := r.Check("vortikai", n); !member || !known {
			t.Errorf("%s: got member=%v known=%v, want true/true", n, member, known)
		}
	}
	if member, _ := r.Check("Vortikai", "Vortikai"); member {
		t.Error("own name must not count as a group member")
	}
	if member, _ := r.Check("Vortikai", "Stranger"); member {
		t.Error("non-member counted as member")
	}
	// A different character (multibox) has its own, unknown roster.
	if _, known := r.Check("Altchar", "Soandso"); known {
		t.Error("other character's roster should be unknown")
	}

	// Member dropped from frames (died / left) still counts for the TTL.
	c.add(rosterMemberTTL - time.Second)
	r.ObserveGroup("Vortikai", []string{"Vortikai", "Healbot"})
	if member, _ := r.Check("Vortikai", "Soandso"); !member {
		t.Error("recently seen member should still count")
	}
	c.add(2 * time.Second)
	r.ObservePipe("Vortikai")
	if member, known := r.Check("Vortikai", "Soandso"); member || !known {
		t.Errorf("after TTL: got member=%v known=%v, want false/true", member, known)
	}

	// Pipe goes quiet: unknown again.
	c.add(rosterPipeLive + time.Second)
	if _, known := r.Check("Vortikai", "Healbot"); known {
		t.Error("stale pipe should make the roster unknown")
	}
}

// {groupmember} gates the shipped "Group Member Died" trigger on the Zeal
// roster when it's live, and falls back to the name-shape excludes when not.
func TestEngine_GroupMemberToken(t *testing.T) {
	s := openTestStore(t)
	e := NewEngine(s, ws.NewHub(), nil, func() string { return "Vortikai" })
	r, c := newTestRoster()
	e.SetGroupRoster(r)

	var def Trigger
	for _, tr := range GeneralTriggersPack().Triggers {
		if tr.Name == "Group Member Died" {
			def = tr
		}
	}
	if def.Pattern != "{groupmember} has been slain by" {
		t.Fatalf("shipped pattern = %q", def.Pattern)
	}
	def.ID = "gmd"
	def.CreatedAt = time.Now().UTC()
	if err := s.Insert(&def); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	e.Reload()

	fires := func(line string) bool {
		before := len(e.GetHistory())
		e.Handle(time.Now(), line)
		return len(e.GetHistory()) > before
	}

	// No Zeal: any one-word victim fires, NPC-shaped victims don't.
	if !fires("Stranger has been slain by a gnoll!") {
		t.Error("no Zeal: player-shaped victim should fire")
	}
	if fires("a gnoll has been slain by Vortikai!") {
		t.Error("no Zeal: NPC victim should not fire")
	}

	// Zeal live, solo: nothing fires.
	r.ObservePipe("Vortikai")
	if fires("Stranger has been slain by a gnoll!") {
		t.Error("solo: non-member death fired")
	}

	// Zeal live, grouped: only members fire.
	r.ObserveGroup("Vortikai", []string{"Vortikai", "Soandso"})
	if !fires("Soandso has been slain by a gnoll guard!") {
		t.Error("grouped: member death should fire")
	}
	if fires("Stranger has been slain by a gnoll!") {
		t.Error("grouped: non-member death fired")
	}
	if fires("Sontalak has been slain by Soandso!") {
		t.Error("grouped: single-word named NPC fired")
	}

	// Log replay ignores the live roster.
	e.SetReplaying(true)
	if !fires("Stranger has been slain by a gnoll!") {
		t.Error("replay: should fall back to no-roster behaviour")
	}
	e.SetReplaying(false)

	// Pipe drops: back to the fallback.
	c.add(rosterPipeLive + time.Second)
	if !fires("Stranger has been slain by a gnoll!") {
		t.Error("stale pipe: should fall back to no-roster behaviour")
	}
}

func TestPlain_GroupMemberTokenRoundTrips(t *testing.T) {
	const plain = "{groupmember} has been slain by"
	re, err := PlainToRegex(plain)
	if err != nil || re != plain {
		t.Fatalf("PlainToRegex = %q, %v; want token passed through", re, err)
	}
	if back, err := RegexToPlain(re); err != nil || back != plain {
		t.Errorf("RegexToPlain = %q, %v", back, err)
	}
}
