package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

type dummyReader struct{}

func (dummyReader) Read() controller.Temps {
	cpu := 55.0
	gpu := 50.0
	return controller.Temps{CPU: &cpu, GPU: &gpu}
}
func (dummyReader) Close() error { return nil }

type dummyWriter struct{}

func (dummyWriter) Write(ctx context.Context, register string, valueHex string) bool {
	return true
}

type mockStartup struct {
	enabled bool
}

func (m *mockStartup) IsEnabled(identifier string) bool {
	return m.enabled
}

func (m *mockStartup) Add(targetPath, identifier string) error {
	m.enabled = true
	return nil
}

func (m *mockStartup) Remove(identifier string) error {
	m.enabled = false
	return nil
}

func TestServiceLifecycleAndPresetManagement(t *testing.T) {
	tempDir := t.TempDir()
	p := paths.Paths{
		StateDir:      tempDir,
		ConfigPath:    filepath.Join(tempDir, "config.json"),
		StartupTarget: filepath.Join(tempDir, "ecc.exe"),
	}

	cfg := config.Default()
	fan := controller.NewFanController(dummyReader{}, dummyWriter{}, cfg.Curve, cfg.Strategy, time.Second, nil)
	svc := New(p, &cfg, fan, nil)
	svc.startup = &mockStartup{enabled: false}
	svc.startupEnabled = false

	state := svc.State()
	if state.Startup {
		t.Fatal("expected startup to be false initially")
	}

	// Test Update
	newCurve := []controller.Point{{35, 20}, {50, 30}, {65, 50}, {80, 75}, {95, 100}}
	updatedCfg, err := svc.Update(Update{
		Curve:         newCurve,
		Strategy:      "max",
		ManualEnabled: true,
		ManualSpeed:   65,
		Theme:         "dark",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updatedCfg.Strategy != "max" || updatedCfg.Theme != "dark" || !updatedCfg.ManualEnabled || updatedCfg.ManualSpeed != 65 {
		t.Fatalf("Update config mismatch: %+v", updatedCfg)
	}

	// Test AddPreset
	addedCfg, err := svc.AddPreset("我的挡位")
	if err != nil {
		t.Fatalf("AddPreset failed: %v", err)
	}
	activeKey := addedCfg.ActivePreset
	if addedCfg.Presets[activeKey].Label != "我的挡位" {
		t.Fatalf("AddPreset label mismatch: %s", addedCfg.Presets[activeKey].Label)
	}

	// Test SavePreset
	savedCfg, err := svc.SavePreset(activeKey)
	if err != nil {
		t.Fatalf("SavePreset failed: %v", err)
	}
	if savedCfg.Presets[activeKey].Strategy != "max" {
		t.Fatalf("SavePreset strategy mismatch: %s", savedCfg.Presets[activeKey].Strategy)
	}

	// Test ApplyPreset
	appliedCfg, err := svc.ApplyPreset("silent")
	if err != nil {
		t.Fatalf("ApplyPreset failed: %v", err)
	}
	if appliedCfg.ActivePreset != "silent" {
		t.Fatalf("ApplyPreset active mismatch: %s", appliedCfg.ActivePreset)
	}

	// Test RestorePreset
	restoredCfg, err := svc.RestorePreset("silent")
	if err != nil {
		t.Fatalf("RestorePreset failed: %v", err)
	}
	if restoredCfg.ActivePreset != "silent" {
		t.Fatalf("RestorePreset active mismatch: %s", restoredCfg.ActivePreset)
	}

	// Test DeletePreset
	delCfg, err := svc.DeletePreset(activeKey)
	if err != nil {
		t.Fatalf("DeletePreset failed: %v", err)
	}
	if _, exists := delCfg.Presets[activeKey]; exists {
		t.Fatalf("deleted preset %s still exists", activeKey)
	}

	// Test SetStartup
	enabled, err := svc.SetStartup(true)
	if err != nil || !enabled {
		t.Fatalf("SetStartup(true) failed: enabled=%t, err=%v", enabled, err)
	}
	enabled, err = svc.SetStartup(false)
	if err != nil || enabled {
		t.Fatalf("SetStartup(false) failed: enabled=%t, err=%v", enabled, err)
	}

	// Test SetTheme
	themeCfg, err := svc.SetTheme("light")
	if err != nil || themeCfg.Theme != "light" {
		t.Fatalf("SetTheme failed: %v", err)
	}

	// Verify file was written
	if _, err := os.Stat(p.ConfigPath); err != nil {
		t.Fatalf("config file was not saved: %v", err)
	}
}
