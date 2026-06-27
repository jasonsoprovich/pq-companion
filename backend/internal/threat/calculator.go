package threat

import (
	"strings"
	"sync"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// SPA (spell-effect) codes relevant to hate, matching the SpellEffect IDs
// stored in spells_new (EQMacEmu/Server numbering — the fork Project Quarm
// runs; see internal/db/enums/spell.go). These mirror the cases of
// Mob::CheckAggroAmount in the SecretsOTheP/EQMacEmu fork (zone/aggro.cpp),
// which is the authoritative source for how much hate a cast generates.
const (
	// Damage effects: hate is the observed damage, read from the player's own
	// damage line, so these contribute NOTHING here (they are not in the switch
	// below) — folding them in would double-count.
	spaCurrentHP     = 0  // SE_CurrentHP
	spaCurrentHPOnce = 79 // SE_CurrentHPOnce

	// setStandardHate triggers → the cast adds the HP-scaled "standard hate"
	// (target maxHP/15, clamped). Independent of whether the spell also does
	// damage, so a damage nuke that ALSO stuns/mezzes gets damage + standardHate.
	spaArmorClass    = 1   // SE_ArmorClass     (if value < 0)
	spaMovementSpeed = 3   // SE_MovementSpeed  (snare; if value < 0)
	spaAttackSpeed   = 11  // SE_AttackSpeed    (slow; if value < 100)
	spaBlind         = 20  // SE_Blind
	spaStun          = 21  // SE_Stun
	spaCharm         = 22  // SE_Charm
	spaFear          = 23  // SE_Fear
	spaPoisonCounter = 36  // SE_PoisonCounter
	spaDestroy       = 41  // SE_Destroy
	spaSpinTarget    = 64  // SE_SpinTarget (spin stun)
	spaSilence       = 96  // SE_Silence
	spaAttackSpeed2  = 98  // SE_AttackSpeed2 (slow; if value < 100)
	spaMez           = 31  // SE_Mez
	spaAttackSpeed3  = 119 // SE_AttackSpeed3 (slow; if value < 100)
	spaAmnesia       = 191 // SE_Amnesia

	// Flat nonDamageHate additions (per matching effect).
	spaRoot              = 99  // SE_Root            +10
	spaATK               = 2   // SE_ATK             +10 if value < 0
	spaSTR               = 4   // SE_STR             +10 if value < 0
	spaDEX               = 5   // SE_DEX             +10 if value < 0
	spaAGI               = 6   // SE_AGI             +10 if value < 0
	spaSTA               = 7   // SE_STA             +10 if value < 0
	spaINT               = 8   // SE_INT             +10 if value < 0
	spaWIS               = 9   // SE_WIS             +10 if value < 0
	spaCHA               = 10  // SE_CHA             +10 if value < 0
	spaCurrentMana       = 15  // SE_CurrentMana     +10 if value < 0
	spaResistFire        = 46  // SE_ResistFire      +10 if value < 0
	spaResistCold        = 47  // SE_ResistCold      +10 if value < 0
	spaResistPoison      = 48  // SE_ResistPoison    +10 if value < 0
	spaResistDisease     = 49  // SE_ResistDisease   +10 if value < 0
	spaResistMagic       = 50  // SE_ResistMagic     +10 if value < 0
	spaManaPool          = 97  // SE_ManaPool        +10 if value < 0
	spaEndurance         = 189 // SE_CurrentEndurance +10 if value < 0
	spaResistAll         = 111 // SE_ResistAll       +50 if value < 0
	spaAllStats          = 159 // SE_AllStats        +70 if value < 0
	spaCancelMagic       = 27  // SE_CancelMagic     +1
	spaDispelDetrimental = 154 // SE_DispelDetrimental +1

	// spaInstantHate (SE_InstantHate): flat signed hate (Terror +200..+510;
	// Jolt/Concussion negative — aggro shed). Added on top of everything else.
	spaInstantHate = 92

	// spaChangeAggro (SE_ChangeAggro) and spaSpellHateMod (SE_SpellHateMod) are
	// percentage hate-generation modifiers carried by self-buffs (Glamorous
	// Visage -10, Voice of Terris +10). Tracked as active modifiers, not hate.
	spaChangeAggro  = 114
	spaSpellHateMod = 130
)

// standardSpellHate is CheckAggroAmount's HP-scaled term: the target NPC's max
// HP / 15, clamped. The server floors max HP at 375 (→ 25 hate) and caps the
// result at 1200.
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

	mu sync.Mutex
	// npcCache memoises GetNPCByName by db name; a present key with a nil value
	// is a known miss, so a busy fight never re-queries the same mob.
	npcCache map[string]*db.NPC
	// spellCache memoises GetSpellByExactName by spell name (rows are static).
	// Keeps the per-land hate-mod-buff check off the DB on a busy raid.
	spellCache map[string]*db.Spell
}

