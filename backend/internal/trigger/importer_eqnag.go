package trigger

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// EQNag trigger-database.json shapes. Only the fields we map are modeled; the
// format carries a large amount of overlay/styling state we ignore.

type eqnagDB struct {
	Triggers []eqnagTrigger `json:"triggers"`
	Folders  []eqnagFolder  `json:"folders"`
}

type eqnagFolder struct {
	FolderID string        `json:"folderId"`
	Name     string        `json:"name"`
	Children []eqnagFolder `json:"children"`
}

type eqnagTrigger struct {
	FolderID         string            `json:"folderId"`
	Name             string            `json:"name"`
	Comments         string            `json:"comments"`
	Enabled          bool              `json:"enabled"`
	CapturePhrases   []eqnagPhrase     `json:"capturePhrases"`
	Actions          []eqnagAction     `json:"actions"`
	UseCooldown      bool              `json:"useCooldown"`
	CooldownDuration float64           `json:"cooldownDuration"`
	Conditions       []json.RawMessage `json:"conditions"`
}

type eqnagPhrase struct {
	PhraseID              string `json:"phraseId"`
	UseRegEx              bool   `json:"useRegEx"`
	RequirePreviousPhrase bool   `json:"requirePreviousPhrase"`
	Phrase                string `json:"phrase"`
}

type eqnagAction struct {
	ActionType    int      `json:"actionType"`
	Phrases       []string `json:"phrases"`
	OverlayID     string   `json:"overlayId"`
	Duration      *float64 `json:"duration"`
	DisplayText   string   `json:"displayText"`
	VariableName  string   `json:"variableName"`
	AudioFileID   string   `json:"audioFileId"`
	TextColor     string   `json:"textColor"`
	TextSize      *int     `json:"textSize"`
	TextGlowColor string   `json:"textGlowColor"`
	TextFont      string   `json:"textFont"`
	// Timer endings. The *SpeakPhrase fields are literal spoken text (not
	// phrase IDs); endEarlyPhrases reference capture phrases that end the timer.
	EndingDuration        *float64         `json:"endingDuration"`
	EndingSoonSpeakPhrase string           `json:"endingSoonSpeakPhrase"`
	EndedSpeakPhrase      string           `json:"endedSpeakPhrase"`
	EndEarlyPhrases       []eqnagEndPhrase `json:"endEarlyPhrases"`
}

// eqnagEndPhrase accepts either a bare phraseId string or an inline
// {phraseId, phrase, useRegEx} object, since EQNag has used both shapes.
type eqnagEndPhrase struct {
	PhraseID string `json:"phraseId"`
	Phrase   string `json:"phrase"`
	UseRegEx bool   `json:"useRegEx"`
}

func (e *eqnagEndPhrase) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &e.PhraseID)
	}
	type raw eqnagEndPhrase
	return json.Unmarshal(b, (*raw)(e))
}

// EQNag action type enum (from the community-reverse-engineered table plus the
// content-field analysis of real exports).
const (
	eqnagActDisplay     = 0  // timed overlay text (styled)
	eqnagActAudio       = 1  // play audio (opaque file id — not portable)
	eqnagActSpeak       = 2  // TTS, text in displayText
	eqnagActTimer       = 3  // countdown timer
	eqnagActTimer2      = 4  // countdown timer (variant)
	eqnagActStoreVar    = 5  // store captured value into a named variable
	eqnagActDisplay2    = 7  // display text (+ optional variable store)
	eqnagActDisplay3    = 8  // display text
	eqnagActClipboard   = 9  // clipboard / command
	eqnagActTimerNoOver = 12 // timer without overlay
	eqnagActDeactivate  = 13 // deactivate / clear
	eqnagActListDisplay = 14 // list display
)

