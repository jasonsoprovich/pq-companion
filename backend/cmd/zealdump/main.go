// Command zealdump prints the target fields Zeal publishes on its named pipe.
//
// It exists to verify a Zeal build end to end without needing any PQ Companion
// UI wired up: it dials the same pipe the app uses, decodes with the same
// zealpipe package, and reports what the player message (type 3) actually
// carries for the current target.
//
// It is a developer tool. It is not built into or shipped with the app.
//
//	go run ./cmd/zealdump            # summary line per target change
//	go run ./cmd/zealdump -raw       # every player payload, verbatim
//	go run ./cmd/zealdump -every     # every tick, not just on change
//	go run ./cmd/zealdump -types     # count envelopes per type; sample per type
//	go run ./cmd/zealdump -deaths    # one line per death event (type 7)
//	go run ./cmd/zealdump -items     # one line per item window opened
//	go run ./cmd/zealdump -considers # one line per consider event
//	go run ./cmd/zealdump -file cap  # replay a capture instead of dialing
//
// To keep a capture for later: go run ./cmd/zealdump -raw > capture.txt
//
// Windows only in practice — the named-pipe namespace does not exist
// elsewhere, so on other platforms it exits with "no Zeal pipe found".
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// targetKeys are the player-message keys that describe the current target.
// Listed in the order we want them printed, not the order Zeal emits them.
var targetKeys = []string{
	"target_id",
	"target_name",
	"target_type",
	"target_level",
	"target_class",
	"target_race",
	"target_loc",
	"target_texture",
	"target_gender",
	"target_size",
	"target_equipment",
}

// spawnTypeNames maps Zeal's EntityTypes enum (Zeal/game_structures.h:132).
var spawnTypeNames = map[int]string{
	0: "player",
	1: "NPC",
	2: "NPC corpse",
	3: "player corpse",
	4: "unknown",
}

func main() {
	raw := flag.Bool("raw", false, "print every player payload verbatim")
	every := flag.Bool("every", false, "print every tick, not just when the target changes")
	types := flag.Bool("types", false, "count envelopes per type; print one sample payload per type (diagnoses which message types a Zeal build actually emits — e.g. raid/type 5)")
	deaths := flag.Bool("deaths", false, "print one line per death event (type 7, proposed Zeal PR) instead of target changes")
	items := flag.Bool("items", false, "print one line per item display event (proposed Zeal PR; type 7 or 8) instead of target changes")
	considers := flag.Bool("considers", false, "print one line per consider event (proposed Zeal PR; type 7 alone, 9 on the combined build) instead of target changes")
	pipeName := flag.String("pipe", "", "pipe to read (default: first one discovered)")
	fromFile := flag.String("file", "", "read a saved capture instead of dialing a pipe")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var conn io.ReadCloser
	if *fromFile != "" {
		f, err := os.Open(*fromFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open %s: %v\n", *fromFile, err)
			os.Exit(1)
		}
		conn = f
		fmt.Printf("replaying %s\n\n", *fromFile)
	} else {
		conn = dialPipe(ctx, *pipeName)
	}
	defer conn.Close()

	if *deaths {
		runDeaths(ctx, conn)
		return
	}
	if *items {
		runItems(ctx, conn)
		return
	}
	if *considers {
		runConsiders(ctx, conn)
		return
	}
	run(ctx, conn, *raw, *every, *types)
}

// runDeaths prints one line per death event with a running total, so a
// tester can compare the count against the "has been slain" lines in the
// EQ log for the same stretch of play.
func runDeaths(ctx context.Context, conn io.Reader) {
	fmt.Println("waiting for death events (type 7). Ctrl+C to stop.")
	n := 0
	dec := json.NewDecoder(conn)
	for ctx.Err() == nil {
		var env zealpipe.Envelope
		if err := dec.Decode(&env); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "\nread ended: %v\n", err)
			}
			break
		}
		if env.Type != zealpipe.MsgDeath {
			continue
		}
		n++
		fmt.Printf("%s #%d %s\n", time.Now().Format("15:04:05"), n, formatDeath(env.Data))
	}
	fmt.Printf("\n%d death events\n", n)
}

