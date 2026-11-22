package config

import (
	"path/filepath"
	"testing"
)

func TestZealCapsSeenPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	m, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Modify(func(c *Config) {
		c.Preferences.ZealCapsSeen = map[string]string{"pipe_spells": "c:\\eq|1.5.0|123|456"}
	}); err != nil {
		t.Fatal(err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Get().Preferences.ZealCapsSeen["pipe_spells"]; got != "c:\\eq|1.5.0|123|456" {
		t.Fatalf("after reload got %q", got)
	}
}
