package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/popflag"
)

// popflagExportKind / popflagExportVersion identify the JSON envelope returned
// by GET /api/popflags/export so other tools can sniff and version-gate it.
const (
	popflagExportKind    = "pq-companion.popflags"
	popflagExportVersion = 1
)

// popflagSummaryZone is one grid column: a zone with its tier, in dataset order.
type popflagSummaryZone struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Tier  int    `json:"tier"`
}

// popflagSummaryFlag is the compact per-flag state in the overview response.
type popflagSummaryFlag struct {
	ID     string `json:"id"`
	Done   bool   `json:"done"`
	Source string `json:"source,omitempty"`
	Locked bool   `json:"locked"`
}

// popflagSummaryCharacter is one grid row.
type popflagSummaryCharacter struct {
	Name  string               `json:"name"`
	Done  int                  `json:"done"`
	Total int                  `json:"total"`
	Tiers []popflag.Progress   `json:"tiers"`
	Zones []popflag.Progress   `json:"zones"`
	Flags []popflagSummaryFlag `json:"flags"`
}

type popflagSummaryResponse struct {
	Characters []popflagSummaryCharacter `json:"characters"`
	Zones      []popflagSummaryZone      `json:"zones"`
}

// summaryCharacters returns the character names the overview covers: the
// visible characters from the character store, matching the tab strip. Names
// that only have stored PoP flag rows (a deleted profile) are left out unless
// they are the active character; their rows are kept, so re-adding the profile
// restores its progress. Without a character store, falls back to every name
// with flag rows. Hidden characters are dropped unless they are the active one
// (mirrors charactersHandler.list). Case-insensitive de-dupe, sorted.
func (h *popflagHandler) summaryCharacters() ([]string, error) {
	seen := map[string]string{}
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" {
			return
		}
		if _, ok := seen[strings.ToLower(n)]; !ok {
			seen[strings.ToLower(n)] = n
		}
	}

	active := ""
	if h.mgr != nil {
		active = h.mgr.Get().Character
	}

	var hidden map[string]struct{}
	if h.charStore != nil {
		chars, err := h.charStore.List()
		if err != nil {
			return nil, err
		}
		for _, c := range chars {
			add(c.Name)
		}
		add(active)
		if hidden, err = h.charStore.HiddenNames(); err != nil {
			return nil, err
		}
	} else {
		stored, err := h.store.Characters()
		if err != nil {
			return nil, err
		}
		for _, n := range stored {
			add(n)
		}
	}

	out := make([]string, 0, len(seen))
	for lower, name := range seen {
		if _, isHidden := hidden[lower]; isHidden && !strings.EqualFold(name, active) {
			continue
		}
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}

// resolveAll resolves every requested character (all when names is empty).
func (h *popflagHandler) resolveAll(names []string) ([]string, map[string]popflag.Resolved, error) {
	if len(names) == 0 {
		var err error
		if names, err = h.summaryCharacters(); err != nil {
			return nil, nil, err
		}
	}
	resolved := make(map[string]popflag.Resolved, len(names))
	for _, n := range names {
		states, err := h.store.Get(n)
		if err != nil {
			return nil, nil, err
		}
		resolved[n] = popflag.Resolve(states)
	}
	return names, resolved, nil
}

// datasetZones lists distinct zones in dataset order with their tier.
func datasetZones() []popflagSummaryZone {
	var zones []popflagSummaryZone
	seen := map[string]bool{}
	for _, f := range popflag.Flags() {
		if f.Optional || f.Group != "" || seen[f.Zone] {
			continue
		}
		seen[f.Zone] = true
		zones = append(zones, popflagSummaryZone{Key: f.Zone, Label: f.ZoneShort, Tier: f.Tier})
	}
	return zones
}

// GET /api/popflags/summary
// Returns every known character's resolved progress for the overview grid.
func (h *popflagHandler) summary(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	names, resolved, err := h.resolveAll(nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := popflagSummaryResponse{
		Characters: make([]popflagSummaryCharacter, 0, len(names)),
		Zones:      datasetZones(),
	}
	for _, n := range names {
		res := resolved[n]
		c := popflagSummaryCharacter{
			Name: n, Done: res.Done, Total: res.Total,
			Tiers: res.Tiers, Zones: res.Zones,
			Flags: make([]popflagSummaryFlag, 0, len(res.Flags)),
		}
		for _, f := range res.Flags {
			c.Flags = append(c.Flags, popflagSummaryFlag{ID: f.ID, Done: f.Done, Source: f.Source, Locked: f.Locked})
		}
		resp.Characters = append(resp.Characters, c)
	}
	writeJSON(w, http.StatusOK, resp)
}

type popflagExportFlag struct {
	ID     string `json:"id"`
	Tier   int    `json:"tier"`
	Zone   string `json:"zone"`
	Label  string `json:"label"`
	Done   bool   `json:"done"`
	Source string `json:"source,omitempty"`
}

type popflagExportCharacter struct {
	Name  string                    `json:"name"`
	Done  int                       `json:"done"`
	Total int                       `json:"total"`
	Zones map[string]popflagExportZ `json:"zones"`
	Flags []popflagExportFlag       `json:"flags"`
}

type popflagExportZ struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type popflagExport struct {
	Kind       string                   `json:"kind"`
	Version    int                      `json:"version"`
	ExportedAt string                   `json:"exported_at"`
	Characters []popflagExportCharacter `json:"characters"`
}

// GET /api/popflags/export[?characters=a,b]
// Versioned JSON envelope for importing PoP progression into other tools.
func (h *popflagHandler) export(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	var filter []string
	if q := strings.TrimSpace(r.URL.Query().Get("characters")); q != "" {
		for _, n := range strings.Split(q, ",") {
			if n = strings.TrimSpace(n); n != "" {
				filter = append(filter, n)
			}
		}
	}
	names, resolved, err := h.resolveAll(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := popflagExport{
		Kind:       popflagExportKind,
		Version:    popflagExportVersion,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Characters: make([]popflagExportCharacter, 0, len(names)),
	}
	for _, n := range names {
		res := resolved[n]
		c := popflagExportCharacter{
			Name: n, Done: res.Done, Total: res.Total,
			Zones: make(map[string]popflagExportZ, len(res.Zones)),
			Flags: make([]popflagExportFlag, 0, len(res.Flags)),
		}
		for _, z := range res.Zones {
			c.Zones[z.Key] = popflagExportZ{Done: z.Done, Total: z.Total}
		}
		for _, f := range res.Flags {
			c.Flags = append(c.Flags, popflagExportFlag{
				ID: f.ID, Tier: f.Tier, Zone: f.Zone, Label: f.Label, Done: f.Done, Source: f.Source,
			})
		}
		out.Characters = append(out.Characters, c)
	}
	writeJSON(w, http.StatusOK, out)
}
