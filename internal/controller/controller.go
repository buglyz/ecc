package controller

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

type FanController struct {
	reader    SensorReader
	writer    FanWriter
	fanReader FanReader
	logger    *log.Logger

	writeMu        sync.Mutex
	mu             sync.RWMutex
	curve          []Point
	strategy       string
	controlMode    string
	target         TargetConfig
	manualSpeed    *int
	version        uint64
	latest         Latest
	pollInterval   time.Duration
	onWriteFailure func()

	history *History
	nudge   chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}

	// RPM 读取节流：一次 ReadRPM 要为高低字节各启动一次 ec-probe.exe（每次都
	// 加载/卸载 WinRing0 驱动，开销可达几十~上百毫秒），采样循环每秒调用一次会带来
	// 可感知开销。这里把硬件查询降到 RPMReadInterval 一次，中间的采样复用上次读到
	// 的值。仅在控制循环 goroutine 内访问，无需加锁。
	lastRPM     *uint16
	lastRPMTime time.Time
}

type modeState struct {
	manualSpeed *int
	strategy    string
	controlMode string
	target      TargetConfig
	curve       []Point
	version     uint64
}

func NewFanController(reader SensorReader, writer FanWriter, curve []Point, strategy string, pollInterval time.Duration, logger *log.Logger) *FanController {
	return NewFanControllerWithRPM(reader, writer, nil, curve, strategy, pollInterval, logger)
}

func NewFanControllerWithRPM(reader SensorReader, writer FanWriter, fanReader FanReader, curve []Point, strategy string, pollInterval time.Duration, logger *log.Logger) *FanController {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = log.Default()
	}
	if pollInterval <= 0 {
		pollInterval = SampleInterval
	}
	return &FanController{
		reader:       reader,
		writer:       writer,
		fanReader:    fanReader,
		logger:       logger,
		curve:        append([]Point(nil), curve...),
		strategy:     strategy,
		controlMode:  ControlModeCurve,
		target:       DefaultTargetConfig(),
		pollInterval: pollInterval,
		history:      NewHistory(HistoryMaxSamples),
		nudge:        make(chan struct{}, 1),
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}
}

func (c *FanController) Start() {
	go c.run()
}

func (c *FanController) SetCurve(curve []Point) {
	c.mu.Lock()
	c.curve = append([]Point(nil), curve...)
	c.version++
	c.mu.Unlock()
	c.logger.Printf("控制器曲线已更新: curve=%v", curve)
	c.kick()
}

func (c *FanController) SetStrategy(strategy string) {
	c.mu.Lock()
	c.strategy = strategy
	c.version++
	c.mu.Unlock()
	c.logger.Printf("控制器策略已更新: strategy=%s", strategy)
	c.kick()
}

func (c *FanController) SetControlMode(mode string) {
	if !ValidControlMode(mode) {
		mode = ControlModeCurve
	}
	c.mu.Lock()
	c.controlMode = mode
	c.version++
	c.mu.Unlock()
	c.logger.Printf("控制器模式已更新: mode=%s", mode)
	c.kick()
}

func (c *FanController) SetTargetConfig(t TargetConfig) {
	t = NormalizeTargetConfig(t)
	c.mu.Lock()
	c.target = t
	c.version++
	c.mu.Unlock()
	c.logger.Printf("目标温度配置已更新: target=%.1f hysteresis=%.1f kp=%.1f bias=%d range=%d-%d step=+%d/-%d",
		t.Temp, t.Hysteresis, t.KP, t.Bias, t.Min, t.Max, t.UpStep, t.DownStep)
	c.kick()
}

func (c *FanController) kick() {
	select {
	case c.nudge <- struct{}{}:
	default:
	}
}

func (c *FanController) SetManual(speed *int) {
	c.mu.Lock()
	if speed == nil {
		c.manualSpeed = nil
	} else {
		v := *speed
		if v < 0 {
			v = 0
		}
		if v > 100 {
			v = 100
		}
		c.manualSpeed = &v
	}
	c.version++
	c.mu.Unlock()
	if speed != nil {
		c.logger.Printf("控制器手动模式已更新: speed=%d", *speed)
	} else {
		c.logger.Print("控制器手动模式已更新: speed=auto")
	}
	c.kick()
}

