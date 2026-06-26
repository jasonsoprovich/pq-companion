package api

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/jasonsoprovich/pq-companion/backend/internal/charm"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/era"
	"github.com/jasonsoprovich/pq-companion/backend/internal/resist"
)

// charmHandler powers the (developer-tab) Charm Pet Finder: given a zone, a
// charm-capable class, the player's level and a charm spell, it lists the zone's
// charmable NPCs ranked by melee DPS, with the level cap warnings and per-NPC
// charm land odds a kiter cares about. The catalog + scaling math live in
// internal/charm; the land-chance math reuses internal/resist (charm is a
// binary-resist magic spell that engine already handles).
type charmHandler struct {
	db     *db.DB
	cfgMgr *config.Manager
}

// charmSpellOption is one selectable charm spell for a class.
type charmSpellOption struct {
	SpellID       int    `json:"spell_id"`
	Name          string `json:"name"`
	ReqLevel      int    `json:"req_level"`
	MaxCharmLevel int    `json:"max_charm_level"`
	Restriction   string `json:"restriction,omitempty"` // "", "animal", "undead"
}

// charmPet is one charmable NPC row. Stats are presented as a min..max range
// across the NPC's spawn level range (min == max for fixed-level spawns).
type charmPet struct {
	NPCID        int      `json:"npc_id"`
	Name         string   `json:"name"`
	Class        int      `json:"class"`
	ClassName    string   `json:"class_name"`
	BodyType     int      `json:"body_type"`
	BodyTypeName string   `json:"body_type_name"`
	LevelMin     int      `json:"level_min"`
	LevelMax     int      `json:"level_max"`
	HPMin        int      `json:"hp_min"`
	HPMax        int      `json:"hp_max"`
	MaxHitMin    int      `json:"max_hit_min"`
	MaxHitMax    int      `json:"max_hit_max"`
	AttackDelay  int      `json:"attack_delay"`
	DPSMin       float64  `json:"dps_min"`
	DPSMax       float64  `json:"dps_max"`
	MR           int      `json:"mr"`
	Summon       bool     `json:"summon"`
	Gate         bool     `json:"gate"`
	Caster       bool     `json:"caster"`
	Abilities    []string `json:"abilities,omitempty"`
	// LevelWarning is set when the top of the spawn range exceeds the charm
	// spell's cap — the name can spawn at levels the charm won't hold.
	LevelWarning bool `json:"level_warning"`
	// LandChance is the probability the charm lands on the worst-case charmable
	// spawn (the top of the range, clamped to the spell cap), at the caster's
	// level. Binary marks the charm as land-or-resist (always true for charm).
	LandChance float64 `json:"land_chance"`
	Binary     bool    `json:"binary"`
}

type charmPetsResponse struct {
	Zone          string     `json:"zone"`
	SpellID       int        `json:"spell_id"`
	SpellName     string     `json:"spell_name"`
	MaxCharmLevel int        `json:"max_charm_level"`
	Restriction   string     `json:"restriction,omitempty"`
	Count         int        `json:"count"`
	Pets          []charmPet `json:"pets"`
}

