package appbackup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// makeTriggersDB creates a minimal triggers table (just the columns the
// sound-path collector/rewriter touch) with one row referencing soundInAction
// via its actions JSON and soundInTimerAlert via its timer_alerts JSON.
func makeTriggersDB(t *testing.T, path, soundInAction, soundInTimerAlert string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE triggers (id TEXT PRIMARY KEY, actions TEXT, timer_alerts TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	actions := `[{"type":"play_sound","sound_path":"` + escapeJSON(soundInAction) + `"},{"type":"overlay_text","sound_path":""}]`
	timerAlerts := `[{"id":"a","sound_path":"` + escapeJSON(soundInTimerAlert) + `"}]`
	if _, err := db.Exec(`INSERT INTO triggers (id, actions, timer_alerts) VALUES ('t1', ?, ?)`, actions, timerAlerts); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// A second row with no sound references at all — must survive untouched.
	if _, err := db.Exec(`INSERT INTO triggers (id, actions, timer_alerts) VALUES ('t2', '[]', '[]')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func escapeJSON(s string) string {
	out := ""
	for _, r := range s {
		if r == '\\' || r == '"' {
			out += `\`
		}
		out += string(r)
	}
	return out
}

func TestCollectSoundPaths(t *testing.T) {
	dir := t.TempDir()

	soundA := filepath.Join(dir, "alarm.wav")
	soundB := filepath.Join(dir, "chime.wav")
	oversized := filepath.Join(dir, "big.wav")
	missing := filepath.Join(dir, "gone.wav")
	for _, p := range []string{soundA, soundB} {
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	big := make([]byte, maxSoundFileBytes+1)
	if err := os.WriteFile(oversized, big, 0o644); err != nil {
		t.Fatalf("write oversized: %v", err)
	}

	dbPath := filepath.Join(dir, "user.db")
	makeTriggersDB(t, dbPath, soundA, missing)

	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Config{
		Preferences: config.Preferences{
			CustomTimerAlert: &config.TimerAlertPref{SoundPath: soundB},
			RespawnAlert:     &config.TimerAlertPref{SoundPath: oversized},
			WishlistWatch:    config.WishlistWatchSettings{SoundPath: soundA}, // dup of trigger's soundA
		},
	}
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := collectSoundPaths(dbPath, cfgPath)
	if err != nil {
		t.Fatalf("collectSoundPaths: %v", err)
	}

	want := map[string]bool{soundA: true, soundB: true}
	if len(got) != len(want) {
		t.Fatalf("collectSoundPaths returned %v, want exactly %v (deduped, missing/oversized excluded)", got, want)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected path %q in result", p)
		}
	}
}

func TestRewriteTriggerSoundPaths(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "user.db")
	oldAction := filepath.Join(dir, "old-action.wav")
	oldTimer := filepath.Join(dir, "old-timer.wav")
	makeTriggersDB(t, dbPath, oldAction, oldTimer)

	newAction := filepath.Join(dir, "sounds", "new-action.wav")
	newTimer := filepath.Join(dir, "sounds", "new-timer.wav")
	remap := map[string]string{oldAction: newAction, oldTimer: newTimer}

	if err := rewriteTriggerSoundPaths(dbPath, remap); err != nil {
		t.Fatalf("rewriteTriggerSoundPaths: %v", err)
	}

	got, err := collectTriggerSoundPaths(dbPath)
	if err != nil {
		t.Fatalf("collectTriggerSoundPaths: %v", err)
	}
	found := map[string]bool{}
	for _, p := range got {
		found[p] = true
	}
	if !found[newAction] || !found[newTimer] {
		t.Errorf("after rewrite, got %v, want to contain %q and %q", got, newAction, newTimer)
	}
	if found[oldAction] || found[oldTimer] {
		t.Errorf("old paths %v still present after rewrite", got)
	}
}

func TestRewriteSoundPathsInConfig(t *testing.T) {
	oldPath := "/old/machine/alarm.wav"
	newPath := "/new/machine/sounds/a1b2c3d4-alarm.wav"
	remap := map[string]string{oldPath: newPath}

	cfg := config.Config{
		Preferences: config.Preferences{
			CustomTimerAlert: &config.TimerAlertPref{SoundPath: oldPath},
			WishlistWatch:    config.WishlistWatchSettings{SoundPath: "/unrelated/other.wav"},
		},
	}

	rewritten, changed, err := rewriteSoundPathsInConfig(cfg, remap)
	if err != nil {
		t.Fatalf("rewriteSoundPathsInConfig: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	if rewritten.Preferences.CustomTimerAlert.SoundPath != newPath {
		t.Errorf("CustomTimerAlert.SoundPath = %q, want %q", rewritten.Preferences.CustomTimerAlert.SoundPath, newPath)
	}
	if rewritten.Preferences.WishlistWatch.SoundPath != "/unrelated/other.wav" {
		t.Errorf("WishlistWatch.SoundPath changed unexpectedly: %q", rewritten.Preferences.WishlistWatch.SoundPath)
	}

	// No matching keys: reports unchanged and returns the input untouched.
	_, changed2, err := rewriteSoundPathsInConfig(cfg, map[string]string{"/no/match": "/x"})
	if err != nil {
		t.Fatalf("rewriteSoundPathsInConfig (no match): %v", err)
	}
	if changed2 {
		t.Errorf("changed = true for a remap with no matching keys, want false")
	}
}

func TestBundleSoundNameAvoidsCollisions(t *testing.T) {
	a := bundleSoundName("/dir/one/alarm.wav")
	b := bundleSoundName("/dir/two/alarm.wav")
	if a == b {
		t.Errorf("bundleSoundName collided for same basename, different dirs: %q == %q", a, b)
	}
	if bundleSoundName("/dir/one/alarm.wav") != a {
		t.Errorf("bundleSoundName not deterministic for the same path")
	}
}
