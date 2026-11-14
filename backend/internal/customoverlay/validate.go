package customoverlay

import (
	"fmt"
	"regexp"
	"strings"
)

var keyRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// widgetOps lists which op types may target each widget type.
var widgetOps = map[OpType]WidgetType{
	OpSetField:       WidgetFields,
	OpClearField:     WidgetFields,
	OpUpsertRow:      WidgetTable,
	OpRemoveRow:      WidgetTable,
	OpAddLine:        WidgetFeed,
	OpCounterAdd:     WidgetCounter,
	OpCounterSet:     WidgetCounter,
	OpCountdownStart: WidgetCountdown,
	OpCountdownStop:  WidgetCountdown,
}

// Validate checks a definition and returns it normalized: names trimmed,
// missing widget/rule IDs generated, table/feed limits defaulted and
// clamped. Every pattern must compile with RE2. It is the gate for both the
// editor and imported/pasted JSON, so it never trusts sizes or references.
func Validate(def OverlayDef) (OverlayDef, error) {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return def, fmt.Errorf("name is required")
	}
	if len(def.Name) > maxNameLen {
		return def, fmt.Errorf("name is longer than %d characters", maxNameLen)
	}
	if len(def.Widgets) == 0 {
		return def, fmt.Errorf("add at least one widget")
	}
	if len(def.Widgets) > maxWidgets {
		return def, fmt.Errorf("at most %d widgets", maxWidgets)
	}
	if len(def.Rules) > maxRules {
		return def, fmt.Errorf("at most %d rules", maxRules)
	}

	chars := def.Characters[:0:0]
	for _, c := range def.Characters {
		if c = strings.TrimSpace(c); c != "" {
			chars = append(chars, c)
		}
	}
	def.Characters = chars

	widgets := make(map[string]*Widget, len(def.Widgets))
	def.Widgets = append([]Widget(nil), def.Widgets...)
	for i := range def.Widgets {
		w := &def.Widgets[i]
		if w.ID == "" {
			w.ID = newID()
		}
		if !keyRe.MatchString(w.ID) {
			return def, fmt.Errorf("widget %d: invalid id %q", i+1, w.ID)
		}
		if widgets[w.ID] != nil {
			return def, fmt.Errorf("widget %d: duplicate id %q", i+1, w.ID)
		}
		w.Title = strings.TrimSpace(w.Title)
		if len(w.Title) > maxNameLen {
			return def, fmt.Errorf("widget %d: title is longer than %d characters", i+1, maxNameLen)
		}
		if err := validateWidget(w); err != nil {
			return def, fmt.Errorf("widget %d (%s): %w", i+1, label(w.Title, string(w.Type)), err)
		}
		widgets[w.ID] = w
	}

	ruleIDs := make(map[string]bool, len(def.Rules))
	def.Rules = append([]Rule(nil), def.Rules...)
	for i := range def.Rules {
		r := &def.Rules[i]
		if r.ID == "" {
			r.ID = newID()
		}
		if !keyRe.MatchString(r.ID) || ruleIDs[r.ID] {
			return def, fmt.Errorf("rule %d: invalid or duplicate id %q", i+1, r.ID)
		}
		ruleIDs[r.ID] = true
		if err := validateRule(r, widgets); err != nil {
			return def, fmt.Errorf("rule %d (%s): %w", i+1, label(r.Name, r.Pattern), err)
		}
	}
	return def, nil
}

func label(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	if len(fallback) > 30 {
		return fallback[:30] + "…"
	}
	return fallback
}

func validateWidget(w *Widget) error {
	switch w.Type {
	case WidgetFields:
		if len(w.Fields) == 0 || len(w.Fields) > maxFields {
			return fmt.Errorf("needs 1–%d fields", maxFields)
		}
		seen := map[string]bool{}
		for _, f := range w.Fields {
			if !keyRe.MatchString(f.Key) || seen[f.Key] {
				return fmt.Errorf("invalid or duplicate field key %q", f.Key)
			}
			seen[f.Key] = true
		}
	case WidgetTable:
		if len(w.Columns) == 0 || len(w.Columns) > maxColumns {
			return fmt.Errorf("needs 1–%d columns", maxColumns)
		}
		seen := map[string]bool{}
		for _, c := range w.Columns {
			if !keyRe.MatchString(c.Key) || seen[c.Key] {
				return fmt.Errorf("invalid or duplicate column key %q", c.Key)
			}
			seen[c.Key] = true
		}
		if w.SortBy != "" && !seen[w.SortBy] {
			return fmt.Errorf("sort column %q doesn't exist", w.SortBy)
		}
		switch w.Merge {
		case "":
			w.Merge = MergeReplace
		case MergeReplace:
		case MergeHighest, MergeAdd:
			if !seen[w.MergeColumn] {
				return fmt.Errorf("merge mode %q needs an existing merge column", w.Merge)
			}
		default:
			return fmt.Errorf("unknown merge mode %q", w.Merge)
		}
		if w.MaxRows <= 0 {
			w.MaxRows = defaultMaxRows
		}
		if w.MaxRows > limitMaxRows {
			w.MaxRows = limitMaxRows
		}
	case WidgetFeed:
		if w.MaxLines <= 0 {
			w.MaxLines = defaultMaxLines
		}
		if w.MaxLines > limitMaxLines {
			w.MaxLines = limitMaxLines
		}
	case WidgetCounter:
	case WidgetCountdown:
		if w.Seconds < 0 || w.Seconds > 24*3600 {
			return fmt.Errorf("default duration must be 0–24h")
		}
	default:
		return fmt.Errorf("unknown widget type %q", w.Type)
	}
	return nil
}

