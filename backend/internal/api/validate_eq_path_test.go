package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func postValidate(t *testing.T, dir string) validateEQPathResponse {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"path": dir})
	rec := httptest.NewRecorder()
	(&configHandler{}).validateEQPath(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))
	var resp validateEQPathResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// A fresh install with logging off has no eqlog files yet; setup must still
// accept it (with a note) so the wizard's logging step can turn logging on.
func TestValidateEQPathAcceptsInstallWithoutLogs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "eqgame.exe"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eqclient.ini"), []byte("[Defaults]\nLog=FALSE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := postValidate(t, dir)
	if !resp.Valid || resp.Error != "" {
		t.Fatalf("valid=%v error=%q, want valid", resp.Valid, resp.Error)
	}
	if resp.HasLogs || resp.Note == "" {
		t.Errorf("has_logs=%v note=%q, want no logs and a note", resp.HasLogs, resp.Note)
	}
}

func TestValidateEQPathRejectsNonEQFolder(t *testing.T) {
	resp := postValidate(t, t.TempDir())
	if resp.Valid || resp.Error == "" {
		t.Fatalf("valid=%v error=%q, want rejected", resp.Valid, resp.Error)
	}
}
