package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

func TestNormalizeCurveFallsBackWhenClampedDuplicatesCannotIncrease(t *testing.T) {
	cfg := Normalize(Config{Curve: []controller.Point{{100, 10}, {100, 20}, {100, 30}, {100, 40}, {100, 50}}})
	if !reflect.DeepEqual(cfg.Curve, controller.DefaultCurve) {
		t.Fatalf("curve=%v, want default %v", cfg.Curve, controller.DefaultCurve)
	}
}

func TestLoadMigratesLegacyJSONNumericStrings(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	legacyDir := filepath.Join(dir, "exe")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"low_t": "45", "low_s": "35", "max_t": "90", "max_s": "100", "manual_enabled": true, "manual_speed": 88}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Load(paths.Paths{StateDir: state, ConfigPath: filepath.Join(state, "config.json"), LegacyData: filepath.Join(state, "data.dat"), ExecutableDir: legacyDir})
	if cfg.Curve[0] != (controller.Point{45, 35}) || cfg.Curve[4] != (controller.Point{90, 100}) {
		t.Fatalf("curve=%v, want migrated endpoints", cfg.Curve)
	}
	if !cfg.ManualEnabled || cfg.ManualSpeed != 88 {
		t.Fatalf("manual=%t speed=%d, want true/88", cfg.ManualEnabled, cfg.ManualSpeed)
	}
}

func TestConfigCloneDeepCopiesPresets(t *testing.T) {
	cfg := Normalize(Default())
	clone := cfg.Clone()
	clone.Presets["balanced"] = PresetConfig{Curve: []controller.Point{{30, 1}}, Strategy: "cpu"}
	if cfg.Presets["balanced"].Strategy == "cpu" {
		t.Fatal("Clone shared preset map with original config")
	}
}
