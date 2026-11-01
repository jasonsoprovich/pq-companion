package petoverlay

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// `/pet health` makes the player's pet report its HP and every effect it
// carries. With `/log on` the reply lands as ordinary timestamped lines, all in
// the same log-second:
//
//	I have 95 percent of my hit points left.
//	Tashanian
//	Boltran`s Agacerie
//	Speed of the Shissar
//	Primal Essence
//
// The header is a bare line (not "<pet> says, ..."), and each effect follows on
// its own bare line by exact spell name — beneficial and detrimental alike, and
// the charm spell itself on a charmed pet. No durations are printed. Ordinary
// chat can interleave in the same second, so only lines that are an exact spell
// name count as members. Observed in testdata/eqlog_Osui_pq.proj.txt.
var reHealthHeader = regexp.MustCompile(`^I have (\d+) percent of my hit points left\.$`)

// healthMaxSpan bounds how long after the header member lines are accepted.
// The client prints the whole reply in one log-second; 1s of slack covers a
// reply that straddles a second boundary.
const healthMaxSpan = time.Second

// healthMaxSpells caps one block so a run of spell-name-shaped lines can never
// grow it without bound. A pet carries at most ~30 effects.
const healthMaxSpells = 40

// HealthSnapshot is one parsed `/pet health` reply.
type HealthSnapshot struct {
	At        time.Time `json:"at"`
	HPPercent int       `json:"hp_percent"`
	Spells    []string  `json:"spells"`
}

// healthAssembler collects a `/pet health` block from the raw line stream.
// Not safe for concurrent use; the Tracker serialises calls under its mutex.
type healthAssembler struct {
	isSpell func(name string) bool

	open  bool
	cur   HealthSnapshot
	start time.Time
}

// handleLine feeds one raw log line. It returns a completed snapshot when this
// line closed the previous block (a new header, or a line past the span), else
// nil. A line that closes a block is not itself consumed unless it is a header.
func (a *healthAssembler) handleLine(ts time.Time, msg string) *HealthSnapshot {
	msg = strings.TrimRight(msg, "\r\n")

	if m := reHealthHeader.FindStringSubmatch(msg); m != nil {
		done := a.flush()
		hp, _ := strconv.Atoi(m[1])
		a.open = true
		a.start = ts
		a.cur = HealthSnapshot{At: ts, HPPercent: hp, Spells: []string{}}
		return done
	}
	if !a.open {
		return nil
	}
	if !ts.IsZero() && !a.start.IsZero() && ts.Sub(a.start) > healthMaxSpan {
		return a.flush()
	}
	if len(a.cur.Spells) < healthMaxSpells && a.isSpell != nil && a.isSpell(msg) {
		a.cur.Spells = append(a.cur.Spells, msg)
	}
	// A same-second non-spell line is interleaved chat: skip it, stay open.
	return nil
}

// flush closes any open block and returns it.
func (a *healthAssembler) flush() *HealthSnapshot {
	if !a.open {
		return nil
	}
	snap := a.cur
	a.open = false
	a.cur = HealthSnapshot{}
	a.start = time.Time{}
	return &snap
}