func (c *FanController) SetWriteFailureHandler(handler func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onWriteFailure = handler
}

func (c *FanController) Latest() Latest {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

func (c *FanController) Snapshot() []HistorySample {
	return c.history.Snapshot()
}

func (c *FanController) SnapshotSince(cutoff time.Time) []HistorySample {
	return c.history.SnapshotSince(cutoff)
}

func (c *FanController) Stop() {
	c.cancel()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
	c.releaseFans()
}

func (c *FanController) currentMode() modeState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state := modeState{
		strategy:    c.strategy,
		controlMode: c.controlMode,
		target:      c.target,
		curve:       append([]Point(nil), c.curve...),
		version:     c.version,
	}
	if c.manualSpeed != nil {
		v := *c.manualSpeed
		state.manualSpeed = &v
	}
	return state
}

// setLatest 同时更新 Latest 与 HistorySample。Latest 和 HistorySample 需要各自的
// 指针字段（JSON 编码时互不影响），但 CPU/GPU/TargetTemp 的浮点值本次采样内共享
// 同一份拷贝，避免逐样本 3 次重复分配。
func (c *FanController) setLatest(cpu, gpu, targetTemp *float64, speed int, rpm *uint16, mode string, lastECWrite time.Time) {
	now := time.Now()
	cpuCopy, gpuCopy, targetCopy := copyFloat(cpu), copyFloat(gpu), copyFloat(targetTemp)
	c.mu.Lock()
	c.latest = Latest{CPU: cpuCopy, GPU: gpuCopy, TargetTemp: targetCopy, Speed: &speed, ActualRPM: rpm, Mode: mode, UpdatedAt: now, LastECWrite: lastECWrite}
	c.mu.Unlock()
	c.history.Add(HistorySample{Time: now, CPU: cpuCopy, GPU: gpuCopy, TargetTemp: targetCopy, Speed: speed, ActualRPM: rpm})
}

// readRPM 返回最近一次读到的风扇转速，并按 RPMReadInterval 节流硬件查询。
// 一次 ReadRPM 要 spawn 两次 ec-probe.exe（读低/高字节，各自加载卸载 WinRing0
// 驱动），采样循环每秒调用不该每次都真查；节流窗口内直接复用上次结果。仅在
// 控制循环 goroutine 内调用，lastRPM/lastRPMTime 无并发访问。
func (c *FanController) readRPM() *uint16 {
	if c.fanReader == nil {
		return nil
	}
	if !c.lastRPMTime.IsZero() && time.Since(c.lastRPMTime) < RPMReadInterval {
		return c.lastRPM
	}
	c.lastRPMTime = time.Now()
	rpm, ok := c.fanReader.ReadRPM(c.ctx, ECRegFan1RPMLow, ECRegFan1RPMHigh)
	// 硬件不支持或读到 0 都记为「无数据」(nil)，避免 UI 显示误导性的 0 RPM。
	// 无论成功与否都刷新缓存：失败时缓存 nil，节流窗口内不再重试白白拖慢采样。
	if !ok || rpm == 0 {
		c.lastRPM = nil
	} else {
		c.lastRPM = &rpm
	}
	return c.lastRPM
}

func initialSpeed(state modeState) int {
	if state.manualSpeed != nil {
		return *state.manualSpeed
	}
	return int(DefaultCurve[0].Speed())
}

func modeName(state modeState) string {
	if state.manualSpeed != nil {
		return "manual"
	}
	if state.controlMode == ControlModeTarget {
		return "target"
	}
	return "auto:" + state.strategy
}

func average(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func toHex(value int) string {
	return fmt.Sprintf("0x%x", value)
}

func formatTemp(temp *float64) any {
	if temp == nil {
		return nil
	}
	return math.Round(*temp*10) / 10
}
