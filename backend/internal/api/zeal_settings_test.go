package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// The onboarding wizard checks the folder it's editing before config is
// saved: ?path= must win over the (empty) configured EQ path.
func TestZealSettings_PathOverrideBeforeConfigSaved(t *testing.T) {
	cfgMgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	eq := t.TempDir()
	asi := []byte("MZ Zeal Version: \x001.5.0\x00")
	if err := os.WriteFile(filepath.Join(eq, "Zeal.asi"), asi, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(eq, "zeal.ini"), []byte("[Zeal]\nPipeVerbose=TRUE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &zealHandler{cfgMgr: cfgMgr}

	rec := httptest.NewRecorder()
	h.settings(rec, httptest.NewRequest(http.MethodGet, "/api/zeal/settings?path="+url.QueryEscape(eq), nil))
	var resp zealSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Installed || resp.Version != "1.5.0" || !resp.INIReadable {
		t.Fatalf("override folder not inspected: %+v", resp)
	}
	found := false
	for _, c := range resp.Capabilities {
		if c.ID == "pipe_verbose" {
			found = c.State == "on"
		}
		if c.Pending {
			t.Errorf("pending %s listed with no proof", c.ID)
		}
	}
	if !found {
		t.Errorf("pipe_verbose should read on from the override folder's zeal.ini: %+v", resp.Capabilities)
	}

	// No override and nothing configured: not installed, empty list.
	rec = httptest.NewRecorder()
	h.settings(rec, httptest.NewRequest(http.MethodGet, "/api/zeal/settings", nil))
	resp = zealSettingsResponse{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Installed || resp.Capabilities == nil || len(resp.Capabilities) != 0 {
		t.Errorf("unconfigured: %+v", resp)
	}
}
