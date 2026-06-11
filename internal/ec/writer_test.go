package ec

import (
	"context"
	"log"
	"testing"
)

func TestWriteDryRunSucceedsWithoutProbe(t *testing.T) {
	w := Writer{ProbePath: "does-not-exist.exe", DryRun: true, Logger: log.Default()}
	if !w.Write(context.Background(), "0x2C", "0x32") {
		t.Fatal("dry-run write should report success without touching hardware")
	}
}

func TestWriteFailsWhenContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := Writer{ProbePath: "does-not-exist.exe", DryRun: true, Logger: log.Default()}
	if w.Write(ctx, "0x2C", "0x32") {
		t.Fatal("write should fail when context is already cancelled")
	}
}

func TestWriteFailsWhenProbeMissing(t *testing.T) {
	w := Writer{ProbePath: "does-not-exist.exe", DryRun: false, Logger: log.Default()}
	if w.Write(context.Background(), "0x2C", "0x32") {
		t.Fatal("write should fail when ec-probe.exe is missing")
	}
}

func TestWriteToleratesNilContextAndLogger(t *testing.T) {
	w := Writer{ProbePath: "does-not-exist.exe", DryRun: true}
	if !w.Write(nil, "0x2D", "0xFF") {
		t.Fatal("dry-run write should succeed with nil context and logger")
	}
}
