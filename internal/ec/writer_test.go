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

// TestWriteSucceededDetectsDriverFailure 锁定关键判读逻辑：ec-probe write 在
// WinRing0 驱动加载失败时仍返回 exit 0，必须靠扫描输出识别，否则会把未生效的
// 写入误判为成功。样本取自真实 ec-probe 输出（管理员成功 / 非管理员驱动失败）。
func TestWriteSucceededDetectsDriverFailure(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"admin write applied", "Writing at 44: 50 (0x32)\nCurrent value at 44: 50 (0x32)\n", true},
		{"driver load failure", "Error: Unable to load the WinRing0 driver!\n", false},
		{"driver failure case insensitive", "unable to load the winring0 driver", false},
		{"empty output", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := writeSucceeded(tc.output); got != tc.want {
				t.Fatalf("writeSucceeded(%q)=%t, want %t", tc.output, got, tc.want)
			}
		})
	}
}
