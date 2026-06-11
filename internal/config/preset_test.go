package config

import (
	"testing"

	"github.com/buglyz/ecc/internal/controller"
)

func TestApplyPresetSwitchesCurveAndStrategy(t *testing.T) {
	cfg := Normalize(Default())
	if !ApplyPreset(&cfg, "performance") {
		t.Fatal("ApplyPreset(performance) returned false")
	}
	if cfg.ActivePreset != "performance" {
		t.Fatalf("active_preset=%q, want performance", cfg.ActivePreset)
	}
	want := cfg.Presets["performance"]
	if cfg.Strategy != want.Strategy {
		t.Fatalf("strategy=%q, want %q", cfg.Strategy, want.Strategy)
	}
	// Mutating the applied curve must not bleed back into the stored preset.
	cfg.Curve[0] = controller.Point{31, 99}
	if cfg.Presets["performance"].Curve[0] == (controller.Point{31, 99}) {
		t.Fatal("ApplyPreset shared curve slice with stored preset")
	}
}

func TestApplyPresetUnknownKeyReturnsFalse(t *testing.T) {
	cfg := Normalize(Default())
	if ApplyPreset(&cfg, "nope") {
		t.Fatal("ApplyPreset(nope) returned true for unknown key")
	}
	if cfg.ActivePreset != "balanced" {
		t.Fatalf("active_preset=%q, want unchanged balanced", cfg.ActivePreset)
	}
}

func TestManualSpeedPtrNilWhenDisabled(t *testing.T) {
	cfg := Default()
	cfg.ManualEnabled = false
	cfg.ManualSpeed = 70
	if cfg.ManualSpeedPtr() != nil {
		t.Fatal("ManualSpeedPtr should be nil when manual mode disabled")
	}
	cfg.ManualEnabled = true
	if ptr := cfg.ManualSpeedPtr(); ptr == nil || *ptr != 70 {
		t.Fatalf("ManualSpeedPtr=%v, want 70", ptr)
	}
}

func TestSavePresetWritesWorkingStateIntoSlot(t *testing.T) {
	cfg := Normalize(Default())
	cfg.ActivePreset = "silent"
	cfg.Strategy = "cpu"
	cfg.Curve = []controller.Point{{40, 12}, {55, 23}, {70, 34}, {80, 45}, {90, 56}}
	if !SavePreset(&cfg, "silent") {
		t.Fatal("SavePreset(silent) returned false")
	}
	stored := cfg.Presets["silent"]
	if stored.Strategy != "cpu" || stored.Curve[0].Speed() != 12 || stored.Curve[4].Speed() != 56 {
		t.Fatalf("slot not updated with working state: %+v", stored)
	}
	// 内置挡位的 Label 必须保留，不能被覆盖丢失。
	if stored.Label == "" {
		t.Fatal("SavePreset dropped the preset label")
	}
}

func TestAddPresetCreatesCustomSlotAndActivates(t *testing.T) {
	cfg := Normalize(Default())
	cfg.Strategy = "max"
	cfg.Curve = []controller.Point{{40, 5}, {55, 15}, {70, 25}, {80, 35}, {90, 45}}
	if !AddPreset(&cfg, "custom1", "我的挡位") {
		t.Fatal("AddPreset returned false")
	}
	if cfg.ActivePreset != "custom1" {
		t.Fatalf("active_preset=%q, want custom1", cfg.ActivePreset)
	}
	got := cfg.Presets["custom1"]
	if got.Label != "我的挡位" || got.Strategy != "max" || got.Curve[0].Speed() != 5 {
		t.Fatalf("custom slot wrong: %+v", got)
	}
	// 重名 key 必须被拒绝；内置 key 不可作为自定义新增。
	if AddPreset(&cfg, "custom1", "dup") {
		t.Fatal("AddPreset allowed duplicate key")
	}
	if AddPreset(&cfg, "balanced", "x") {
		t.Fatal("AddPreset allowed a builtin key")
	}
}

func TestCustomPresetSurvivesNormalize(t *testing.T) {
	cfg := Normalize(Default())
	AddPreset(&cfg, "custom1", "夜间")
	cfg = Normalize(cfg)
	got, ok := cfg.Presets["custom1"]
	if !ok {
		t.Fatal("custom preset dropped by Normalize (regression of audit #9)")
	}
	if got.Label != "夜间" {
		t.Fatalf("custom label=%q, want 夜间", got.Label)
	}
	// 三个内置挡位必须始终存在。
	for _, key := range []string{"silent", "balanced", "performance"} {
		if _, ok := cfg.Presets[key]; !ok {
			t.Fatalf("builtin preset %q missing after Normalize", key)
		}
	}
}

func TestRestorePresetResetsBuiltinAndRefreshesActive(t *testing.T) {
	cfg := Normalize(Default())
	cfg.ActivePreset = "performance"
	cfg.Strategy = "cpu"
	cfg.Curve = []controller.Point{{40, 1}, {55, 2}, {70, 3}, {80, 4}, {90, 5}}
	SavePreset(&cfg, "performance")
	if !RestorePreset(&cfg, "performance") {
		t.Fatal("RestorePreset(performance) returned false")
	}
	def, _ := DefaultPresetConfig("performance")
	if cfg.Presets["performance"].Strategy != def.Strategy {
		t.Fatalf("restored strategy=%q, want factory %q", cfg.Presets["performance"].Strategy, def.Strategy)
	}
	// 恢复的是当前激活挡位，工作状态须同步刷新为出厂值。
	if !curvesEqual(cfg.Curve, def.Curve) || cfg.Strategy != def.Strategy {
		t.Fatalf("active working state not refreshed to factory: curve=%v strategy=%q", cfg.Curve, cfg.Strategy)
	}
	// 自定义挡位不可恢复默认。
	AddPreset(&cfg, "custom1", "x")
	if RestorePreset(&cfg, "custom1") {
		t.Fatal("RestorePreset allowed a custom preset")
	}
}

func TestDeletePresetRemovesCustomAndFallsBack(t *testing.T) {
	cfg := Normalize(Default())
	AddPreset(&cfg, "custom1", "x") // active becomes custom1
	if !DeletePreset(&cfg, "custom1") {
		t.Fatal("DeletePreset(custom1) returned false")
	}
	if _, ok := cfg.Presets["custom1"]; ok {
		t.Fatal("custom preset not removed")
	}
	if cfg.ActivePreset != "balanced" {
		t.Fatalf("active_preset=%q, want fallback balanced", cfg.ActivePreset)
	}
	// 内置挡位不可删除。
	if DeletePreset(&cfg, "silent") {
		t.Fatal("DeletePreset allowed a builtin preset")
	}
}

func curvesEqual(a, b []controller.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNormalizeClampsAndSortsCurve(t *testing.T) {
	cfg := Normalize(Config{Curve: []controller.Point{{90, 150}, {40, -20}, {70, 60}, {55, 40}, {80, 85}}})
	for i := 1; i < len(cfg.Curve); i++ {
		if cfg.Curve[i].Temp() <= cfg.Curve[i-1].Temp() {
			t.Fatalf("curve temps not strictly increasing: %v", cfg.Curve)
		}
	}
	if cfg.Curve[0].Speed() < controller.CurveSpeedMin || cfg.Curve[4].Speed() > controller.CurveSpeedMax {
		t.Fatalf("curve speeds not clamped: %v", cfg.Curve)
	}
}