// runItems prints one line per item display event. The message type isn't
// settled (7 alone, 8 alongside the death PR), so any envelope whose payload
// decodes as an item counts, and the type it arrived under is shown.
func runItems(ctx context.Context, conn io.Reader) {
	fmt.Println("waiting for item display events. Click an item link or inspect an item. Ctrl+C to stop.")
	n := 0
	dec := json.NewDecoder(conn)
	for ctx.Err() == nil {
		var env zealpipe.Envelope
		if err := dec.Decode(&env); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "\nread ended: %v\n", err)
			}
			break
		}
		if env.Type != 7 && env.Type != 8 {
			continue
		}
		it, err := zealpipe.DecodeItemDisplay(env.Data)
		if err != nil {
			continue
		}
		n++
		fmt.Printf("%s #%d type=%d item_id=%d name=%s\n", time.Now().Format("15:04:05"), n, env.Type, it.ItemID, it.Name)
	}
	fmt.Printf("\n%d item display events\n", n)
}

// runConsiders prints one line per consider event. Like item display, the
// message type isn't settled, so any envelope from type 7 up whose payload is
// shaped like a consider counts, and the type it arrived under is shown.
func runConsiders(ctx context.Context, conn io.Reader) {
	fmt.Println("waiting for consider events. Con an NPC (or click one with /leftclickcon on). Ctrl+C to stop.")
	n := 0
	dec := json.NewDecoder(conn)
	for ctx.Err() == nil {
		var env zealpipe.Envelope
		if err := dec.Decode(&env); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "\nread ended: %v\n", err)
			}
			break
		}
		line, ok := formatConsider(env)
		if !ok {
			continue
		}
		n++
		fmt.Printf("%s #%d %s\n", time.Now().Format("15:04:05"), n, line)
	}
	fmt.Printf("\n%d consider events\n", n)
}

// formatConsider renders one consider envelope as a single line, or ok=false
// when the envelope isn't a consider. A consider-shaped payload with a
// faction outside 1..9 is still shown, flagged, since that's what a tester
// needs to see.
func formatConsider(env zealpipe.Envelope) (string, bool) {
	if env.Type < zealpipe.MsgDeath {
		return "", false
	}
	c, err := zealpipe.DecodeConsider(env.Data)
	if errors.Is(err, zealpipe.ErrNotConsider) {
		return "", false
	}
	if err != nil {
		return fmt.Sprintf("type=%d UNUSABLE %v: %s", env.Type, err, env.Data), true
	}
	return fmt.Sprintf("type=%d spawn_id=%d name=%s faction=%d (%s)",
		env.Type, c.SpawnID, c.Name, c.Faction, zealpipe.ConsiderStanding(c.Faction)), true
}

// formatDeath renders one death payload as a single line.
func formatDeath(payload string) string {
	d, err := zealpipe.DecodeDeath(payload)
	if err != nil {
		return "undecodable: " + payload
	}
	name := d.Name
	if name == "" {
		name = "(unknown spawn)"
	}
	typ := "?"
	if d.Type != nil {
		typ = spawnTypeNames[*d.Type]
	}
	lvl := "?"
	if d.Level != nil {
		lvl = fmt.Sprint(*d.Level)
	}
	killer := "no killer"
	if d.KillerID != 0 {
		killer = fmt.Sprintf("killer=%d %s", d.KillerID, d.KillerName)
	}
	return fmt.Sprintf("id=%d name=%s type=%s level=%s %s", d.SpawnID, name, typ, lvl, killer)
}

