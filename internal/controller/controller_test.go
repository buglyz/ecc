package controller

import (
	"context"
	"log"
	"testing"
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
