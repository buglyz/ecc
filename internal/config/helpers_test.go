package config

import (
	"strconv"
	"testing"

	"github.com/buglyz/ecc/internal/controller"
)

func TestSanitizeLabelTrimsAndTruncates(t *testing.T) {
	// leading/trailing whitespace trimmed
	if got := sanitizeLabel("  hello  "); got != "hello" {
		t.Errorf("sanitizeLabel(%q) = %q, want %q", "  hello  ", got, "hello")
	}
	// truncated to MaxPresetLabel runes
	long := "abcdefghijabcdefghijabcdefghijabcdefghijabcdefghij" // 50 chars
	if got := sanitizeLabel(long); len([]rune(got)) > MaxPresetLabel {
		t.Errorf("sanitizeLabel result too long: %d runes, max %d", len([]rune(got)), MaxPresetLabel)
	}
	// empty after trim
	if got := sanitizeLabel("   "); got != "" {
		t.Errorf("sanitizeLabel(spaces) = %q, want empty", got)
	}
}

func TestIsBuiltinPreset(t *testing.T) {
	for _, key := range []string{"silent", "balanced", "performance"} {
		if !IsBuiltinPreset(key) {
			t.Errorf("IsBuiltinPreset(%q) = false, want true", key)
		}
	}
	if IsBuiltinPreset("custom1") {
		t.Error("IsBuiltinPreset(custom1) = true, want false")
	}
	if IsBuiltinPreset("") {
		t.Error("IsBuiltinPreset(empty) = true, want false")
	}
}

func TestDefaultPresetConfig(t *testing.T) {
	cfg, ok := DefaultPresetConfig("balanced")
	if !ok {
		t.Fatal("DefaultPresetConfig(balanced) returned false")
	}
	if cfg.Label == "" {
		t.Error("default preset label is empty")
	}
	if len(cfg.Curve) != controller.CurvePoints {
		t.Errorf("default preset curve len=%d, want %d", len(cfg.Curve), controller.CurvePoints)
	}
	_, ok = DefaultPresetConfig("nonexistent")
	if ok {
		t.Error("DefaultPresetConfig(nonexistent) should return false")
	}
}

func TestMakeLegacyCurve(t *testing.T) {
	curve := makeLegacyCurve(40, 30, 90, 100)
	if len(curve) != controller.CurvePoints {
		t.Fatalf("curve len=%d, want %d", len(curve), controller.CurvePoints)
	}
	if curve[0].Temp() != 40 || curve[0].Speed() != 30 {
		t.Errorf("first point=%v, want [40,30]", curve[0])
	}
	if curve[4].Temp() != 90 || curve[4].Speed() != 100 {
		t.Errorf("last point=%v, want [90,100]", curve[4])
	}
	// monotonic
	for i := 1; i < len(curve); i++ {
		if curve[i].Temp() <= curve[i-1].Temp() {
			t.Errorf("non-monotonic at %d: %v -> %v", i-1, curve[i-1], curve[i])
		}
	}
}

func TestNormalizePresetsAlwaysIncludesBuiltins(t *testing.T) {
	raw := map[string]PresetConfig{"custom1": {Label: "我的", Curve: controller.DefaultCurve, Strategy: "weighted"}}
	out := normalizePresets(raw)
	for _, key := range []string{"silent", "balanced", "performance"} {
		if _, ok := out[key]; !ok {
			t.Errorf("builtin preset %q missing from normalized output", key)
		}
	}
	if _, ok := out["custom1"]; !ok {
		t.Error("custom preset dropped by normalizePresets")
	}
}

func TestNormalizePresetsSkipsEmptyKey(t *testing.T) {
	raw := map[string]PresetConfig{"": {Label: "bad", Curve: controller.DefaultCurve, Strategy: "weighted"}}
	out := normalizePresets(raw)
	if _, ok := out[""]; ok {
		t.Error("empty key should be filtered out")
	}
}

func TestNormalizeStrategyValidReturnsSame(t *testing.T) {
	if got := normalizeStrategy("max", "weighted"); got != "max" {
		t.Errorf("normalizeStrategy(max) = %q, want max", got)
	}
}