// dialPipe discovers and opens a live Zeal pipe, exiting with a diagnostic if
// there isn't exactly one obvious candidate.
func dialPipe(ctx context.Context, pipeName string) io.ReadCloser {
	name := pipeName
	if name == "" {
		refs, err := zealpipe.Discover()
		if err != nil {
			fmt.Fprintf(os.Stderr, "discover failed: %v\n", err)
			os.Exit(1)
		}
		if len(refs) == 0 {
			fmt.Fprintln(os.Stderr, "no Zeal pipe found — is EQ running with Zeal loaded and sound enabled?")
			os.Exit(1)
		}

		if len(refs) > 1 {
			fmt.Fprintf(os.Stderr, "note: %d pipes found, using the first (PID %d)\n", len(refs), refs[0].PID)
		}
		name = refs[0].Name
	}

	conn, err := zealpipe.Dial(ctx, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial %s: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("connected to %s — target an NPC in game. Ctrl+C to stop.\n\n", name)
	return conn
}

// run stream-decodes envelopes until EOF, error or cancellation, printing a
// line per player message and a coverage checklist at the end. With types,
// it instead counts every envelope by type and prints one verbatim sample
// payload per type — the "which types does this Zeal build actually emit"
// diagnostic. Prints a running summary line each time a count updates.
func run(ctx context.Context, conn io.Reader, raw, every, types bool) {
	if types {
		runTypes(ctx, conn)
		return
	}
	seen := newCoverage()
	last := ""
	dec := json.NewDecoder(conn)
	for {
		if ctx.Err() != nil {
			break
		}
		var env zealpipe.Envelope
		if err := dec.Decode(&env); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "\nread ended: %v\n", err)
			}
			break
		}
		if env.Type != zealpipe.MsgPlayer {
			continue
		}
		if raw {
			fmt.Println(env.Data)
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(env.Data), &payload); err != nil {
			fmt.Fprintf(os.Stderr, "decode player payload: %v\n", err)
			continue
		}
		seen.observe(payload)
		line := formatPlayer(payload)
		if every || line != last {
			fmt.Println(line)
			last = line
		}
	}
	fmt.Print("\n" + seen.report())
}

// formatPlayer renders one player payload as a single reviewable line. Kept
// free of I/O so it can be tested against captured fixtures.
func formatPlayer(p map[string]any) string {
	if _, ok := p["target_id"]; !ok {
		return "no target"
	}
	var parts []string
	for _, k := range targetKeys {
		v, ok := p[k]
		if !ok {
			continue
		}
		switch k {
		case "target_type":
			n, isNum := toInt(v)
			if isNum {
				if label, known := spawnTypeNames[n]; known {
					parts = append(parts, fmt.Sprintf("type=%d (%s)", n, label))
					continue
				}
			}
			parts = append(parts, fmt.Sprintf("type=%v", v))
		case "target_loc":
			parts = append(parts, "loc="+formatLoc(v))
			if d, ok := distance(p["location"], v); ok {
				// Rounded to 5 so walking around doesn't print a line per tick.
				parts = append(parts, fmt.Sprintf("dist≈%.0f", math.Round(d/5)*5))
			}
		default:
			parts = append(parts, strings.TrimPrefix(k, "target_")+"="+fmt.Sprintf("%v", compact(v)))
		}
	}
	if _, hasLoc := p["target_loc"]; !hasLoc && hasDescriptors(p) {
		// Expected, not a fault: Zeal withholds the position of a target 250+
		// units away (Project Quarm server policy).
		parts = append(parts, "loc=withheld (target 250+ away)")
	}
	missing := missingKeys(p)
	if len(missing) > 0 {
		parts = append(parts, "MISSING["+strings.Join(missing, ",")+"]")
	}
	return strings.Join(parts, " ")
}

// runTypes counts envelopes per pipe type and prints a running tally plus one
// verbatim sample payload per type. Built to answer "is Zeal emitting type 5
// (raid) at all?" on a live client without flooding the console.
func runTypes(ctx context.Context, conn io.Reader) {
	counts := map[zealpipe.PipeMessageType]int{}
	samples := map[zealpipe.PipeMessageType]string{}
	dec := json.NewDecoder(conn)
	for {
		if ctx.Err() != nil {
			break
		}
		var env zealpipe.Envelope
		if err := dec.Decode(&env); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "\nread ended: %v\n", err)
			}
			break
		}
		counts[env.Type]++
		if _, ok := samples[env.Type]; !ok {
			s := env.Data
			if len(s) > 400 {
				s = s[:400] + " …"
			}
			samples[env.Type] = s
			fmt.Printf("FIRST %v (type %d) char=%s: %s\n", env.Type, env.Type, env.Character, s)
		}
		if counts[env.Type]%100 == 0 {
			fmt.Printf("… %v count=%d\n", env.Type, counts[env.Type])
		}
	}
	fmt.Println("\n=== envelope type totals ===")
	for t := zealpipe.MsgLog; t <= 20; t++ {
		if c, ok := counts[t]; ok {
			fmt.Printf("type %d (%v): %d\n", t, t, c)
		}
	}
	if len(counts) == 0 {
		fmt.Println("(no envelopes decoded)")
	}
}

