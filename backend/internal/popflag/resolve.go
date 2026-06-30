package popflag

import (
	"sort"
	"strconv"
)

// FlagStatus is the resolved per-flag view returned to the frontend: the
// dataset node plus the character's effective completion and lock state.
type FlagStatus struct {
	PoPFlag
	Done       bool     `json:"done"`
	Source     string   `json:"source,omitempty"`     // provenance of Done (manual/seer/auto)
	Locked     bool     `json:"locked"`               // a prereq is not yet done
	Missing    []string `json:"missing,omitempty"`    // prereq IDs that are not done
	Superseded bool     `json:"superseded,omitempty"` // an unchosen member of a satisfied any-of group
}

// Progress is a completed/total tally for a tier or zone.
type Progress struct {
	Tier  int    `json:"tier,omitempty"` // set for tier tallies; 0 for zones
	Key   string `json:"key"`            // stable key (tier label or zone name)
	Label string `json:"label"`          // display label
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// Resolved is the full resolver output for one character.
type Resolved struct {
	Flags []FlagStatus `json:"flags"`
	Tiers []Progress   `json:"tiers"`
	Zones []Progress   `json:"zones"`
	Done  int          `json:"done"`
	Total int          `json:"total"`
}

// Resolve merges the embedded dataset with the character's stored state into
// effective per-flag status (done + provenance), computed lock state (a prereq
// is unmet), and per-tier / per-zone progress tallies. Pure and order-stable —
// the returned Flags preserve dataset order; Zones follow first-seen order;
// Tiers are sorted ascending.
//
// Note: lock state is computed from the pure AND prereq list. The SatisfiedBy
// replacement any-of (cipher/zebuxoruk) operates on a Seer qglobal snapshot and
// is applied in Phase 2 when filling seer-sourced rows; it does not affect this
// Phase 1 resolution, which works off the per-flag done rows directly.
//
// Any-of groups (e.g. the six PoJ Trials) roll up here: an anchor milestone is
// effectively done once any member is done, the unchosen members are flagged
// Superseded for the faded "not needed" UI, and both members and Optional rows
// (keys, keyrings, bonus content) are excluded from the done/total tallies.
func Resolve(states []State) Resolved {
	load()

	type st struct {
		done   bool
		source string
	}
	byFlag := make(map[string]st, len(states))
	for _, s := range states {
		byFlag[s.FlagID] = st{done: s.Done, source: s.Source}
	}

	// Any-of group roll-up: an anchor milestone is effectively done once any of
	// its members is done. Capture the satisfying member's source so the anchor
	// can show the right provenance chip when it was completed via a member.
	groupDone := map[string]bool{}
	groupSource := map[string]string{}
	for _, f := range flags {
		if f.Group != "" && byFlag[f.ID].done {
			groupDone[f.Group] = true
			if groupSource[f.Group] == "" {
				groupSource[f.Group] = byFlag[f.ID].source
			}
		}
	}
	// effDone folds the member roll-up into a flag's own stored state. Used for
	// both the anchor's effective completion and downstream prereq satisfaction.
	effDone := func(id string) bool {
		if byFlag[id].done {
			return true
		}
		return groupDone[id]
	}

	out := Resolved{Flags: make([]FlagStatus, 0, len(flags))}

	tierIdx := map[int]int{}
	zoneIdx := map[string]int{}

	for _, f := range flags {
		cur := byFlag[f.ID]
		done := effDone(f.ID)
		source := cur.source
		if !cur.done && groupDone[f.ID] { // anchor satisfied via a member
			source = groupSource[f.ID]
		}
		fs := FlagStatus{PoPFlag: f, Done: done, Source: source}

		// Locked when any prereq is not effectively done.
		missing := []string{}
		for _, p := range f.Prereqs {
			if !effDone(p) {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			fs.Locked = true
			fs.Missing = missing
		}
		// A member of an any-of group that is satisfied by some OTHER member is
		// superseded — still listed, but rendered faded ("not needed").
		if f.Group != "" && groupDone[f.Group] && !cur.done {
			fs.Superseded = true
		}
		out.Flags = append(out.Flags, fs)

		// Optional rows (keys, keyrings, bonus content) and any-of members are
		// display-only: they neither count toward the personal tally nor block.
		if f.Optional || f.Group != "" {
			continue
		}

		out.Total++
		if done {
			out.Done++
		}

		ti, ok := tierIdx[f.Tier]
		if !ok {
			ti = len(out.Tiers)
			tierIdx[f.Tier] = ti
			out.Tiers = append(out.Tiers, Progress{Tier: f.Tier, Key: tierLabel(f.Tier), Label: tierLabel(f.Tier)})
		}
		out.Tiers[ti].Total++
		if done {
			out.Tiers[ti].Done++
		}

		zi, ok := zoneIdx[f.Zone]
		if !ok {
			zi = len(out.Zones)
			zoneIdx[f.Zone] = zi
			out.Zones = append(out.Zones, Progress{Key: f.Zone, Label: f.Zone})
		}
		out.Zones[zi].Total++
		if done {
			out.Zones[zi].Done++
		}
	}

	sort.SliceStable(out.Tiers, func(i, j int) bool { return out.Tiers[i].Tier < out.Tiers[j].Tier })
	return out
}

func tierLabel(t int) string {
	if t == 5 {
		return "Plane of Time"
	}
	return "Tier " + strconv.Itoa(t)
}