// NewCalculator returns a Calculator backed by the given sources. spells may be
// nil (spell-cast hate is then skipped). npcs may be nil (the HP-scaled
// standard-hate term is then skipped; instant hate still works).
func NewCalculator(spells SpellSource, npcs NPCSource) *Calculator {
	return &Calculator{
		spells:     spells,
		npcs:       npcs,
		npcCache:   make(map[string]*db.NPC),
		spellCache: make(map[string]*db.Spell),
	}
}

// lookupSpell returns a spell row by exact name, memoising both hits and misses.
// Returns nil when unknown or when no spell source is wired.
func (c *Calculator) lookupSpell(name string) *db.Spell {
	if c == nil || c.spells == nil || name == "" {
		return nil
	}
	c.mu.Lock()
	if sp, ok := c.spellCache[name]; ok {
		c.mu.Unlock()
		return sp
	}
	c.mu.Unlock()

	var sp *db.Spell
	if s, err := c.spells.GetSpellByExactName(name); err == nil {
		sp = s
	}

	c.mu.Lock()
	c.spellCache[name] = sp
	c.mu.Unlock()
	return sp
}

// HateModBuff reports whether spellName is a beneficial hate-generation buff
// (SPA 114/130) and, if so, its signed percent and buff duration in ticks. Used
// to register a hate-mod buff that LANDS on the player — including one cast by
// another player, which never produces a local "You begin casting" line.
func (c *Calculator) HateModBuff(spellName string) (pct, durationTicks int, ok bool) {
	sp := c.lookupSpell(spellName)
	if sp == nil || sp.GoodEffect != 1 {
		return 0, 0, false
	}
	for i := 0; i < len(sp.EffectIDs); i++ {
		if sp.EffectIDs[i] == spaChangeAggro || sp.EffectIDs[i] == spaSpellHateMod {
			pct += sp.EffectBaseValues[i]
		}
	}
	if pct == 0 {
		return 0, 0, false
	}
	return pct, sp.BuffDuration, true
}

