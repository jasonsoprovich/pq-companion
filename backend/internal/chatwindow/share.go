package chatwindow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Share file format: one or more chat window definitions wrapped with a
// format tag and version, for moving a setup to another PC or passing it
// around. Personal fields (IDs and character filters) are left out.
const (
	ShareFormat  = "pq-companion-chat-windows"
	ShareVersion = 1
	maxShareSize = 1 << 20
)

// Share is the exported form of one or more windows.
type Share struct {
	Format  string      `json:"format"`
	Version int         `json:"version"`
	Windows []WindowDef `json:"windows"`
}

func shareDef(def WindowDef) WindowDef {
	def.ID = ""
	def.Characters = nil
	return def
}

// Export returns the share form of the given windows, or of every window
// when ids is empty.
func (e *Engine) Export(ids []string) (Share, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := Share{Format: ShareFormat, Version: ShareVersion, Windows: []WindowDef{}}
	if len(ids) == 0 {
		for _, w := range e.windows {
			out.Windows = append(out.Windows, shareDef(w.Def))
		}
		return out, nil
	}
	for _, id := range ids {
		w := e.findLocked(id)
		if w == nil {
			return Share{}, ErrNotFound
		}
		out.Windows = append(out.Windows, shareDef(w.Def))
	}
	return out, nil
}

// ParseShare decodes and validates a share file without adding anything.
func ParseShare(data []byte) ([]WindowDef, error) {
	if len(data) > maxShareSize {
		return nil, fmt.Errorf("file is larger than %d KB", maxShareSize/1024)
	}
	var s Share
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("not a chat window file: %w", err)
	}
	if s.Format != ShareFormat {
		return nil, errors.New("not a PQ Companion chat window file")
	}
	if s.Version > ShareVersion {
		return nil, fmt.Errorf("made by a newer PQ Companion (version %d); update to import it", s.Version)
	}
	if len(s.Windows) == 0 {
		return nil, errors.New("the file has no chat windows")
	}
	if len(s.Windows) > MaxWindows {
		return nil, fmt.Errorf("at most %d chat windows", MaxWindows)
	}
	out := make([]WindowDef, 0, len(s.Windows))
	for i, def := range s.Windows {
		def = shareDef(def)
		v, err := Validate(def)
		if err != nil {
			return nil, fmt.Errorf("window %d (%s): %w", i+1, def.Name, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// Import adds every window in a share file as a new window. It checks the
// whole file, and the window limit, before adding any.
func (e *Engine) Import(data []byte) ([]WindowDef, error) {
	defs, err := ParseShare(data)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	room := MaxWindows - len(e.windows)
	e.mu.Unlock()
	if len(defs) > room {
		return nil, fmt.Errorf("importing %d windows would pass the limit of %d", len(defs), MaxWindows)
	}
	out := make([]WindowDef, 0, len(defs))
	for _, def := range defs {
		v, err := e.Create(def)
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, nil
}
