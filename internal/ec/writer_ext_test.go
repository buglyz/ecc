package ec

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteDryRunLogsButDoesNotExecute(t *testing.T) {
	w := Writer{ProbePath: "nonexistent-ec-probe.exe", DryRun: true, Logger: log.Default()}
	if !w.Write(context.Background(), "0x2C", "0x32") {
		t.Fatal("dry-run should always succeed")
	}
}

func TestWriteFailsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := Writer{ProbePath: "nonexistent.exe", DryRun: false, Logger: log.Default()}
	if w.Write(ctx, "0x2C", "0x32") {
		t.Fatal("should fail with cancelled context")
	}
}

func TestWriteContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // ensure deadline passed
	w := Writer{ProbePath: "nonexistent.exe", DryRun: false, Logger: log.Default()}
	if w.Write(ctx, "0x2C", "0x32") {
		t.Fatal("should fail with expired context")
	}
}

func TestWriteSkipsWhenProbeMissing(t *testing.T) {
	w := Writer{ProbePath: "does-not-exist-at-all.exe", DryRun: false, Logger: log.Default()}
	if w.Write(context.Background(), "0x2C", "0x32") {
		t.Fatal("should fail when probe is missing")
	}
}

func TestWriteNilLoggerDoesNotPanic(t *testing.T) {
	w := Writer{ProbePath: "nonexistent.exe", DryRun: true}
	if !w.Write(context.Background(), "0x2C", "0x32") {
		t.Fatal("dry-run with nil logger should succeed")
	}
}

func TestWriteNilContextDoesNotPanic(t *testing.T) {
	w := Writer{ProbePath: "nonexistent.exe", DryRun: true, Logger: log.Default()}
	if !w.Write(nil, "0x2C", "0x32") {
		t.Fatal("dry-run with nil context should succeed")
	}
}

func TestWriteWithExistingFileButDryRun(t *testing.T) {
	// Even if ec-probe.exe exists, dry-run should not call it
	dir := t.TempDir()
	fakeProbe := filepath.Join(dir, "ec-probe.exe")
	if err := os.WriteFile(fakeProbe, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := Writer{ProbePath: fakeProbe, DryRun: true, Logger: log.Default()}
	if !w.Write(context.Background(), "0x2C", "0x32") {
		t.Fatal("dry-run should succeed even with existing probe")
	}
}
