package sensors

import (
	"math"
	"testing"
	"time"

	"github.com/buglyz/ecc/internal/controller"
)

func TestSimulatedReaderReturnsTemps(t *testing.T) {
	r := NewSimulatedReader()
	defer r.Close()

	temps := r.Read()
	if temps.CPU == nil {
		t.Error("SimulatedReader.CPU is nil, want a value")
	}
	if temps.GPU == nil {
		t.Error("SimulatedReader.GPU is nil, want a value")
	}
}

func TestSimulatedReaderCPUInRange(t *testing.T) {
	r := NewSimulatedReader()
	defer r.Close()

	// CPU oscillates around 62 ± 12 → [50, 74]
	for i := 0; i < 20; i++ {
		temps := r.Read()
		if temps.CPU != nil {
			if *temps.CPU < 50 || *temps.CPU > 74 {
				t.Errorf("CPU=%v, expected range [50, 74]", *temps.CPU)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSimulatedReaderGPUInRange(t *testing.T) {
	r := NewSimulatedReader()
	defer r.Close()

	// GPU oscillates around 55 ± 10 → [45, 65]
	for i := 0; i < 20; i++ {
		temps := r.Read()
		if temps.GPU != nil {
			if *temps.GPU < 45 || *temps.GPU > 65 {
				t.Errorf("GPU=%v, expected range [45, 65]", *temps.GPU)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestSimulatedReaderTempsVaryOverTime(t *testing.T) {
	r := NewSimulatedReader()
	defer r.Close()

	first := r.Read()
	time.Sleep(500 * time.Millisecond)
	second := r.Read()

	if first.CPU == nil || second.CPU == nil {
		t.Fatal("CPU temps should not be nil")
	}
	// After 500ms the sinusoidal should produce a different reading.
	// Tolerance: exact equality is astronomically unlikely.
	if math.Abs(*first.CPU-*second.CPU) < 0.001 {
		t.Errorf("CPU temps unchanged after 500ms: first=%v second=%v", *first.CPU, *second.CPU)
	}
}

func TestSimulatedReaderImplementsSensorReader(t *testing.T) {
	// Compile-time interface check.
	var _ controller.SensorReader = NewSimulatedReader()
}

func TestSimulatedReaderCloseIsIdempotent(t *testing.T) {
	r := NewSimulatedReader()
	if err := r.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
