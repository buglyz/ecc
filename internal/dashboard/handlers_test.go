package dashboard

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

func newTestServer(t *testing.T) (*Server, config.Config) {
	t.Helper()
	dir := t.TempDir()
	p := paths.Paths{StateDir: dir, ConfigPath: filepath.Join(dir, "config.json"), LegacyData: filepath.Join(dir, "data.dat")}
	cfg := config.Normalize(config.Default())
	fan := controller.NewFanController(staticReader{}, okWriter{}, cfg.Curve, cfg.Strategy, 0, log.New(os.Stderr, "", 0))
	server := New("127.0.0.1:0", p, &cfg, fan, log.New(os.Stderr, "", 0))
	server.startup = &fakeStartupController{}
	return server, cfg
}

func postJSON(t *testing.T, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestHandleConfigRejectsInvalidStrategy(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handleConfig, map[string]any{"strategy": "bogus"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestHandleConfigClampsManualSpeedAndPersists(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handleConfig, map[string]any{"manual_speed": 250, "manual_enabled": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if got := server.configSnapshot().ManualSpeed; got != 100 {
		t.Fatalf("manual_speed=%d, want clamped to 100", got)
	}
	// Verify it was written to disk.
	reloaded := config.Load(server.paths).Config
	if reloaded.ManualSpeed != 100 || !reloaded.ManualEnabled {
		t.Fatalf("persisted manual_speed=%d enabled=%t, want 100/true", reloaded.ManualSpeed, reloaded.ManualEnabled)
	}
}

func TestHandleConfigUpdatesCurve(t *testing.T) {
	server, _ := newTestServer(t)
	curve := []controller.Point{{40, 10}, {55, 20}, {70, 30}, {80, 40}, {90, 50}}
	rec := postJSON(t, server.handleConfig, map[string]any{"curve": curve})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	got := server.configSnapshot().Curve
	if got[0].Speed() != 10 || got[4].Speed() != 50 {
		t.Fatalf("curve=%v, want updated speeds", got)
	}
}

func TestHandleConfigTimeEntryClamped(t *testing.T) {
	server, _ := newTestServer(t)
	postJSON(t, server.handleConfig, map[string]any{"time_entry": "9999"})
	if got := server.configSnapshot().TimeEntry; got != "480" {
		t.Fatalf("time_entry=%q, want clamped to 480", got)
	}
}

func TestHandlePresetAppliesKnownPreset(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handlePreset, map[string]any{"key": "silent"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	snap := server.configSnapshot()
	if snap.ActivePreset != "silent" {
		t.Fatalf("active_preset=%q, want silent", snap.ActivePreset)
	}
}

func TestHandlePresetRejectsUnknownPreset(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handlePreset, map[string]any{"key": "turbo"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestHandlePresetAddCreatesCustomPreset(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "add", "label": "我的挡位"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	snap := server.configSnapshot()
	if snap.ActivePreset != "custom1" {
		t.Fatalf("active_preset=%q, want custom1", snap.ActivePreset)
	}
	if snap.Presets["custom1"].Label != "我的挡位" {
		t.Fatalf("label=%q, want 我的挡位", snap.Presets["custom1"].Label)
	}
}

func TestHandlePresetSaveWritesWorkingStateToSlot(t *testing.T) {
	server, _ := newTestServer(t)
	// Change the working curve via /api/config, then save it into the active preset slot.
	curve := []controller.Point{{40, 12}, {55, 22}, {70, 32}, {80, 42}, {90, 52}}
	if rec := postJSON(t, server.handleConfig, map[string]any{"curve": curve}); rec.Code != http.StatusOK {
		t.Fatalf("config status=%d", rec.Code)
	}
	active := server.configSnapshot().ActivePreset
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "save", "key": active})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	slot := server.configSnapshot().Presets[active]
	if slot.Curve[0].Speed() != 12 || slot.Curve[4].Speed() != 52 {
		t.Fatalf("saved slot curve=%v, want working curve", slot.Curve)
	}
}

func TestHandlePresetRestoreResetsBuiltin(t *testing.T) {
	server, _ := newTestServer(t)
	// Mutate balanced, save it, then restore to factory defaults.
	curve := []controller.Point{{40, 99}, {55, 99}, {70, 99}, {80, 99}, {90, 99}}
	postJSON(t, server.handleConfig, map[string]any{"curve": curve})
	postJSON(t, server.handlePreset, map[string]any{"action": "save", "key": "balanced"})
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "restore", "key": "balanced"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	def, _ := config.DefaultPresetConfig("balanced")
	slot := server.configSnapshot().Presets["balanced"]
	if slot.Curve[0] != def.Curve[0] || slot.Curve[4] != def.Curve[4] {
		t.Fatalf("restored slot=%v, want factory %v", slot.Curve, def.Curve)
	}
}

func TestHandlePresetRestoreRejectsCustom(t *testing.T) {
	server, _ := newTestServer(t)
	postJSON(t, server.handlePreset, map[string]any{"action": "add", "label": "tmp"})
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "restore", "key": "custom1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 (custom presets have no factory default)", rec.Code)
	}
}

func TestHandlePresetDeleteRemovesCustomAndFallsBack(t *testing.T) {
	server, _ := newTestServer(t)
	postJSON(t, server.handlePreset, map[string]any{"action": "add", "label": "tmp"})
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "delete", "key": "custom1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	snap := server.configSnapshot()
	if _, exists := snap.Presets["custom1"]; exists {
		t.Fatal("custom1 should be deleted")
	}
	if snap.ActivePreset != "balanced" {
		t.Fatalf("active_preset=%q, want fallback balanced", snap.ActivePreset)
	}
}

func TestHandlePresetDeleteRejectsBuiltin(t *testing.T) {
	server, _ := newTestServer(t)
	rec := postJSON(t, server.handlePreset, map[string]any{"action": "delete", "key": "silent"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 (builtin presets cannot be deleted)", rec.Code)
	}
}

func TestHandlersRejectWrongMethod(t *testing.T) {
	server, _ := newTestServer(t)
	for _, h := range []http.HandlerFunc{server.handleConfig, server.handlePreset, server.handleStartup} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d, want 405", rec.Code)
		}
	}
}

func TestHandleConfigRejectsMalformedJSON(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{not json"))
	rec := httptest.NewRecorder()
	server.handleConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestHandleStateReturnsConfigAndMetadata(t *testing.T) {
	server, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	rec := httptest.NewRecorder()
	server.handleState(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	var resp stateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Strategies) != len(controller.Strategies) {
		t.Fatalf("strategies=%d, want %d", len(resp.Strategies), len(controller.Strategies))
	}
	if len(resp.Presets) != len(controller.Presets) {
		t.Fatalf("presets=%d, want %d", len(resp.Presets), len(controller.Presets))
	}
}