// parseEQNagImport parses an EQNag trigger-database.json into an import preview.
func parseEQNagImport(data []byte, sourceName string) (ImportPreview, error) {
	var dbase eqnagDB
	if err := json.Unmarshal(data, &dbase); err != nil {
		return ImportPreview{}, fmt.Errorf("parse EQNag database: %w", err)
	}

	paths := eqnagFolderPaths(dbase.Folders)

	out := make([]ImportedTrigger, 0, len(dbase.Triggers))
	for i := range dbase.Triggers {
		if it, ok := convertEQNag(dbase.Triggers[i], paths); ok {
			out = append(out, it)
		}
	}

	name := strings.TrimSpace(sourceName)
	if name == "" {
		name = "EQNag Import"
	}
	return ImportPreview{Format: FormatEQNag, SourceName: name, Triggers: out}, nil
}

// eqnagFolderPaths builds a folderId → slash-joined path map by walking the
// nested folder tree.
func eqnagFolderPaths(folders []eqnagFolder) map[string]string {
	paths := make(map[string]string)
	var walk func(fs []eqnagFolder, prefix string)
	walk = func(fs []eqnagFolder, prefix string) {
		for _, f := range fs {
			p := strings.TrimSpace(f.Name)
			if prefix != "" {
				p = prefix + "/" + p
			}
			if f.FolderID != "" {
				paths[f.FolderID] = p
			}
			walk(f.Children, p)
		}
	}
	walk(folders, "")
	return paths
}

// convertEQNag maps a single EQNag trigger into an ImportedTrigger. Returns
// ok=false when the trigger has no usable capture phrase.
func convertEQNag(t eqnagTrigger, paths map[string]string) (ImportedTrigger, bool) {
	name := strings.TrimSpace(t.Name)
	if name == "" || len(t.CapturePhrases) == 0 {
		return ImportedTrigger{}, false
	}

	var warnings []string

	// The variable a store-action (type 5) binds the capture to — used to name
	// the first unnamed group in each phrase so ${var} references in the action
	// text resolve consistently no matter which phrase matched.
	storedVar := ""
	for _, a := range t.Actions {
		if a.ActionType == eqnagActStoreVar && a.VariableName != "" {
			storedVar = a.VariableName
			break
		}
	}

	// Primary pattern from the first phrase; remaining phrases (always
	// alternatives in practice — requirePreviousPhrase is unused) become extra
	// patterns. A sequential (require-previous) phrase can't be modeled, so it
	// is flagged and treated as a plain alternative.
	primary, pWarn := eqnagPattern(t.CapturePhrases[0], storedVar)
	warnings = append(warnings, pWarn...)

	var extras []ExtraPattern
	for _, cp := range t.CapturePhrases[1:] {
		if cp.RequirePreviousPhrase {
			warnings = append(warnings, "sequential multi-phrase trigger flattened — verify it still fires correctly")
		}
		pat, w := eqnagPattern(cp, storedVar)
		warnings = append(warnings, w...)
		if pat != "" {
			extras = append(extras, ExtraPattern{Pattern: pat, Enabled: true})
		}
	}

	// Build a phraseId → pattern map for resolving timer end-early references.
	phrasePatterns := make(map[string]string, len(t.CapturePhrases))
	for _, cp := range t.CapturePhrases {
		pat, _ := eqnagPattern(cp, storedVar)
		phrasePatterns[cp.PhraseID] = pat
	}

	tr := Trigger{
		Name:          name,
		Enabled:       true,
		Pattern:       primary,
		Actions:       []Action{},
		ExtraPatterns: extras,
		TimerType:     TimerTypeNone,
	}

	hasAudio := false
	for _, a := range t.Actions {
		switch a.ActionType {
		case eqnagActDisplay, eqnagActDisplay2, eqnagActDisplay3:
			if txt := strings.TrimSpace(a.DisplayText); txt != "" {
				tr.Actions = append(tr.Actions, eqnagOverlayAction(a))
			}
		case eqnagActSpeak:
			if txt := strings.TrimSpace(a.DisplayText); txt != "" {
				tr.Actions = append(tr.Actions, Action{
					Type: ActionTextToSpeech,
					Text: eqnagText(a.DisplayText),
				})
			}
		case eqnagActClipboard:
			if txt := strings.TrimSpace(a.DisplayText); txt != "" {
				tr.Actions = append(tr.Actions, Action{
					Type: ActionClipboard,
					Text: eqnagText(a.DisplayText),
				})
			}
		case eqnagActTimer, eqnagActTimer2, eqnagActTimerNoOver:
			applyEQNagTimer(&tr, a, phrasePatterns, &warnings)
		case eqnagActAudio:
			hasAudio = true
		case eqnagActStoreVar:
			// Plumbing — the value is captured by the named group; no action.
		case eqnagActDeactivate:
			warnings = append(warnings, "EQNag deactivate/clear action dropped (no equivalent)")
		case eqnagActListDisplay:
			warnings = append(warnings, "EQNag list-display action dropped (no equivalent)")
		default:
			warnings = append(warnings, fmt.Sprintf("EQNag action type %d dropped (unsupported)", a.ActionType))
		}
	}

	// Audio → shared sound fallback (convert to speech or drop, with a warning).
	// EQNag references audio by an opaque internal id, so no filename survives.
	if hasAudio {
		warnings = append(warnings, applySoundFallback(&tr, ""))
	}

	if t.UseCooldown && t.CooldownDuration > 0 {
		tr.RefireCooldownSecs = t.CooldownDuration
	}
	if len(t.Conditions) > 0 {
		warnings = append(warnings, "EQNag conditions dropped — imported as a plain match; re-add in-app if needed")
	}

	regexOK := validatePattern(tr.Pattern)
	if !regexOK {
		tr.Enabled = false
		warnings = append(warnings, "pattern doesn't compile under RE2 — imported disabled, edit it in-app")
	}

	return ImportedTrigger{
		Trigger:       tr,
		OriginalGroup: paths[t.FolderID],
		Warnings:      dedupeWarnings(warnings),
		RegexOK:       regexOK,
	}, true
}

