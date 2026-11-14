package customoverlay

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type fakeHub struct {
	mu     sync.Mutex
	events []ws.Event
}

func (f *fakeHub) Broadcast(ev ws.Event) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
}

func (f *fakeHub) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

var t0 = time.Date(2026, 11, 13, 21, 0, 0, 0, time.UTC)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestEngine(t *testing.T, store *Store, self string) (*Engine, *fakeHub) {
	t.Helper()
	hub := &fakeHub{}
	e := NewEngine(store, hub, func() string { return self })
	e.now = func() time.Time { return t0 }
	return e, hub
}

// dkpDef is a bid board driven by a loot master's guild-chat calls and
// bidders' guild-chat numbers.
func dkpDef() OverlayDef {
	return OverlayDef{
		Name:    "DKP Bids",
		Enabled: true,
		Widgets: []Widget{
			{ID: "status", Type: WidgetFields, Fields: []Field{
				{Key: "item", Label: "Item"},
				{Key: "status", Label: "Status", Colors: []ColorRule{{Contains: "sold", Color: "#4ade80"}}},
				{Key: "winner", Label: "Winner"},
			}},
			{ID: "bids", Type: WidgetTable, Columns: []Column{{Key: "bidder", Label: "Bidder"}, {Key: "amount", Label: "DKP"}},
				SortBy: "amount", SortDesc: true, Merge: MergeHighest, MergeColumn: "amount"},
			{ID: "log", Type: WidgetFeed, MaxLines: 3},
			{ID: "timer", Type: WidgetCountdown, Seconds: 30},
		},
		Rules: []Rule{
			{ID: "open", Enabled: true, Channel: ChannelGuild, Speakers: []string{"Lootmaster"},
				Pattern: `^Bids open on (?P<item>.+?)[.!]?$`,
				Ops: []Op{
					{Type: OpResetWidget, Widget: "bids"},
					{Type: OpSetField, Widget: "status", Field: "item", Value: "{item}"},
					{Type: OpSetField, Widget: "status", Field: "status", Value: "Open"},
					{Type: OpClearField, Widget: "status", Field: "winner"},
					{Type: OpCountdownStart, Widget: "timer"},
				}},
			{ID: "bid", Enabled: true, Channel: ChannelGuild, Direction: DirectionIn, Pattern: `^(\d+)$`,
				Ops: []Op{
					{Type: OpUpsertRow, Widget: "bids", Key: "{speaker}", Cells: map[string]string{"amount": "$1"}},
					{Type: OpAddLine, Widget: "log", Value: "{speaker} bid {1}"},
				}},
			{ID: "closing", Enabled: true, Channel: ChannelGuild, Pattern: `CLOSING`,
				Ops: []Op{{Type: OpSetField, Widget: "status", Field: "status", Value: "CLOSING"}}},
			{ID: "sold", Enabled: true, Channel: ChannelGuild, Pattern: `^SOLD! .+ to (\w+) for (\d+)`,
				Ops: []Op{
					{Type: OpSetField, Widget: "status", Field: "status", Value: "SOLD"},
					{Type: OpSetField, Widget: "status", Field: "winner", Value: "{1} ({2})"},
					{Type: OpCountdownStop, Widget: "timer"},
				}},
		},
	}
}

