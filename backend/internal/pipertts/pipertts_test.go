package pipertts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVoiceName(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"", ""},
		{"/voices/en_US-amy-medium.onnx", "en_US-amy-medium"},
		{`C:\voices\en_GB-alba-medium.onnx`, "en_GB-alba-medium"},
		{"en_US-lessac-high.onnx", "en_US-lessac-high"},
		{"/voices/noext", "noext"},
	}
	for _, tt := range tests {
		got := Config{ModelPath: tt.model}.VoiceName()
		if got != tt.want {
			t.Errorf("VoiceName(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}

func TestEffectiveMode(t *testing.T) {
	if got := (Config{Mode: ""}).EffectiveMode(); got != "spawn" {
		t.Errorf("empty mode should default to spawn, got %q", got)
	}
	if got := (Config{Mode: "spawn"}).EffectiveMode(); got != "spawn" {
		t.Errorf("spawn mode should stay spawn, got %q", got)
	}
	if got := (Config{Mode: "warm"}).EffectiveMode(); got != "warm" {
		t.Errorf("warm mode should stay warm, got %q", got)
	}
	if got := (Config{Mode: "bogus"}).EffectiveMode(); got != "spawn" {
		t.Errorf("unrecognized mode should fall back to spawn, got %q", got)
	}
}

func TestNormalizeText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  Mez  break  ", "Mez break"},
		{"Mez\tbreak\non\r\ngnoll", "Mez break on gnoll"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeText(tt.in); got != tt.want {
			t.Errorf("normalizeText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCacheKeyStableAndScoped(t *testing.T) {
	a := Config{ModelPath: "/v/amy.onnx"}
	b := Config{ModelPath: "/v/alba.onnx"}

	// Same voice + cosmetically-different-but-equal text → same key.
	if cacheKey(a, "Mez break") != cacheKey(a, "  Mez   break ") {
		t.Error("normalized-equal text should share a cache key")
	}
	// Different voice → different key for the same text.
	if cacheKey(a, "Mez break") == cacheKey(b, "Mez break") {
		t.Error("different model should produce a different cache key")
	}
	// Different text → different key.
	if cacheKey(a, "Mez break") == cacheKey(a, "Charm break") {
		t.Error("different text should produce a different cache key")
	}
}

func TestCachePathLayout(t *testing.T) {
	base := "/home/u/.pq-companion"
	p := cachePath(base, Config{ModelPath: "/v/amy.onnx"}, "hi")
	wantDir := filepath.Join(base, cacheDirName)
	if filepath.Dir(p) != wantDir {
		t.Errorf("cache path dir = %q, want %q", filepath.Dir(p), wantDir)
	}
	if !strings.HasSuffix(p, ".wav") {
		t.Errorf("cache path %q should end in .wav", p)
	}
}

func TestClearCache(t *testing.T) {
	base := t.TempDir()
	dir := cacheDir(base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.wav", "b.wav", "keep.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := clearCache(base)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("clearCache removed %d, want 2", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Error("clearCache should not remove non-.wav files")
	}
}

func TestClearCacheMissingDirIsNoError(t *testing.T) {
	n, err := clearCache(t.TempDir()) // no tts-cache subdir created
	if err != nil {
		t.Errorf("clearCache on missing dir should not error, got %v", err)
	}
	if n != 0 {
		t.Errorf("clearCache on missing dir removed %d, want 0", n)
	}
}

func TestSweepOldCacheAgeBoundary(t *testing.T) {
	base := t.TempDir()
	dir := cacheDir(base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dir, "old.wav")
	freshPath := filepath.Join(dir, "fresh.wav")
	for _, p := range []string{oldPath, freshPath} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}
	// freshPath keeps its just-written (now) mtime.

	n, err := sweepOldCache(base, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("sweepOldCache removed %d, want 1", n)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old.wav should have been removed")
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Error("fresh.wav should NOT have been removed")
	}
}

func TestSweepOldCacheMissingDirIsNoError(t *testing.T) {
	n, err := sweepOldCache(t.TempDir(), 24*time.Hour)
	if err != nil {
		t.Errorf("sweepOldCache on missing dir should not error, got %v", err)
	}
	if n != 0 {
		t.Errorf("sweepOldCache on missing dir removed %d, want 0", n)
	}
}

func TestTouchCacheFileRescuesFromSweep(t *testing.T) {
	base := t.TempDir()
	dir := cacheDir(base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "active.wav")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}

	touchCacheFile(p) // simulates a cache hit refreshing the mtime

	n, err := sweepOldCache(base, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("sweepOldCache removed %d files, want 0 (touch should have rescued it)", n)
	}
	if _, err := os.Stat(p); err != nil {
		t.Error("active.wav should still exist after touch + sweep")
	}
}

func TestCacheStats(t *testing.T) {
	base := t.TempDir()
	dir := cacheDir(base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{"a.wav": "1234", "b.wav": "12345678", "notes.txt": "ignored"}
	for name, content := range contents {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, bytes, err := cacheStats(base)
	if err != nil {
		t.Fatal(err)
	}
	if files != 2 {
		t.Errorf("cacheStats files = %d, want 2 (non-.wav should be excluded)", files)
	}
	if bytes != 12 { // "1234" (4) + "12345678" (8)
		t.Errorf("cacheStats bytes = %d, want 12", bytes)
	}
}

func TestCacheStatsMissingDirIsNoError(t *testing.T) {
	files, bytes, err := cacheStats(t.TempDir())
	if err != nil {
		t.Errorf("cacheStats on missing dir should not error, got %v", err)
	}
	if files != 0 || bytes != 0 {
		t.Errorf("cacheStats on missing dir = (%d, %d), want (0, 0)", files, bytes)
	}
}

// TestDetectStatusModelConfigSidecar is a regression test for a bug where the
// primary sidecar check appended ".onnx.json" to a ModelPath that ALREADY ends
// in ".onnx" (producing "<voice>.onnx.onnx.json", which never exists) instead
// of just ".json". Real files on disk, mirroring the actual piper-voices
// download convention: "<voice>.onnx" + "<voice>.onnx.json" side by side.
func TestDetectStatusModelConfigSidecar(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "piper")
	modelPath := filepath.Join(dir, "en_US-amy-medium.onnx")
	sidecarPath := modelPath + ".json" // -> en_US-amy-medium.onnx.json

	for _, p := range []string{exePath, modelPath, sidecarPath} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	st := DetectStatus(context.Background(), Config{
		Enabled:   true,
		ExePath:   exePath,
		ModelPath: modelPath,
	})
	if !st.ModelFound {
		t.Error("ModelFound should be true")
	}
	if !st.ModelConfigFound {
		t.Errorf("ModelConfigFound should be true for sidecar at %s (this was the bug)", sidecarPath)
	}
}

func TestReadinessError(t *testing.T) {
	// Nothing configured yet → no error surfaced.
	if got := readinessError(Status{}); got != "" {
		t.Errorf("empty status error = %q, want empty", got)
	}
	// Exe set but missing.
	st := Status{ExePath: "/nope/piper"}
	if got := readinessError(st); !strings.Contains(got, "executable not found") {
		t.Errorf("missing exe error = %q", got)
	}
	// Model present, config missing.
	st = Status{ExePath: "/p", ExeFound: true, ModelPath: "/m.onnx", ModelFound: true}
	if got := readinessError(st); !strings.Contains(got, "onnx.json") {
		t.Errorf("missing model config error = %q", got)
	}
}
