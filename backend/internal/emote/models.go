// Package emote lets developer-mode users customize the client-visible spell
// emote text in <EQPath>/spells_en.txt (e.g. disambiguating the shared
// " yawns." slow emote, or adding an emote to a spell that ships without
// one). Overrides are stored structurally in user.db, not as a raw file
// diff, so they survive a server patch replacing spells_en.txt: the patch's
// new content becomes the new base and the user's overrides are re-applied
// on top of it.
package emote

// Field indices within a spells_en.txt line (0-based, after splitting on
// '^'). Field 0 is the spell id (joins to spells_new.id).
const (
	idxYouCast     = 4
	idxOtherCasts  = 5
	idxCastOnYou   = 6
	idxCastOnOther = 7
	idxSpellFades  = 8
	minEmoteFields = idxSpellFades + 1
)

// EmoteText is the five editable emote columns, always populated (never
// nil) — used to report a spell's default or current values.
type EmoteText struct {
	YouCast     string `json:"you_cast"`
	OtherCasts  string `json:"other_casts"`
	CastOnYou   string `json:"cast_on_you"`
	CastOnOther string `json:"cast_on_other"`
	SpellFades  string `json:"spell_fades"`
}

// ColumnsPatch is a PUT request body: only non-nil fields are changed. A
// present-but-empty string is a deliberate override to blank (e.g. clearing
// an emote), distinct from a nil field, which leaves that column's override
// state untouched.
type ColumnsPatch struct {
	YouCast     *string `json:"you_cast"`
	OtherCasts  *string `json:"other_casts"`
	CastOnYou   *string `json:"cast_on_you"`
	CastOnOther *string `json:"cast_on_other"`
	SpellFades  *string `json:"spell_fades"`
}

// Empty reports whether the patch sets no columns at all.
func (p ColumnsPatch) Empty() bool {
	return p.YouCast == nil && p.OtherCasts == nil && p.CastOnYou == nil &&
		p.CastOnOther == nil && p.SpellFades == nil
}

// OverrideRow is one spell's stored override state. A nil field means that
// column is not overridden (falls through to the live file's value).
type OverrideRow struct {
	SpellID     int     `json:"spell_id"`
	YouCast     *string `json:"you_cast"`
	OtherCasts  *string `json:"other_casts"`
	CastOnYou   *string `json:"cast_on_you"`
	CastOnOther *string `json:"cast_on_other"`
	SpellFades  *string `json:"spell_fades"`
	UpdatedAt   int64   `json:"updated_at"`
}

// fieldPtrs returns the row's five columns in file-column order, matching
// idxYouCast..idxSpellFades.
func (o OverrideRow) fieldPtrs() [5]*string {
	return [5]*string{o.YouCast, o.OtherCasts, o.CastOnYou, o.CastOnOther, o.SpellFades}
}

// columnLabels names the five columns in file-column order, for diff display.
var columnLabels = [5]string{"You cast", "Others cast", "Cast on you", "Cast on other", "Spell fades"}

// SpellEmote is the full editor payload for one spell: its default (pristine
// backup) values, its current (live file) values, and which columns are
// actively overridden.
type SpellEmote struct {
	SpellID          int       `json:"spell_id"`
	Name             string    `json:"name"`
	Default          EmoteText `json:"default"`
	Current          EmoteText `json:"current"`
	Customized       bool      `json:"customized"`
	OverriddenFields []string  `json:"overridden_fields"`
}

// FieldDiff is one changed emote column between the pristine default backup
// and the edited backup.
type FieldDiff struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// SpellDiff groups the changed columns for one spell.
type SpellDiff struct {
	SpellID int         `json:"spell_id"`
	Name    string      `json:"name"`
	Fields  []FieldDiff `json:"fields"`
}

// Status is the panel's at-a-glance state.
type Status struct {
	Configured            bool  `json:"configured"`   // EQPath set
	FilePresent           bool  `json:"file_present"` // spells_en.txt found
	HasDefaultBackup      bool  `json:"has_default_backup"`
	OverrideCount         int   `json:"override_count"`
	PendingImportCount    int   `json:"pending_import_count"`
	PendingExternalChange bool  `json:"pending_external_change"`
	ExternalChangeAt      int64 `json:"external_change_at,omitempty"`
}
