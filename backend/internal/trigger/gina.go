package trigger

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ginaDocument is the root of GINA's <SharedData>…</SharedData> trigger share
// XML. GINA packages triggers inside TriggerGroups, optionally nested; the
// same Trigger shape also appears at the SharedData level on some exports.
type ginaDocument struct {
	XMLName       xml.Name           `xml:"SharedData"`
	TriggerGroups []ginaTriggerGroup `xml:"TriggerGroups>TriggerGroup"`
	Triggers      []ginaTrigger      `xml:"Triggers>Trigger"`
}

type ginaTriggerGroup struct {
	Name          string             `xml:"Name"`
	TriggerGroups []ginaTriggerGroup `xml:"TriggerGroups>TriggerGroup"`
	Triggers      []ginaTrigger      `xml:"Triggers>Trigger"`
}

// ginaTrigger mirrors GINA's Trigger element. Unused fields are omitted.
type ginaTrigger struct {
	Name                     string `xml:"Name"`
	TriggerText              string `xml:"TriggerText"`
	EnableRegex              string `xml:"EnableRegex"` // "True" / "False"
	Category                 string `xml:"Category"`
	Comments                 string `xml:"Comments"`
	UseText                  string `xml:"UseText"`
	DisplayText              string `xml:"DisplayText"`
	UseTextToVoice           string `xml:"UseTextToVoice"`
	TextToVoiceText          string `xml:"TextToVoiceText"`
	PlayMediaFile            string `xml:"PlayMediaFile"`
	TimerType                string `xml:"TimerType"`     // "NoTimer", "Timer", "RepeatingTimer", "Stopwatch"
	TimerDuration            string `xml:"TimerDuration"` // seconds OR HH:MM:SS
	TimerMillisecondDuration string `xml:"TimerMillisecondDuration"`
	TimerEndingTrigger       string `xml:"TimerEndingTrigger"`
	TimerEarlyEnders         struct {
		Enders []struct {
			EnableRegex   string `xml:"EnableRegex"`
			EndingTrigger string `xml:"EndingTrigger"`
		} `xml:"EarlyEnder"`
	} `xml:"TimerEarlyEnders"`
}

// parseGINAImport parses a GINA trigger share XML document into an import
// preview, walking the nested TriggerGroup hierarchy so each trigger carries
// the slash-joined group path it lived under.
func parseGINAImport(data []byte, sourceName string) (ImportPreview, error) {
	var doc ginaDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return ImportPreview{}, fmt.Errorf("parse gina xml: %w", err)
	}

	var out []ImportedTrigger
	var walk func(group ginaTriggerGroup, path []string)
	walk = func(group ginaTriggerGroup, path []string) {
		here := path
		if n := strings.TrimSpace(group.Name); n != "" {
			here = append(append([]string(nil), path...), n)
		}
		for _, gt := range group.Triggers {
			if it, ok := convertGINA(gt); ok {
				it.OriginalGroup = strings.Join(here, "/")
				out = append(out, it)
			}
		}
		for _, sub := range group.TriggerGroups {
			walk(sub, here)
		}
	}

	// Top-level Triggers (rare but valid)
	for _, gt := range doc.Triggers {
		if it, ok := convertGINA(gt); ok {
			out = append(out, it)
		}
	}
	for _, group := range doc.TriggerGroups {
		walk(group, nil)
	}

	name := strings.TrimSpace(sourceName)
	if name == "" {
		name = "GINA Import"
	}
	return ImportPreview{Format: FormatGINA, SourceName: name, Triggers: out}, nil
}

// convertGINA maps a single ginaTrigger into an ImportedTrigger. Returns
// ok=false when the GINA entry lacks the minimum required fields (name +
// trigger text).
func convertGINA(g ginaTrigger) (ImportedTrigger, bool) {
	name := strings.TrimSpace(g.Name)
	pattern := strings.TrimSpace(g.TriggerText)
	if name == "" || pattern == "" {
		return ImportedTrigger{}, false
	}

	// Convert literal text into a regex when GINA marked it as non-regex.
	if !truthy(g.EnableRegex) {
		pattern = regexp.QuoteMeta(pattern)
	}

	var warnings []string
	actions := make([]Action, 0, 3)
	if truthy(g.UseText) && strings.TrimSpace(g.DisplayText) != "" {
		actions = append(actions, Action{
			Type:         ActionOverlayText,
			Text:         normalizeActionText(g.DisplayText),
			DurationSecs: 5,
			Color:        "#ffffff",
		})
	}
	if truthy(g.UseTextToVoice) && strings.TrimSpace(g.TextToVoiceText) != "" {
		actions = append(actions, Action{
			Type: ActionTextToSpeech,
			Text: normalizeActionText(g.TextToVoiceText),
		})
	}

	tt := TimerTypeNone
	duration := 0
	if strings.EqualFold(g.TimerType, "Timer") || strings.EqualFold(g.TimerType, "RepeatingTimer") {
		duration = ginaDurationSecs(g.TimerDuration, g.TimerMillisecondDuration)
		if duration > 0 {
			// GINA doesn't distinguish buff vs detrimental — default to
			// detrimental since that's more common in share packs. Users can
			// re-categorize in the UI after import.
			tt = TimerTypeDetrimental
		}
	}

	wornOff := strings.TrimSpace(g.TimerEndingTrigger)
	if wornOff == "" && len(g.TimerEarlyEnders.Enders) > 0 {
		// Combine early-ender patterns into a single alternation.
		var parts []string
		for _, ee := range g.TimerEarlyEnders.Enders {
			s := strings.TrimSpace(ee.EndingTrigger)
			if s == "" {
				continue
			}
			if !truthy(ee.EnableRegex) {
				s = regexp.QuoteMeta(s)
			}
			parts = append(parts, "(?:"+s+")")
		}
		wornOff = strings.Join(parts, "|")
	}

	t := Trigger{
		Name:              name,
		Enabled:           true,
		Pattern:           pattern,
		Actions:           actions,
		TimerType:         tt,
		TimerDurationSecs: duration,
		WornOffPattern:    wornOff,
	}

	// GINA's <PlayMediaFile> is a bare boolean — the export carries no filename
	// and the .gtp bundles no audio. When set, route through the shared sound
	// fallback (convert to speech or drop, with a warning) rather than the old
	// behaviour of attaching a bogus SoundPath of "True"/"False".
	if truthy(g.PlayMediaFile) {
		warnings = append(warnings, applySoundFallback(&t, ""))
	}

	regexOK := validatePattern(t.Pattern)
	if !regexOK {
		t.Enabled = false
		warnings = append(warnings, "pattern doesn't compile under RE2 — imported disabled, edit it in-app")
	}

	return ImportedTrigger{Trigger: t, Warnings: warnings, RegexOK: regexOK}, true
}

// truthy returns true for GINA-style boolean strings ("True", "true", "1", "yes").
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes":
		return true
	}
	return false
}

// ginaDurationSecs parses a GINA timer duration. GINA emits either a bare
// seconds integer, a float, an HH:MM:SS string, or a milliseconds field.
// Returns 0 when no duration could be parsed.
func ginaDurationSecs(timerDuration, timerMsDuration string) int {
	if ms := strings.TrimSpace(timerMsDuration); ms != "" {
		if n, err := strconv.Atoi(ms); err == nil && n > 0 {
			return n / 1000
		}
	}
	s := strings.TrimSpace(timerDuration)
	if s == "" {
		return 0
	}
	// HH:MM:SS or MM:SS
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		var total int
		for _, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				return 0
			}
			total = total*60 + n
		}
		return total
	}
	// Plain seconds (int or float).
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}
