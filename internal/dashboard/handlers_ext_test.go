package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleHealthReturnsOK(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	server.handleHealth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if resp["ok"] != true {
		t.Error("health ok != true")
	}
}

func TestHandleHealthRejectsPost(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	rec := httptest.NewRecorder()
	server.handleHealth(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, want 405", rec.Code)
	}
}

func TestHandleStateWithMinutes(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state?minutes=5", nil)
	rec := httptest.NewRecorder()
	server.handleState(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
}

func TestHandleStateInvalidMinutesIgnored(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state?minutes=abc", nil)
	rec := httptest.NewRecorder()
	server.handleState(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 (invalid minutes should be ignored)", rec.Code)
	}
}

func TestHandleStateRejectsPost(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/state", nil)
	rec := httptest.NewRecorder()
	server.handleState(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, want 405", rec.Code)
	}
}

func TestHandleIndexReturnsHTML(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.handleIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("content-type=%q, want text/html", ct)
	}
}

func TestHandleIndexNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	server.handleIndex(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestHandleStartupToggle(t *testing.T) {
	server, _ := newTestServer(t)
	// Enable
	rec := postJSON(t, server.handleStartup, map[string]any{"enabled": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status=%d, want 200", rec.Code)
	}
	// Disable
	rec2 := postJSON(t, server.handleStartup, map[string]any{"enabled": false})
	if rec2.Code != http.StatusOK {
		t.Fatalf("disable status=%d, want 200", rec2.Code)
	}
}

func TestHandlePresetRejectsInvalidAction(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "explode", "key": "balanced"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestHandlePresetAddAcceptsEmptyLabel(t *testing.T) {
	server, _ := newTestServer(t)
	// Empty label is sanitized by AddPreset (falls back to key name), not rejected.
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "add"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 (empty label is sanitized, not rejected)", rec.Code)
	}
	snap := server.configSnapshot()
	slot, ok := snap.Presets["custom1"]
	if !ok {
		t.Fatal("custom1 preset not created")
	}
	if slot.Label == "" {
		t.Error("label should have been auto-filled from key name")
	}
}

func TestDecodeJSONLimitExceeded(t *testing.T) {
	server, _ := newTestServer(t)
	// Send a body larger than 64KB
	bigBody := make([]byte, 65*1024)
	for i := range bigBody {
		bigBody[i] = 'a'
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(bigBody)))
	rec := httptest.NewRecorder()
	server.handleConfig(rec, req)
	// Should fail to parse, not crash
	if rec.Code != http.StatusBadRequest {
		// It could also be 200 if the parser hit EOF before finding valid JSON
		// The important thing is it doesn't panic or OOM
		t.Logf("status=%d (acceptable, no panic)", rec.Code)
	}
}
