package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/resist"
)

// resistHandler powers the (developer-tab) interactive resist calculator:
// given a spell, the caster, and a targeted NPC's resists, estimate the odds
// the spell lands. The math lives in internal/resist.
type resistHandler struct {
	db     *db.DB
	cfgMgr *config.Manager
}

// resistCheckRequest is the POST body. The caller passes the NPC's five resist
// values (as shown on the Stats tab) and TargetLevel (the top end of the
// NPC's level range, worst case); the backend selects the resist matching the
// spell's resist type.
type resistCheckRequest struct {
	SpellID     int `json:"spell_id"`
	CasterLevel int `json:"caster_level"`
	CasterClass int `json:"caster_class"` // 0-based (Warrior=0 … Beastlord=14)
	CasterCHA   int `json:"caster_cha"`
	TargetLevel int `json:"target_level"`
	TargetMR    int `json:"target_mr"`
	TargetCR    int `json:"target_cr"`
	TargetFR    int `json:"target_fr"`
	TargetDR    int `json:"target_dr"`
	TargetPR    int `json:"target_pr"`
}

// resistCheckResponse wraps the computed distribution with the context needed
// to render and caveat it.
type resistCheckResponse struct {
	SpellID         int    `json:"spell_id"`
	SpellName       string `json:"spell_name"`
	ResistType      int    `json:"resist_type"`
	ResistTypeLabel string `json:"resist_type_label"`
	TargetResist    int    `json:"target_resist"` // the value actually used
	Binary          bool   `json:"binary"`
	Unresistable    bool   `json:"unresistable"`

	LandChance     float64 `json:"land_chance"`
	AvgCastsToLand float64 `json:"avg_casts_to_land"`
	FullResist     float64 `json:"full_resist"`
	Partial        float64 `json:"partial"`
	FullDamage     float64 `json:"full_damage"`

	ExpectedEffectiveness float64 `json:"expected_effectiveness"`
	PartialMin            float64 `json:"partial_min"`
	PartialMax            float64 `json:"partial_max"`

	ResistChance int `json:"resist_chance"` // pre-roll value, for transparency
}

var resistTypeLabels = map[int]string{
	0: "Unresistable",
	1: "Magic",
	2: "Fire",
	3: "Cold",
	4: "Poison",
	5: "Disease",
	6: "Chromatic",
	7: "Prismatic",
	8: "Physical",
	9: "Corruption",
}

// POST /api/resist-check
func (h *resistHandler) check(w http.ResponseWriter, r *http.Request) {
	var body resistCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.CasterLevel < 1 || body.TargetLevel < 1 {
		writeError(w, http.StatusBadRequest, "caster_level and target_level are required")
		return
	}

	spell, err := h.db.GetSpell(body.SpellID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "spell not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	targetResist := resistForType(spell.ResistType, body)

	in := resist.Input{
		Spell:        toResistSpell(spell),
		CasterLevel:  body.CasterLevel,
		CasterClass:  body.CasterClass,
		CasterCHA:    body.CasterCHA,
		TargetLevel:  body.TargetLevel,
		TargetResist: targetResist,
		Era: resist.Era{
			PoPEnabled: h.cfgMgr.Get().Preferences.PoPEnabled,
			// Project Quarm is currently in the Luclin era, which disables
			// the pre-Luclin "six-level rule". Flip this when era modeling
			// gains a Luclin toggle.
			LuclinEnabled: true,
		},
	}

	res := resist.ComputeChances(in)
	writeJSON(w, http.StatusOK, resistCheckResponse{
		SpellID:               spell.ID,
		SpellName:             spell.Name,
		ResistType:            spell.ResistType,
		ResistTypeLabel:       resistTypeLabels[spell.ResistType],
		TargetResist:          targetResist,
		Binary:                res.Binary,
		Unresistable:          res.Unresistable,
		LandChance:            res.LandChance,
		AvgCastsToLand:        res.AvgCastsToLand,
		FullResist:            res.FullResist,
		Partial:               res.Partial,
		FullDamage:            res.FullDamage,
		ExpectedEffectiveness: res.ExpectedEffectiveness,
		PartialMin:            res.PartialMin,
		PartialMax:            res.PartialMax,
		ResistChance:          res.ResistChance,
	})
}

// resistForType selects the NPC resist value matching the spell's resist type.
func resistForType(resistType int, b resistCheckRequest) int {
	switch resistType {
	case 1:
		return b.TargetMR
	case 2:
		return b.TargetFR
	case 3:
		return b.TargetCR
	case 4:
		return b.TargetPR
	case 5:
		return b.TargetDR
	default:
		return 0
	}
}

func toResistSpell(s *db.Spell) resist.Spell {
	rs := resist.Spell{
		ResistType:      s.ResistType,
		ResistDiff:      s.ResistDiff,
		NoPartialResist: s.NoPartialResist != 0,
		TargetType:      s.TargetType,
		BuffDuration:    s.BuffDuration,
		AEDuration:      s.AEDuration,
		GoodEffect:      s.GoodEffect,
	}
	rs.EffectIDs = s.EffectIDs
	rs.EffectBase = s.EffectBaseValues
	rs.EffectFormula = s.EffectFormulas
	return rs
}
