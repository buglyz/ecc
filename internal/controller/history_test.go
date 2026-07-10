package controller

import (
	"testing"
	"time"
)

func TestHistoryRingBufferWraps(t *testing.T) {
	h := NewHistory(3)
	for i := 0; i < 5; i++ {
		h.Add(HistorySample{Speed: i})
	}
	got := h.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3 (capped)", len(got))
	}
	// Oldest two (0,1) evicted; remaining must be 2,3,4 in order.
	want := []int{2, 3, 4}
	for i, sample := range got {
		if sample.Speed != want[i] {
			t.Fatalf("snapshot[%d].Speed=%d, want %d", i, sample.Speed, want[i])
		}
	}
}

func TestHistorySnapshotBeforeFull(t *testing.T) {
	h := NewHistory(10)
	now := time.Now()
	h.Add(HistorySample{Time: now, Speed: 1})
	h.Add(HistorySample{Time: now.Add(time.Second), Speed: 2})
	got := h.Snapshot()
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Speed != 1 || got[1].Speed != 2 {
		t.Fatalf("order wrong: %d,%d want 1,2", got[0].Speed, got[1].Speed)
	}
}

func TestHistorySnapshotIsCopy(t *testing.T) {
	h := NewHistory(4)
	h.Add(HistorySample{Speed: 1})
	got := h.Snapshot()
	got[0].Speed = 999
	if h.Snapshot()[0].Speed != 1 {
		t.Fatal("Snapshot returned a slice aliasing internal storage")
	}
}

func TestSnapshotSinceCutoffInMiddle(t *testing.T) {
	h := NewHistory(10)
	base := time.Now()
	for i := 0; i < 6; i++ {
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Minute), Speed: i})
	}
	// Cutoff at minute 3 should keep samples 3,4,5 (Time >= cutoff), ascending.
	got := h.SnapshotSince(base.Add(3 * time.Minute))
	want := []int{3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d (%v)", len(got), len(want), got)
	}
	for i, sample := range got {
		if sample.Speed != want[i] {
			t.Fatalf("got[%d].Speed=%d, want %d", i, sample.Speed, want[i])
		}
	}
}

func TestSnapshotSinceAllocatesOnlyMatchingWindow(t *testing.T) {
	h := NewHistory(10)
	base := time.Now()
	for i := 0; i < 6; i++ {
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Minute), Speed: i})
	}

	got := h.SnapshotSince(base.Add(3 * time.Minute))
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	if cap(got) != len(got) {
		t.Fatalf("cap=%d, want %d matching samples", cap(got), len(got))
	}
}

func TestSnapshotSinceCutoffBeforeAll(t *testing.T) {
	h := NewHistory(10)
	base := time.Now()
	for i := 0; i < 4; i++ {
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Second), Speed: i})
	}
	got := h.SnapshotSince(base.Add(-time.Hour))
	if len(got) != 4 || got[0].Speed != 0 || got[3].Speed != 3 {
		t.Fatalf("got=%v, want all four ascending", got)
	}
}

func TestSnapshotSinceZeroCutoffReturnsAll(t *testing.T) {
	h := NewHistory(5)
	base := time.Now()
	for i := 0; i < 3; i++ {
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Second), Speed: i})
	}
	got := h.SnapshotSince(time.Time{})
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3 for zero cutoff", len(got))
	}
}

func TestSnapshotSinceAcrossWrap(t *testing.T) {
	h := NewHistory(3)
	base := time.Now()
	// Overfill so the ring wraps; only the last 3 survive (2,3,4).
	for i := 0; i < 5; i++ {
		h.Add(HistorySample{Time: base.Add(time.Duration(i) * time.Minute), Speed: i})
	}
	got := h.SnapshotSince(base.Add(3 * time.Minute))
	want := []int{3, 4}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d (%v)", len(got), len(want), got)
	}
	for i, sample := range got {
		if sample.Speed != want[i] {
			t.Fatalf("got[%d].Speed=%d, want %d", i, sample.Speed, want[i])
		}
	}
}

func TestSnapshotSinceIsCopy(t *testing.T) {
	h := NewHistory(4)
	now := time.Now()
	h.Add(HistorySample{Time: now, Speed: 1})
	got := h.SnapshotSince(time.Time{})
	got[0].Speed = 999
	if h.SnapshotSince(time.Time{})[0].Speed != 1 {
		t.Fatal("SnapshotSince returned a slice aliasing internal storage")
	}
}
