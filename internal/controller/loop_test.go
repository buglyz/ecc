package controller

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"
)

// syncWriter is a thread-safe FanWriter that records every register write.
// The control loop runs in its own goroutine, so recording must be guarded.
type syncWriter struct {
	mu     sync.Mutex
	ok     bool
	writes [][2]string
}

func (w *syncWriter) Write(_ context.Context, register, valueHex string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, [2]string{register, valueHex})
	return w.ok
}

func (w *syncWriter) snapshot() [][2]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([][2]string, len(w.writes))
	copy(out, w.writes)
	return out
}

func (w *syncWriter) last(register string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := len(w.writes) - 1; i >= 0; i-- {
		if w.writes[i][0] == register {
			return w.writes[i][1], true
		}
	}
	return "", false
}

func TestStopReleasesBothFans(t *testing.T) {
	writer := &syncWriter{ok: true}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, 0, log.New(testWriter{t}, "", 0))
	fan.Start()
	// Stop before the initial 1s write so the only writes are the release writes.
	fan.Stop()

	v1, ok1 := writer.last(ECRegFan1)
	v2, ok2 := writer.last(ECRegFan2)
	if !ok1 || !ok2 {
		t.Fatalf("expected release writes to both fans, fan1=%t fan2=%t", ok1, ok2)
	}
	if v1 != ECFanRelease || v2 != ECFanRelease {
		t.Fatalf("release values fan1=%q fan2=%q, want %q", v1, v2, ECFanRelease)
	}
}

func TestInitialSpeedWrittenAfterStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-dependent control loop test in -short mode")
	}
	writer := &syncWriter{ok: true}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, 0, log.New(testWriter{t}, "", 0))
	fan.Start()
	// The loop sleeps 1s before writing the initial speed.
	time.Sleep(1500 * time.Millisecond)
	fan.Stop()

	want := toHex(int(DefaultCurve[0].Speed()))
	writes := writer.snapshot()
	var sawFan1, sawFan2 bool
	for _, write := range writes {
		if write[0] == ECRegFan1 && write[1] == want {
			sawFan1 = true
		}
		if write[0] == ECRegFan2 && write[1] == want {
			sawFan2 = true
		}
	}
	if !sawFan1 || !sawFan2 {
		t.Fatalf("expected initial speed %q written to both fans, fan1=%t fan2=%t, writes=%v", want, sawFan1, sawFan2, writes)
	}
}

func TestLatestReflectsManualMode(t *testing.T) {
	writer := &syncWriter{ok: true}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, 0, log.New(testWriter{t}, "", 0))
	speed := 73
	fan.SetManual(&speed)
	if got := fan.currentMode(); got.manualSpeed == nil || *got.manualSpeed != 73 {
		t.Fatalf("manual speed=%v, want 73", got.manualSpeed)
	}
	fan.SetManual(nil)
	if got := fan.currentMode(); got.manualSpeed != nil {
		t.Fatalf("manual speed=%v, want nil after clearing", got.manualSpeed)
	}
}

func TestSetManualClampsRange(t *testing.T) {
	writer := &syncWriter{ok: true}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, 0, log.New(testWriter{t}, "", 0))
	over := 250
	fan.SetManual(&over)
	if got := *fan.currentMode().manualSpeed; got != 100 {
		t.Fatalf("clamped manual speed=%d, want 100", got)
	}
	under := -10
	fan.SetManual(&under)
	if got := *fan.currentMode().manualSpeed; got != 0 {
		t.Fatalf("clamped manual speed=%d, want 0", got)
	}
}

func TestSetCurveAndStrategyBumpVersion(t *testing.T) {
	writer := &syncWriter{ok: true}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, 0, log.New(testWriter{t}, "", 0))
	v0 := fan.currentMode().version
	fan.SetCurve([]Point{{30, 10}, {50, 30}, {70, 50}, {80, 70}, {90, 90}})
	v1 := fan.currentMode().version
	fan.SetStrategy("max")
	v2 := fan.currentMode().version
	if !(v1 > v0 && v2 > v1) {
		t.Fatalf("versions did not increase: v0=%d v1=%d v2=%d", v0, v1, v2)
	}
	if fan.currentMode().strategy != "max" {
		t.Fatalf("strategy=%q, want max", fan.currentMode().strategy)
	}
}

// testWriter routes logger output to the test log so failures stay readable.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

// TestConfigChangeForcesFanWrite 验证配置变化（曲线/策略）后即使目标转速不变也会立即写入。
// 回归测试：修复前依赖 30 秒心跳兜底，用户改完配置可能要等最多 30 秒才生效。
func TestConfigChangeForcesFanWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-dependent control loop test in -short mode")
	}

	// 构造一个特殊场景：两条不同曲线在某温度点插值出相同转速
	curve1 := []Point{{40, 50}, {60, 50}, {80, 80}} // 40-60°C 平坦 50%
	curve2 := []Point{{40, 50}, {50, 50}, {80, 90}} // 40-50°C 平坦 50%（不同曲线但该区间转速相同）

	// 构造恒定返回 45°C 的 reader，两条曲线插值都是 50%
	fixedReader := &fixedTempReader{cpu: 45.0, gpu: 45.0}
	writer := &syncWriter{ok: true}
	fan := NewFanController(fixedReader, writer, curve1, "weighted", 100*time.Millisecond, log.New(testWriter{t}, "", 0))
	fan.Start()
	defer fan.Stop()

	// 等初始写入完成（1s 延迟 + 首周期 ~600ms）
	time.Sleep(1800 * time.Millisecond)
	initialWrites := len(writer.snapshot())

	// 切换到 curve2（版本递增，但目标转速仍是 50%）
	// SetCurve 会自动调用 kick() 触发采样周期
	fan.SetCurve(curve2)

	// 给控制循环时间处理（一个周期 ~600ms + kick 响应）
	time.Sleep(800 * time.Millisecond)

	finalWrites := writer.snapshot()
	// 配置变化应触发新写入（即使转速未变），所以 writes 应增加
	if len(finalWrites) <= initialWrites {
		t.Fatalf("配置变化后未触发写入: initial=%d final=%d, writes=%v", initialWrites, len(finalWrites), finalWrites)
	}

	// 验证写入的确实是 50% (0x32)
	lastFan1, ok := writer.last(ECRegFan1)
	if !ok || lastFan1 != "0x32" {
		t.Fatalf("配置变化后 fan1=%q, want 0x32", lastFan1)
	}
}

// fixedTempReader 返回固定温度，用于构造可控测试场景
type fixedTempReader struct {
	cpu, gpu float64
}

func (r *fixedTempReader) Read() Temps {
	return Temps{CPU: &r.cpu, GPU: &r.gpu}
}

func (r *fixedTempReader) Close() error {
	return nil
}
