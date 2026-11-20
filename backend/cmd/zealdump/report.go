package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// Report mode: listen to everything while you play, then on Ctrl+C write one
// text file summarising what each pending Zeal feature sent, with the events
// behind it, so a test session can be reviewed from the file alone.

// reportSpellLogCap bounds the spell lines kept for the file; the summary
// still counts every one.
const reportSpellLogCap = 3000

type reportState struct {
	start   time.Time
	counts  map[zealpipe.PipeMessageType]int
	events  []string
	spellsN int // spell lines written to events

	selfID   int
	selfName string
	zones    map[int]bool

	// Pet (pet_id / pet_loc on the player message).
	petIDs    map[int]bool
	petLocs   int
	petNoLocs int // ticks with pet_id but no pet_loc

	// Roster appearance.
	verboseSeen   bool              // hp_current present: /pipeverbose is on
	members       map[string]string // kind/name -> last rendered line
	withApp       map[string]bool
	inZoneNoApp   map[string]bool
	outZoneApp    map[string]bool
	outZoneOK     map[string]bool
	appChanges    int
	rosterTicks   int
	rosterBytes   int
	rosterMaxSize int

	// Consider.
	considers     int
	considerLast  map[int]time.Time
	considerRapid int // same spawn within 3s
	considerDup   int // same spawn within 250ms: one target sent two considers
	considerFac   map[string]int

	// Spells.
	spells       int
	spellOK      int
	spellSelf    int
	spellSelfOK  int
	spellSecs    map[int64]int
	spellTargets map[string]map[int]bool // target name -> ids
	spellIDs     map[int]int

	deaths int

	// Target spawn location (target_spawn_loc), per zone|spawn id NPC target.
	npcTargets   map[string]bool
	spawnLocs    map[string]bool
	spawnLocFar  int // seen 50+ units from where it spawned: the case it's for
	spawnLocSeen map[string]bool
}

func newReportState() *reportState {
	return &reportState{
		start:        time.Now(),
		counts:       map[zealpipe.PipeMessageType]int{},
		zones:        map[int]bool{},
		petIDs:       map[int]bool{},
		members:      map[string]string{},
		withApp:      map[string]bool{},
		inZoneNoApp:  map[string]bool{},
		outZoneApp:   map[string]bool{},
		outZoneOK:    map[string]bool{},
		considerLast: map[int]time.Time{},
		considerFac:  map[string]int{},
		spellSecs:    map[int64]int{},
		spellTargets: map[string]map[int]bool{},
		spellIDs:     map[int]int{},
	}
}

func (r *reportState) event(now time.Time, format string, args ...any) {
	r.events = append(r.events, now.Format("15:04:05")+" "+fmt.Sprintf(format, args...))
}

// runReport records every envelope until the pipe closes or Ctrl+C, then
// writes the report to outPath (and prints where it went).
func runReport(ctx context.Context, conn io.Reader, outPath string) {
	if outPath == "" {
		outPath = "zealdump-report-" + time.Now().Format("20060102-150405") + ".txt"
	}
	fmt.Printf("recording everything Zeal sends. Play normally, then press Ctrl+C to write %s\n", outPath)
	r := newReportState()
	dec := json.NewDecoder(conn)
	lastDot := time.Now()
	for ctx.Err() == nil {
		var env zealpipe.Envelope
		if err := dec.Decode(&env); err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "\nread ended: %v\n", err)
			}
			break
		}
		r.add(env, time.Now())
		if time.Since(lastDot) > 30*time.Second {
			fmt.Printf("  %s: %d messages so far (%d spell, %d consider, %d roster ticks)\n",
				time.Now().Format("15:04:05"), total(r.counts), r.spells, r.considers, r.rosterTicks)
			lastDot = time.Now()
		}
	}
	text := r.render(time.Now())
	if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n%s", outPath, err, text)
		os.Exit(1)
	}
	fmt.Printf("\nwrote %s\n", outPath)
}

