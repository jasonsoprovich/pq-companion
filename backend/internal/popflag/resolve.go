package popflag

import (
	"sort"
	"strconv"
)

// FlagStatus is the resolved per-flag view returned to the frontend: the
// dataset node plus the character's effective completion and lock state.
type FlagStatus struct {
	PoPFlag
	Done    bool     `json:"done"`
	Source  string   `json:"source,omitempty"`  // provenance of Done (manual/seer/auto)
	Locked  bool     `json:"locked"`            // a prereq is not yet done
	Missing []string `json:"missing,omitempty"` // prereq IDs that are not done
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

	out := Resolved{Flags: make([]FlagStatus, 0, len(flags))}

	tierIdx := map[int]int{}
	zoneIdx := map[string]int{}

	for _, f := range flags {
		cur := byFlag[f.ID]
		fs := FlagStatus{PoPFlag: f, Done: cur.done, Source: cur.source}

		// Locked when any prereq is not effectively done.
		missing := []string{}
		for _, p := range f.Prereqs {
			if !byFlag[p].done {
				missing = append(missing, p)
			}
		}
		if len(missing) > 0 {
			fs.Locked = true
			fs.Missing = missing
		}
		out.Flags = append(out.Flags, fs)

		out.Total++
		if cur.done {
			out.Done++
		}

		ti, ok := tierIdx[f.Tier]
		if !ok {
			ti = len(out.Tiers)
			tierIdx[f.Tier] = ti
			out.Tiers = append(out.Tiers, Progress{Tier: f.Tier, Key: tierLabel(f.Tier), Label: tierLabel(f.Tier)})
		}
		out.Tiers[ti].Total++
		if cur.done {
			out.Tiers[ti].Done++
		}

		zi, ok := zoneIdx[f.Zone]
		if !ok {
			zi = len(out.Zones)
			zoneIdx[f.Zone] = zi
			out.Zones = append(out.Zones, Progress{Key: f.Zone, Label: f.Zone})
		}
		out.Zones[zi].Total++
		if cur.done {
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
