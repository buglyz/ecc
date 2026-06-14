package controller

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPointUnmarshalJSONArray(t *testing.T) {
	var p Point
	if err := json.Unmarshal([]byte(`[55, 40]`), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if p.Temp() != 55 || p.Speed() != 40 {
		t.Errorf("point=%v, want [55,40]", p)
	}
}

func TestPointUnmarshalJSONStringNumbers(t *testing.T) {
	// Some legacy configs store numbers as strings
	var p Point
	if err := json.Unmarshal([]byte(`["55", "40"]`), &p); err != nil {
		t.Fatalf("Unmarshal string numbers: %v", err)
	}
	if p.Temp() != 55 || p.Speed() != 40 {
		t.Errorf("point=%v, want [55,40]", p)
	}
}

func TestPointUnmarshalJSONRejectsWrongArity(t *testing.T) {
	var p Point
	if err := json.Unmarshal([]byte(`[40]`), &p); err == nil {
		t.Error("expected error for single-element array")
	}
	if err := json.Unmarshal([]byte(`[40, 30, 20]`), &p); err == nil {
		t.Error("expected error for triple-element array")
	}
}

func TestPointUnmarshalJSONRejectsGarbage(t *testing.T) {
	var p Point
	if err := json.Unmarshal([]byte(`"not an array"`), &p); err == nil {
		t.Error("expected error for non-array")
	}
}

func TestHistorySnapshotSince(t *testing.T) {
	h := NewHistory(100)
	base := time.Now()

	for i := 0; i < 5; i++ {
		cpu := 50.0 + float64(i)
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Minute), CPU: &cpu, Speed: 30 + i})
	}

	// Get samples since 2 minutes after base (should return 3 samples: i=2,3,4)
	cutoff := base.Add(2 * time.Minute)
	samples := h.SnapshotSince(cutoff)
	if len(samples) != 3 {
		t.Fatalf("SnapshotSince count=%d, want 3", len(samples))
	}
	// Should be in chronological order
	if samples[0].Time.After(samples[1].Time) {
		t.Error("SnapshotSince not in chronological order")
	}
	if samples[0].CPU == nil || *samples[0].CPU != 52 {
		t.Errorf("first sample CPU=%v, want 52", samples[0].CPU)
	}
}

func TestHistorySnapshotSinceZeroCutoff(t *testing.T) {
	h := NewHistory(100)
	cpu := 50.0
	h.Add(HistorySample{Time: time.Now(), CPU: &cpu, Speed: 30})

	// Zero cutoff returns all samples
	samples := h.SnapshotSince(time.Time{})
	if len(samples) != 1 {
		t.Fatalf("SnapshotSince(zero) count=%d, want 1", len(samples))
	}
}

func TestHistorySnapshotSinceNoMatch(t *testing.T) {
	h := NewHistory(100)
	cpu := 50.0
	h.Add(HistorySample{Time: time.Now(), CPU: &cpu, Speed: 30})

	// Cutoff in the future: no samples match
	samples := h.SnapshotSince(time.Now().Add(time.Hour))
	if len(samples) != 0 {
		t.Fatalf("SnapshotSince(future) count=%d, want 0", len(samples))
	}
}

func TestHistoryWraparound(t *testing.T) {
	h := NewHistory(5)
	base := time.Now()

	// Write 8 samples into a buffer of size 5
	for i := 0; i < 8; i++ {
		cpu := float64(i)
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Second), CPU: &cpu, Speed: i})
	}

	// Should only have last 5
	snap := h.Snapshot()
	if len(snap) != 5 {
		t.Fatalf("Snapshot count=%d, want 5", len(snap))
	}
	// Oldest should be i=3, newest i=7
	if snap[0].CPU == nil || *snap[0].CPU != 3 {
		t.Errorf("oldest=%v, want 3", snap[0].CPU)
	}
	if snap[4].CPU == nil || *snap[4].CPU != 7 {
		t.Errorf("newest=%v, want 7", snap[4].CPU)
	}
}

func TestNumberFromJSON(t *testing.T) {
	if v, err := numberFromJSON([]byte(`42`)); err != nil || v != 42 {
		t.Errorf("numberFromJSON(42) = %v %v, want 42 nil", v, err)
	}
	if v, err := numberFromJSON([]byte(`"42"`)); err != nil || v != 42 {
		t.Errorf("numberFromJSON(\"42\") = %v %v, want 42 nil", v, err)
	}
	if _, err := numberFromJSON([]byte(`"abc"`)); err == nil {
		t.Error("numberFromJSON(\"abc\") should fail")
	}
	if _, err := numberFromJSON([]byte(`{}`)); err == nil {
		t.Error("numberFromJSON({}) should fail")
	}
}
