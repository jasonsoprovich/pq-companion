package chatwindow

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type fakeHub struct {
	mu     sync.Mutex
	events []ws.Event
}

func (h *fakeHub) Broadcast(ev ws.Event) {
	h.mu.Lock()
	h.events = append(h.events, ev)
	h.mu.Unlock()
}

func (h *fakeHub) take() []ws.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.events
	h.events = nil
	return out
}

type fakeHistory struct {
	msgs []chat.Message
}

func (f *fakeHistory) Feed(ff chat.FeedFilters) ([]chat.Message, error) {
	var out []chat.Message
	for i := len(f.msgs) - 1; i >= 0; i-- { // newest first, like SortDesc
		m := f.msgs[i]
		if m.Character == ff.Character && m.Channel == ff.Channel {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeHistory) Channels(character string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, m := range f.msgs {
		if m.Character == character && !seen[m.Channel] {
			seen[m.Channel] = true
			out = append(out, m.Channel)
		}
	}
	return out, nil
}

var t0 = time.Date(2026, 11, 21, 20, 0, 0, 0, time.Local)

func newTestEngine(hist History) (*Engine, *fakeHub, *time.Time) {
	hub := &fakeHub{}
	now := t0
	e := NewEngine(nil, hub, hist, func() string { return "Osui" })
	e.now = func() time.Time { return now }
	return e, hub, &now
}

func mustCreate(t *testing.T, e *Engine, def WindowDef) WindowDef {
	t.Helper()
	def.Enabled = true
	v, err := e.Create(def)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return v
}

func texts(e *Engine, id string) []string {
	var out []string
	for _, w := range e.State().Windows {
		if w.Def.ID == id {
			for _, l := range w.State.Lines {
				out = append(out, l.Text)
			}
		}
	}
	return out
}

func window(e *Engine, id string) Window {
	for _, w := range e.State().Windows {
		if w.Def.ID == id {
			return w
		}
	}
	return Window{}
}

func TestEvaluate(t *testing.T) {
	def := WindowDef{
		Name:    "Raid",
		Sources: Sources{Channels: []string{"raid", "guild", "dkp"}},
		Keep: []Rule{
			{Name: "mentions me", Enabled: true, Text: "{c}", Color: "#ff0000", PinMinutes: 5},
			{Name: "leader", Enabled: true, Speakers: []string{"Sandrian"}, Channel: "raid"},
		},
		Hide: []Rule{
			{Name: "all caps spam", Enabled: true, Mode: MatchRegex, Text: `^[A-Z !]+$`, Channel: "guild"},
			{Name: "inc", Enabled: true, Kind: "contains", Text: "inc *"},
			{Name: "leader spam", Enabled: true, Speakers: []string{"Sandrian"}},
		},
	}
	v, err := Validate(def)
	if err != nil {
		t.Fatal(err)
	}
	cw := compileWindow(v, "Osui")
	cases := []struct {
		line string
		want string
		rule string
	}{
		{"Klaital tells the raid,  'Osui mez the adds'", VerdictKept, "mentions me"},
		{"Sandrian tells the raid,  'inc boss'", VerdictKept, "leader"},
		{"Klaital tells the raid,  'inc boss'", VerdictHidden, "inc"},
		{"Klaital tells the guild, 'WTS STUFF'", VerdictHidden, "all caps spam"},
		{"Sandrian tells the guild, 'hi'", VerdictHidden, "leader spam"},
		{"Klaital tells the raid,  'ready'", VerdictShown, ""},
		{"Klaital tells Dkp:3, 'bid 50'", VerdictShown, ""},
		{"Klaital tells the group, 'hi'", VerdictFiltered, ""},
		{"a geonid shaman begins to cast a spell.", VerdictFiltered, ""},
	}
	for _, tc := range cases {
		l := lineFromLog(t0, tc.line, "Osui")
		got := cw.evaluate(&l)
		rule := ""
		if got.rule != nil {
			rule = got.rule.Name
		}
		if got.result != tc.want || rule != tc.rule {
			t.Errorf("%q = %s (%s), want %s (%s)", tc.line, got.result, rule, tc.want, tc.rule)
		}
	}
}

func TestSources(t *testing.T) {
	cases := []struct {
		src  Sources
		line string
		want bool
	}{
		{Sources{Other: true}, "a geonid shaman begins to cast a spell.", true},
		{Sources{Channels: []string{"guild"}}, "a geonid shaman begins to cast a spell.", false},
		{Sources{AllNamed: true}, "Klaital tells General:1, 'lfg'", true},
		{Sources{AllNamed: true}, "Klaital tells the guild, 'hi'", false},
		{Sources{Channels: []string{"say"}}, "Klaital says, 'hail'", true},
		{Sources{Channels: []string{"tell"}}, "You told Bob, 'hi'", true},
	}
	for _, tc := range cases {
		v, err := Validate(WindowDef{Name: "w", Sources: tc.src})
		if err != nil {
			t.Fatal(err)
		}
		l := lineFromLog(t0, tc.line, "Osui")
		if got := compileWindow(v, "Osui").evaluate(&l).shown(); got != tc.want {
			t.Errorf("%+v on %q = %v", tc.src, tc.line, got)
		}
	}
}

func TestStandardText(t *testing.T) {
	cases := map[string]string{
		"Klaital tells the raid,  'go'":  "Klaital tells the raid, 'go'",
		"Klaital tells General:4, 'lfg'": "Klaital tells General, 'lfg'",
		"You told bob, 'hi'":             "You told Bob, 'hi'",
		"You tell your party, 'pull'":    "You tell your party, 'pull'",
		"Klaital says, 'hail'":           "Klaital says, 'hail'",
	}
	for in, want := range cases {
		if got := lineFromLog(t0, in, "Osui").Text; got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	// A history row rebuilds to the same text as the live line.
	live := lineFromLog(t0, "Klaital tells General:4, 'lfg'", "Osui")
	hist := lineFromHistory(chat.Message{Character: "Osui", Channel: "general", Direction: chat.DirectionIn,
		Peer: "Klaital", Message: "lfg", TS: t0.Unix()})
	if live.Text != hist.Text || keyOf(live) != keyOf(hist) {
		t.Errorf("live %+v and history %+v differ", live, hist)
	}
}

func TestRefillDedupesAndKeepsTellRecipients(t *testing.T) {
	hist := &fakeHistory{msgs: []chat.Message{
		{Character: "Osui", Channel: "guild", Direction: chat.DirectionIn, Peer: "Klaital", Message: "old", TS: t0.Add(-time.Hour).Unix()},
		{Character: "Osui", Channel: "guild", Direction: chat.DirectionIn, Peer: "Klaital", Message: "live too", TS: t0.Unix()},
		{Character: "Osui", Channel: "tell", Direction: chat.DirectionOut, Peer: "Bob", Message: "inv", TS: t0.Unix()},
		{Character: "Osui", Channel: "tell", Direction: chat.DirectionOut, Peer: "Amy", Message: "inv", TS: t0.Unix()},
		{Character: "Other", Channel: "guild", Direction: chat.DirectionIn, Peer: "X", Message: "not mine", TS: t0.Unix()},
	}}
	e, _, _ := newTestEngine(hist)
	e.HandleLine(t0, "Klaital tells the guild, 'live too'") // arrives before the window exists: ignored
	def := mustCreate(t, e, WindowDef{Name: "g", Sources: Sources{Channels: []string{"guild", "tell"}}})
	got := texts(e, def.ID)
	want := []string{"Klaital tells the guild, 'old'", "Klaital tells the guild, 'live too'",
		"You told Bob, 'inv'", "You told Amy, 'inv'"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("after refill:\n got %q\nwant %q", got, want)
	}

	// The live copy of a history line arriving now doesn't double it when a
	// later refill (another channel added) merges history again.
	e.HandleLine(t0, "Klaital tells the guild, 'live too'")
	def.Sources.Channels = append(def.Sources.Channels, "raid")
	if _, err := e.Update(def); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, s := range texts(e, def.ID) {
		if s == "Klaital tells the guild, 'live too'" {
			n++
		}
	}
	if n != 2 {
		// One from history, one live line seen after the window existed; the
		// refill must not add a third.
		t.Errorf("'live too' appears %d times, want 2", n)
	}
	seqs := map[int64]bool{}
	for _, l := range window(e, def.ID).State.Lines {
		if seqs[l.Seq] {
			t.Fatalf("duplicate seq %d", l.Seq)
		}
		seqs[l.Seq] = true
	}
}

func TestBatchesAndEpochs(t *testing.T) {
	e, hub, _ := newTestEngine(nil)
	def := mustCreate(t, e, WindowDef{Name: "g", Sources: Sources{Channels: []string{"guild"}},
		Keep: []Rule{{Enabled: true, Text: "pin me", PinMinutes: 1}}})
	hub.take()
	e.HandleLine(t0, "A tells the guild, 'one'")
	e.HandleLine(t0, "A tells the guild, 'pin me'")
	e.flush()
	evs := hub.take()
	if len(evs) != 1 || evs[0].Type != WSEventLines {
		t.Fatalf("events = %+v", evs)
	}
	b := evs[0].Data.(LinesEvent).Windows[0]
	ep := window(e, def.ID).State.Epoch
	if b.Epoch != ep || len(b.Lines) != 2 || b.Lines[0].Seq+1 != b.Lines[1].Seq || len(b.Pins) != 1 {
		t.Fatalf("batch = %+v (epoch %d)", b, ep)
	}

	// A queued append is dropped when a replacement goes out instead.
	e.HandleLine(t0, "A tells the guild, 'two'")
	if err := e.Dismiss(def.ID, b.Pins[0].Seq); err != nil {
		t.Fatal(err)
	}
	e.flush()
	evs = hub.take()
	if len(evs) != 1 || evs[0].Type != WSEventWindow {
		t.Fatalf("after dismiss: %+v", evs)
	}
	w := evs[0].Data.(Window)
	if w.State.Epoch <= ep || len(w.State.Pins) != 0 || len(w.State.Lines) != 3 {
		t.Errorf("replacement = epoch %d pins %d lines %d", w.State.Epoch, len(w.State.Pins), len(w.State.Lines))
	}
}

func TestPinsExpireAndCap(t *testing.T) {
	e, hub, now := newTestEngine(nil)
	def := mustCreate(t, e, WindowDef{Name: "g", Sources: Sources{Channels: []string{"raid"}},
		Keep: []Rule{{Enabled: true, Text: "", Channel: "raid", PinMinutes: 2}}})
	for i := 0; i < MaxPins+3; i++ {
		e.HandleLine(t0, fmt.Sprintf("A tells the raid,  'call %d'", i))
	}
	if n := len(window(e, def.ID).State.Pins); n != MaxPins {
		t.Fatalf("pins = %d, want cap %d", n, MaxPins)
	}
	hub.take()
	*now = now.Add(3 * time.Minute)
	e.expirePins() // no new chat needed
	evs := hub.take()
	if len(window(e, def.ID).State.Pins) != 0 || len(evs) != 1 || evs[0].Type != WSEventWindow {
		t.Errorf("after expiry: pins %d events %+v", len(window(e, def.ID).State.Pins), evs)
	}
}

func TestPinOutlivesEviction(t *testing.T) {
	e, _, _ := newTestEngine(nil)
	def := mustCreate(t, e, WindowDef{Name: "g", MaxLines: 3, Sources: Sources{Channels: []string{"raid"}},
		Keep: []Rule{{Enabled: true, Text: "assign", PinMinutes: 10}}})
	e.HandleLine(t0, "Lead tells the raid,  'assign: Osui on adds'")
	for i := 0; i < 5; i++ {
		e.HandleLine(t0, fmt.Sprintf("A tells the raid,  'x%d'", i))
	}
	w := window(e, def.ID)
	if len(w.State.Lines) != 3 || len(w.State.Pins) != 1 || !strings.Contains(w.State.Pins[0].Line.Text, "assign") {
		t.Errorf("lines %d pins %+v", len(w.State.Lines), w.State.Pins)
	}
}

func TestUpdateRefiltersKeepingProtectedLines(t *testing.T) {
	e, _, _ := newTestEngine(nil)
	def := mustCreate(t, e, WindowDef{Name: "r", Sources: Sources{Channels: []string{"raid", "guild"}},
		Keep: []Rule{{Enabled: true, Speakers: []string{"Lead"}}}})
	e.HandleLine(t0, "Lead tells the raid,  'inc'")
	e.HandleLine(t0, "Bob tells the raid,  'inc'")
	e.HandleLine(t0, "Bob tells the guild, 'hi'")
	def.Hide = []Rule{{Enabled: true, Text: "inc"}}
	def.Sources.Channels = []string{"raid"}
	if _, err := e.Update(def); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(texts(e, def.ID), "|")
	if got != "Lead tells the raid, 'inc'" {
		t.Errorf("after update: %q", got)
	}
}

func TestReplayRestores(t *testing.T) {
	e, _, _ := newTestEngine(nil)
	def := mustCreate(t, e, WindowDef{Name: "g", Sources: Sources{Channels: []string{"guild"}},
		Keep: []Rule{{Enabled: true, Text: "pin", PinMinutes: 5}}})
	e.HandleLine(t0, "A tells the guild, 'pin before'")
	before := window(e, def.ID).State
	e.SetReplaying(true)
	if n := len(window(e, def.ID).State.Lines); n != 0 {
		t.Fatalf("replay starts with %d lines", n)
	}
	e.HandleLine(t0, "A tells the guild, 'replayed'")
	during := window(e, def.ID).State
	e.SetReplaying(false)
	after := window(e, def.ID).State
	if len(after.Lines) != 1 || after.Lines[0].Text != before.Lines[0].Text || len(after.Pins) != 1 {
		t.Errorf("restored %+v", after)
	}
	if after.Seq < during.Seq || after.Epoch <= during.Epoch || during.Epoch <= before.Epoch {
		t.Errorf("seq/epoch went backwards: before %d/%d during %d/%d after %d/%d",
			before.Seq, before.Epoch, during.Seq, during.Epoch, after.Seq, after.Epoch)
	}
}

func TestSuggest(t *testing.T) {
	hist := &fakeHistory{msgs: []chat.Message{
		{Character: "Osui", Channel: "general", Direction: chat.DirectionIn, Peer: "Zed", Message: "WTS Cloak 500pp", TS: t0.Add(-time.Minute).Unix()},
	}}
	e, _, _ := newTestEngine(hist)
	e.SetNameSource(func() map[string]bool { return map[string]bool{"klaital": true} })
	def := mustCreate(t, e, WindowDef{Name: "w", Sources: Sources{AllNamed: true, Other: true, Channels: []string{"raid"}}})
	e.HandleLine(t0, "Bob tells General:1, 'WTS Cloak 500pp'")
	e.HandleLine(t0, "Klaital tells the raid,  'Klaital and Osui to 12, 34 and 56'")
	e.HandleLine(t0, "Klaital begins to cast a spell.")
	e.HandleLine(t0, "Soandso begins to cast a spell.")
	e.HandleLine(t0, "Bob tells General:1, 'price check * {x}'")

	lines := window(e, def.ID).State.Lines
	bySubstr := func(s string) Line {
		for _, l := range lines {
			if strings.Contains(l.Text, s) {
				return l
			}
		}
		t.Fatalf("no line %q", s)
		return Line{}
	}
	cases := []struct {
		name      string
		line      Line
		mode      string
		channel   string
		minMatch  int
		wantRegex bool
	}{
		{"backfilled named channel", lines[0], SuggestHide, "general", 2, false},
		{"live named channel", bySubstr("Bob tells General, 'WTS"), SuggestHide, "general", 2, false},
		{"many names and numbers", bySubstr("12, 34"), SuggestKeep, "raid", 1, false},
		{"other line, known name", bySubstr("Klaital begins"), SuggestHide, "", 2, false},
		{"other line, leading name", bySubstr("Soandso begins"), SuggestHide, "", 2, false},
		{"literal * and braces", bySubstr("price check"), SuggestHide, "general", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := e.Suggest(def.ID, tc.line.Seq, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			r := s.Rule
			if r.Channel != tc.channel || (r.Mode == MatchRegex) != tc.wantRegex || s.Matches < tc.minMatch {
				t.Errorf("rule %+v matches %d", r, s.Matches)
			}
			// Every suggestion is a valid rule that matches its own line.
			if _, err := Validate(WindowDef{Name: "x", Sources: Sources{Other: true}, Hide: []Rule{r}}); err != nil {
				t.Errorf("invalid suggestion %+v: %v", r, err)
			}
			cr, err := compileRule(r, "Osui")
			if err != nil || !cr.match(&tc.line) {
				t.Errorf("suggestion %+v doesn't match its line %q (%v)", r, tc.line.Text, err)
			}
			if tc.mode == SuggestKeep && r.Color == "" {
				t.Error("keep suggestion has no color")
			}
		})
	}
	if _, err := e.Suggest(def.ID, 9999, SuggestHide); err != ErrNotFound {
		t.Errorf("unknown seq: %v", err)
	}
}

func TestShareRoundTrip(t *testing.T) {
	e, _, _ := newTestEngine(nil)
	mustCreate(t, e, WindowDef{Name: "Guild", Characters: []string{"Osui"}, Sources: Sources{Channels: []string{"guild"}},
		Hide: []Rule{{Enabled: true, Text: "WTS *"}}, Appearance: Appearance{FontScale: 1.5, Opacity: 1}})
	sh, err := e.Export(nil)
	if err != nil {
		t.Fatal(err)
	}
	if sh.Windows[0].ID != "" || sh.Windows[0].Characters != nil {
		t.Fatalf("export kept personal fields: %+v", sh.Windows[0])
	}
	data := []byte(fmt.Sprintf(`{"format":%q,"version":1,"windows":[{"name":"Guild","sources":{"channels":["guild"]},"keep":[],"hide":[{"text":"WTS *","enabled":true}],"appearance":{"font_scale":1.5,"opacity":1},"max_lines":0}]}`, ShareFormat))
	e2, _, _ := newTestEngine(nil)
	got, err := e2.Import(data)
	if err != nil || len(got) != 1 || got[0].ID == "" || got[0].Appearance.FontScale != 1.5 || got[0].MaxLines != DefaultMaxLines {
		t.Fatalf("import = %+v, %v", got, err)
	}
	if _, err := ParseShare([]byte(`{"format":"other","windows":[]}`)); err == nil {
		t.Error("accepted a foreign file")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		def  WindowDef
		err  string
	}{
		{"no name", WindowDef{Sources: Sources{Other: true}}, "name"},
		{"no source", WindowDef{Name: "x"}, "pick at least one"},
		{"bad regex", WindowDef{Name: "x", Sources: Sources{Other: true}, Hide: []Rule{{Mode: MatchRegex, Text: "("}}}, "missing"},
		{"empty rule", WindowDef{Name: "x", Sources: Sources{Other: true}, Hide: []Rule{{}}}, "needs a pattern"},
		{"bad color", WindowDef{Name: "x", Sources: Sources{Other: true}, Keep: []Rule{{Text: "a", Color: "red"}}}, "#RRGGBB"},
		{"pin too long", WindowDef{Name: "x", Sources: Sources{Other: true}, Keep: []Rule{{Text: "a", PinMinutes: 61}}}, "pin minutes"},
		{"named channel slot dropped", WindowDef{Name: "x", Sources: Sources{Channels: []string{"General:3"}}}, ""},
	}
	for _, tc := range cases {
		v, err := Validate(tc.def)
		if tc.err == "" {
			if err != nil || v.Sources.Channels[0] != "general" {
				t.Errorf("%s: %v %+v", tc.name, err, v.Sources)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.err)
		}
	}
}

func TestSelfTokenRecompiles(t *testing.T) {
	name := "Osui"
	e := NewEngine(nil, nil, nil, func() string { return name })
	def := mustCreate(t, e, WindowDef{Name: "me", Sources: Sources{Channels: []string{"raid"}},
		Hide: []Rule{{Enabled: true, Channel: "raid"}}, Keep: []Rule{{Enabled: true, Text: "{c}"}}})
	e.HandleLine(t0, "A tells the raid,  'Osui go'")
	name = "Klaital"
	e.HandleLine(t0, "A tells the raid,  'Osui go again'")
	e.HandleLine(t0, "A tells the raid,  'Klaital go'")
	got := strings.Join(texts(e, def.ID), "|")
	if got != "A tells the raid, 'Osui go'|A tells the raid, 'Klaital go'" {
		t.Errorf("got %q", got)
	}
}

// BenchmarkHandleLine runs combat-heavy traffic through five windows, one
// with every other log line on and a few hide rules.
func BenchmarkHandleLine(b *testing.B) {
	e := NewEngine(nil, nil, nil, func() string { return "Osui" })
	for i := 0; i < 5; i++ {
		def := WindowDef{Name: fmt.Sprintf("w%d", i), Enabled: true,
			Sources: Sources{Channels: []string{"raid", "guild", "tell"}, Other: i == 0},
			Keep:    []Rule{{Enabled: true, Text: "{c}", Color: "#ff0000"}},
			Hide: []Rule{{Enabled: true, Text: "* begins to cast a spell."},
				{Enabled: true, Mode: MatchRegex, Text: `(?i)\b(slashes|pierces|hits|kicks)\b`}}}
		if _, err := e.Create(def); err != nil {
			b.Fatal(err)
		}
	}
	lines := []string{
		"A corrupted faun pierces a geonid for 240 points of damage.",
		"a geonid shaman begins to cast a spell.",
		"Klaital tells the raid,  'Osui mez the adds'",
		"You have been healed for 400 points.",
		"Bob tells the guild, 'lol'",
	}
	re := regexp.MustCompile(`x`)
	_ = re
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.HandleLine(t0, lines[i%len(lines)])
	}
}