// eqnagOverlayAction maps an EQNag display action (type 0/7/8) to an
// overlay_text Action, carrying over styling where present.
func eqnagOverlayAction(a eqnagAction) Action {
	dur := 5
	if a.Duration != nil && *a.Duration > 0 {
		dur = int(*a.Duration)
	}
	act := Action{
		Type:         ActionOverlayText,
		Text:         eqnagText(a.DisplayText),
		DurationSecs: dur,
		Color:        a.TextColor,
		GlowColor:    a.TextGlowColor,
		FontFamily:   strings.TrimSpace(a.TextFont),
	}
	if a.TextSize != nil && *a.TextSize > 0 {
		act.FontSize = *a.TextSize
	}
	return act
}

// applyEQNagTimer maps an EQNag timer action onto the trigger's timer fields.
// EQNag doesn't distinguish buff vs detrimental, so we default to detrimental
// (matching the GINA import); the user can re-categorize after import.
func applyEQNagTimer(tr *Trigger, a eqnagAction, phrasePatterns map[string]string, warnings *[]string) {
	if a.Duration == nil || *a.Duration <= 0 {
		return
	}
	tr.TimerType = TimerTypeDetrimental
	tr.TimerDurationSecs = int(*a.Duration)

	// End-early phrases clear the timer before expiry → worn-off alternation.
	var enders []string
	for _, ep := range a.EndEarlyPhrases {
		pat := strings.TrimSpace(ep.Phrase)
		if pat == "" {
			pat = phrasePatterns[ep.PhraseID]
		}
		if pat != "" {
			enders = append(enders, "(?:"+pat+")")
		}
	}
	if len(enders) > 0 {
		tr.WornOffPattern = strings.Join(enders, "|")
	}

	// "Ending soon" spoken cue → a fading TimerAlert.
	if txt := strings.TrimSpace(a.EndingSoonSpeakPhrase); txt != "" {
		secs := 5
		if a.EndingDuration != nil && *a.EndingDuration > 0 {
			secs = int(*a.EndingDuration)
		}
		tr.TimerAlerts = append(tr.TimerAlerts, TimerAlert{
			Seconds:     secs,
			Type:        TimerAlertTypeTextToSpeech,
			TTSTemplate: eqnagText(txt),
		})
	}
	if strings.TrimSpace(a.EndedSpeakPhrase) != "" {
		*warnings = append(*warnings, "EQNag timer-ended speech not mapped — add a fading alert in-app if wanted")
	}
}

