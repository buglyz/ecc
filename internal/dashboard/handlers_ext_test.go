package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buglyz/ecc/internal/startup"
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

func TestHandleStartupRejectsUnsafeTarget(t *testing.T) {
	server, _ := newTestServer(t)
	fake := server.startup.(*fakeStartupController)
	fake.addErr = startup.ErrUnsafeStartupTarget

	rec := postJSON(t, server.handleStartup, map[string]any{"enabled": true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("enable status=%d, want 400", rec.Code)
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
	bigBody := `{"theme":"` + strings.Repeat("a", maxRequestBody) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))
	rec := httptest.NewRecorder()
	server.handleConfig(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d, want 413", rec.Code)
	}
}

func TestHandleConfigRejectsTrailingJSON(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"theme":"dark"} {"theme":"light"}`))
	rec := httptest.NewRecorder()
	server.handleConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestRejectCrossOriginBlocksForeignOrigin(t *testing.T) {
	wrapped := rejectCrossOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Same-origin (no Origin header) — allowed
	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Host = "127.0.0.1:8765"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("same-origin status=%d, want 200", rec.Code)
	}

	// Matching Origin — allowed
	req2 := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req2.Host = "127.0.0.1:8765"
	req2.Header.Set("Origin", "http://127.0.0.1:8765")
	rec2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("matching origin status=%d, want 200", rec2.Code)
	}

	// Foreign Origin — blocked
	req3 := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req3.Host = "127.0.0.1:8765"
	req3.Header.Set("Origin", "http://evil.example.com")
	rec3 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status=%d, want 403", rec3.Code)
	}

	// Foreign Origin on non-API path — allowed (only /api/* is guarded)
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.Host = "127.0.0.1:8765"
	req4.Header.Set("Origin", "http://evil.example.com")
	rec4 := httptest.NewRecorder()
	wrapped.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("non-API foreign origin status=%d, want 200", rec4.Code)
	}
}
