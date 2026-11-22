package zeal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CapTracker records which checklist features have been seen working on the
// Zeal pipe, and which EverQuest install that pipe belongs to.
//
// The pipe supervisor picks a game process by pipe name/PID, not by EQ
// folder, so on connect the tracker resolves the process's executable folder.
// Live proof only counts for that folder, and is only persisted when the
// folder is known. Proof resets on every connect and disconnect: the next
// connection may be another game process or another Zeal build.
type CapTracker struct {
	mu sync.Mutex
	// pipeDir is the connected game's folder, "" when unknown or not
	// connected; identity is its Zeal build identity.
	pipeDir  string
	identity string
	live     map[CapID]bool

	// exeDir resolves a PID to its executable's folder ("" if unknown).
	exeDir func(pid uint32) string
	// loadSeen / saveSeen persist {cap: build identity} (config.yaml).
	loadSeen func() map[string]string
	saveSeen func(id CapID, identity string)
	// onChange is called after a transition (connect, disconnect, first
	// sighting of a feature this session). Never called with mu held.
	onChange func()
}

// NewCapTracker builds a tracker. Any func may be nil.
func NewCapTracker(loadSeen func() map[string]string, saveSeen func(CapID, string), onChange func()) *CapTracker {
	return &CapTracker{
		live:     map[CapID]bool{},
		exeDir:   processExeDir,
		loadSeen: loadSeen,
		saveSeen: saveSeen,
		onChange: onChange,
	}
}

// Connected starts a pipe session for game process pid.
func (t *CapTracker) Connected(pid uint32) {
	dir := ""
	if t.exeDir != nil && pid != 0 {
		dir = t.exeDir(pid)
	}
	id := ""
	if dir != "" {
		id = BuildIdentity(dir)
	}
	t.mu.Lock()
	t.pipeDir, t.identity = dir, id
	t.live = map[CapID]bool{}
	t.mu.Unlock()
	t.changed()
}

// Disconnected ends the pipe session.
func (t *CapTracker) Disconnected() {
	t.mu.Lock()
	t.pipeDir, t.identity = "", ""
	t.live = map[CapID]bool{}
	t.mu.Unlock()
	t.changed()
}

// Saw records that the pipe just carried data only feature id produces.
// Cheap after the first call per session, so it can run on every message.
func (t *CapTracker) Saw(id CapID) {
	t.mu.Lock()
	if t.live[id] {
		t.mu.Unlock()
		return
	}
	t.live[id] = true
	identity := t.identity
	t.mu.Unlock()
	if identity != "" && t.saveSeen != nil {
		if prev := t.seen()[string(id)]; prev != identity {
			t.saveSeen(id, identity)
		}
	}
	t.changed()
}

// Evidence returns the live and persisted proof that applies to the install
// in eqDir. Live proof counts only when the connected game runs from eqDir.
func (t *CapTracker) Evidence(eqDir string) (live, persisted map[CapID]bool, pipeMatches bool) {
	live, persisted = map[CapID]bool{}, map[CapID]bool{}
	if strings.TrimSpace(eqDir) == "" {
		return live, persisted, false
	}
	t.mu.Lock()
	pipeMatches = t.pipeDir != "" && samePath(t.pipeDir, eqDir)
	if pipeMatches {
		for id := range t.live {
			live[id] = true
		}
	}
	t.mu.Unlock()
	if identity := BuildIdentity(eqDir); identity != "" {
		for id, seenOn := range t.seen() {
			if seenOn == identity {
				persisted[CapID(id)] = true
			}
		}
	}
	return live, persisted, pipeMatches
}

func (t *CapTracker) seen() map[string]string {
	if t.loadSeen == nil {
		return nil
	}
	return t.loadSeen()
}

func (t *CapTracker) changed() {
	if t.onChange != nil {
		t.onChange()
	}
}

// BuildIdentity identifies the Zeal build installed in eqDir: folder, version,
// and Zeal.asi's size and modified time (test builds share a version string).
// "" when Zeal.asi isn't there.
func BuildIdentity(eqDir string) string {
	asi := findASI(eqDir)
	if asi == "" {
		return ""
	}
	fi, err := os.Stat(asi)
	if err != nil {
		return ""
	}
	v, _ := ReadInstalledVersion(asi)
	return fmt.Sprintf("%s|%s|%d|%d", normPath(eqDir), v, fi.Size(), fi.ModTime().Unix())
}

// findASI returns the path of Zeal.asi in dir, matching the name in any case.
func findASI(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), "zeal.asi") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func normPath(p string) string {
	return strings.ToLower(filepath.Clean(strings.TrimSpace(p)))
}

// samePath compares folders the way Windows does: case-insensitively.
func samePath(a, b string) bool {
	return normPath(a) == normPath(b)
}

// WatchINI polls <eqPath>/zeal.ini every interval and calls onChange when it
// appears, disappears or is rewritten (Zeal saves it the moment a setting is
// toggled in game). Runs on its own, not in the export watcher, which waits
// for a character before checking anything.
func (t *CapTracker) WatchINI(ctx context.Context, eqPath func() string, interval time.Duration) {
	var last string
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		sig := iniSignature(eqPath())
		if sig != last {
			if last != "" || sig != "" {
				t.changed()
			}
			last = sig
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// iniSignature is "" when zeal.ini is missing, else its path, size and mtime.
func iniSignature(eqPath string) string {
	if strings.TrimSpace(eqPath) == "" {
		return ""
	}
	p := filepath.Join(eqPath, ZealINIFilename)
	fi, err := os.Stat(p)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d", normPath(p), fi.Size(), fi.ModTime().UnixNano())
}
