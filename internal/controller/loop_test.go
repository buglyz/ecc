package controller

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
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

// 自动模式在拿到首个有效温度前不得接管 EC：启动窗口内不应出现任何写入。
func TestAutoModeWaitsForFirstValidTemperature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-dependent control loop test in -short mode")
	}
	writer := &syncWriter{ok: true}
	fan := NewFanController(testReader{}, writer, DefaultCurve, DefaultStrategy, 50*time.Millisecond, log.New(testWriter{t}, "", 0))
	fan.Start()
	// 1 秒启动等待 + 首个周期 300ms，睡眠 1.2 秒时首个周期尚未走完。
	time.Sleep(1200 * time.Millisecond)
	writes := writer.snapshot()
	fan.Stop()

	if len(writes) != 0 {
		t.Fatalf("自动模式在无有效温度时不应写 EC, writes=%v", writes)
	}
}

// 自动模式拿到有效温度后，首周期结束时应按曲线写入目标转速。
func TestAutoModeWritesAfterFirstValidTemperature(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-dependent control loop test in -short mode")
	}
	reader := &fixedTempReader{cpu: 45.0, gpu: 45.0}
	writer := &syncWriter{ok: true}
	fan := NewFanController(reader, writer, DefaultCurve, DefaultStrategy, 50*time.Millisecond, log.New(testWriter{t}, "", 0))
	fan.Start()
	time.Sleep(2 * time.Second)
	fan.Stop()

	want := toHex(ClampSpeed(InterpolateCurve(DefaultCurve, 45)))
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
		t.Fatalf("expected target speed %q written to both fans, fan1=%t fan2=%t, writes=%v", want, sawFan1, sawFan2, writes)
	}
}

// 接管风扇后温度持续不可用，应释放 EC 控制权交还固件，而不是停在最后转速。
func TestNoTempFailsafeReleasesControl(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-dependent control loop test in -short mode")
	}
	reader := &switchableReader{cpu: 45.0, gpu: 45.0}
	writer := &syncWriter{ok: true}
	fan := NewFanController(reader, writer, DefaultCurve, DefaultStrategy, 30*time.Millisecond, log.New(testWriter{t}, "", 0))
	fan.Start()
	// 1 秒启动等待 + 首个周期 180ms，确保已经接管风扇。
	time.Sleep(1600 * time.Millisecond)
	reader.setBroken(true)
	// NoTempFailSafeCycles(3) 个周期 × 180ms ≈ 540ms，留足余量。
	time.Sleep(1200 * time.Millisecond)

	v1, ok := writer.last(ECRegFan1)
	fan.Stop()
	if !ok || v1 != ECFanRelease {
		t.Fatalf("温度持续缺失后 fan1=%q (ok=%t), want release %q", v1, ok, ECFanRelease)
	}
}

type switchableReader struct {
	cpu, gpu float64
	broken   atomic.Bool
}

func (r *switchableReader) Read() Temps {
	if r.broken.Load() {
		return Temps{}
	}
	return Temps{CPU: &r.cpu, GPU: &r.gpu}
}

func (r *switchableReader) Close() error { return nil }

func (r *switchableReader) setBroken(broken bool) { r.broken.Store(broken) }

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
