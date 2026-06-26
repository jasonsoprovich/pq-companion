package trigger

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ImportFormat identifies which external app a trigger import file came from.
type ImportFormat string

const (
	// FormatPQC is a PQ Companion JSON trigger pack (our own export).
	FormatPQC ImportFormat = "pqc"
	// FormatGINA is a GINA trigger share — a .gtp zip (containing ShareData.xml)
	// or a raw ShareData.xml document.
	FormatGINA ImportFormat = "gina"
	// FormatEQNag is an EQNag trigger database — a backup .zip (containing a
	// triggers JSON) or the raw trigger-database.json. Parser lands in phase 2.
	FormatEQNag ImportFormat = "eqnag"
	// FormatEQLogParser is an EQLogParser trigger export (.tgf JSON node tree).
	// Parser lands in phase 3.
	FormatEQLogParser ImportFormat = "eqlogparser"
)

// ImportedTrigger is one trigger produced by parsing an import file, paired
// with the lossy-mapping warnings the UI surfaces and the source group path so
// the wizard can show where it lived in the originating app.
type ImportedTrigger struct {
	Trigger Trigger `json:"trigger"`
	// OriginalGroup is the slash-joined folder/group path the trigger lived
	// under in the source app (e.g. "Production/Class/Enchanter"). Empty when
	// the source is flat or ungrouped.
	OriginalGroup string `json:"original_group,omitempty"`
	// Warnings describe pieces of the source trigger that couldn't be mapped
	// 1:1 (dropped sound, flattened condition, etc.). Empty = clean mapping.
	Warnings []string `json:"warnings,omitempty"`
	// RegexOK reports whether the mapped Pattern compiles under Go's RE2. When
	// false the trigger is imported disabled with a warning — .NET/EQNag
	// regexes occasionally use backreferences or lookbehind RE2 rejects.
	RegexOK bool `json:"regex_ok"`
}

// ImportPreview is the result of detecting and parsing an import file. It is
// returned to the wizard for review/selection; nothing is persisted until the
// user commits a chosen subset.
type ImportPreview struct {
	Format     ImportFormat      `json:"format"`
	SourceName string            `json:"source_name"`
	Triggers   []ImportedTrigger `json:"triggers"`
}

// utf8BOM is the byte-order mark GINA prepends to its ShareData.xml exports.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// zipMagic is the local-file-header signature every PKZIP archive starts with
// (.gtp GINA packages and EQNag backup zips).
var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// DetectAndParse identifies the source app of an import file and parses it into
// a normalized preview. filename is used only to suggest a default category
// name and as a weak hint; detection is driven by the bytes.
func DetectAndParse(filename string, data []byte) (ImportPreview, error) {
	if len(data) == 0 {
		return ImportPreview{}, fmt.Errorf("empty file")
	}

	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	// Zip container: GINA .gtp (ShareData.xml inside) or EQNag backup .zip
	// (a triggers JSON inside).
	if bytes.HasPrefix(data, zipMagic) {
		inner, innerName, err := unwrapZip(data)
		if err != nil {
			return ImportPreview{}, err
		}
		// Recurse on the extracted member, but keep the outer archive's name as
		// the suggested category (more meaningful than "ShareData").
		prev, err := DetectAndParse(innerName, inner)
		if err != nil {
			return ImportPreview{}, err
		}
		if base != "" {
			prev.SourceName = base
		}
		return prev, nil
	}

	trimmed := bytes.TrimLeftFunc(bytes.TrimPrefix(data, utf8BOM), unicode.IsSpace)
	if len(trimmed) == 0 {
		return ImportPreview{}, fmt.Errorf("file contains no data")
	}

	switch trimmed[0] {
	case '<':
		return parseGINAImport(data, base)
	case '{', '[':
		return parseJSONImport(trimmed, base)
	}
	return ImportPreview{}, fmt.Errorf("unrecognized file format")
}

// unwrapZip extracts the single relevant member from a GINA .gtp or EQNag
// backup zip: the ShareData.xml if present, otherwise the JSON file most likely
// to hold triggers (preferring a name containing "trigger"). Returns the raw
// member bytes and its name.
func unwrapZip(data []byte) ([]byte, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", fmt.Errorf("read zip: %w", err)
	}
	var xmlFile, bestJSON, anyJSON *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := strings.ToLower(filepath.Base(f.Name))
		switch {
		case strings.EqualFold(filepath.Base(f.Name), "ShareData.xml"):
			xmlFile = f
		case strings.HasSuffix(name, ".json"):
			if anyJSON == nil {
				anyJSON = f
			}
			if strings.Contains(name, "trigger") {
				bestJSON = f
			}
		}
	}
	pick := xmlFile
	if pick == nil {
		pick = bestJSON
	}
	if pick == nil {
		pick = anyJSON
	}
	if pick == nil {
		return nil, "", fmt.Errorf("zip contains no ShareData.xml or triggers JSON")
	}
	rc, err := pick.Open()
	if err != nil {
		return nil, "", fmt.Errorf("open %s in zip: %w", pick.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", fmt.Errorf("read %s in zip: %w", pick.Name, err)
	}
	return b, filepath.Base(pick.Name), nil
}