func total(m map[zealpipe.PipeMessageType]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func (r *reportState) add(env zealpipe.Envelope, now time.Time) {
	r.counts[env.Type]++
	if env.Character != "" && r.selfName == "" {
		r.selfName = env.Character
	}
	switch env.Type {
	case zealpipe.MsgPlayer:
		p, err := zealpipe.DecodePlayer(env.Data)
		if err != nil {
			return
		}
		if p.SpawnID != nil {
			r.selfID = *p.SpawnID
		}
		if !r.zones[p.Zone] {
			r.zones[p.Zone] = true
			r.event(now, "zone id %d", p.Zone)
		}
		r.addTargetSpawn(p, now)
		if p.PetID != nil && *p.PetID > 0 {
			if !r.petIDs[*p.PetID] {
				r.petIDs[*p.PetID] = true
				r.event(now, "pet id %d (pet_loc present: %v)", *p.PetID, p.PetLoc != nil)
			}
			if p.PetLoc != nil {
				r.petLocs++
			} else {
				r.petNoLocs++
			}
		}
		return
	case zealpipe.MsgRaid, zealpipe.MsgGroup:
		r.addRoster(env, now)
		return
	case zealpipe.MsgDeath:
		if _, err := zealpipe.DecodeDeath(env.Data); err == nil {
			r.deaths++
			return
		}
	}
	p, ok := zealpipe.DecodeProposed(env)
	if !ok {
		return
	}
	switch {
	case p.Consider != nil:
		c := p.Consider
		r.considers++
		if last, ok := r.considerLast[c.SpawnID]; ok && now.Sub(last) < 3*time.Second {
			r.considerRapid++
			if now.Sub(last) < 250*time.Millisecond {
				r.considerDup++
			}
		}
		r.considerLast[c.SpawnID] = now
		standing := zealpipe.ConsiderStanding(c.Faction)
		r.considerFac[standing]++
		r.event(now, "consider type=%d %s(%d) faction=%d %s", env.Type, c.Name, c.SpawnID, c.Faction, standing)
	case p.Spell != nil:
		a := p.Spell
		r.spells++
		r.spellSecs[now.Unix()]++
		r.spellIDs[a.SpellID]++
		if a.Success {
			r.spellOK++
		}
		if r.selfID != 0 && a.SourceID == r.selfID {
			r.spellSelf++
			if a.Success {
				r.spellSelfOK++
			}
		}
		if a.TargetName != "" {
			ids := r.spellTargets[a.TargetName]
			if ids == nil {
				ids = map[int]bool{}
				r.spellTargets[a.TargetName] = ids
			}
			ids[a.TargetID] = true
		}
		if r.spellsN < reportSpellLogCap {
			r.spellsN++
			res := "attempt"
			if a.Success {
				res = "SUCCESS"
			}
			r.event(now, "spell type=%d id=%d %s(%d) -> %s(%d) %s", env.Type, a.SpellID,
				nameOr(a.SourceName), a.SourceID, nameOr(a.TargetName), a.TargetID, res)
		}
	}
}

// addTargetSpawn tracks which NPC targets came with target_spawn_loc, logging
// each one's spawn point once so it can be checked against spawn2 afterwards.
func (r *reportState) addTargetSpawn(p zealpipe.Player, now time.Time) {
	if p.TargetID == nil || p.TargetType == nil || *p.TargetType != zealpipe.TargetTypeNPC {
		return
	}
	if r.npcTargets == nil {
		r.npcTargets = map[string]bool{}
		r.spawnLocs = map[string]bool{}
		r.spawnLocSeen = map[string]bool{}
	}
	key := fmt.Sprintf("%d|%d", p.Zone, *p.TargetID)
	r.npcTargets[key] = true
	sl := p.TargetSpawnLoc
	if sl == nil {
		return
	}
	name := ""
	if p.TargetName != nil {
		name = *p.TargetName
	}
	if !r.spawnLocs[key] {
		r.spawnLocs[key] = true
		r.event(now, "spawn loc: %s (id %d, zone %d) spawned at %.0f, %.0f, %.0f",
			nameOr(name), *p.TargetID, p.Zone, sl.GameX(), sl.GameY(), sl.Z)
	}
	if tl := p.TargetLoc; tl != nil && !r.spawnLocSeen[key] {
		dx, dy := tl.X-sl.X, tl.Y-sl.Y
		if dx*dx+dy*dy >= 50*50 {
			r.spawnLocSeen[key] = true
			r.spawnLocFar++
		}
	}
}

func (r *reportState) addRoster(env zealpipe.Envelope, now time.Time) {
	r.rosterTicks++
	r.rosterBytes += len(env.Data)
	if len(env.Data) > r.rosterMaxSize {
		r.rosterMaxSize = len(env.Data)
	}
	kind := "raid"
	var entries []rosterEntry
	verbose := false
	if env.Type == zealpipe.MsgRaid {
		members, err := zealpipe.DecodeRaid(env.Data)
		if err != nil {
			return
		}
		for _, m := range members {
			entries = append(entries, rosterEntry{m.Name, m.SpawnID != nil, m.Appearance})
			verbose = verbose || m.HPCur != nil
		}
	} else {
		kind = "group"
		members, err := zealpipe.DecodeGroup(env.Data)
		if err != nil {
			return
		}
		for _, m := range members {
			entries = append(entries, rosterEntry{m.Name, m.SpawnID != nil, m.Appearance})
			verbose = verbose || m.HPCur != nil
		}
	}
	if verbose && !r.verboseSeen {
		r.verboseSeen = true
		r.event(now, "/pipeverbose is on (hp_current seen)")
	}
	for _, e := range entries {
		key := kind + "/" + e.name
		switch {
		case e.inZone && e.appearance != nil:
			r.withApp[e.name] = true
			delete(r.inZoneNoApp, e.name)
		case e.inZone:
			if !r.withApp[e.name] {
				r.inZoneNoApp[e.name] = true
			}
		case e.appearance != nil:
			r.outZoneApp[e.name] = true
		default:
			r.outZoneOK[e.name] = true
		}
		line := formatRosterEntry(e)
		prev, seen := r.members[key]
		if prev == line {
			continue
		}
		if seen && e.appearance != nil && strings.Contains(prev, "race=") {
			r.appChanges++
			r.event(now, "%-5s CHANGED %s", kind, line)
		} else {
			r.event(now, "%-5s %s", kind, line)
		}
		r.members[key] = line
	}
}

func (r *reportState) render(now time.Time) string {
	var b strings.Builder
	dur := now.Sub(r.start)
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	check := func(ok bool, format string, args ...any) {
		mark := "[ ]"
		if ok {
			mark = "[x]"
		}
		w("  %s "+format, append([]any{mark}, args...)...)
	}
	names := func(m map[string]bool) string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		if len(out) > 15 {
			return strings.Join(out[:15], ", ") + fmt.Sprintf(" and %d more", len(out)-15)
		}
		return strings.Join(out, ", ")
	}

	w("zealdump report")
	w("character %s (spawn id %d), %s to %s (%s)", nameOr(r.selfName), r.selfID,
		r.start.Format("2006-01-02 15:04:05"), now.Format("15:04:05"), dur.Round(time.Second))
	types := make([]int, 0, len(r.counts))
	for t := range r.counts {
		types = append(types, int(t))
	}
	sort.Ints(types)
	parts := make([]string, len(types))
	for i, t := range types {
		parts[i] = fmt.Sprintf("type %d: %d", t, r.counts[zealpipe.PipeMessageType(t)])
	}
	w("messages: %s", strings.Join(parts, ", "))
	w("")

	w("ROSTER APPEARANCE (raid type 5, group type 6)")
	check(r.verboseSeen, "/pipeverbose on (appearance is only sent with it)")
	check(len(r.withApp) > 0, "in-zone members with appearance: %d (%s)", len(r.withApp), names(r.withApp))
	check(len(r.inZoneNoApp) == 0, "in-zone members never seen with appearance: %d %s", len(r.inZoneNoApp), names(r.inZoneNoApp))
	check(len(r.outZoneApp) == 0, "out-of-zone members WITH appearance (should be 0): %d %s", len(r.outZoneApp), names(r.outZoneApp))
	w("      out-of-zone members correctly without it: %d", len(r.outZoneOK))
	w("      appearance changes seen (gear swaps, illusions, size): %d", r.appChanges)
	if r.rosterTicks > 0 {
		w("      roster payload: avg %d bytes, largest %d bytes, over %d ticks",
			r.rosterBytes/r.rosterTicks, r.rosterMaxSize, r.rosterTicks)
	}
	w("")

	w("CONSIDER (/pipeconsider)")
	check(r.considers > 0, "consider messages: %d", r.considers)
	check(r.considerRapid == 0, "same NPC considered again within 3s (spam): %d", r.considerRapid)
	check(r.considerDup == 0, "  of those, back to back within 250ms (doubled): %d", r.considerDup)
	if len(r.considerFac) > 0 {
		fac := make([]string, 0, len(r.considerFac))
		for k, v := range r.considerFac {
			fac = append(fac, fmt.Sprintf("%s %d", k, v))
		}
		sort.Strings(fac)
		w("      standings: %s", strings.Join(fac, ", "))
	}
	w("      check the EQ log for the same stretch: no con lines for NPCs you only targeted")
	w("")

	w("TARGET SPAWN LOCATION (target_spawn_loc)")
	check(len(r.spawnLocs) > 0, "NPC targets with a spawn location: %d of %d", len(r.spawnLocs), len(r.npcTargets))
	w("      of those, targeted 50+ units from where they spawned: %d", r.spawnLocFar)
	w("")

	w("SPELL ACTIONS (/pipespells)")
	check(r.spells > 0, "spell actions: %d (%d with success)", r.spells, r.spellOK)
	check(r.spellSelf > 0 && r.spellSelfOK > 0, "your own casts: %d, %d with success (caster gets attempt + success)", r.spellSelf, r.spellSelfOK)
	check(r.spells-r.spellSelf > 0, "others' casts: %d", r.spells-r.spellSelf)
	namesakes := map[string]bool{}
	for name, ids := range r.spellTargets {
		if len(ids) > 1 {
			namesakes[fmt.Sprintf("%s x%d", name, len(ids))] = true
		}
	}
	check(len(namesakes) > 0, "same-named targets told apart by id: %d (%s)", len(namesakes), names(namesakes))
	if len(r.spellSecs) > 0 {
		peak := 0
		for _, n := range r.spellSecs {
			if n > peak {
				peak = n
			}
		}
		w("      volume: %.1f/sec over the session, %.1f/sec in seconds with any, peak %d in one second",
			float64(r.spells)/dur.Seconds(), float64(r.spells)/float64(len(r.spellSecs)), peak)
		w("      distinct spells: %d", len(r.spellIDs))
	}
	if r.spells > reportSpellLogCap {
		w("      (only the first %d spell lines are listed below)", reportSpellLogCap)
	}
	w("")

	w("PET (pet_loc, Zeal 1.5.0+)")
	ids := make([]string, 0, len(r.petIDs))
	for id := range r.petIDs {
		ids = append(ids, fmt.Sprint(id))
	}
	check(len(r.petIDs) > 0, "pet seen: ids %s", strings.Join(ids, ", "))
	check(r.petLocs > 0 && r.petNoLocs == 0, "ticks with pet_loc: %d, with a pet but no pet_loc: %d", r.petLocs, r.petNoLocs)
	w("")

	w("DEATHS (type 7): %d", r.deaths)
	w("")
	w("---- events ----")
	for _, e := range r.events {
		w("%s", e)
	}
	return b.String()
}
