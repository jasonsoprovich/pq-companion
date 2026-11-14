package customoverlay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Share file format: a custom overlay definition wrapped with a format tag
// and version, for passing around as a .json file or a pasted block in
// Discord. Personal fields (the ID and the character filter) are left out.
const (
	ShareFormat  = "pq-companion-custom-overlay"
	ShareVersion = 1
	maxShareSize = 1 << 20
)

// Share is the exported form of one overlay.
type Share struct {
	Format  string     `json:"format"`
	Version int        `json:"version"`
	Overlay OverlayDef `json:"overlay"`
}

// shareDef strips the fields that only make sense on the exporter's machine.
func shareDef(def OverlayDef) OverlayDef {
	def.ID = ""
	def.Characters = nil
	return def
}

// Export returns the share form of an overlay.
func (e *Engine) Export(id string) (Share, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	o := e.findLocked(id)
	if o == nil {
		return Share{}, ErrNotFound
	}
	return Share{Format: ShareFormat, Version: ShareVersion, Overlay: shareDef(o.Def)}, nil
}

// ParseShare reads pasted or loaded share text and returns the validated
// definition it holds, ready to Create. It also accepts a bare definition
// (as copied from the editor's JSON tab) and text wrapped in a Markdown code
// fence, since shares get pasted through Discord.
func ParseShare(data []byte) (OverlayDef, error) {
	if len(data) > maxShareSize {
		return OverlayDef{}, errors.New("that's too big to be a custom overlay")
	}
	data = stripFence(bytes.TrimSpace(data))
	if len(data) == 0 {
		return OverlayDef{}, errors.New("nothing to import")
	}

	var probe struct {
		Format  string          `json:"format"`
		Version int             `json:"version"`
		Overlay json.RawMessage `json:"overlay"`
		Widgets json.RawMessage `json:"widgets"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return OverlayDef{}, fmt.Errorf("not a custom overlay (invalid JSON: %v)", err)
	}

	var def OverlayDef
	switch {
	case probe.Format != "":
		if probe.Format != ShareFormat {
			return OverlayDef{}, fmt.Errorf("not a custom overlay (format %q)", probe.Format)
		}
		if probe.Version > ShareVersion {
			return OverlayDef{}, errors.New("this overlay was made with a newer version of PQ Companion; update to import it")
		}
		if len(probe.Overlay) == 0 {
			return OverlayDef{}, errors.New("the overlay definition is missing")
		}
		if err := json.Unmarshal(probe.Overlay, &def); err != nil {
			return OverlayDef{}, fmt.Errorf("the overlay definition is invalid: %v", err)
		}
	case len(probe.Widgets) > 0:
		if err := json.Unmarshal(data, &def); err != nil {
			return OverlayDef{}, fmt.Errorf("the overlay definition is invalid: %v", err)
		}
	default:
		return OverlayDef{}, errors.New("not a custom overlay")
	}

	def, err := Validate(shareDef(def))
	if err != nil {
		return OverlayDef{}, err
	}
	def.ID = ""
	return def, nil
}

// stripFence removes a surrounding ``` or ```json fence.
func stripFence(b []byte) []byte {
	if !bytes.HasPrefix(b, []byte("```")) {
		return b
	}
	b = b[3:]
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[i+1:] // drop the info string ("json")
	} else {
		b = bytes.TrimPrefix(b, []byte("json"))
	}
	b = bytes.TrimSpace(b)
	return bytes.TrimSpace(bytes.TrimSuffix(b, []byte("```")))
}