// parseJSONImport dispatches a JSON import file to the right parser based on
// distinctive keys. trimmed has had any BOM/leading whitespace removed.
func parseJSONImport(trimmed []byte, sourceName string) (ImportPreview, error) {
	switch {
	case bytes.Contains(trimmed, []byte(`"capturePhrases"`)):
		return parseEQNagImport(trimmed, sourceName)
	case bytes.Contains(trimmed, []byte(`"TriggerData"`)):
		return parseEQLogParserImport(trimmed, sourceName)
	case bytes.Contains(trimmed, []byte(`"pack_name"`)):
		return parsePQCImport(trimmed, sourceName)
	}
	return ImportPreview{}, fmt.Errorf("unrecognized JSON trigger format")
}

// parsePQCImport parses a PQ Companion JSON trigger pack (our own export). It
// still runs every trigger through regex validation so a hand-edited pack with
// a broken pattern surfaces the same warning as a foreign import.
func parsePQCImport(data []byte, sourceName string) (ImportPreview, error) {
	var pack TriggerPack
	if err := json.Unmarshal(data, &pack); err != nil {
		return ImportPreview{}, fmt.Errorf("parse PQ Companion pack: %w", err)
	}
	name := strings.TrimSpace(pack.PackName)
	if sourceName != "" {
		name = sourceName
	}
	out := make([]ImportedTrigger, 0, len(pack.Triggers))
	for i := range pack.Triggers {
		t := pack.Triggers[i]
		ok := t.Pattern == "" || validatePattern(t.Pattern)
		it := ImportedTrigger{Trigger: t, RegexOK: ok}
		if !ok {
			it.Trigger.Enabled = false
			it.Warnings = append(it.Warnings, "pattern doesn't compile — imported disabled, edit it in-app")
		}
		out = append(out, it)
	}
	return ImportPreview{Format: FormatPQC, SourceName: name, Triggers: out}, nil
}

// parseEQNagImport is implemented in phase 2 (importer_eqnag.go).
func parseEQNagImport(data []byte, sourceName string) (ImportPreview, error) {
	return ImportPreview{}, fmt.Errorf("EQNag import is coming in a later update")
}

// parseEQLogParserImport is implemented in phase 3 (importer_eqlogparser.go).
func parseEQLogParserImport(data []byte, sourceName string) (ImportPreview, error) {
	return ImportPreview{}, fmt.Errorf("EQLogParser import is coming in a later update")
}

// dollarBraceRe matches GINA/EQNag braced-dollar capture references like
// ${2} or ${SpellBeingCast} in action text. We rewrite them to our native
// {2} / {SpellBeingCast} form at import time so the runtime engine — which
// already resolves {N} and {name} — substitutes them without any hot-path
// change.
var dollarBraceRe = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// normalizeActionText rewrites foreign capture-reference syntax in alert/TTS
// text into the tokens our engine understands. Currently: ${X} → {X}.
func normalizeActionText(s string) string {
	if s == "" || !strings.Contains(s, "${") {
		return s
	}
	return dollarBraceRe.ReplaceAllString(s, "{$1}")
}

// validatePattern reports whether a trigger pattern compiles under Go's RE2
// after the engine's token normalization. Used to flag (and disable on import)
// foreign regexes that rely on backreferences or lookbehind RE2 rejects.
func validatePattern(pattern string) bool {
	if pattern == "" {
		return false
	}
	_, err := regexp.Compile(normalizePattern(pattern, ""))
	return err == nil
}

// applySoundFallback handles a source trigger whose only-or-additional feedback
// was a sound file we can't carry over (the path points at the other user's
// machine, or — for GINA — no filename exists at all). The rule:
//
//   - If the trigger already alerts via overlay text or TTS, drop the sound to
//     avoid redundant double-audio, and warn.
//   - Otherwise convert it to a TTS action speaking the display text (or, if
//     none, the trigger name) so the audible cue survives, and warn.
//
// origFile is the source filename when known (EQNag/EQLogParser) or "" (GINA).
// The returned warning is appended by the caller.
func applySoundFallback(t *Trigger, origFile string) string {
	hasAudibleOrVisible := false
	speakText := strings.TrimSpace(t.Name)
	for _, a := range t.Actions {
		switch a.Type {
		case ActionOverlayText:
			hasAudibleOrVisible = true
			if txt := strings.TrimSpace(a.Text); txt != "" {
				speakText = txt
			}
		case ActionTextToSpeech:
			hasAudibleOrVisible = true
		}
	}

	named := ""
	if origFile != "" {
		named = fmt.Sprintf(` "%s"`, origFile)
	}
	if hasAudibleOrVisible {
		return fmt.Sprintf("audio%s not recoverable — removed (trigger already alerts via text/speech)", named)
	}
	t.Actions = append(t.Actions, Action{Type: ActionTextToSpeech, Text: speakText})
	if origFile == "" {
		return "sound converted to speech (GINA exports don't include the audio file) — re-add it manually if desired"
	}
	return fmt.Sprintf("audio%s not found — converted to speech; re-add the file manually if desired", named)
}
