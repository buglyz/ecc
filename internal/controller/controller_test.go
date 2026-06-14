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

// --- Additional coverage (not duplicated in logic_test.go or loop_test.go) ---

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

func TestCombineTempsWeightedFormula(t *testing.T) {
	cpu, gpu := 60.0, 70.0
	// weighted: (cpu-gpu)*CPUWeight + gpu = -10*0.7 + 70 = 63
	got := CombineTemps("weighted", &cpu, &gpu)
	if got == nil || math.Abs(*got-63) > 0.001 {
		t.Errorf("CombineTemps(weighted) = %v, want 63", got)
	}
}

func TestCombineTempsBothNil(t *testing.T) {
	if got := CombineTemps("weighted", nil, nil); got != nil {
		t.Errorf("CombineTemps(nil,nil) = %v, want nil", got)
	}
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

func TestControllerStartStop(t *testing.T) {
	w := &recordingWriter{ok: true}
	fan := NewFanController(testReader{}, w, DefaultCurve, DefaultStrategy, log.Default())
	fan.Start()
	fan.Stop()
	// Should not panic; done channel should be closed.
}