func TestNormalizeStrategyInvalidFallsBack(t *testing.T) {
	if got := normalizeStrategy("bogus", "weighted"); got != "weighted" {
		t.Errorf("normalizeStrategy(bogus) = %q, want weighted", got)
	}
}

func TestClamp(t *testing.T) {
	if got := clamp(-5, 0, 100); got != 0 {
		t.Errorf("clamp(-5,0,100) = %v, want 0", got)
	}
	if got := clamp(150, 0, 100); got != 100 {
		t.Errorf("clamp(150,0,100) = %v, want 100", got)
	}
	if got := clamp(50, 0, 100); got != 50 {
		t.Errorf("clamp(50,0,100) = %v, want 50", got)
	}
}

func TestBoolFromAny(t *testing.T) {
	if v, ok := boolFromAny(true); !ok || !v {
		t.Error("boolFromAny(true) failed")
	}
	if v, ok := boolFromAny(false); !ok || v {
		t.Error("boolFromAny(false) should return false")
	}
	if v, ok := boolFromAny(1); !ok || !v {
		t.Error("boolFromAny(1) should return true")
	}
	if v, ok := boolFromAny(0); !ok || v {
		t.Error("boolFromAny(0) should return false")
	}
	if v, ok := boolFromAny(int64(1)); !ok || !v {
		t.Error("boolFromAny(int64(1)) should return true")
	}
	if _, ok := boolFromAny("true"); ok {
		t.Error("boolFromAny(string) should return false")
	}
}

func TestIntFromAny(t *testing.T) {
	if v, ok := intFromAny(42); !ok || v != 42 {
		t.Errorf("intFromAny(42) = %d, want 42", v)
	}
	if v, ok := intFromAny(int64(99)); !ok || v != 99 {
		t.Errorf("intFromAny(int64(99)) = %d, want 99", v)
	}
	if v, ok := intFromAny(float64(7)); !ok || v != 7 {
		t.Errorf("intFromAny(float64(7)) = %d, want 7", v)
	}
	if v, ok := intFromAny("15"); !ok || v != 15 {
		t.Errorf("intFromAny("15") = %d, want 15", v)
	}
	if _, ok := intFromAny("abc"); ok {
		t.Error("intFromAny("abc") should return false")
	}
	if _, ok := intFromAny([]int{}); ok {
		t.Error("intFromAny(slice) should return false")
	}
}

func TestFloatFromAny(t *testing.T) {
	if v, ok := floatFromAny(42); !ok || v != 42 {
		t.Errorf("floatFromAny(42) = %v, want 42", v)
	}
	if v, ok := floatFromAny(3.14); !ok || v != 3.14 {
		t.Errorf("floatFromAny(3.14) = %v, want 3.14", v)
	}
	if v, ok := floatFromAny("2.5"); !ok || v != 2.5 {
		t.Errorf("floatFromAny("2.5") = %v, want 2.5", v)
	}
	if _, ok := floatFromAny("nope"); ok {
		t.Error("floatFromAny("nope") should return false")
	}
}

func TestStringFromAny(t *testing.T) {
	if v, ok := stringFromAny("hello"); !ok || v != "hello" {
		t.Errorf("stringFromAny(hello) = %q, want hello", v)
	}
	if _, ok := stringFromAny(42); ok {
		t.Error("stringFromAny(42) should return false")
	}
}

func TestCurveFromAny(t *testing.T) {
	valid := []any{[]any{40.0, 30.0}, []any{55.0, 40.0}, []any{70.0, 60.0}, []any{80.0, 85.0}, []any{90.0, 100.0}}
	curve, ok := curveFromAny(valid)
	if !ok {
		t.Fatal("curveFromAny returned false for valid input")
	}
	if len(curve) != 5 {
		t.Fatalf("curve len=%d, want 5", len(curve))
	}
	if curve[0].Temp() != 40 || curve[0].Speed() != 30 {
		t.Errorf("first point=%v, want [40,30]", curve[0])
	}
	// wrong length
	if _, ok := curveFromAny([]any{[]any{40.0, 30.0}}); ok {
		t.Error("curveFromAny should reject wrong length")
	}
	// not a slice
	if _, ok := curveFromAny("hello"); ok {
		t.Error("curveFromAny should reject non-slice")
	}
}