func TestDKPFlow(t *testing.T) {
	e, hub := newTestEngine(t, nil, "Healer")
	o, err := e.Create(dkpDef())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	lines := []string{
		"Lootmaster tells the guild, 'Bids open on Robe of the Oracle'",
		"Bidder tells the guild, '50'",
		"Another tells the guild, '75'",
		"Bidder tells the guild, '60'",  // below their 50? no, above — raises
		"Another tells the guild, '20'", // lower than 75: ignored by highest
		"Randomguy tells the guild, 'grats all'",
		"Lootmaster tells the guild, 'Robe of the Oracle CLOSING'",
		"You say to your guild, '99'", // outgoing: excluded by Direction in
	}
	for i, l := range lines {
		e.HandleLine(t0.Add(time.Duration(i)*time.Second), l)
	}
	snap := e.State().Overlays[0]
	if snap.Def.ID != o.Def.ID {
		t.Fatalf("wrong overlay")
	}
	st := snap.State.Widgets
	if got := st["status"].Fields; got["item"] != "Robe of the Oracle" || got["status"] != "CLOSING" {
		t.Errorf("fields = %v", got)
	}
	var order []string
	for _, r := range st["bids"].Rows {
		order = append(order, r.Key+"="+r.Cells["amount"])
	}
	if got := strings.Join(order, ","); got != "Another=75,Bidder=60" {
		t.Errorf("rows = %s", got)
	}
	if n := len(st["log"].Lines); n != 3 {
		t.Errorf("feed lines = %d, want capped at 3", n)
	}
	if st["log"].Lines[0].Text != "Another bid 20" {
		t.Errorf("feed newest = %q", st["log"].Lines[0].Text)
	}
	if want := t0.Add(30 * time.Second).UnixMilli(); st["timer"].EndsAt != want {
		t.Errorf("countdown ends %d, want %d", st["timer"].EndsAt, want)
	}

	e.HandleLine(t0.Add(time.Minute), "Lootmaster tells the guild, 'SOLD! Robe of the Oracle to Another for 75'")
	st = e.State().Overlays[0].State.Widgets
	if st["status"].Fields["winner"] != "Another (75)" || st["timer"].EndsAt != 0 {
		t.Errorf("after sold: fields=%v endsAt=%d", st["status"].Fields, st["timer"].EndsAt)
	}

	// A non-loot-master "Bids open" doesn't reset the table.
	e.HandleLine(t0.Add(2*time.Minute), "Prankster tells the guild, 'Bids open on Nothing'")
	if n := len(e.State().Overlays[0].State.Widgets["bids"].Rows); n != 2 {
		t.Errorf("speaker filter failed: rows = %d", n)
	}
	if hub.count() == 0 {
		t.Error("expected broadcasts")
	}
}

