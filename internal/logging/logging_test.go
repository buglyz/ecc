package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	logger, cleanup := New(logPath)
	defer cleanup()

	logger.Print("hello test")

	// Verify file was created
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file not created: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log: %v", err)
	}
	if len(data) == 0 {
		t.Error("log file is empty")
	}
}

func TestNewFallsBackOnBadDir(t *testing.T) {
	// A path that can't be created as a directory
	logPath := "/dev/null/impossible/test.log"
	logger, cleanup := New(logPath)
	defer cleanup()

	// Should not panic, returns a fallback stderr logger
	if logger == nil {
		t.Fatal("logger should not be nil even on bad dir")
	}
}

func TestRotateSkipsSmallFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "small.log")
	// Write a small file (under maxLogBytes)
	if err := os.WriteFile(logPath, []byte("tiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotate(logPath)

	// No .1 backup should be created for small files
	if _, err := os.Stat(logPath + ".1"); err == nil {
		t.Error("rotate should not create backup for small log file")
	}
}

func TestRotateCreatesBackupsForLargeFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "large.log")

	// Create a file larger than maxLogBytes
	bigData := make([]byte, maxLogBytes+1)
	if err := os.WriteFile(logPath, bigData, 0o644); err != nil {
		t.Fatal(err)
	}
	rotate(logPath)

	// Current file should have been renamed to .1
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("expected .1 backup, got: %v", err)
	}
}

func TestRotateShiftsExistingBackups(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "shift.log")

	// Create .1 backup
	if err := os.WriteFile(logPath+".1", []byte("old-backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create large current file
	bigData := make([]byte, maxLogBytes+1)
	if err := os.WriteFile(logPath, bigData, 0o644); err != nil {
		t.Fatal(err)
	}
	rotate(logPath)

	// .1 should be moved to .2
	if _, err := os.Stat(logPath + ".2"); err != nil {
		t.Fatalf("expected .2 backup from shifted .1, got: %v", err)
	}
	// .1 should now contain the old current file
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("expected .1 backup, got: %v", err)
	}
}
