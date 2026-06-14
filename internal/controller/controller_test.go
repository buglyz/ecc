package controller

import (
	"context"
	"log"
	"math"
	"testing"
	"time"
)

type testReader struct{}

func (testReader) Read() Temps  { return Temps{} }
func (testReader) Close() error { return nil }

type recordingWriter struct {
	ok     bool
	writes [][2]string
}

func (w *recordingWriter) Write(_ context.Context, register string, valueHex string) bool {
	w.writes = append(w.writes, [2]string{register, valueHex})
	return w.ok
}

func TestInitialSpeedUsesDefaultStartupSpeedInAuto(t *testing.T) {
	state := modeState{curve: DefaultCurve, strategy: "weighted"}
	if got := initialSpeed(state); got != int(DefaultCurve[0].Speed()) {
		t.Fatalf("initialSpeed=%d, want %d", got, int(DefaultCurve[0].Speed()))
	}
}

func TestInitialSpeedHonorsManualMode(t *testing.T) {
	speed := 87
	state := modeState{manualSpeed: &speed, curve: DefaultCurve, strategy: "weighted"}
	if got := initialSpeed(state); got != speed {
		t.Fatalf("initialSpeed=%d, want manual %d", got, speed)
	}
}

func TestWriteSpeedRequiresBothRegisters(t *testing.T) {
	writer := &recordingWriter{ok: false}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, log.Default())
	if fan.writeSpeed(42) {
		t.Fatal("writeSpeed returned success when writer failed")
	}
	if len(writer.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(writer.writes))
	}
}

// --- Additional coverage ---

func TestToHex(t *testing.T) {
	tests := []struct {
		value int
		want  string
	}{
		{0, "0x0"},
		{1, "0x1"},
		{10, "0xa"},
		{16, "0x10"},
		{100, "0x64"},
		{255, "0xff"},
	}
	for _, tt := range tests {
		if got := toHex(tt.value); got != tt.want {
			t.Errorf("toHex(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestAverage(t *testing.T) {
	if got := average([]float64{10, 20, 30}); got != 20 {
		t.Errorf("average([10,20,30]) = %v, want 20", got)
	}
	if got := average([]float64{42}); got != 42 {
		t.Errorf("average([42]) = %v, want 42", got)
	}
	if got := average([]float64{0, 0, 0}); got != 0 {
		t.Errorf("average([0,0,0]) = %v, want 0", got)
	}
}

func TestAbsDuration(t *testing.T) {
	if got := absDuration(-5 * time.Second); got != 5*time.Second {
		t.Errorf("absDuration(-5s) = %v, want 5s", got)
	}
	if got := absDuration(5 * time.Second); got != 5*time.Second {
		t.Errorf("absDuration(5s) = %v, want 5s", got)
	}
	if got := absDuration(0); got != time.Duration(0) {
		t.Errorf("absDuration(0) = %v, want 0", got)
	}
}

func TestModeName(t *testing.T) {
	speed := 50
	if got := modeName(modeState{manualSpeed: &speed}); got != "manual" {
		t.Errorf("modeName(manual) = %q, want %q", got, "manual")
	}
	if got := modeName(modeState{strategy: "max"}); got != "auto:max" {
		t.Errorf("modeName(auto) = %q, want %q", got, "auto:max")
	}
}

func TestFormatTemp(t *testing.T) {
	if got := formatTemp(nil); got != nil {
		t.Errorf("formatTemp(nil) = %v, want nil", got)
	}
	v := 65.666
	if got := formatTemp(&v); got != 65.7 {
		t.Errorf("formatTemp(65.666) = %v, want 65.7", got)
	}
}

func TestCombineTempsBothNil(t *testing.T) {
	if got := CombineTemps("weighted", nil, nil); got != nil {
		t.Errorf("CombineTemps(nil,nil) = %v, want nil", got)
	}
}

func TestCombineTempsStrategyFallbacks(t *testing.T) {
	cpu, gpu := 60.0, 70.0
	// cpu strategy returns cpu
	if got := CombineTemps("cpu", &cpu, &gpu); got == nil || *got != 60 {
		t.Errorf("CombineTemps(cpu) = %v, want 60", got)
	}
	// gpu strategy returns gpu
	if got := CombineTemps("gpu", &cpu, &gpu); got == nil || *got != 70 {
		t.Errorf("CombineTemps(gpu) = %v, want 70", got)
	}
	// max strategy
	if got := CombineTemps("max", &cpu, &gpu); got == nil || *got != 70 {
		t.Errorf("CombineTemps(max) = %v, want 70", got)
	}
	// weighted: 0.7*60 + 0.3*70 = 63
	got := CombineTemps("weighted", &cpu, &gpu)
	if got == nil || math.Abs(*got-63) > 0.001 {
		t.Errorf("CombineTemps(weighted) = %v, want 63", got)
	}
}

func TestCombineTempsNilFallbacks(t *testing.T) {
	gpu := 70.0
	if got := CombineTemps("cpu", nil, &gpu); got == nil || *got != 70 {
		t.Errorf("CombineTemps(cpu,nil,70) = %v, want 70", got)
	}
	cpu := 60.0
	if got := CombineTemps("gpu", &cpu, nil); got == nil || *got != 60 {
		t.Errorf("CombineTemps(gpu,60,nil) = %v, want 60", got)
	}
}

func TestInterpolateCurveEdgeCases(t *testing.T) {
	if got := InterpolateCurve(nil, 50); got != 0 {
		t.Errorf("InterpolateCurve(nil) = %v, want 0", got)
	}
	curve := []Point{{50, 60}}
	if got := InterpolateCurve(curve, 30); got != 60 {
		t.Errorf("InterpolateCurve(single,below) = %v, want 60", got)
	}
	if got := InterpolateCurve(curve, 70); got != 60 {
		t.Errorf("InterpolateCurve(single,above) = %v, want 60", got)
	}
	two := []Point{{40, 30}, {60, 50}}
	if got := InterpolateCurve(two, 30); got != 30 {
		t.Errorf("InterpolateCurve(clamp-low) = %v, want 30", got)
	}
	if got := InterpolateCurve(two, 70); got != 50 {
		t.Errorf("InterpolateCurve(clamp-high) = %v, want 50", got)
	}
	if got := InterpolateCurve(two, 50); got != 40 {
		t.Errorf("InterpolateCurve(midpoint) = %v, want 40", got)
	}
}

func TestClampSpeed(t *testing.T) {
	if got := ClampSpeed(-10); got != 0 {
		t.Errorf("ClampSpeed(-10) = %d, want 0", got)
	}
	if got := ClampSpeed(150); got != 100 {
		t.Errorf("ClampSpeed(150) = %d, want 100", got)
	}
	if got := ClampSpeed(50.7); got != 51 {
		t.Errorf("ClampSpeed(50.7) = %d, want 51", got)
	}
}

func TestValidStrategy(t *testing.T) {
	for _, s := range []string{"weighted", "max", "cpu", "gpu"} {
		if !ValidStrategy(s) {
			t.Errorf("ValidStrategy(%q) = false, want true", s)
		}
	}
	if ValidStrategy("invalid") {
		t.Error("ValidStrategy(invalid) = true, want false")
	}
}

func TestSetManualClampsRange(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())

	v := -5
	fan.SetManual(&v)
	if got := *fan.currentMode().manualSpeed; got != 0 {
		t.Errorf("SetManual(-5) = %d, want 0", got)
	}

	v2 := 150
	fan.SetManual(&v2)
	if got := *fan.currentMode().manualSpeed; got != 100 {
		t.Errorf("SetManual(150) = %d, want 100", got)
	}
}

func TestSetManualNilDisables(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())

	speed := 50
	fan.SetManual(&speed)
	fan.SetManual(nil)
	if fan.currentMode().manualSpeed != nil {
		t.Error("SetManual(nil) should clear manual speed")
	}
}

func TestSetCurveUpdatesVersion(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())

	v0 := fan.currentMode().version
	fan.SetCurve([]Point{{40, 20}, {60, 80}})
	v1 := fan.currentMode().version
	if v1 <= v0 {
		t.Errorf("SetCurve did not increment version: %d -> %d", v0, v1)
	}
}

