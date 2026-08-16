package appbackup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

func TestMergeImportedConfig(t *testing.T) {
	dir := t.TempDir()
	validEQPath := filepath.Join(dir, "eq")
	if err := os.MkdirAll(validEQPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	validPiperExe := filepath.Join(dir, "piper.exe")
	if err := os.WriteFile(validPiperExe, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	validPiperModel := filepath.Join(dir, "voice.onnx")
	if err := os.WriteFile(validPiperModel, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	missing := filepath.Join(dir, "does-not-exist")

	t.Run("server addr never imported when local exists", func(t *testing.T) {
		imported := config.Config{ServerAddr: ":9999"}
		local := config.Config{ServerAddr: ":17654"}
		got := mergeImportedConfig(imported, &local)
		if got.ServerAddr != ":17654" {
			t.Errorf("ServerAddr = %q, want local's %q", got.ServerAddr, ":17654")
		}
	})

	t.Run("server addr from imported when no local config", func(t *testing.T) {
		imported := config.Config{ServerAddr: ":9999"}
		got := mergeImportedConfig(imported, nil)
		if got.ServerAddr != ":9999" {
			t.Errorf("ServerAddr = %q, want imported's %q", got.ServerAddr, ":9999")
		}
	})

	t.Run("EQPath: imported wins when it resolves on this machine", func(t *testing.T) {
		imported := config.Config{EQPath: validEQPath, Character: "Bob", CharacterClass: 1}
		local := config.Config{EQPath: missing}
		got := mergeImportedConfig(imported, &local)
		if got.EQPath != validEQPath {
			t.Errorf("EQPath = %q, want imported %q", got.EQPath, validEQPath)
		}
	})

	t.Run("EQPath: falls back to local when imported doesn't resolve", func(t *testing.T) {
		imported := config.Config{EQPath: missing, Character: "Alice"}
		local := config.Config{EQPath: validEQPath, Character: "Carol", CharacterClass: 2}
		got := mergeImportedConfig(imported, &local)
		if got.EQPath != validEQPath || got.Character != "Carol" || got.CharacterClass != 2 {
			t.Errorf("got EQPath=%q Character=%q CharacterClass=%d, want local's values",
				got.EQPath, got.Character, got.CharacterClass)
		}
	})

	t.Run("EQPath: blanked and onboarding reset when neither resolves", func(t *testing.T) {
		imported := config.Config{EQPath: missing, OnboardingCompleted: true}
		local := config.Config{EQPath: missing, OnboardingCompleted: true}
		got := mergeImportedConfig(imported, &local)
		if got.EQPath != "" || got.OnboardingCompleted {
			t.Errorf("got EQPath=%q OnboardingCompleted=%v, want blank/false", got.EQPath, got.OnboardingCompleted)
		}
	})

	t.Run("Piper disabled when either path is missing on this machine", func(t *testing.T) {
		imported := config.Config{Preferences: config.Preferences{
			PiperEnabled: true, PiperExePath: validPiperExe, PiperModelPath: missing,
		}}
		got := mergeImportedConfig(imported, nil)
		if got.Preferences.PiperEnabled || got.Preferences.PiperExePath != "" || got.Preferences.PiperModelPath != "" {
			t.Errorf("got PiperEnabled=%v ExePath=%q ModelPath=%q, want disabled+blank",
				got.Preferences.PiperEnabled, got.Preferences.PiperExePath, got.Preferences.PiperModelPath)
		}
	})

	t.Run("Piper preserved when both paths resolve", func(t *testing.T) {
		imported := config.Config{Preferences: config.Preferences{
			PiperEnabled: true, PiperExePath: validPiperExe, PiperModelPath: validPiperModel,
		}}
		got := mergeImportedConfig(imported, nil)
		if !got.Preferences.PiperEnabled || got.Preferences.PiperExePath != validPiperExe {
			t.Errorf("got PiperEnabled=%v ExePath=%q, want preserved", got.Preferences.PiperEnabled, got.Preferences.PiperExePath)
		}
	})

	t.Run("everything else: imported wins", func(t *testing.T) {
		imported := config.Config{ChatRetentionDays: 999}
		local := config.Config{ChatRetentionDays: 30}
		got := mergeImportedConfig(imported, &local)
		if got.ChatRetentionDays != 999 {
			t.Errorf("ChatRetentionDays = %d, want imported's 999", got.ChatRetentionDays)
		}
	})
}
