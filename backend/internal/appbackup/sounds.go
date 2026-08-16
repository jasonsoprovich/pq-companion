package appbackup

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// maxSoundFileBytes bounds a single sound file that gets bundled. Custom
// alert sounds are short clips; anything past this is almost certainly not
// meant to be a UI sound and shouldn't silently balloon the export.
const maxSoundFileBytes = 25 << 20 // 25 MiB

// collectSoundPaths finds every "sound_path" value referenced by triggers
// (the actions/timer_alerts JSON columns in dbPath, a user.db snapshot) and
// by configPath (config.yaml's TimerAlertPref/WishlistWatch sound fields). It
// walks generically for the sound_path key rather than enumerating every
// struct that carries one, so new alert types are covered without touching
// this file. Returns deduped absolute paths that exist on disk and are under
// maxSoundFileBytes — a stale or oversized reference is skipped rather than
// failing the whole export.
func collectSoundPaths(dbPath, configPath string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	add := func(p string) {
		if p == "" || seen[p] || !filepath.IsAbs(p) {
			return
		}
		seen[p] = true
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Size() > maxSoundFileBytes {
			return
		}
		out = append(out, p)
	}

	triggerPaths, err := collectTriggerSoundPaths(dbPath)
	if err != nil {
		return nil, err
	}
	for _, p := range triggerPaths {
		add(p)
	}

	if data, err := os.ReadFile(configPath); err == nil {
		var raw any
		if err := yaml.Unmarshal(data, &raw); err == nil {
			for _, p := range walkSoundPaths(raw) {
				add(p)
			}
		}
	}

	return out, nil
}

// collectTriggerSoundPaths reads the actions/timer_alerts JSON columns from
// every row of the triggers table in a user.db (or snapshot) and extracts any
// sound_path values found inside.
func collectTriggerSoundPaths(dbPath string) ([]string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil
	}
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(30000)", dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.Query(`SELECT actions, timer_alerts FROM triggers`)
	if err != nil {
		// Schema mismatch against an unexpected snapshot shouldn't fail the
		// whole export over a cosmetic sound-carrying feature.
		return nil, nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var actions, timerAlerts string
		if err := rows.Scan(&actions, &timerAlerts); err != nil {
			continue
		}
		out = append(out, extractJSONSoundPaths(actions)...)
		out = append(out, extractJSONSoundPaths(timerAlerts)...)
	}
	return out, rows.Err()
}

func extractJSONSoundPaths(raw string) []string {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil
	}
	return walkSoundPaths(v)
}

// walkSoundPaths recursively searches a decoded JSON/YAML value (nested maps
// and slices) for any "sound_path" key and returns its string values.
func walkSoundPaths(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "sound_path" {
				if s, ok := val.(string); ok && s != "" {
					out = append(out, s)
				}
				continue
			}
			out = append(out, walkSoundPaths(val)...)
		}
	case []any:
		for _, item := range t {
			out = append(out, walkSoundPaths(item)...)
		}
	}
	return out
}

// bundleSoundName returns the name a sound file gets inside the bundle:
// sounds/<hash8>-<basename>. Hashing the source path (not the file contents)
// avoids collisions between same-named files picked from different
// directories without needing to read the file twice.
func bundleSoundName(path string) string {
	h := sha256.Sum256([]byte(path))
	return soundsDir + "/" + hex.EncodeToString(h[:])[:8] + "-" + filepath.Base(path)
}