func TestChannelHelper(t *testing.T) {
	base := func(r Rule) OverlayDef {
		r.ID, r.Enabled = "r", true
		r.Ops = []Op{{Type: OpAddLine, Widget: "f", Value: "{speaker}|{channel}|{message}|{1}"}}
		return OverlayDef{Name: "x", Enabled: true, Widgets: []Widget{{ID: "f", Type: WidgetFeed}}, Rules: []Rule{r}}
	}
	tests := []struct {
		name string
		rule Rule
		line string
		want string // "" = no match
	}{
		{"guild in", Rule{Channel: "guild", Pattern: `(\d+)`}, "Bob tells the guild, 'bid 40'", "Bob|guild|bid 40|40"},
		{"guild out uses self", Rule{Channel: "guild"}, "You say to your guild, 'hello'", "Me|guild|hello|{1}"},
		{"raid double space", Rule{Channel: "raid"}, "Bob tells the raid,  'go'", "Bob|raid|go|{1}"},
		{"wrong channel", Rule{Channel: "raid"}, "Bob tells the guild, 'go'", ""},
		{"any channel tell", Rule{Channel: "any"}, "Bob tells you, 'hi'", "Bob|tell|hi|{1}"},
		{"say", Rule{Channel: "say"}, "Bob says, 'hail'", "Bob|say|hail|{1}"},
		{"named channel", Rule{Channel: "dkp"}, "Bob tells Dkp:3, 'x'", "Bob|dkp|x|{1}"},
		{"direction out only", Rule{Channel: "guild", Direction: "out"}, "Bob tells the guild, 'x'", ""},
		{"speaker filter case-insensitive", Rule{Channel: "guild", Speakers: []string{"bob"}}, "Bob tells the guild, 'x'", "Bob|guild|x|{1}"},
		{"speaker filter rejects", Rule{Channel: "guild", Speakers: []string{"Ann"}}, "Bob tells the guild, 'x'", ""},
		{"non-chat with channel", Rule{Channel: "any"}, "You have entered The North Karana.", ""},
		{"raw pattern whole line", Rule{Pattern: `^You have entered (.+)\.$`}, "You have entered The North Karana.", "{speaker}|{channel}|You have entered The North Karana.|The North Karana"},
		{"merchant tell excluded", Rule{Channel: "tell"}, "Merchant tells you, 'That'll be 5 platinum for the item.'", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newTestEngine(t, nil, "Me")
			res := e.TestLine(base(tc.rule), tc.line, "")
			if res.Error != "" {
				t.Fatalf("error: %s", res.Error)
			}
			lines := res.State.Widgets["f"]
			got := ""
			if lines != nil && len(lines.Lines) > 0 {
				got = lines.Lines[0].Text
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTableMerge(t *testing.T) {
	tests := []struct {
		merge string
		bids  []string
		want  string
	}{
		{MergeReplace, []string{"50", "20"}, "20"},
		{MergeHighest, []string{"50", "20", "70"}, "70"},
		{MergeHighest, []string{"50", "abc"}, "50"},
		{MergeAdd, []string{"1", "1", "3"}, "5"},
		{MergeAdd, []string{"1.5", "2"}, "3.5"},
	}
	for _, tc := range tests {
		t.Run(tc.merge+"/"+strings.Join(tc.bids, ","), func(t *testing.T) {
			def := OverlayDef{Name: "t", Enabled: true,
				Widgets: []Widget{{ID: "t", Type: WidgetTable, Columns: []Column{{Key: "who"}, {Key: "n"}},
					Merge: tc.merge, MergeColumn: "n"}},
				Rules: []Rule{{ID: "r", Enabled: true, Pattern: `^(\w+) (\S+)$`,
					Ops: []Op{{Type: OpUpsertRow, Widget: "t", Key: "{1}", Cells: map[string]string{"n": "{2}"}}}}},
			}
			e, _ := newTestEngine(t, nil, "")
			if _, err := e.Create(def); err != nil {
				t.Fatalf("Create: %v", err)
			}
			for _, b := range tc.bids {
				e.HandleLine(t0, "bob "+b)
			}
			rows := e.State().Overlays[0].State.Widgets["t"].Rows
			if len(rows) != 1 || rows[0].Cells["n"] != tc.want || rows[0].Cells["who"] != "bob" {
				t.Errorf("rows = %+v, want n=%s", rows, tc.want)
			}
		})
	}
}

func TestTableSortAndCap(t *testing.T) {
	w := &Widget{Type: WidgetTable, Columns: []Column{{Key: "k"}, {Key: "v"}}, SortBy: "v", SortDesc: true, MaxRows: 3}
	rows := []Row{
		{Key: "a", Cells: map[string]string{"v": "10"}, Seq: 1},
		{Key: "b", Cells: map[string]string{"v": ""}, Seq: 2},
		{Key: "c", Cells: map[string]string{"v": "1,000"}, Seq: 3},
		{Key: "d", Cells: map[string]string{"v": "10"}, Seq: 4},
	}
	var got []string
	for _, r := range sortedRows(w, rows) {
		got = append(got, r.Key)
	}
	if s := strings.Join(got, ""); s != "cadb" {
		t.Errorf("sorted = %s, want cadb (numeric desc, ties by arrival, blanks last)", s)
	}

	ws, st := &WidgetState{}, &OverlayState{}
	for _, k := range []string{"a", "b", "c", "d"} {
		upsertRow(w, ws, st, k, map[string]string{"k": k}, 0)
	}
	if len(ws.Rows) != 3 || ws.Rows[0].Key != "b" {
		t.Errorf("cap kept %+v, want oldest dropped", ws.Rows)
	}
}

func TestCounterAndCountdownOps(t *testing.T) {
	def := OverlayDef{Name: "c", Enabled: true,
		Widgets: []Widget{{ID: "n", Type: WidgetCounter}, {ID: "cd", Type: WidgetCountdown}},
		Rules: []Rule{
			{ID: "inc", Enabled: true, Pattern: `^tick$`, Ops: []Op{{Type: OpCounterAdd, Widget: "n", Amount: 2}}},
			{ID: "set", Enabled: true, Pattern: `^set (\S+)$`, Ops: []Op{{Type: OpCounterSet, Widget: "n", Value: "{1}"}}},
			{ID: "go", Enabled: true, Pattern: `^start (\S+)$`, Ops: []Op{{Type: OpCountdownStart, Widget: "cd", Value: "{1}"}}},
			{ID: "off", Enabled: false, Pattern: `^tick$`, Ops: []Op{{Type: OpCounterAdd, Widget: "n", Amount: 100}}},
		},
	}
	e, _ := newTestEngine(t, nil, "")
	if _, err := e.Create(def); err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.HandleLine(t0, "tick")
	e.HandleLine(t0, "tick")
	st := e.State().Overlays[0].State.Widgets
	if st["n"].Count != 4 {
		t.Errorf("count = %d, want 4 (disabled rule ignored)", st["n"].Count)
	}
	e.HandleLine(t0, "set 1,234")
	e.HandleLine(t0, "set nope")
	e.HandleLine(t0, "start 1m30s")
	st = e.State().Overlays[0].State.Widgets
	if st["n"].Count != 1234 {
		t.Errorf("count = %d, want 1234", st["n"].Count)
	}
	if want := t0.Add(90 * time.Second).UnixMilli(); st["cd"].EndsAt != want {
		t.Errorf("endsAt = %d, want %d", st["cd"].EndsAt, want)
	}
}

func TestCharacterScope(t *testing.T) {
	def := dkpDef()
	def.Characters = []string{"Lootmaster"}
	e, _ := newTestEngine(t, nil, "Healer")
	if _, err := e.Create(def); err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.HandleLine(t0, "Bidder tells the guild, '50'")
	if ws := e.State().Overlays[0].State.Widgets["bids"]; ws != nil && len(ws.Rows) > 0 {
		t.Error("overlay limited to another character still ran")
	}
}

func TestPersistenceAndManualActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	store := openTestStore(t, path)
	e, _ := newTestEngine(t, store, "Me")
	o, err := e.Create(dkpDef())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := o.Def.ID
	e.HandleLine(t0, "Lootmaster tells the guild, 'Bids open on Robe'")
	e.HandleLine(t0, "Ann tells the guild, '10'")
	e.HandleLine(t0, "Bob tells the guild, '20'")

	e2, _ := newTestEngine(t, store, "Me")
	st := e2.State().Overlays[0].State.Widgets
	if st["status"].Fields["item"] != "Robe" || len(st["bids"].Rows) != 2 {
		t.Fatalf("state not restored: %+v", st)
	}

	if err := e2.RemoveRow(id, "bids", "ann"); err != nil {
		t.Fatalf("RemoveRow: %v", err)
	}
	if rows := e2.State().Overlays[0].State.Widgets["bids"].Rows; len(rows) != 1 || rows[0].Key != "Bob" {
		t.Errorf("after remove: %+v", rows)
	}

	// Editing the definition keeps state for surviving widgets only.
	def := e2.State().Overlays[0].Def
	def.Widgets = def.Widgets[:2] // drop feed + countdown
	def.Rules = def.Rules[2:3]    // keep a rule that only targets status
	if _, err := e2.Update(def); err != nil {
		t.Fatalf("Update: %v", err)
	}
	st = e2.State().Overlays[0].State.Widgets
	if _, ok := st["log"]; ok || len(st["bids"].Rows) != 1 {
		t.Errorf("prune after update: %+v", st)
	}

	if err := e2.Reset(id); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if n := len(e2.State().Overlays[0].State.Widgets); n != 0 {
		t.Errorf("reset left %d widget states", n)
	}
	if err := e2.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	e3, _ := newTestEngine(t, store, "")
	if n := len(e3.State().Overlays); n != 0 {
		t.Errorf("deleted overlay came back: %d", n)
	}
	if err := e3.Reset("missing"); err != ErrNotFound {
		t.Errorf("Reset(missing) = %v", err)
	}
}

func TestTestLineStartsFromSavedState(t *testing.T) {
	e, hub := newTestEngine(t, nil, "Me")
	o, err := e.Create(dkpDef())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.HandleLine(t0, "Ann tells the guild, '10'")
	before := hub.count()
	res := e.TestLine(o.Def, "Bob tells the guild, '20'", "")
	if strings.Join(res.Matched, ",") != "bid" || len(res.State.Widgets["bids"].Rows) != 2 {
		t.Errorf("test result = %+v", res)
	}
	if hub.count() != before || len(e.State().Overlays[0].State.Widgets["bids"].Rows) != 1 {
		t.Error("TestLine changed live state or broadcast")
	}
}

func TestValidate(t *testing.T) {
	ok := dkpDef()
	tests := []struct {
		name string
		mut  func(d *OverlayDef)
		want string // substring of the error; "" = valid
	}{
		{"valid", func(d *OverlayDef) {}, ""},
		{"no name", func(d *OverlayDef) { d.Name = "  " }, "name is required"},
		{"no widgets", func(d *OverlayDef) { d.Widgets = nil }, "at least one widget"},
		{"bad regex", func(d *OverlayDef) { d.Rules[0].Pattern = `(` }, "pattern"},
		{"lookahead", func(d *OverlayDef) { d.Rules[0].Pattern = `foo(?=bar)` }, "lookahead"},
		{"unknown widget", func(d *OverlayDef) { d.Rules[0].Ops[0].Widget = "nope" }, "unknown widget"},
		{"op type mismatch", func(d *OverlayDef) { d.Rules[0].Ops[1].Widget = "bids" }, "can't target"},
		{"unknown field", func(d *OverlayDef) { d.Rules[0].Ops[1].Field = "zzz" }, "unknown field"},
		{"unknown column", func(d *OverlayDef) { d.Rules[1].Ops[0].Cells = map[string]string{"zz": "1"} }, "unknown column"},
		{"merge needs column", func(d *OverlayDef) { d.Widgets[1].MergeColumn = "" }, "merge column"},
		{"bad sort", func(d *OverlayDef) { d.Widgets[1].SortBy = "zz" }, "sort column"},
		{"dup widget", func(d *OverlayDef) { d.Widgets[2].ID = "bids" }, "duplicate"},
		{"no pattern no channel", func(d *OverlayDef) { d.Rules[2].Channel = ""; d.Rules[2].Pattern = "" }, "pattern is required"},
		{"speakers need channel", func(d *OverlayDef) { d.Rules[0].Channel = "" }, "need a channel"},
		{"bad channel", func(d *OverlayDef) { d.Rules[0].Channel = "a b" }, "invalid channel"},
		{"too many rules", func(d *OverlayDef) {
			for len(d.Rules) <= maxRules {
				d.Rules = append(d.Rules, Rule{Pattern: "x", Ops: []Op{{Type: OpResetAll}}})
			}
		}, "at most"},
		{"unknown widget type", func(d *OverlayDef) { d.Widgets[2].Type = "html" }, "unknown widget type"},
		{"countdown needs duration", func(d *OverlayDef) { d.Widgets[3].Seconds = 0 }, "duration"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := dkpDef()
			tc.mut(&d)
			_, err := Validate(d)
			switch {
			case tc.want == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Errorf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
	// Normalization: generated IDs, defaulted limits.
	ok.Rules = append(ok.Rules, Rule{Enabled: true, Pattern: "x", Ops: []Op{{Type: OpResetAll}}})
	ok.Widgets[2].MaxLines = 9999
	got, err := Validate(ok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rules[len(got.Rules)-1].ID == "" || got.Widgets[2].MaxLines != limitMaxLines || got.Widgets[1].MaxRows != defaultMaxRows {
		t.Errorf("normalization: %+v", got)
	}
}

func TestReplayRestoresState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.db")
	store := openTestStore(t, path)
	e, _ := newTestEngine(t, store, "Me")
	o, err := e.Create(dkpDef())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.HandleLine(t0, "Ann tells the guild, '10'")

	e.SetReplaying(true)
	e.SetReplaying(true) // pause/resume style repeat is a no-op
	e.HandleLine(t0, "Old tells the guild, '99'")
	if n := len(e.State().Overlays[0].State.Widgets["bids"].Rows); n != 2 {
		t.Fatalf("replayed line not shown live: rows = %d", n)
	}
	// A definition edit mid-replay saves, but with pre-replay contents.
	def := o.Def
	def.Name = "Renamed"
	if _, err := e.Update(def); err != nil {
		t.Fatalf("Update: %v", err)
	}
	e.HandleLine(t0, "Older tells the guild, '5'")
	disk, _ := newTestEngine(t, store, "Me")
	if rows := disk.State().Overlays[0].State.Widgets["bids"].Rows; len(rows) != 1 || disk.State().Overlays[0].Def.Name != "Renamed" {
		t.Errorf("disk during replay: name=%q rows=%+v", disk.State().Overlays[0].Def.Name, rows)
	}

	e.SetReplaying(false)
	rows := e.State().Overlays[0].State.Widgets["bids"].Rows
	if len(rows) != 1 || rows[0].Key != "Ann" {
		t.Errorf("after replay rows = %+v, want just Ann", rows)
	}
	e.HandleLine(t0, "Bob tells the guild, '20'")
	disk2, _ := newTestEngine(t, store, "Me")
	if n := len(disk2.State().Overlays[0].State.Widgets["bids"].Rows); n != 2 {
		t.Errorf("saving didn't resume after replay: rows on disk = %d", n)
	}
}
