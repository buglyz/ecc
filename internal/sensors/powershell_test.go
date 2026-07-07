package sensors

import (
	"testing"
	"time"
)

func TestNoteStartFailureBacksOffExponentiallyAndCaps(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &PowerShellReader{now: func() time.Time { return base }}

	// 首次失败 -> startBackoffMin，随后每次翻倍，最终封顶 startBackoffMax 不再增长。
	want := []time.Duration{
		startBackoffMin,
		startBackoffMin * 2,
		startBackoffMin * 4,
		startBackoffMin * 8,
		startBackoffMin * 16,
		startBackoffMax, // 2s*32 = 64s 会被封顶到 60s
		startBackoffMax, // 保持封顶
	}
	for i, w := range want {
		r.noteStartFailureLocked()
		if r.backoff != w {
			t.Fatalf("第 %d 次失败后 backoff=%s, want %s", i+1, r.backoff, w)
		}
		if got := r.nextStart.Sub(base); got != w {
			t.Fatalf("第 %d 次失败后 nextStart 偏移=%s, want %s", i+1, got, w)
		}
	}
}

func TestClearBackoffResets(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &PowerShellReader{now: func() time.Time { return base }}
	r.noteStartFailureLocked()
	r.noteStartFailureLocked()
	if r.backoff == 0 || r.nextStart.IsZero() {
		t.Fatal("前置条件：退避应已被设置")
	}
	r.clearBackoff()
	if r.backoff != 0 {
		t.Fatalf("clearBackoff 后 backoff=%s, want 0", r.backoff)
	}
	if !r.nextStart.IsZero() {
		t.Fatalf("clearBackoff 后 nextStart=%v, want 零值", r.nextStart)
	}
}

// TestReadSkipsSpawnWithinBackoffWindow 验证退避窗内 Read 直接返回空温度、
// 不尝试启动子进程（cmd 保持 nil），这是防进程风暴的关键行为。
func TestReadSkipsSpawnWithinBackoffWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := &PowerShellReader{now: func() time.Time { return now }}
	// 模拟一次启动失败，进入退避窗。
	r.noteStartFailureLocked()

	// 仍在退避窗内（时钟未推进）：Read 应返回空且不 spawn。
	temps := r.Read()
	if temps.CPU != nil || temps.GPU != nil {
		t.Fatalf("退避窗内应返回空温度, got cpu=%v gpu=%v", temps.CPU, temps.GPU)
	}
	if r.cmd != nil {
		t.Fatal("退避窗内不应启动子进程 (cmd 应保持 nil)")
	}
}
