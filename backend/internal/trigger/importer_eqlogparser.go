package trigger

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// EQLogParser .tgf export shapes. The file is a JSON array of nodes forming a
// folder tree; leaf nodes carry a TriggerData object, group nodes carry child
// Nodes. Only the fields we map are modeled. JSON keys are PascalCase and match
// the Go field names (encoding/json matches case-insensitively).

type eqlpNode struct {
	Name        string       `json:"Name"`
	Nodes       []eqlpNode   `json:"Nodes"`
	TriggerData *eqlpTrigger `json:"TriggerData"`
}

type eqlpTrigger struct {
	Pattern             string  `json:"Pattern"`
	UseRegex            bool    `json:"UseRegex"`
	PreviousPattern     string  `json:"PreviousPattern"`
	TextToDisplay       string  `json:"TextToDisplay"`
	TextToSpeak         string  `json:"TextToSpeak"`
	SoundToPlay         string  `json:"SoundToPlay"`
	DurationSeconds     float64 `json:"DurationSeconds"`
	EnableTimer         bool    `json:"EnableTimer"`
	TimerType           int     `json:"TimerType"`
	AltTimerName        string  `json:"AltTimerName"`
	EndEarlyPattern     string  `json:"EndEarlyPattern"`
	EndUseRegex         bool    `json:"EndUseRegex"`
	EndEarlyPattern2    string  `json:"EndEarlyPattern2"`
	EndUseRegex2        bool    `json:"EndUseRegex2"`
	WarningSeconds      float64 `json:"WarningSeconds"`
	WarningTextToSpeak  string  `json:"WarningTextToSpeak"`
	EndEarlyTextToSpeak string  `json:"EndEarlyTextToSpeak"`
	EndTextToSpeak      string  `json:"EndTextToSpeak"`
	LockoutTime         float64 `json:"LockoutTime"`
	TextToSendToChat    string  `json:"TextToSendToChat"`
	ChatWebhook         string  `json:"ChatWebhook"`
}

// parseEQLogParserImport parses an EQLogParser .tgf trigger export into an
// import preview, walking the node tree so each trigger carries its group path.
func parseEQLogParserImport(data []byte, sourceName string) (ImportPreview, error) {
	var roots []eqlpNode
	if err := json.Unmarshal(data, &roots); err != nil {
		return ImportPreview{}, fmt.Errorf("parse EQLogParser export: %w", err)
	}

	var out []ImportedTrigger
	var walk func(nodes []eqlpNode, path []string)
	walk = func(nodes []eqlpNode, path []string) {
		for i := range nodes {
			n := &nodes[i]
			if n.TriggerData != nil {
				if it, ok := convertEQLP(n, strings.Join(path, "/")); ok {
					out = append(out, it)
				}
			}
			if len(n.Nodes) > 0 {
				// Group node — extend the path with its name.
				here := path
				if name := strings.TrimSpace(n.Name); name != "" {
					here = append(append([]string(nil), path...), name)
				}
				walk(n.Nodes, here)
			}
		}
	}
	walk(roots, nil)

	name := strings.TrimSpace(sourceName)
	if name == "" {
		name = "EQLogParser Import"
	}
	return ImportPreview{Format: FormatEQLogParser, SourceName: name, Triggers: out}, nil
}