// copyStagedSounds moves every sound file extracted into staging (under
// staging/sounds/) to its permanent local home under destDir
// (~/.pq-companion/sounds), returning a map from each file's *original*
// absolute path (on the exporting machine) to its new local path. Same
// filesystem as staging (both under appHome), so a rename is safe and cheap.
// A soundMap entry whose staged file is missing (e.g. it was skipped as
// oversized at export time) is silently dropped — the corresponding
// sound_path reference is simply left pointing at the old, nonexistent path.
func copyStagedSounds(staging, destDir string, soundMap map[string]string) (map[string]string, error) {
	if len(soundMap) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	remap := make(map[string]string, len(soundMap))
	for originalPath, bundleName := range soundMap {
		stagedPath := filepath.Join(staging, filepath.FromSlash(bundleName))
		if _, err := os.Stat(stagedPath); err != nil {
			continue
		}
		destPath := filepath.Join(destDir, filepath.Base(bundleName))
		if err := os.Rename(stagedPath, destPath); err != nil {
			return nil, fmt.Errorf("install sound %s: %w", filepath.Base(bundleName), err)
		}
		remap[originalPath] = destPath
	}
	return remap, nil
}

// rewriteTriggerSoundPaths rewrites every actions/timer_alerts sound_path
// value in the triggers table that matches a key in remap. Only rows that
// actually change are written back, so triggers with no bundled sound are
// left byte-for-byte untouched.
func rewriteTriggerSoundPaths(dbPath string, remap map[string]string) error {
	if len(remap) == 0 {
		return nil
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)", dbPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer conn.Close()

	rows, err := conn.Query(`SELECT id, actions, timer_alerts FROM triggers`)
	if err != nil {
		// Schema mismatch shouldn't fail the whole import over sound remap.
		return nil
	}
	type rowUpdate struct {
		id                   string
		actions, timerAlerts string
	}
	var updates []rowUpdate
	for rows.Next() {
		var id string
		var actions, timerAlerts string
		if err := rows.Scan(&id, &actions, &timerAlerts); err != nil {
			continue
		}
		newActions, changedA := rewriteJSONSoundPaths(actions, remap)
		newTimerAlerts, changedT := rewriteJSONSoundPaths(timerAlerts, remap)
		if changedA || changedT {
			updates = append(updates, rowUpdate{id, newActions, newTimerAlerts})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, u := range updates {
		if _, err := conn.Exec(`UPDATE triggers SET actions = ?, timer_alerts = ? WHERE id = ?`,
			u.actions, u.timerAlerts, u.id); err != nil {
			return err
		}
	}
	return nil
}

func rewriteJSONSoundPaths(raw string, remap map[string]string) (string, bool) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw, false
	}
	if !rewriteSoundPathsInPlace(v, remap) {
		return raw, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return raw, false
	}
	return string(out), true
}

// rewriteSoundPathsInConfig applies remap to every sound_path in cfg via a
// YAML round-trip (mirroring the generic collection walk in
// collectSoundPaths), rather than enumerating the several *TimerAlertPref
// fields and WishlistWatchSettings.SoundPath by hand — new alert types stay
// covered without touching this file. Returns the rewritten config and
// whether anything actually changed.
func rewriteSoundPathsInConfig(cfg config.Config, remap map[string]string) (config.Config, bool, error) {
	if len(remap) == 0 {
		return cfg, false, nil
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return cfg, false, err
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return cfg, false, err
	}
	if !rewriteSoundPathsInPlace(raw, remap) {
		return cfg, false, nil
	}
	rewritten, err := yaml.Marshal(raw)
	if err != nil {
		return cfg, false, err
	}
	var out config.Config
	if err := yaml.Unmarshal(rewritten, &out); err != nil {
		return cfg, false, err
	}
	return out, true, nil
}

// rewriteSoundPathsInPlace recursively walks a decoded JSON/YAML value,
// replacing any "sound_path" string that matches a remap key with its new
// path. Mutates maps in place (maps are reference types) and reports whether
// anything changed.
func rewriteSoundPathsInPlace(v any, remap map[string]string) bool {
	changed := false
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "sound_path" {
				if s, ok := val.(string); ok {
					if newPath, found := remap[s]; found && newPath != s {
						t[k] = newPath
						changed = true
					}
				}
				continue
			}
			if rewriteSoundPathsInPlace(val, remap) {
				changed = true
			}
		}
	case []any:
		for _, item := range t {
			if rewriteSoundPathsInPlace(item, remap) {
				changed = true
			}
		}
	}
	return changed
}
