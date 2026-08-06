// Package progress implements the Progression Recap: a per-character,
// timestamped journal of level, AA, spell, and skill milestones (parsed from
// the EQ log, and therefore backfillable), plus a forward-only snapshot of
// values that only exist in a Zeal export and can't be recovered from the log
// (coin, absolute AA/tradeskill totals, spells known).
package progress

import "time"

// Kind identifies what a journal Event records.
type Kind string

const (
	KindLevel Kind = "level" // Detail empty, Value = resulting level, Delta = levels gained (or -1 on a level-drain loss)
	KindAA    Kind = "aa"    // Detail empty, Value = unspent AA pool after the gain, Delta unused
	KindSkill Kind = "skill" // Detail = skill display name, Value = new rank, Delta unused
	KindSpell Kind = "spell" // Detail = spell name, Value/Delta unused
)

// Event is one row of the append-only progression journal.
type Event struct {
	Character string    `json:"character"`
	At        time.Time `json:"at"`
	Kind      Kind      `json:"kind"`
	Detail    string    `json:"detail,omitempty"`
	Value     int       `json:"value"`
	Delta     int       `json:"delta,omitempty"`
}

// Snapshot is one row of the forward-only totals capture, taken from a
// character's Quarmy/inventory export.
type Snapshot struct {
	Character       string    `json:"character"`
	TakenAt         time.Time `json:"taken_at"`
	Level           int       `json:"level"`
	AARanks         int       `json:"aa_ranks"`
	TradeskillTotal int       `json:"tradeskill_total"`
	SpellsKnown     int       `json:"spells_known"`
	Copper          int64     `json:"copper"`
}
