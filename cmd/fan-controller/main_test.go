package main

import (
	"os"
	"path/filepath"
	"testing"

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