func TestLegacyCurveFromValues(t *testing.T) {
	values := map[string]any{"low_t": 40, "low_s": 30, "max_t": 90, "max_s": 100}
	curve, ok := legacyCurveFromValues(values)
	if !ok {
		t.Fatal("legacyCurveFromValues returned false")
	}
	if curve[0].Temp() != 40 || curve[4].Temp() != 90 {
		t.Errorf("curve endpoints=%v, want [40,90]", curve)
	}
	// missing key
	if _, ok := legacyCurveFromValues(map[string]any{"low_t": 40}); ok {
		t.Error("legacyCurveFromValues should fail with missing keys")
	}
}

func TestPresetsFromAny(t *testing.T) {
	raw := map[string]any{
		"silent": map[string]any{
			"curve":    []any{[]any{40.0, 15.0}, []any{55.0, 25.0}, []any{70.0, 40.0}, []any{80.0, 60.0}, []any{90.0, 85.0}},
			"strategy": "weighted",
		},
	}
	presets, ok := presetsFromAny(raw)
	if !ok {
		t.Fatal("presetsFromAny returned false")
	}
	slot, exists := presets["silent"]
	if !exists {
		t.Fatal("silent preset missing")
	}
	if slot.Strategy != "weighted" {
		t.Errorf("strategy=%q, want weighted", slot.Strategy)
	}
	// empty map
	if _, ok := presetsFromAny(map[string]any{}); ok {
		t.Error("presetsFromAny should return false for empty result")
	}
	// non-map
	if _, ok := presetsFromAny("hello"); ok {
		t.Error("presetsFromAny should reject non-map")
	}
}

func TestAddPresetRejectsEmptyKey(t *testing.T) {
	cfg := Normalize(Default())
	if AddPreset(&cfg, "", "test") {
		t.Error("AddPreset should reject empty key")
	}
}

func TestAddPresetMaxLimit(t *testing.T) {
	cfg := Normalize(Default())
	// Fill up to MaxPresets
	for i := 3; i < MaxPresets; i++ {
		key := "custom" + strconv.Itoa(i)
		if !AddPreset(&cfg, key, "slot") {
			t.Fatalf("AddPreset(%q) failed at count %d", key, i)
		}
	}
	if AddPreset(&cfg, "overflow", "nope") {
		t.Error("AddPreset should reject when MaxPresets reached")
	}
}

func TestAddPresetSanitizesLongLabel(t *testing.T) {
	cfg := Normalize(Default())
	longLabel := "超长名称测试超长名称测试超长名称测试超长名称测试超长名称测试超长名称测试"
	if !AddPreset(&cfg, "custom1", longLabel) {
		t.Fatal("AddPreset failed")
	}
	stored := cfg.Presets["custom1"].Label
	if len([]rune(stored)) > MaxPresetLabel {
		t.Errorf("label %d runes, max %d", len([]rune(stored)), MaxPresetLabel)
	}
}

func TestSavePresetUnknownKeyFails(t *testing.T) {
	cfg := Normalize(Default())
	if SavePreset(&cfg, "nonexistent") {
		t.Error("SavePreset should return false for unknown key")
	}
}

func TestDeletePresetUnknownKeyFails(t *testing.T) {
	cfg := Normalize(Default())
	if DeletePreset(&cfg, "nonexistent") {
		t.Error("DeletePreset should return false for unknown key")
	}
}

func TestNormalizeCurveWrongLengthFallsBack(t *testing.T) {
	tooShort := []controller.Point{{40, 30}, {55, 40}}
	result := normalizeCurve(tooShort)
	if len(result) != controller.CurvePoints {
		t.Fatalf("normalizeCurve returned %d points, want %d", len(result), controller.CurvePoints)
	}
	// Should be the default curve
	for i, p := range controller.DefaultCurve {
		if result[i] != p {
			t.Errorf("point[%d]=%v, want default %v", i, result[i], p)
		}
	}
}