func TestSetStrategyUpdatesVersion(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())

	v0 := fan.currentMode().version
	fan.SetStrategy("max")
	v1 := fan.currentMode().version
	if v1 <= v0 {
		t.Errorf("SetStrategy did not increment version: %d -> %d", v0, v1)
	}
}

func TestLatestReflectsSetLatest(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())

	cpu, gpu, target := 65.0, 55.0, 63.0
	now := time.Now()
	fan.setLatest(&cpu, &gpu, &target, 42, "auto:weighted", now)

	latest := fan.Latest()
	if latest.CPU == nil || *latest.CPU != 65 {
		t.Errorf("Latest.CPU = %v, want 65", latest.CPU)
	}
	if latest.GPU == nil || *latest.GPU != 55 {
		t.Errorf("Latest.GPU = %v, want 55", latest.GPU)
	}
	if latest.Speed == nil || *latest.Speed != 42 {
		t.Errorf("Latest.Speed = %v, want 42", latest.Speed)
	}
	if latest.Mode != "auto:weighted" {
		t.Errorf("Latest.Mode = %q, want auto:weighted", latest.Mode)
	}
}

func TestControllerStartStop(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())
	fan.Start()
	fan.Stop()
	// Should not panic; done channel should be closed.
}

func TestWriteSpeedSuccessRecordsBothRegisters(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())
	if !fan.writeSpeed(50) {
		t.Fatal("writeSpeed(50) should succeed")
	}
	if len(w.writes) != 2 {
		t.Fatalf("writes=%d, want 2", len(w.writes))
	}
	if w.writes[0][0] != ECRegFan1 || w.writes[1][0] != ECRegFan2 {
		t.Errorf("registers=%v, want [%s,%s]", [][2]string(w.writes), ECRegFan1, ECRegFan2)
	}
}
