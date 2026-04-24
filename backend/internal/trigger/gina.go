package trigger

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ginaDocument is the root of GINA's <SharedData>…</SharedData> trigger share
// XML. GINA packages triggers inside TriggerGroups, optionally nested; the
// same Trigger shape also appears at the SharedData level on some exports.
type ginaDocument struct {
	XMLName       xml.Name          `xml:"SharedData"`
	TriggerGroups []ginaTriggerGroup `xml:"TriggerGroups>TriggerGroup"`
	Triggers      []ginaTrigger      `xml:"Triggers>Trigger"`
}

type ginaTriggerGroup struct {
	Name          string            `xml:"Name"`
	TriggerGroups []ginaTriggerGroup `xml:"TriggerGroups>TriggerGroup"`
	Triggers      []ginaTrigger      `xml:"Triggers>Trigger"`
}

// ginaTrigger mirrors GINA's Trigger element. Unused fields are omitted.
type ginaTrigger struct {
	Name               string `xml:"Name"`
	TriggerText        string `xml:"TriggerText"`
	EnableRegex        string `xml:"EnableRegex"` // "True" / "False"
	Category           string `xml:"Category"`
	Comments           string `xml:"Comments"`
	UseText            string `xml:"UseText"`
	DisplayText        string `xml:"DisplayText"`
	UseTextToVoice     string `xml:"UseTextToVoice"`
	TextToVoiceText    string `xml:"TextToVoiceText"`
	PlayMediaFile      string `xml:"PlayMediaFile"`
	TimerType          string `xml:"TimerType"`      // "NoTimer", "Timer", "RepeatingTimer", "Stopwatch"
	TimerDuration      string `xml:"TimerDuration"`  // seconds OR HH:MM:SS
	TimerMillisecondDuration string `xml:"TimerMillisecondDuration"`
	TimerEndingTrigger string `xml:"TimerEndingTrigger"`
	TimerEarlyEnders   struct {
		Enders []struct {
			EnableRegex string `xml:"EnableRegex"`
			EndingTrigger string `xml:"EndingTrigger"`
		} `xml:"EarlyEnder"`
	} `xml:"TimerEarlyEnders"`
}

// ParseGINA parses a GINA trigger share XML document into a TriggerPack.
// packName is used as the pack_name applied to every imported trigger (if
// empty, a default is derived from the document).
func ParseGINA(data []byte, packName string) (TriggerPack, error) {
	var doc ginaDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return TriggerPack{}, fmt.Errorf("parse gina xml: %w", err)
	}

	var out []Trigger
	var walk func(group ginaTriggerGroup)
	walk = func(group ginaTriggerGroup) {
		for _, gt := range group.Triggers {
			if t, ok := convertGINA(gt); ok {
				out = append(out, t)
			}
		}
		for _, sub := range group.TriggerGroups {
			walk(sub)
		}
	}

	// Top-level Triggers (rare but valid)
	for _, gt := range doc.Triggers {
		if t, ok := convertGINA(gt); ok {
			out = append(out, t)
		}
	}
	for _, group := range doc.TriggerGroups {
		walk(group)
	}

	name := packName
	if name == "" {
		name = "GINA Import"
	}

	// Ensure every trigger carries the pack name.
	now := time.Now().UTC()
	for i := range out {
		out[i].PackName = name
		out[i].CreatedAt = now
	}

	return TriggerPack{
		PackName:    name,
		Description: "Imported from GINA share XML",
		Triggers:    out,
	}, nil
}

// convertGINA maps a single ginaTrigger into a Trigger. Returns ok=false when
// the GINA entry lacks the minimum required fields (name + trigger text).
func convertGINA(g ginaTrigger) (Trigger, bool) {
	name := strings.TrimSpace(g.Name)
	pattern := strings.TrimSpace(g.TriggerText)
	if name == "" || pattern == "" {
		return Trigger{}, false
	}

	// Convert literal text into a regex when GINA marked it as non-regex.
	if !truthy(g.EnableRegex) {
		pattern = regexp.QuoteMeta(pattern)
	}

	actions := make([]Action, 0, 3)
	if truthy(g.UseText) && strings.TrimSpace(g.DisplayText) != "" {
		actions = append(actions, Action{
			Type:         ActionOverlayText,
			Text:         g.DisplayText,
			DurationSecs: 5,
			Color:        "#ffffff",
		})
	}
	if truthy(g.UseTextToVoice) && strings.TrimSpace(g.TextToVoiceText) != "" {
		actions = append(actions, Action{
			Type: ActionTextToSpeech,
			Text: g.TextToVoiceText,
		})
	}
	if strings.TrimSpace(g.PlayMediaFile) != "" {
		actions = append(actions, Action{
			Type:      ActionPlaySound,
			SoundPath: g.PlayMediaFile,
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

	return Trigger{
		Name:              name,
		Enabled:           true,
		Pattern:           pattern,
		Actions:           actions,
		TimerType:         tt,
		TimerDurationSecs: duration,
		WornOffPattern:    wornOff,
	}, true
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