// convertEQLP maps a single EQLogParser leaf node into an ImportedTrigger.
func convertEQLP(n *eqlpNode, group string) (ImportedTrigger, bool) {
	td := n.TriggerData
	pattern := strings.TrimSpace(td.Pattern)
	if pattern == "" {
		return ImportedTrigger{}, false
	}
	name := strings.TrimSpace(n.Name)
	if name == "" {
		name = pattern
	}

	var warnings []string

	// Non-regex patterns are literal text (no tokens) — escape them so spell
	// names like "Words of Acquisition (Beza)" don't become regex groups.
	if !td.UseRegex {
		pattern = regexp.QuoteMeta(pattern)
	}

	tr := Trigger{
		Name:      name,
		Enabled:   true,
		Pattern:   pattern,
		Actions:   []Action{},
		TimerType: TimerTypeNone,
	}

	if txt := strings.TrimSpace(td.TextToDisplay); txt != "" {
		tr.Actions = append(tr.Actions, Action{
			Type:         ActionOverlayText,
			Text:         eqlpText(td.TextToDisplay),
			DurationSecs: 5,
			Color:        "#ffffff",
		})
	}
	if txt := strings.TrimSpace(td.TextToSpeak); txt != "" {
		tr.Actions = append(tr.Actions, Action{
			Type: ActionTextToSpeech,
			Text: eqlpText(td.TextToSpeak),
		})
	}

	// Timer.
	if td.EnableTimer && td.TimerType != 0 && td.DurationSeconds > 0 {
		tr.TimerType = TimerTypeDetrimental
		tr.TimerDurationSecs = int(td.DurationSeconds)

		// AltTimerName like "Enraged: {s1}" → per-target timer: the first
		// capture token becomes the timer key + target suffix so each mob gets
		// its own row labeled "<name> on <mob>".
		if grp := eqlpFirstCaptureGroup(td.AltTimerName); grp != "" {
			tr.TimerKeyCapture = grp
			tr.TimerTargetCapture = grp
		}

		// End-early patterns clear the timer early → worn-off alternation.
		var enders []string
		for _, ep := range []struct {
			pat   string
			regex bool
		}{
			{td.EndEarlyPattern, td.EndUseRegex},
			{td.EndEarlyPattern2, td.EndUseRegex2},
		} {
			s := strings.TrimSpace(ep.pat)
			if s == "" {
				continue
			}
			if !ep.regex {
				s = regexp.QuoteMeta(s)
			}
			enders = append(enders, "(?:"+s+")")
		}
		if len(enders) > 0 {
			tr.WornOffPattern = strings.Join(enders, "|")
		}

		// "Warning" cue → a fading TimerAlert.
		if txt := strings.TrimSpace(td.WarningTextToSpeak); txt != "" && td.WarningSeconds > 0 {
			tr.TimerAlerts = append(tr.TimerAlerts, TimerAlert{
				Seconds:     int(td.WarningSeconds),
				Type:        TimerAlertTypeTextToSpeech,
				TTSTemplate: eqlpText(txt),
			})
		}
		if strings.TrimSpace(td.EndEarlyTextToSpeak) != "" || strings.TrimSpace(td.EndTextToSpeak) != "" {
			warnings = append(warnings, "EQLogParser timer end/early speech not mapped — add a fading alert in-app if wanted")
		}
	}

	// Sound → shared fallback (convert to speech or drop, with a warning).
	if sound := strings.TrimSpace(td.SoundToPlay); sound != "" {
		warnings = append(warnings, applySoundFallback(&tr, sound))
	}

	if strings.TrimSpace(td.PreviousPattern) != "" {
		warnings = append(warnings, "EQLogParser previous-line condition dropped — imported as a plain match")
	}
	if td.LockoutTime > 0 {
		tr.RefireCooldownSecs = td.LockoutTime
	}
	if strings.TrimSpace(td.TextToSendToChat) != "" || strings.TrimSpace(td.ChatWebhook) != "" {
		warnings = append(warnings, "EQLogParser chat/webhook action dropped (no equivalent)")
	}

	regexOK := validatePattern(tr.Pattern)
	if !regexOK {
		tr.Enabled = false
		warnings = append(warnings, "pattern doesn't compile under RE2 — imported disabled, edit it in-app")
	}

	return ImportedTrigger{
		Trigger:       tr,
		OriginalGroup: group,
		Warnings:      dedupeWarnings(warnings),
		RegexOK:       regexOK,
	}, true
}

// eqlpText normalizes EQLogParser action text tokens. {L} (the whole matched
// line) maps to our {0} group; {sN}/{nN} capture tokens are resolved by the
// engine as-is.
func eqlpText(s string) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(s, "{L}", "{0}")
}

// eqlpFirstCaptureGroup returns the normalized regex group name of the first
// {sN}/{nN}/{name} token in an AltTimerName template, or "" if there is none.
// {s1} → "S1" (matching how normalizePattern names wildcard groups); a named
// token like {caster} is returned verbatim.
func eqlpFirstCaptureGroup(altName string) string {
	m := patternTokenRe.FindStringSubmatch(altName)
	if m == nil {
		return ""
	}
	key := m[1]
	up := strings.ToUpper(key)
	if len(up) <= 2 && (up[0] == 'S' || up[0] == 'N') &&
		(len(up) == 1 || (up[1] >= '1' && up[1] <= '9')) {
		return up
	}
	return key
}
