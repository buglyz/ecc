package controller

import "testing"

func TestTargetControlStep(t *testing.T) {
	cfg := DefaultTargetConfig() // 66C, 4C deadband, KP 5, bias 55, 40-100, +30/-10
	cases := []struct {
		name    string
		current int
		temp    float64
		applied bool
		want    int
	}{
		{"deadband holds", 60, 67, true, 60},
		{"raises with rate limit", 60, 80, true, 90},
		{"lowers with rate limit", 80, 50, true, 70},
		{"initial starts at bias", 0, 71, false, 80},
		{"max clamp", 95, 90, true, 100},
		{"min clamp", 45, 55, true, 45},
	}
	for _, tc := range cases {
		if got := targetControlStep(tc.current, tc.temp, cfg, tc.applied); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeTargetConfig(t *testing.T) {
	def := NormalizeTargetConfig(TargetConfig{})
	if def != DefaultTargetConfig() {
		t.Fatalf("zero config = %+v, want defaults %+v", def, DefaultTargetConfig())
	}

	got := NormalizeTargetConfig(TargetConfig{
		Temp:       200,
		Hysteresis: 0.1,
		KP:         100,
		Bias:       -5,
		Min:        120,
		Max:        10,
		UpStep:     0,
		DownStep:   500,
	})
	if got.Temp != 95 {
		t.Errorf("Temp=%v, want 95", got.Temp)
	}
	if got.Hysteresis != 1 {
		t.Errorf("Hysteresis=%v, want 1", got.Hysteresis)
	}
	if got.KP != 20 {
		t.Errorf("KP=%v, want 20", got.KP)
	}
	if got.Bias != 0 {
		t.Errorf("Bias=%d, want 0", got.Bias)
	}
	if got.Min != 100 || got.Max != 100 {
		t.Errorf("range=%d-%d, want 100-100", got.Min, got.Max)
	}
	if got.UpStep != DefaultTargetUpStep || got.DownStep != 100 {
		t.Errorf("steps=+%d/-%d, want +%d/-100", got.UpStep, got.DownStep, DefaultTargetUpStep)
	}
}

func TestModeNameTarget(t *testing.T) {
	if got := modeName(modeState{controlMode: ControlModeTarget}); got != "target" {
		t.Fatalf("modeName(target)=%q, want target", got)
	}
	if got := modeName(modeState{controlMode: ControlModeCurve, strategy: DefaultStrategy}); got != "auto:"+DefaultStrategy {
		t.Fatalf("modeName(curve)=%q", got)
	}
}
