package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugMemory(t *testing.T) {
	rec := httptest.NewRecorder()
	debugMemory(rec, httptest.NewRequest(http.MethodGet, "/api/debug/memory", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got memoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.WorkingSetMB <= 0 || got.HeapInuseMB <= 0 || got.NumGoroutine <= 0 {
		t.Fatalf("expected positive stats, got %+v", got)
	}
}
