package threat

import (
	"strings"
	"sync"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// SPA (spell-effect) codes relevant to hate, matching the SpellEffect IDs
// stored in spells_new (EQMacEmu/Server numbering — the fork Project Quarm
// runs; see internal/db/enums/spell.go).
const (
	// spaCurrentHP (SE_CurrentHP) and spaCurrentHPOnce (SE_CurrentHPOnce) carry
	// damage when their base value is negative. Damage-driven hate is observed
	// from the character's own damage lines, so a spell with a damage effect
	// does NOT also get the HP-scaled "standard" hate below.
	spaCurrentHP     = 0
	spaCurrentHPOnce = 79
	// spaInstantHate (SE_InstantHate): flat signed hate applied on land, stored
	// as the effect base value (Terror +200..+510; Jolt/Concussion negative).
	spaInstantHate = 92
	// spaChangeAggro (SE_ChangeAggro) and spaSpellHateMod (SE_SpellHateMod) are
	// percentage hate-generation modifiers carried by self-buffs (Glamorous
	// Visage -10, Voice of Terris +10). Tracked as active modifiers, not as hate
	// on a mob.
	spaChangeAggro  = 114
	spaSpellHateMod = 130
)

// standardSpellHate is the EQMacEmu CheckAggroAmount value a no-damage
// detrimental control/debuff spell (mez/slow/snare/root/tash/debuff) generates:
// the target NPC's max HP / 15, clamped. Damage spells and instant-hate spells
// don't use it (their hate is the observed damage / the SE_InstantHate value).
const (
	standardHateDivisor = 15
	standardHateCap     = 1200
	standardHateFloor   = 25
)

// healHateCap caps CheckHealAggroAmount. The server uses 800 for casters at or
// below level 50 and 1500 above; this meter targets max-level raiders (CH
// chains), so it uses the higher cap.
const healHateCap = 1500

// SpellSource looks up a spell row by its exact name (the form in a "You begin
// casting <Name>" line). Backed by *db.DB.
type SpellSource interface {
	GetSpellByExactName(name string) (*db.Spell, error)
}

// NPCSource looks up an NPC row by its database name (underscores, not spaces).
// Backed by *db.DB. Used for the HP-scaled standard-hate formula.
type NPCSource interface {
	GetNPCByName(name string) (*db.NPC, error)
}

// SpellHate is the classified hate contribution of a cast spell.
type SpellHate struct {
	Found bool
	// HatemodPct, when non-zero, marks this as a beneficial self-buff that
	// modifies hate generation by this signed percentage (SPA 114/130). The
	// tracker registers it as an active modifier rather than adding hate.
	HatemodPct int
	// DurationTicks is the buff's duration (6s ticks) used to expire the
	// modifier. Only meaningful when HatemodPct != 0.
	DurationTicks int
	// OffensiveHate is the non-damage hate the cast adds to its target mob
	// (instant hate + HP-scaled standard hate). Signed: aggro shedders are
	// negative. Zero when the spell is a pure nuke/DoT (hate comes from damage)
	// or a hate-mod buff.
	OffensiveHate int64
}

// Calculator converts cast spells and heals into the hate they generate beyond
// observed damage. Damage hate is intentionally NOT computed here — it is read
// from the character's own damage lines, so folding it in would double-count.
type Calculator struct {
	spells SpellSource
	npcs   NPCSource

	mu      sync.Mutex
	hpCache map[string]int // db npc name → max HP (negative cache: 0 = unknown)
}

// NewCalculator returns a Calculator backed by the given sources. spells may be
// nil (spell-cast hate is then skipped). npcs may be nil (the HP-scaled
// standard-hate term is then skipped; instant hate still works).
func NewCalculator(spells SpellSource, npcs NPCSource) *Calculator {
	return &Calculator{spells: spells, npcs: npcs, hpCache: make(map[string]int)}
}

// Classify resolves a cast spell into its hate contribution, attributing the
// HP-scaled standard-hate term to targetNPC (the mob the cast lands on, by
// display name). Returns Found=false when the spell isn't in the DB.
func (c *Calculator) Classify(spellName, targetNPC string) SpellHate {
	if c == nil || c.spells == nil || spellName == "" {
		return SpellHate{}
	}
	sp, err := c.spells.GetSpellByExactName(spellName)
	if err != nil || sp == nil {
		return SpellHate{}
	}

	var instant int64
	var hatemod int
	hasDamage := false
	for i := 0; i < len(sp.EffectIDs); i++ {
		switch sp.EffectIDs[i] {
		case spaInstantHate:
			instant += int64(sp.EffectBaseValues[i])
		case spaChangeAggro, spaSpellHateMod:
			hatemod += sp.EffectBaseValues[i]
		case spaCurrentHP, spaCurrentHPOnce:
			if sp.EffectBaseValues[i] < 0 {
				hasDamage = true
			}
		}
	}

	// A beneficial self-buff that modifies hate generation → active modifier.
	if hatemod != 0 && sp.GoodEffect == 1 {
		return SpellHate{Found: true, HatemodPct: hatemod, DurationTicks: sp.BuffDuration}
	}

	offensive := instant
	// HP-scaled standard hate for a no-damage detrimental utility spell. Skipped
	// for damage spells (hate = observed damage) and instant-hate spells
	// (fully described by their SE_InstantHate value, e.g. Jolt sheds, Terror
	// adds) so neither is over-counted.
	if instant == 0 && !hasDamage && sp.GoodEffect == 0 && targetNPC != "" {
		if hp := c.npcMaxHP(targetNPC); hp > 0 {
			sh := hp / standardHateDivisor
			if sh > standardHateCap {
				sh = standardHateCap
			}
			if sh < standardHateFloor {
				sh = standardHateFloor
			}
			offensive += int64(sh)
		}
	}
	return SpellHate{Found: true, OffensiveHate: offensive}
}

// npcMaxHP returns the NPC's database max HP by display name (spaces converted
// to the underscores the DB stores), caching both hits and misses so a busy
// fight doesn't re-query the same mob. 0 means unknown.
func (c *Calculator) npcMaxHP(displayName string) int {
	key := strings.ReplaceAll(displayName, " ", "_")
	c.mu.Lock()
	if hp, ok := c.hpCache[key]; ok {
		c.mu.Unlock()
		return hp
	}
	c.mu.Unlock()

	hp := 0
	if c.npcs != nil {
		if n, err := c.npcs.GetNPCByName(key); err == nil && n != nil {
			hp = n.HP
		}
	}

	c.mu.Lock()
	c.hpCache[key] = hp
	c.mu.Unlock()
	return hp
}

// HealHate is the EQMacEmu CheckHealAggroAmount value for a heal of the given
// amount: 1 + 2*amount/3, capped. Pure (no DB), so it lives as a function.
func HealHate(amount int) int64 {
	if amount <= 0 {
		return 0
	}
	h := int64(1 + 2*amount/3)
	if h > healHateCap {
		h = healHateCap
	}
	return h
}