func validateRule(r *Rule, widgets map[string]*Widget) error {
	r.Channel = strings.ToLower(strings.TrimSpace(r.Channel))
	if r.Channel != "" && !keyRe.MatchString(r.Channel) {
		return fmt.Errorf("invalid channel %q", r.Channel)
	}
	switch r.Direction {
	case "", DirectionIn, DirectionOut:
	default:
		return fmt.Errorf("unknown direction %q", r.Direction)
	}
	if r.Channel == "" && (r.Direction != "" || len(r.Speakers) > 0) {
		return fmt.Errorf("direction and speaker filters need a channel")
	}
	speakers := r.Speakers[:0:0]
	for _, s := range r.Speakers {
		if s = strings.TrimSpace(s); s != "" {
			speakers = append(speakers, s)
		}
	}
	r.Speakers = speakers

	if r.Simple != nil {
		p, err := simplePattern(*r.Simple)
		if err != nil {
			return err
		}
		r.Simple = &SimpleMatch{Kind: r.Simple.Kind, Text: strings.TrimSpace(r.Simple.Text)}
		r.Pattern = p
	}
	if r.Pattern == "" && r.Channel == "" {
		return fmt.Errorf("pattern is required (or pick a channel)")
	}
	if len(r.Pattern) > maxPatternLen {
		return fmt.Errorf("pattern is longer than %d characters", maxPatternLen)
	}
	if r.Pattern != "" {
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return fmt.Errorf("pattern: %w (lookahead/lookbehind and backreferences aren't supported)", err)
		}
	}
	if len(r.Ops) == 0 || len(r.Ops) > maxOpsPerRule {
		return fmt.Errorf("needs 1–%d actions", maxOpsPerRule)
	}
	for j, op := range r.Ops {
		if err := validateOp(op, widgets); err != nil {
			return fmt.Errorf("action %d: %w", j+1, err)
		}
	}
	return nil
}

func validateOp(op Op, widgets map[string]*Widget) error {
	for _, t := range append([]string{op.Value, op.Key, op.Field}, cellValues(op.Cells)...) {
		if len(t) > maxTemplateLen {
			return fmt.Errorf("text is longer than %d characters", maxTemplateLen)
		}
	}
	if op.Type == OpResetAll {
		return nil
	}
	w := widgets[op.Widget]
	if w == nil {
		return fmt.Errorf("unknown widget %q", op.Widget)
	}
	if op.Type == OpResetWidget {
		return nil
	}
	want, ok := widgetOps[op.Type]
	if !ok {
		return fmt.Errorf("unknown action %q", op.Type)
	}
	if w.Type != want {
		return fmt.Errorf("%s can't target a %s widget", op.Type, w.Type)
	}
	switch op.Type {
	case OpSetField:
		if !hasField(w, op.Field) {
			return fmt.Errorf("unknown field %q", op.Field)
		}
	case OpClearField:
		if op.Field != "" && !hasField(w, op.Field) {
			return fmt.Errorf("unknown field %q", op.Field)
		}
	case OpUpsertRow:
		if strings.TrimSpace(op.Key) == "" {
			return fmt.Errorf("row key is required")
		}
		if len(op.Cells) > maxColumns {
			return fmt.Errorf("too many cells")
		}
		for k := range op.Cells {
			if !hasColumn(w, k) {
				return fmt.Errorf("unknown column %q", k)
			}
		}
	case OpRemoveRow:
		if strings.TrimSpace(op.Key) == "" {
			return fmt.Errorf("row key is required")
		}
	case OpAddLine:
		if op.Value == "" {
			return fmt.Errorf("line text is required")
		}
	case OpCounterSet:
		if op.Value == "" {
			return fmt.Errorf("value is required")
		}
	case OpCountdownStart:
		if op.Value == "" && w.Seconds <= 0 {
			return fmt.Errorf("set a duration here or on the countdown widget")
		}
	}
	return nil
}

func cellValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func hasField(w *Widget, key string) bool {
	for _, f := range w.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

func hasColumn(w *Widget, key string) bool {
	for _, c := range w.Columns {
		if c.Key == key {
			return true
		}
	}
	return false
}
