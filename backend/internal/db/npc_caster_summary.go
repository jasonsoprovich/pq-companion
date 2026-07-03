package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// NPCCasterSummary is the distilled, overlay-friendly view of an NPC's
// caster AI. Where GetNPCSpells returns the full enumerated list (used by the
// database explorer), this collapses it to the handful of things a player
// actually cares about mid-fight:
//
//   - Highlights: curated callouts (Complete Heal, Gate, AE, mez/charm/etc.).
//   - Procs:      attack/range/defensive procs, named, with their chance.
//   - Signature:  the NPC's own-list ("hand-picked") spells, by name.
//   - ClassLists: inherited parent lists summarized as "<name> (N spells)",
//     never enumerated — the whole point of the feature request.
//
// A nil summary means the NPC has no caster AI (npc_spells_id == 0); the UI
// hides the section entirely.
type NPCCasterSummary struct {
	Highlights        []CasterHighlight  `json:"highlights,omitempty"`
	Procs             []NamedSpell       `json:"procs,omitempty"`
	Signature         []NamedSpell       `json:"signature,omitempty"`
	SignatureOverflow int                `json:"signature_overflow,omitempty"`
	ClassLists        []ClassListSummary `json:"class_lists,omitempty"`
}

// CasterHighlight is one curated callout. Severity is "danger" (combat threat —
// rendered red) or "info" (utility — rendered neutral) so the overlay can colour
// chips without re-deriving meaning.
type CasterHighlight struct {
	Tag      string `json:"tag"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
}

// NamedSpell is a spell referenced by id + name. Chance/Kind are populated only
// for procs ("attack"|"range"|"defensive"); they're omitted for signature casts.
//
// RecastSecs/AEType/AERange/ResistType/ResistDiff are the raid-relevant combat
// details, populated for Signature spells so the overlay's "copy target stats"
// button can print e.g. "Word of Command (30s, 35 PBAE, -100 MR)". They stay
// zero/empty for procs and for spells lacking that attribute.
type NamedSpell struct {
	SpellID   int    `json:"spell_id"`
	SpellName string `json:"spell_name"`
	Chance    int    `json:"chance,omitempty"`
	Kind      string `json:"kind,omitempty"`

	RecastSecs int    `json:"recast_secs,omitempty"` // effective recast, seconds
	AEType     string `json:"ae_type,omitempty"`     // "PBAE" | "TAE" | "AE"
	AERange    int    `json:"ae_range,omitempty"`    // AE radius (game units)
	ResistType string `json:"resist_type,omitempty"` // "MR"|"FR"|"CR"|"PR"|"DR"
	ResistDiff int    `json:"resist_diff,omitempty"` // resist adjust; neg = easier
}

// ClassListSummary is an inherited parent list collapsed to a count.
type ClassListSummary struct {
	ListName string `json:"list_name"`
	Count    int    `json:"count"`
}

// signatureCap bounds how many own-list spells are listed by name before the
// rest are rolled into SignatureOverflow ("+N more").
const signatureCap = 10

// completeHealMinBase is the effect_base_value1 threshold separating a true
// Complete Heal (spell 13 "Complete Healing" heals 7500) from the rest of the
// shared category-20 healing line (Light/Minor/Greater Healing). Verified
// against quarm.db.
const completeHealMinBase = 5000

// casterSpellRow is the enriched in-memory shape used for categorization —
// the spells_new columns needed to classify a spell, plus its source list.
// Internal to the summarizer.
type casterSpellRow struct {
	spellID       int
	name          string
	targetType    int
	aoeRange      int
	category      int
	baseValue1    int
	effects       [12]int
	recastDelayMS int // npc_spells_entries.recast_delay; -1 = use spell default
	recastTimeMS  int // spells_new.recast_time
	resistType    int // spells_new.resisttype (1=magic..5=disease)
	resistDiff    int // spells_new.ResistDiff; negative = easier to land
	ownList       bool // belongs to the NPC's own list (not an inherited parent)
	sourceID      int
	sourceName    string
}

// hasEffect reports whether any of the 12 effect slots carries the given SPA.
func (r casterSpellRow) hasEffect(spa int) bool {
	for _, e := range r.effects {
		if e == spa {
			return true
		}
	}
	return false
}

func (r casterSpellRow) hasTargetType(tts ...int) bool {
	for _, tt := range tts {
		if r.targetType == tt {
			return true
		}
	}
	return false
}

// resistAbbrevs maps spells_new.resisttype to the short label used in-game
// (and on the copy-to-clipboard line). Only the five checkable resists are
// named; unresistable/chromatic/etc. carry no adjust worth printing.
var resistAbbrevs = map[int]string{1: "MR", 2: "FR", 3: "CR", 4: "PR", 5: "DR"}

// namedSpell distills a row into the combat-detail-bearing NamedSpell used for
// signature casts: effective recast (seconds), AE type + radius, and resist
// adjust. Fields stay zero/empty when the spell lacks that attribute.
func (r casterSpellRow) namedSpell() NamedSpell {
	ns := NamedSpell{SpellID: r.spellID, SpellName: r.name}

	// Effective recast: the NPC AI list's own recast_delay takes precedence
	// (that's the mob's re-use cadence); fall back to the spell's recast_time.
	// Both are milliseconds; -1/0 means "unset". Round to the nearest second.
	ms := 0
	if r.recastDelayMS > 0 {
		ms = r.recastDelayMS
	} else if r.recastTimeMS > 0 {
		ms = r.recastTimeMS
	}
	if ms > 0 {
		ns.RecastSecs = (ms + 500) / 1000
	}

	// AE classification: PB targettypes centre on the caster; targeted-AE
	// targettypes centre on the victim. A bare positive radius with a
	// non-AE targettype is labelled generically.
	switch {
	case r.hasTargetType(2, 4, 40):
		ns.AEType, ns.AERange = "PBAE", r.aoeRange
	case r.hasTargetType(8, 20, 24, 25):
		ns.AEType, ns.AERange = "TAE", r.aoeRange
	case r.aoeRange > 0:
		ns.AEType, ns.AERange = "AE", r.aoeRange
	}

	// Resist adjust only reads meaningfully for the five checkable resists and
	// only when there's an actual non-zero modifier.
	if abbrev, ok := resistAbbrevs[r.resistType]; ok && r.resistDiff != 0 {
		ns.ResistType, ns.ResistDiff = abbrev, r.resistDiff
	}

	return ns
}

// SummarizeNPCCaster builds the overlay caster summary for an NPC. Returns
// (nil, nil) when the NPC has no npc_spells_id — same contract as GetNPCSpells.
//
// It reuses the same list + parent_list walk as GetNPCSpells (depth-limited to
// 4 to survive a cyclic/mistyped chain), but selects the extra spells_new
// columns needed for categorization.
func (db *DB) SummarizeNPCCaster(npcID int) (*NPCCasterSummary, error) {
	var listID int
	if err := db.QueryRow(`SELECT npc_spells_id FROM npc_types WHERE id = ?`, npcID).Scan(&listID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get npc %d spells id: %w", npcID, err)
	}
	if listID == 0 {
		return nil, nil
	}

	head, err := db.fetchNPCSpellListRow(listID)
	if err != nil {
		return nil, err
	}
	if head == nil {
		return nil, nil
	}

	// Walk the list + its parent chain, collecting enriched rows. Rows from the
	// head list are tagged ownList=true (signature); the rest are inherited.
	var rows []casterSpellRow
	current := head
	visited := map[int]bool{current.id: true}
	for depth := 0; depth < 4 && current != nil; depth++ {
		entries, err := db.fetchCasterSpellRows(current.id, current.name, current.id == head.id)
		if err != nil {
			return nil, err
		}
		rows = append(rows, entries...)
		if current.parentList == 0 || visited[current.parentList] {
			break
		}
		visited[current.parentList] = true
		current, err = db.fetchNPCSpellListRow(current.parentList)
		if err != nil {
			return nil, err
		}
	}

	out := &NPCCasterSummary{
		Highlights: buildHighlights(rows),
	}

	// Procs come from the head list row only — they don't inherit.
	for _, p := range []struct {
		id, chance int
		kind       string
	}{
		{head.attackProc, head.attackChance, "attack"},
		{head.rangeProc, head.rangeChance, "range"},
		{head.defensiveProc, head.defensiveChance, "defensive"},
	} {
		if p.id > 0 {
			out.Procs = append(out.Procs, NamedSpell{
				SpellID:   p.id,
				SpellName: db.lookupSpellName(p.id),
				Chance:    p.chance,
				Kind:      p.kind,
			})
		}
	}

	// Signature: own-list spells by name, deduped, capped. ClassLists: inherited
	// lists collapsed to a count (grouped by source, preserving first-seen order).
	seenSig := map[int]bool{}
	classByID := map[int]*ClassListSummary{}
	var classOrder []int
	for _, r := range rows {
		if r.ownList {
			if seenSig[r.spellID] {
				continue
			}
			seenSig[r.spellID] = true
			if len(out.Signature) < signatureCap {
				out.Signature = append(out.Signature, r.namedSpell())
			} else {
				out.SignatureOverflow++
			}
			continue
		}
		c, ok := classByID[r.sourceID]
		if !ok {
			c = &ClassListSummary{ListName: r.sourceName}
			classByID[r.sourceID] = c
			classOrder = append(classOrder, r.sourceID)
		}
		c.Count++
	}
	for _, id := range classOrder {
		out.ClassLists = append(out.ClassLists, *classByID[id])
	}

	return out, nil
}

// fetchCasterSpellRows loads the enriched categorization columns for every entry
// in a single npc_spells list (no parent walk — the caller drives that).
func (db *DB) fetchCasterSpellRows(listID int, listName string, ownList bool) ([]casterSpellRow, error) {
	rows, err := db.Query(`
		SELECT e.spellid, COALESCE(s.name, ''),
		       COALESCE(s.targettype, 0), COALESCE(s.aoerange, 0),
		       COALESCE(s.spell_category, 0), COALESCE(s.effect_base_value1, 0),
		       COALESCE(s.effectid1, 254), COALESCE(s.effectid2, 254),
		       COALESCE(s.effectid3, 254), COALESCE(s.effectid4, 254),
		       COALESCE(s.effectid5, 254), COALESCE(s.effectid6, 254),
		       COALESCE(s.effectid7, 254), COALESCE(s.effectid8, 254),
		       COALESCE(s.effectid9, 254), COALESCE(s.effectid10, 254),
		       COALESCE(s.effectid11, 254), COALESCE(s.effectid12, 254),
		       COALESCE(e.recast_delay, 0), COALESCE(s.recast_time, 0),
		       COALESCE(s.resisttype, 0), COALESCE(s.ResistDiff, 0)
		FROM npc_spells_entries e
		LEFT JOIN spells_new s ON s.id = e.spellid
		WHERE e.npc_spells_id = ?
		ORDER BY e.priority DESC, e.minlevel ASC, e.spellid ASC`, listID)
	if err != nil {
		return nil, fmt.Errorf("fetch caster spell rows %d: %w", listID, err)
	}
	defer rows.Close()

	var out []casterSpellRow
	for rows.Next() {
		var r casterSpellRow
		if err := rows.Scan(
			&r.spellID, &r.name,
			&r.targetType, &r.aoeRange, &r.category, &r.baseValue1,
			&r.effects[0], &r.effects[1], &r.effects[2], &r.effects[3],
			&r.effects[4], &r.effects[5], &r.effects[6], &r.effects[7],
			&r.effects[8], &r.effects[9], &r.effects[10], &r.effects[11],
			&r.recastDelayMS, &r.recastTimeMS, &r.resistType, &r.resistDiff,
		); err != nil {
			return nil, fmt.Errorf("scan caster spell row: %w", err)
		}
		r.ownList = ownList
		r.sourceID = listID
		r.sourceName = listName
		out = append(out, r)
	}
	return out, rows.Err()
}

// highlightRule is one curated detection rule. Tags are unique; buildHighlights
// emits each matched tag once, in rule order, so the chip order is stable.
type highlightRule struct {
	tag      string
	label    string
	severity string
	match    func(casterSpellRow) bool
}

// SPA effect codes (spells_new.effectidN) and targettypes used below are the
// EQMacEmu/Quarm values mirrored in internal/db/enums/spell.go.
var highlightRules = []highlightRule{
	{"complete_heal", "Complete Heal", "info", func(r casterSpellRow) bool {
		// SE_CompleteHeal (101), or a big category-20 instant heal (SE_CurrentHP).
		return r.hasEffect(101) || (r.hasEffect(0) && r.category == 20 && r.baseValue1 >= completeHealMinBase)
	}},
	{"gate", "Gate / Port", "info", func(r casterSpellRow) bool {
		// Gate (26), Evacuate/Succor (88), Translocate (104).
		return r.hasEffect(26) || r.hasEffect(88) || r.hasEffect(104)
	}},
	{"pb_ae", "PB AE", "danger", func(r casterSpellRow) bool {
		// ST_AEClientV1 (2), ST_AECaster (4), ST_AEBard (40).
		return r.hasTargetType(2, 4, 40)
	}},
	{"targeted_ae", "AE", "danger", func(r casterSpellRow) bool {
		// ST_AETarget (8), ST_UndeadAE (24), ST_SummonedAE (25), or any radius —
		// excluding the PB targettypes already covered by pb_ae.
		if r.hasTargetType(2, 4, 40) {
			return false
		}
		return r.hasTargetType(8, 24, 25) || r.aoeRange > 0
	}},
	{"mez", "Mez", "danger", func(r casterSpellRow) bool { return r.hasEffect(31) }},
	{"charm", "Charm", "danger", func(r casterSpellRow) bool { return r.hasEffect(22) }},
	{"fear", "Fear", "danger", func(r casterSpellRow) bool { return r.hasEffect(23) }},
	{"root", "Root", "danger", func(r casterSpellRow) bool { return r.hasEffect(99) }},
	{"stun", "Stun", "danger", func(r casterSpellRow) bool { return r.hasEffect(21) || r.hasEffect(64) }},
	{"silence", "Silence", "danger", func(r casterSpellRow) bool { return r.hasEffect(96) }},
	{"dispel", "Dispel", "danger", func(r casterSpellRow) bool { return r.hasEffect(27) || r.hasEffect(154) }},
	{"lifetap", "Lifetap", "danger", func(r casterSpellRow) bool {
		// ST_Tap (13), ST_TargetAETap (20).
		return r.hasTargetType(13, 20)
	}},
	{"memblur", "Memblur", "info", func(r casterSpellRow) bool { return r.hasEffect(63) }},
}

// buildHighlights scans the resolved spell rows and emits the curated callouts.
// Pure (no DB) so it's unit-testable. Each tag appears at most once, in the
// fixed order of highlightRules.
func buildHighlights(rows []casterSpellRow) []CasterHighlight {
	var out []CasterHighlight
	for _, rule := range highlightRules {
		for _, r := range rows {
			if rule.match(r) {
				out = append(out, CasterHighlight{Tag: rule.tag, Label: rule.label, Severity: rule.severity})
				break
			}
		}
	}
	return out
}
