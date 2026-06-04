package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/skills"
)

// skillsHandler serves the per-character Skill Tracker. It resolves a
// character by id (for class/level), reads tracked skill values from the
// skills store, and looks up each skill's cap from quarm.db skill_caps.
type skillsHandler struct {
	charStore *character.Store
	store     *skills.Store
	db        *db.DB
}

// skillView is one row in the Skills tab: the tracked value plus the resolved
// cap for the character's class/level. Cap is 0 when unknown (the class never
// trains the skill, the skill name didn't map to a skill_id, or the
// character's level isn't known).
type skillView struct {
	SkillID   int    `json:"skill_id"`
	SkillName string `json:"skill_name"`
	Value     int    `json:"value"`
	Cap       int    `json:"cap"`
	UpdatedAt int64  `json:"updated_at"`
}

type skillsResponse struct {
	Character string      `json:"character"`
	Class     int         `json:"class"` // 0-indexed EQ class, -1 if unknown
	Level     int         `json:"level"`
	Skills    []skillView `json:"skills"`
}

// specializeCapLock is the value the non-primary Specialize <school> skills are
// locked at on Project Quarm once another school has been raised past it.
const specializeCapLock = 50

func (h *skillsHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.charStore.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}

	// Skill tracking is disabled when user.db couldn't be opened — return the
	// character with an empty skill list so the tab still renders cleanly.
	if h.store == nil {
		writeJSON(w, http.StatusOK, skillsResponse{
			Character: char.Name, Class: char.Class, Level: char.Level, Skills: []skillView{},
		})
		return
	}

	records, err := h.store.GetByCharacter(char.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// classIdx in skill_caps is 1-indexed (1=Warrior … 15=Beastlord); our
	// character store class is 0-indexed.
	classIdx := char.Class + 1

	// First pass: resolve raw caps. Also find the highest observed Specialize
	// value so we can apply the single-primary lock in the second pass.
	views := make([]skillView, 0, len(records))
	maxSpecialize := 0
	for _, rec := range records {
		cap := 0
		if rec.SkillID >= 0 && h.db != nil {
			c, err := h.db.SkillCap(rec.SkillID, classIdx, char.Level)
			if err != nil {
				slog.Warn("skills: cap lookup failed", "skill", rec.SkillName, "err", err)
			} else {
				cap = c
			}
		}
		if skills.IsSpecialize(rec.SkillID) && rec.Value > maxSpecialize {
			maxSpecialize = rec.Value
		}
		views = append(views, skillView{
			SkillID:   rec.SkillID,
			SkillName: rec.SkillName,
			Value:     rec.Value,
			Cap:       cap,
			UpdatedAt: rec.UpdatedAt,
		})
	}

	// Second pass: once one Specialize school has been raised past 50, the
	// others are permanently locked at 50 on Quarm. We can't read the chosen
	// school from skill_caps, but the observed values reveal it — so cap the
	// non-primary specializations' displayed cap at 50. The primary (the one
	// that's already above 50) keeps its real cap.
	if maxSpecialize > specializeCapLock {
		for i := range views {
			v := &views[i]
			if skills.IsSpecialize(v.SkillID) && v.Value <= specializeCapLock && v.Cap > specializeCapLock {
				v.Cap = specializeCapLock
			}
		}
	}

	writeJSON(w, http.StatusOK, skillsResponse{
		Character: char.Name,
		Class:     char.Class,
		Level:     char.Level,
		Skills:    views,
	})
}