// GET /api/charm/spells?class={idx}
// Lists the charm spells a class can use this era, each with its required level,
// maximum charmable NPC level, and body restriction.
func (h *charmHandler) spells(w http.ResponseWriter, r *http.Request) {
	classIdx, err := strconv.Atoi(r.URL.Query().Get("class"))
	if err != nil || !charm.IsCharmClass(classIdx) {
		writeError(w, http.StatusBadRequest, "class must be a charm-capable class index")
		return
	}

	maxLevel := era.MaxLevel(h.cfgMgr.Get().Preferences.PoPEnabled)

	opts := []charmSpellOption{}
	for _, name := range charm.SpellsForClass(classIdx) {
		spell, err := h.db.GetSpellByExactName(name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if spell == nil {
			// Spell isn't in this DB build (shouldn't happen for the catalog);
			// skip rather than surface a half-populated option.
			continue
		}
		// Required level comes from the spell's own class column (quarm.db),
		// which encodes era: PoP charms sit above the level-60 cap and so fall
		// out while pop_enabled is off. A 255 means "not trainer-taught" —
		// available only if it's a known pre-PoP research spell.
		reqLevel := spell.ClassLevels[classIdx]
		if reqLevel == 255 {
			override, ok := charm.ResearchLevel(name)
			if !ok {
				continue
			}
			reqLevel = override
		}
		if reqLevel < 1 || reqLevel > maxLevel {
			continue
		}
		opts = append(opts, charmSpellOption{
			SpellID:       spell.ID,
			Name:          name,
			ReqLevel:      reqLevel,
			MaxCharmLevel: charmMaxLevel(spell),
			Restriction:   charm.RestrictionForTargetType(spell.TargetType).String(),
		})
	}
	// Highest-tier charm first, matching how players think about their line.
	sort.Slice(opts, func(i, j int) bool { return opts[i].ReqLevel > opts[j].ReqLevel })

	writeJSON(w, http.StatusOK, opts)
}

// GET /api/charm/pets?zone={short}&class={idx}&level={n}&spell_id={id}&caster_cha={c}
// Lists the charmable NPCs in a zone for the given charm spell, ranked by DPS.
func (h *charmHandler) pets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zone := q.Get("zone")
	if zone == "" {
		writeError(w, http.StatusBadRequest, "zone is required")
		return
	}
	classIdx, err := strconv.Atoi(q.Get("class"))
	if err != nil || !charm.IsCharmClass(classIdx) {
		writeError(w, http.StatusBadRequest, "class must be a charm-capable class index")
		return
	}
	casterLevel, err := strconv.Atoi(q.Get("level"))
	if err != nil || casterLevel < 1 {
		writeError(w, http.StatusBadRequest, "level is required")
		return
	}
	spellID, err := strconv.Atoi(q.Get("spell_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "spell_id is required")
		return
	}
	casterCHA, _ := strconv.Atoi(q.Get("caster_cha")) // optional; 0 = no CHA bonus

	spell, err := h.db.GetSpell(spellID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "spell not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	charmCap := charmMaxLevel(spell)
	restriction := charm.RestrictionForTargetType(spell.TargetType)
	resistSpell := toResistSpell(spell)
	resistEra := resist.Era{
		PoPEnabled: h.cfgMgr.Get().Preferences.PoPEnabled,
		// Project Quarm is in the Luclin era (disables the pre-Luclin
		// "six-level rule"), matching the resist calculator.
		LuclinEnabled: true,
	}

	candidates, err := h.db.CharmCandidatesByZone(zone)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pets := []charmPet{}
	for _, c := range candidates {
		// Body-type gate (animal/undead charm lines).
		if !charm.BodyTypeAllowed(restriction, c.BodyType) {
			continue
		}
		abilities := db.ParseSpecialAbilities(c.SpecialAbilities)
		// Immune-to-charm NPCs are never pets.
		if hasAbility(abilities, 14) {
			continue
		}

		levelMin := c.Level
		levelMax := c.MaxLevel
		if levelMax < levelMin {
			levelMax = levelMin
		}
		// The whole name is over the cap — none of its spawns can be charmed.
		if charmCap > 0 && levelMin > charmCap {
			continue
		}

		hpLo := charm.ScaledHP(c.HP, c.Level, levelMin)
		hpHi := charm.ScaledHP(c.HP, c.Level, levelMax)
		hitLo := charm.ScaledMaxHit(c.MaxDmg, c.Level, levelMin)
		hitHi := charm.ScaledMaxHit(c.MaxDmg, c.Level, levelMax)
		dpsLo := round1(charm.DPS(c.MinDmg, hitLo, c.AttackDelay))
		dpsHi := round1(charm.DPS(c.MinDmg, hitHi, c.AttackDelay))

		// Worst-case charmable spawn level for the land check: the top of the
		// range, clamped to the spell's cap (spawns above the cap are flagged
		// separately and excluded from the odds).
		landLevel := levelMax
		if charmCap > 0 && landLevel > charmCap {
			landLevel = charmCap
		}
		landRes := resist.ComputeChances(resist.Input{
			Spell:            resistSpell,
			CasterLevel:      casterLevel,
			CasterClass:      classIdx,
			CasterCHA:        casterCHA,
			TargetLevel:      landLevel,
			TargetResist:     candidateResistFor(spell.ResistType, c),
			TargetImmunities: parseImmunities(c.SpecialAbilities),
			Era:              resistEra,
		})

		pets = append(pets, charmPet{
			NPCID:        c.ID,
			Name:         c.Name,
			Class:        c.Class,
			ClassName:    enums.NPCClassName(c.Class),
			BodyType:     c.BodyType,
			BodyTypeName: enums.NPCBodyTypeName(c.BodyType),
			LevelMin:     levelMin,
			LevelMax:     levelMax,
			HPMin:        hpLo,
			HPMax:        hpHi,
			MaxHitMin:    hitLo,
			MaxHitMax:    hitHi,
			AttackDelay:  c.AttackDelay,
			DPSMin:       dpsLo,
			DPSMax:       dpsHi,
			MR:           c.MR,
			Summon:       hasAbility(abilities, 1),
			Gate:         h.npcGates(c.NPCSpellsID, c.ID),
			Caster:       c.NPCSpellsID != 0,
			Abilities:    otherAbilities(abilities),
			LevelWarning: charmCap > 0 && levelMax > charmCap,
			LandChance:   landRes.LandChance,
			Binary:       landRes.Binary,
		})
	}

	// Default ranking: highest DPS first (the headline "best pet" metric).
	sort.SliceStable(pets, func(i, j int) bool { return pets[i].DPSMax > pets[j].DPSMax })

	writeJSON(w, http.StatusOK, charmPetsResponse{
		Zone:          zone,
		SpellID:       spell.ID,
		SpellName:     spell.Name,
		MaxCharmLevel: charmCap,
		Restriction:   restriction.String(),
		Count:         len(pets),
		Pets:          pets,
	})
}

// charmMaxLevel returns the maximum NPC level a spell's charm effect can affect
// (the SPA-22 slot's `max`), or 0 when the spell has no charm effect.
func charmMaxLevel(s *db.Spell) int {
	for i, e := range s.EffectIDs {
		if e == charm.CharmEffectSPA {
			return s.EffectMaxValues[i]
		}
	}
	return 0
}

// npcGates reports whether the NPC's caster AI includes a gate/port spell. Only
// NPCs with a spell list are inspected; the rest short-circuit to false.
func (h *charmHandler) npcGates(npcSpellsID, npcID int) bool {
	if npcSpellsID == 0 {
		return false
	}
	summary, err := h.db.SummarizeNPCCaster(npcID)
	if err != nil || summary == nil {
		return false
	}
	for _, hl := range summary.Highlights {
		if hl.Tag == "gate" {
			return true
		}
	}
	return false
}

// candidateResistFor selects the NPC resist value matching the spell's resist
// type (charm is magic, but selecting by type keeps this correct for any line).
func candidateResistFor(resistType int, c db.CharmCandidate) int {
	switch resistType {
	case 1:
		return c.MR
	case 2:
		return c.FR
	case 3:
		return c.CR
	case 4:
		return c.PR
	case 5:
		return c.DR
	default:
		return 0
	}
}

func hasAbility(abilities []db.SpecialAbility, code int) bool {
	for _, a := range abilities {
		if a.Code == code {
			return true
		}
	}
	return false
}

// otherAbilities renders the notable melee/combat abilities for the "Other
// Abilities" column. Summon (1) is excluded — it has its own column — as are
// the pure immunity flags, which aren't relevant to a charmed pet's offense.
func otherAbilities(abilities []db.SpecialAbility) []string {
	var out []string
	for _, a := range abilities {
		if a.Code == 1 || a.Name == "" {
			continue
		}
		// Immunity-style flags (12+) describe how the mob resists being
		// controlled/damaged, not what it does as a pet; skip them here.
		if a.Code >= 12 {
			continue
		}
		out = append(out, a.Name)
	}
	return out
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}
