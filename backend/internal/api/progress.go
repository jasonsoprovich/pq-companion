package api

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/progress"
)

// progressHandler powers the Character Info Recap tab: a rolling-window
// summary of level/AA/spell/skill milestones (from the backfillable log
// journal) plus coin/totals deltas (from the forward-only snapshot table).
type progressHandler struct {
	store     *progress.Store
	charStore *character.Store
}

// defaultRecapDays is used when the request omits ?days or sends an
// out-of-range value.
const defaultRecapDays = 30

func recapWindowDays(r *http.Request) int {
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || days <= 0 || days > 365 {
		return defaultRecapDays
	}
	return days
}

// buildOneRecap loads a character's journal events and boundary snapshots for
// [since, now] and aggregates them via progress.BuildRecap.
func (h *progressHandler) buildOneRecap(name string, since, now time.Time) (progress.CharacterRecap, error) {
	events, err := h.store.EventsSince(name, since)
	if err != nil {
		return progress.CharacterRecap{}, err
	}
	loginDays, err := h.store.ActiveDaysSince(name, since)
	if err != nil {
		return progress.CharacterRecap{}, err
	}
	startSnap, _, err := h.store.SnapshotAtOrBefore(name, since)
	if err != nil {
		return progress.CharacterRecap{}, err
	}
	endSnap, _, err := h.store.SnapshotAtOrBefore(name, now)
	if err != nil {
		return progress.CharacterRecap{}, err
	}
	return progress.BuildRecap(name, events, loginDays, startSnap, endSnap, since, now), nil
}

// GET /api/progress/recap?days=30[&character=X]
// With character set, returns a single CharacterRecap. Otherwise returns one
// recap per tracked character, most-active first.
func (h *progressHandler) recap(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	since := now.AddDate(0, 0, -recapWindowDays(r))

	if name := r.URL.Query().Get("character"); name != "" {
		rec, err := h.buildOneRecap(name, since, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)
		return
	}

	chars, err := h.charStore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]progress.CharacterRecap, 0, len(chars))
	for _, c := range chars {
		rec, err := h.buildOneRecap(c.Name, since, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, rec)
	}
	// Most-active character first — the "hey, where do I spend my time?"
	// framing this feature was requested for.
	sort.Slice(out, func(i, j int) bool {
		return activityScore(out[i]) > activityScore(out[j])
	})
	writeJSON(w, http.StatusOK, out)
}

// activityScore ranks recaps for the all-characters view. Coin isn't
// included — it swings on gear purchases unrelated to "time spent playing."
func activityScore(r progress.CharacterRecap) int {
	return r.LevelsGained*10 + r.AAsGained + r.SpellsScribed + r.SkillUps + r.TradeskillUps
}

// GET /api/progress/events?character=X&days=30
// Returns the raw journal drill-down for one character, oldest first.
func (h *progressHandler) events(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("character")
	if name == "" {
		writeError(w, http.StatusBadRequest, "character is required")
		return
	}
	since := time.Now().AddDate(0, 0, -recapWindowDays(r))
	events, err := h.store.EventsSince(name, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}
