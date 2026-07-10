package config

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

func TestSaveAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	p := paths.Paths{StateDir: dir, ConfigPath: cfgPath, LegacyData: filepath.Join(dir, "data.dat")}

	cfg := Default()
	cfg.ManualSpeed = 77

	if err := Save(p, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify the file exists and is valid JSON (written via tmp + rename).
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if loaded.ManualSpeed != 77 {
		t.Errorf("manual_speed=%d, want 77", loaded.ManualSpeed)
	}

	// Verify no stale .tmp file left behind.
	if _, err := os.Stat(cfgPath + ".tmp"); err == nil {
		t.Error("stale .tmp file should not exist after Save")
	}
}

func TestSaveCreatesStateDir(t *testing.T) {
	dir := t.TempDir()
	nestedStateDir := filepath.Join(dir, "deep", "nested")
	cfgPath := filepath.Join(nestedStateDir, "config.json")
	p := paths.Paths{StateDir: nestedStateDir, ConfigPath: cfgPath, LegacyData: filepath.Join(nestedStateDir, "data.dat")}

	cfg := Default()
	if err := Save(p, cfg); err != nil {
		t.Fatalf("Save with nested dir: %v", err)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestSaveNormalizesNonFiniteCurveValues(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	p := paths.Paths{StateDir: dir, ConfigPath: cfgPath, LegacyData: filepath.Join(dir, "data.dat")}
	cfg := Default()
	cfg.Curve[0][0] = math.NaN()

	if err := Save(p, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("saved config is not valid JSON: %s", data)
	}
}

func TestNormalizeClampsManualSpeed(t *testing.T) {
	cfg := Default()
	cfg.ManualSpeed = 200
	normalized := Normalize(cfg)
	if normalized.ManualSpeed != 100 {
		t.Errorf("Normalize(200) = %d, want 100", normalized.ManualSpeed)
	}
}

func TestNormalizeFixesInvalidStrategy(t *testing.T) {
	cfg := Default()
	cfg.Strategy = "bogus"
	normalized := Normalize(cfg)
	if normalized.Strategy != controller.DefaultStrategy {
		t.Errorf("Normalize(bogus) = %q, want %q", normalized.Strategy, controller.DefaultStrategy)
	}
}

func TestNormalizeFixesInvalidTheme(t *testing.T) {
	cfg := Default()
	cfg.Theme = "neon"
	normalized := Normalize(cfg)
	if normalized.Theme != "light" {
		t.Errorf("Normalize(neon) = %q, want light", normalized.Theme)
	}
}

func TestNormalizeFixesInvalidActivePreset(t *testing.T) {
	cfg := Default()
	cfg.ActivePreset = "nonexistent"
	normalized := Normalize(cfg)
	if normalized.ActivePreset != "balanced" {
		t.Errorf("Normalize(nonexistent preset) = %q, want balanced", normalized.ActivePreset)
	}
}

func TestNormalizeClampsTimeEntry(t *testing.T) {
	cfg := Default()
	cfg.TimeEntry = "9999"
	normalized := Normalize(cfg)
	if normalized.TimeEntry != "480" {
		t.Errorf("Normalize(9999) = %q, want 480", normalized.TimeEntry)
	}
}
