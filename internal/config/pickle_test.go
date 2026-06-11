package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/paths"
)

// realPickle is a genuine protocol-4 pickle stream produced by CPython's
// pickle.dumps for the config dict below. It guards the hand-written pickle
// parser against the exact opcode sequence the Python original writes.
var realPickle = []byte{
	128, 4, 149, 14, 1, 0, 0, 0, 0, 0, 0, 125, 148, 40, 140, 5, 99, 117, 114, 118,
	101, 148, 93, 148, 40, 93, 148, 40, 75, 40, 75, 30, 101, 93, 148, 40, 75, 55,
	75, 40, 101, 93, 148, 40, 75, 70, 75, 60, 101, 93, 148, 40, 75, 80, 75, 85,
	101, 93, 148, 40, 75, 90, 75, 100, 101, 101, 140, 8, 115, 116, 114, 97, 116,
	101, 103, 121, 148, 140, 3, 109, 97, 120, 148, 140, 5, 116, 104, 101, 109, 101,
	148, 140, 4, 100, 97, 114, 107, 148, 140, 10, 116, 105, 109, 101, 95, 101, 110,
	116, 114, 121, 148, 140, 2, 49, 50, 148, 140, 8, 109, 105, 110, 105, 109, 105,
	122, 101, 148, 75, 1, 140, 14, 109, 97, 110, 117, 97, 108, 95, 101, 110, 97,
	98, 108, 101, 100, 148, 136, 140, 12, 109, 97, 110, 117, 97, 108, 95, 115, 112,
	101, 101, 100, 148, 75, 77, 140, 13, 97, 99, 116, 105, 118, 101, 95, 112, 114,
	101, 115, 101, 116, 148, 140, 11, 112, 101, 114, 102, 111, 114, 109, 97, 110,
	99, 101, 148, 140, 7, 112, 114, 101, 115, 101, 116, 115, 148, 125, 148, 140, 6,
	115, 105, 108, 101, 110, 116, 148, 125, 148, 40, 104, 1, 93, 148, 40, 93, 148,
	40, 75, 40, 75, 15, 101, 93, 148, 40, 75, 55, 75, 25, 101, 93, 148, 40, 75, 70,
	75, 40, 101, 93, 148, 40, 75, 80, 75, 60, 101, 93, 148, 40, 75, 90, 75, 85,
	101, 101, 104, 8, 140, 8, 119, 101, 105, 103, 104, 116, 101, 100, 148, 117,
	115, 117, 46,
}

func TestParsePickleDictDecodesRealStream(t *testing.T) {
	values, err := parsePickleDict(realPickle)
	if err != nil {
		t.Fatalf("parsePickleDict: %v", err)
	}
	cfg := configFromValues(values)

	if cfg.Strategy != "max" {
		t.Errorf("strategy=%q, want max", cfg.Strategy)
	}
	if cfg.Theme != "dark" {
		t.Errorf("theme=%q, want dark", cfg.Theme)
	}
	if cfg.TimeEntry != "12" {
		t.Errorf("time_entry=%q, want 12", cfg.TimeEntry)
	}
	if cfg.Minimize != 1 {
		t.Errorf("minimize=%d, want 1", cfg.Minimize)
	}
	if !cfg.ManualEnabled {
		t.Error("manual_enabled=false, want true")
	}
	if cfg.ManualSpeed != 77 {
		t.Errorf("manual_speed=%d, want 77", cfg.ManualSpeed)
	}
	if cfg.ActivePreset != "performance" {
		t.Errorf("active_preset=%q, want performance", cfg.ActivePreset)
	}
	wantCurve := []controller.Point{{40, 30}, {55, 40}, {70, 60}, {80, 85}, {90, 100}}
	for i, p := range wantCurve {
		if cfg.Curve[i] != p {
			t.Fatalf("curve[%d]=%v, want %v", i, cfg.Curve[i], p)
		}
	}
	silent, ok := cfg.Presets["silent"]
	if !ok {
		t.Fatal("missing silent preset from pickle")
	}
	if silent.Strategy != "weighted" || silent.Curve[0] != (controller.Point{40, 15}) {
		t.Fatalf("silent preset=%+v, want weighted curve starting at 40/15", silent)
	}
}

func TestLoadReadsPickleFromStateDir(t *testing.T) {
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "data.dat")
	if err := os.WriteFile(dataPath, realPickle, 0o644); err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{
		StateDir:   dir,
		ConfigPath: filepath.Join(dir, "config.json"),
		LegacyData: dataPath,
	}
	cfg := Load(p)
	if cfg.Strategy != "max" || cfg.ManualSpeed != 77 {
		t.Fatalf("loaded pickle strategy=%q speed=%d, want max/77", cfg.Strategy, cfg.ManualSpeed)
	}
}

func TestParsePickleDictRejectsGarbage(t *testing.T) {
	if _, err := parsePickleDict([]byte{0x01, 0x02, 0x03}); err == nil {
		t.Fatal("expected error decoding non-pickle bytes")
	}
}