// Classify resolves a cast spell into its hate contribution, attributing the
// HP-scaled standard-hate term to targetNPC (the mob the cast lands on, by
// display name). Returns Found=false when the spell isn't in the DB.
func (c *Calculator) Classify(spellName, targetNPC string) SpellHate {
	if c == nil || c.spells == nil || spellName == "" {
		return SpellHate{}
	}
	sp := c.lookupSpell(spellName)
	if sp == nil {
		return SpellHate{}
	}

	// A beneficial self-buff that modifies hate generation → active modifier,
	// not hate on a mob. Checked first so it short-circuits the offensive path.
	hatemod := 0
	for i := 0; i < len(sp.EffectIDs); i++ {
		if sp.EffectIDs[i] == spaChangeAggro || sp.EffectIDs[i] == spaSpellHateMod {
			hatemod += sp.EffectBaseValues[i]
		}
	}
	if hatemod != 0 && sp.GoodEffect == 1 {
		return SpellHate{Found: true, HatemodPct: hatemod, DurationTicks: sp.BuffDuration}
	}

	// Only detrimental spells generate aggro against their target.
	if sp.GoodEffect != 0 {
		return SpellHate{Found: true}
	}

	// Faithful port of Mob::CheckAggroAmount (SecretsOTheP/EQMacEmu
	// zone/aggro.cpp), computing the NON-damage hate a detrimental cast adds.
	// The `damage` accumulator of the original is excluded — the tracker reads
	// damage hate from the observed damage line — so this returns exactly the
	// cast-added component: instant hate + flat debuff hate + (HP-scaled
	// standard hate when a control/slow/AC effect sets it, regardless of whether
	// the spell ALSO does damage; that is what makes a stun-nuke aggro for
	// damage + standard hate).
	//
	// Known-omitted minor cases (rare in offensive casting, and avoided to keep
	// the SPA mapping reliable): the `slevel*2` melee-debuff/Harmony group, the
	// off-class-clicky 400 cap, the bard-class clamp, and the HateAdded DB
	// override (column is empty in the Quarm dump). These under-count a few
	// uncommon spells slightly rather than risk a wrong value.
	instant := 0
	nonDamageHate := 0
	setStandardHate := false
	for i := 0; i < len(sp.EffectIDs); i++ {
		val := sp.EffectBaseValues[i]
		switch sp.EffectIDs[i] {
		case spaStun, spaBlind, spaMez, spaCharm, spaFear,
			spaSpinTarget, spaAmnesia, spaSilence, spaDestroy, spaPoisonCounter:
			setStandardHate = true
		case spaMovementSpeed, spaArmorClass:
			if val < 0 {
				setStandardHate = true
			}
		case spaAttackSpeed, spaAttackSpeed2, spaAttackSpeed3:
			if val < 100 {
				setStandardHate = true
			}
		case spaRoot:
			nonDamageHate += 10
		case spaATK, spaSTR, spaSTA, spaDEX, spaAGI, spaINT, spaWIS, spaCHA,
			spaResistMagic, spaResistFire, spaResistCold, spaResistPoison,
			spaResistDisease, spaCurrentMana, spaManaPool, spaEndurance:
			if val < 0 {
				nonDamageHate += 10
			}
		case spaResistAll:
			if val < 0 {
				nonDamageHate += 50
			}
		case spaAllStats:
			if val < 0 {
				nonDamageHate += 70
			}
		case spaCancelMagic, spaDispelDetrimental:
			nonDamageHate += 1
		case spaInstantHate:
			instant += val
		}
	}

	// Standard hate: target maxHP/15, clamped. Only addable when the mob's HP is
	// known; otherwise the term is dropped (the instant + flat hate still apply).
	if setStandardHate && targetNPC != "" {
		if hp := c.npcMaxHP(targetNPC); hp > 0 {
			sh := hp / standardHateDivisor
			if sh > standardHateCap {
				sh = standardHateCap
			}
			if sh < standardHateFloor {
				sh = standardHateFloor
			}
			nonDamageHate += sh
		}
	}

	return SpellHate{Found: true, OffensiveHate: int64(nonDamageHate + instant)}
}

// lookupNPC returns the NPC row by display name (spaces converted to the
// underscores the DB stores), caching both hits and misses so a busy fight
// doesn't re-query the same mob. Returns nil when unknown.
func (c *Calculator) lookupNPC(displayName string) *db.NPC {
	key := strings.ReplaceAll(displayName, " ", "_")
	c.mu.Lock()
	if n, ok := c.npcCache[key]; ok {
		c.mu.Unlock()
		return n
	}
	c.mu.Unlock()

	var npc *db.NPC
	if c.npcs != nil {
		if n, err := c.npcs.GetNPCByName(key); err == nil {
			npc = n
		}
	}

	c.mu.Lock()
	c.npcCache[key] = npc
	c.mu.Unlock()
	return npc
}

// npcMaxHP returns the NPC's database max HP by display name. 0 means unknown.
func (c *Calculator) npcMaxHP(displayName string) int {
	if n := c.lookupNPC(displayName); n != nil {
		return n.HP
	}
	return 0
}

// NPCLevel returns the NPC's database level by display name. 0 means unknown.
func (c *Calculator) NPCLevel(displayName string) int {
	if n := c.lookupNPC(displayName); n != nil {
		return n.Level
	}
	return 0
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
