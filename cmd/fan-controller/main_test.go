package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buglyz/ecc/internal/paths"
)

func TestRequiredRuntimeFilesDryRunDoesNotRequireECProbe(t *testing.T) {
	dir := t.TempDir()
	dll := filepath.Join(dir, "LibreHardwareMonitorLib.dll")
	if err := os.WriteFile(dll, []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := requiredRuntimeFiles(paths.Paths{ECProbe: filepath.Join(dir, "missing-ec-probe.exe"), HardwareDLL: dll}, true, false)
	if len(missing) != 0 {
		t.Fatalf("missing=%v, want none", missing)
	}
}

func TestRequiredRuntimeFilesProductionRequiresBothFiles(t *testing.T) {
	dir := t.TempDir()
	missing := requiredRuntimeFiles(paths.Paths{ECProbe: filepath.Join(dir, "missing-ec-probe.exe"), HardwareDLL: filepath.Join(dir, "missing.dll")}, false, false)
	if len(missing) != 2 {
		t.Fatalf("missing=%v, want both runtime files", missing)
	}
}

func TestPollIntervalFromMilliseconds(t *testing.T) {
	got, err := pollIntervalFromMilliseconds(250)
	if err != nil {
		t.Fatalf("pollIntervalFromMilliseconds: %v", err)
	}
	if got != 250*time.Millisecond {
		t.Fatalf("interval=%v, want 250ms", got)
	}
}

func TestPollIntervalFromMillisecondsRejectsInvalidValues(t *testing.T) {
	for _, value := range []int{0, -1} {
		if _, err := pollIntervalFromMilliseconds(value); err == nil {
			t.Errorf("pollIntervalFromMilliseconds(%d) error=nil, want error", value)
		}
	}
	const maxDuration = time.Duration(1<<63 - 1)
	tooLarge := int(maxDuration/time.Millisecond) + 1
	if _, err := pollIntervalFromMilliseconds(tooLarge); err == nil {
		t.Errorf("pollIntervalFromMilliseconds(%d) error=nil, want error", tooLarge)
	}
}
