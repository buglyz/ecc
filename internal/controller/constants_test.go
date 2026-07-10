package controller

import (
	"testing"
	"time"
)

func TestExpectedCycleDurationUsesConfiguredPollInterval(t *testing.T) {
	got := ExpectedCycleDuration(250 * time.Millisecond)
	want := 6*250*time.Millisecond + ExpectedCycleJitter
	if got != want {
		t.Fatalf("ExpectedCycleDuration(250ms)=%v, want %v", got, want)
	}
}
