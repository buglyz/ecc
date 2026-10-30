package config

import (
	"testing"

	"github.com/buglyz/ecc/internal/controller"
)

func TestApplyPresetSwitchesToCurveMode(t *testing.T) {
	cfg := Default()
	cfg.ControlMode = controller.ControlModeTarget

	if !ApplyPreset(&cfg, "silent") {
		t.Fatal("ApplyPreset(silent) returned false")
	}
	if cfg.ControlMode != controller.ControlModeCurve {
		t.Fatalf("ControlMode=%q, want %q", cfg.ControlMode, controller.ControlModeCurve)
	}
}

func TestNormalizeTargetFields(t *testing.T) {
	cfg := Normalize(Config{
		ControlMode:      "invalid",
		TargetTemp:       10,
		TargetHysteresis: 0,
		TargetKP:         100,
		TargetMin:        90,
		TargetMax:        10,
	})
	if cfg.ControlMode != controller.ControlModeCurve {
		t.Fatalf("ControlMode=%q, want curve", cfg.ControlMode)
	}
	if cfg.TargetTemp != 40 {
		t.Fatalf("TargetTemp=%v, want 40", cfg.TargetTemp)
	}
	if cfg.TargetHysteresis != controller.DefaultTargetHysteresis {
		t.Fatalf("TargetHysteresis=%v, want default", cfg.TargetHysteresis)
	}
	if cfg.TargetKP != 20 {
		t.Fatalf("TargetKP=%v, want 20", cfg.TargetKP)
	}
	if cfg.TargetMin != 90 || cfg.TargetMax != 90 {
		t.Fatalf("range=%d-%d, want 90-90", cfg.TargetMin, cfg.TargetMax)
	}
}