// eqnagVarRe matches EQNag ${VarName} references in patterns and text.
var eqnagVarRe = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// eqnagPattern maps an EQNag capture phrase to a native regex pattern. EQNag's
// cross-line ${Var} references are rewritten to inline named captures so the
// line self-captures the value (the captured text — e.g. a spell name — is
// almost always present in the referencing line too). ${Character} maps to the
// built-in {c} active-character token. When a store-variable name is known and
// the phrase has an unnamed capture group, that group is named so ${Var} in the
// action text resolves regardless of which phrase matched.
func eqnagPattern(cp eqnagPhrase, storedVar string) (string, []string) {
	pat := strings.TrimSpace(cp.Phrase)
	if pat == "" {
		return "", nil
	}

	var warnings []string
	seen := map[string]bool{}
	hadVar := false
	pat = eqnagVarRe.ReplaceAllStringFunc(pat, func(tok string) string {
		varName := tok[2 : len(tok)-1]
		if strings.EqualFold(varName, "Character") {
			return "{c}" // engine substitutes the active character name
		}
		hadVar = true
		if seen[varName] {
			return ".+?" // RE2 has no backreferences for a repeated reference
		}
		seen[varName] = true
		return "(?P<" + varName + ">.+?)"
	})
	if hadVar {
		warnings = append(warnings,
			"EQNag stored variable approximated as an inline capture — verify the trigger still matches")
	}

	// Name the first unnamed group with the stored variable so the action text
	// ${storedVar} → {storedVar} resolves on this phrase too.
	if storedVar != "" && !seen[storedVar] {
		pat = nameFirstUnnamedGroup(pat, storedVar)
	}

	// {TS} is EQNag's timestamp/duration token — we have no equivalent, so the
	// pattern compiles but won't match the time text. Flag it for manual edit.
	if strings.Contains(pat, "{TS}") {
		warnings = append(warnings,
			"EQNag {TS} timestamp token isn't supported — edit the pattern so it matches")
	}

	return pat, warnings
}

// nameFirstUnnamedGroup converts the first plain capturing group "(" in a
// pattern into a named group "(?P<name>". Escaped "\(" and existing "(?…"
// groups are skipped. If there is no unnamed group, the pattern is unchanged.
func nameFirstUnnamedGroup(pattern, name string) string {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '(' {
			continue
		}
		// Count preceding backslashes — an odd count means this "(" is escaped.
		bs := 0
		for j := i - 1; j >= 0 && pattern[j] == '\\'; j-- {
			bs++
		}
		if bs%2 == 1 {
			continue
		}
		if i+1 < len(pattern) && pattern[i+1] == '?' {
			continue // already a non-capturing or named/flag group
		}
		return pattern[:i] + "(?P<" + name + ">" + pattern[i+1:]
	}
	return pattern
}

// eqnagText normalizes EQNag action/alert text into native tokens: ${Character}
// → {c}, other ${X} → {X}.
func eqnagText(s string) string {
	if s == "" {
		return s
	}
	s = eqnagVarRe.ReplaceAllStringFunc(s, func(tok string) string {
		varName := tok[2 : len(tok)-1]
		if strings.EqualFold(varName, "Character") {
			return "{c}"
		}
		return "{" + varName + "}"
	})
	return s
}
