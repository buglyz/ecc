package sensors

import "testing"

func TestSimulatedReaderProducesPlausibleTemps(t *testing.T) {
	reader := NewSimulatedReader()
	temps := reader.Read()
	if temps.CPU == nil || temps.GPU == nil {
		t.Fatal("simulated reader returned nil temperatures")
	}
	// Sine/cosine ranges: CPU 62±12 -> [50,74], GPU 55±10 -> [45,65].
	if *temps.CPU < 50 || *temps.CPU > 74 {
		t.Errorf("cpu=%.1f, want within [50,74]", *temps.CPU)
	}
	if *temps.GPU < 45 || *temps.GPU > 65 {
		t.Errorf("gpu=%.1f, want within [45,65]", *temps.GPU)
	}
	if err := reader.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