// formatLoc renders Zeal's {x,y,z} object. Zeal's x/y are transposed relative
// to the game's /loc order — see zealpipe.Location — so both readings are
// printed to save the reader converting in their head.
func formatLoc(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	x, _ := toFloat(m["x"])
	y, _ := toFloat(m["y"])
	z, _ := toFloat(m["z"])
	return fmt.Sprintf("(%.1f,%.1f,%.1f) [game /loc %.1f,%.1f]", x, y, z, y, x)
}

// hasDescriptors reports whether a payload carries the target-descriptor
// change (keyed on target_level, which is always sent alongside a target).
func hasDescriptors(p map[string]any) bool {
	_, ok := p["target_level"]
	return ok
}

// distance is the 3D distance between two Zeal {x,y,z} objects. Both share
// Zeal's axis order, so no transposition is needed.
func distance(a, b any) (float64, bool) {
	am, ok1 := a.(map[string]any)
	bm, ok2 := b.(map[string]any)
	if !ok1 || !ok2 {
		return 0, false
	}
	var sum float64
	for _, k := range []string{"x", "y", "z"} {
		av, _ := toFloat(am[k])
		bv, _ := toFloat(bm[k])
		sum += (av - bv) * (av - bv)
	}
	return math.Sqrt(sum), true
}

// appearanceKeys are the optional appearance fields (a later Zeal addition).
// They print when present but their absence is not a fault: v1.4.8 and
// earlier don't send them.
var appearanceKeys = map[string]bool{
	"target_texture": true, "target_gender": true, "target_size": true, "target_equipment": true,
}

// missingKeys lists target descriptors absent from a payload that does have a
// target_id — i.e. the Zeal build predates the target-descriptor change.
// target_loc alone being absent is not a fault on a build that has the
// change: it's withheld for targets 250+ units away.
func missingKeys(p map[string]any) []string {
	var missing []string
	for _, k := range targetKeys {
		if k == "target_id" || (k == "target_loc" && hasDescriptors(p)) || appearanceKeys[k] {
			continue
		}
		if _, ok := p[k]; !ok {
			missing = append(missing, strings.TrimPrefix(k, "target_"))
		}
	}
	return missing
}

func compact(v any) any {
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f)
	}
	return v
}

func toInt(v any) (int, bool) {
	f, ok := toFloat(v)
	return int(f), ok
}

func toFloat(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// coverage tracks which of the manual test conditions have actually been hit,
// so the operator gets a checklist at exit instead of having to remember.
type coverage struct {
	noTarget    bool
	locWithheld bool
	spawnTypes  map[int]bool
	keys        map[string]bool
	names       map[string]bool
}

func newCoverage() *coverage {
	return &coverage{
		spawnTypes: map[int]bool{},
		keys:       map[string]bool{},
		names:      map[string]bool{},
	}
}

func (c *coverage) observe(p map[string]any) {
	if _, ok := p["target_id"]; !ok {
		c.noTarget = true
		return
	}
	for _, k := range targetKeys {
		if _, ok := p[k]; ok {
			c.keys[k] = true
		}
	}
	if n, ok := toInt(p["target_type"]); ok {
		c.spawnTypes[n] = true
	}
	if _, hasLoc := p["target_loc"]; !hasLoc && hasDescriptors(p) {
		c.locWithheld = true
	}
	if s, ok := p["target_name"].(string); ok && s != "" {
		c.names[s] = true
	}
}

func (c *coverage) report() string {
	var b strings.Builder
	b.WriteString("---- coverage ----\n")
	b.WriteString(check(c.noTarget) + " saw a tick with no target (no target_* keys)\n")
	for _, k := range targetKeys {
		b.WriteString(check(c.keys[k]) + " saw " + k + "\n")
	}
	b.WriteString(check(c.locWithheld) + " saw target_loc withheld (target 250+ units away)\n")
	for _, t := range []int{0, 1, 2, 3} {
		b.WriteString(check(c.spawnTypes[t]) + fmt.Sprintf(" saw target_type=%d (%s)\n", t, spawnTypeNames[t]))
	}
	if len(c.names) > 0 {
		names := make([]string, 0, len(c.names))
		for n := range c.names {
			names = append(names, n)
		}
		sort.Strings(names)
		b.WriteString("\ndistinct target names seen: " + strings.Join(names, ", ") + "\n")
	}
	return b.String()
}

func check(ok bool) string {
	if ok {
		return "[x]"
	}
	return "[ ]"
}
