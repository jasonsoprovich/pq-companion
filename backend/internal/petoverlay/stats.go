package petoverlay

import (
	"math"

	"github.com/jasonsoprovich/pq-companion/backend/internal/charm"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
)

// Special-ability codes the pet card surfaces (EQMacEmu SpecialAbility
// numbering — see internal/db/enums/special_abilities.go).
const (
	abilitySummon        = 1
	abilityImmuneSlow    = 12
	abilityImmuneMez     = 13
	abilityImmuneStun    = 15
	abilityImmuneSnare   = 16 // snare and root
	abilityImmuneFear    = 17
	abilityImmuneRanged  = 26 // "belly caster"
	abilityReverseSlow   = 50
	abilityImmuneHaste   = 51
	abilityFirstImmunity = 12 // codes >= this are immunity/AI flags, not offense
	gateHighlightTag     = "gate"
)

// Stats is the pet card's summary of the pet's npc_types row, scaled to its
// spawn level. When the exact level is unknown (no Zeal target descriptor was
// ever seen for the pet) the scaled fields span the row's level range.
type Stats struct {
	NPCID     int    `json:"npc_id"`
	Class     int    `json:"class"`
	ClassName string `json:"class_name"`
	Race      int    `json:"race"`
	RaceName  string `json:"race_name"`
	BodyType  int    `json:"body_type"`

	// Level is the exact live level when known (0 otherwise); LevelMin/Max is
	// the row's spawn range and collapses to Level when it is known.
	Level    int `json:"level,omitempty"`
	LevelMin int `json:"level_min"`
	LevelMax int `json:"level_max"`

	HPMin       int     `json:"hp_min"`
	HPMax       int     `json:"hp_max"`
	MinHit      int     `json:"min_hit"`
	MaxHitMin   int     `json:"max_hit_min"`
	MaxHitMax   int     `json:"max_hit_max"`
	AttackDelay int     `json:"attack_delay"` // tenths of a second
	DPSMin      float64 `json:"dps_min"`
	DPSMax      float64 `json:"dps_max"`

	MR int `json:"mr"`
	FR int `json:"fr"`
	CR int `json:"cr"`
	PR int `json:"pr"`
	DR int `json:"dr"`

	Summons bool `json:"summons"`
	Gates   bool `json:"gates"`
	Caster  bool `json:"caster"`

	// Immunities are the control/buff immunities that matter when managing a
	// pet: whether it can be rooted, stunned, mezzed, feared, slowed, hasted.
	Immunities Immunities `json:"immunities"`

	// Offense lists the notable melee abilities (Enrage, Rampage, Flurry,
	// Dual Wield, ...) by display name, Summon excluded (it has its own flag).
	Offense []string `json:"offense,omitempty"`
}

// Immunities flags the special abilities a pet owner cares about.
type Immunities struct {
	Slow        bool `json:"slow,omitempty"`
	Mez         bool `json:"mez,omitempty"`
	Stun        bool `json:"stun,omitempty"`
	SnareRoot   bool `json:"snare_root,omitempty"`
	Fear        bool `json:"fear,omitempty"`
	Haste       bool `json:"haste,omitempty"`
	ReverseSlow bool `json:"reverse_slow,omitempty"`
	Ranged      bool `json:"ranged,omitempty"`
}

// BuildStats summarises npc for the pet card. level is the pet's exact live
// level, or 0 when unknown. Scaling matches the Charm Pet Finder (HP linear in
// level, max hit +2/level, min hit constant) so both views agree.
func BuildStats(npc *db.NPC, abilities []db.SpecialAbility, summary *db.NPCCasterSummary, level int) *Stats {
	if npc == nil {
		return nil
	}
	lo, hi := npc.Level, npc.MaxLevel
	if hi < lo {
		hi = lo
	}
	if level > 0 {
		lo, hi = level, level
	}

	maxHitLo := charm.ScaledMaxHit(npc.MaxDmg, npc.Level, lo)
	maxHitHi := charm.ScaledMaxHit(npc.MaxDmg, npc.Level, hi)
	s := &Stats{
		NPCID:       npc.ID,
		Class:       npc.Class,
		ClassName:   enums.NPCClassName(npc.Class),
		Race:        npc.Race,
		RaceName:    npc.RaceName,
		BodyType:    npc.BodyType,
		LevelMin:    lo,
		LevelMax:    hi,
		HPMin:       charm.ScaledHP(npc.HP, npc.Level, lo),
		HPMax:       charm.ScaledHP(npc.HP, npc.Level, hi),
		MinHit:      npc.MinDmg,
		MaxHitMin:   maxHitLo,
		MaxHitMax:   maxHitHi,
		AttackDelay: npc.AttackDelay,
		DPSMin:      round1(charm.DPS(npc.MinDmg, maxHitLo, npc.AttackDelay)),
		DPSMax:      round1(charm.DPS(npc.MinDmg, maxHitHi, npc.AttackDelay)),
		MR:          npc.MR,
		FR:          npc.FR,
		CR:          npc.CR,
		PR:          npc.PR,
		DR:          npc.DR,
		Caster:      npc.NPCSpellsID != 0,
	}
	if level > 0 {
		s.Level = level
	}
	if summary != nil {
		for _, h := range summary.Highlights {
			if h.Tag == gateHighlightTag {
				s.Gates = true
				break
			}
		}
	}
	for _, a := range abilities {
		switch a.Code {
		case abilitySummon:
			s.Summons = true
		case abilityImmuneSlow:
			s.Immunities.Slow = true
		case abilityImmuneMez:
			s.Immunities.Mez = true
		case abilityImmuneStun:
			s.Immunities.Stun = true
		case abilityImmuneSnare:
			s.Immunities.SnareRoot = true
		case abilityImmuneFear:
			s.Immunities.Fear = true
		case abilityImmuneRanged:
			s.Immunities.Ranged = true
		case abilityReverseSlow:
			s.Immunities.ReverseSlow = true
		case abilityImmuneHaste:
			s.Immunities.Haste = true
		default:
			if a.Code < abilityFirstImmunity && a.Name != "" {
				s.Offense = append(s.Offense, a.Name)
			}
		}
	}
	return s
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
