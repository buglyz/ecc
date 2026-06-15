package controller

import (
	"context"
	"log"
	"math"
	"sync"
	"time"
)

type FanController struct {
	reader SensorReader
	writer FanWriter
	logger *log.Logger

	writeMu         sync.Mutex
	mu              sync.RWMutex
	curve           []Point
	strategy        string
	manualSpeed     *int
	version         uint64
	latest          Latest
	pollInterval    time.Duration
	onWriteFailure  func()

	history *History
	nudge   chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

type modeState struct {
	manualSpeed *int
	strategy    string
	curve       []Point
	version     uint64
}

func NewFanController(reader SensorReader, writer FanWriter, curve []Point, strategy string, pollInterval time.Duration, logger *log.Logger) *FanController {
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
		logger:       logger,
		curve:        append([]Point(nil), curve...),
		strategy:     strategy,
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
	state := modeState{strategy: c.strategy, curve: append([]Point(nil), c.curve...), version: c.version}
	if c.manualSpeed != nil {
		v := *c.manualSpeed
		state.manualSpeed = &v
	}
	return state
}

func (c *FanController) setLatest(cpu, gpu, targetTemp *float64, speed int, mode string, lastECWrite time.Time) {
	now := time.Now()
	c.mu.Lock()
	c.latest = Latest{CPU: copyFloat(cpu), GPU: copyFloat(gpu), TargetTemp: copyFloat(targetTemp), Speed: &speed, Mode: mode, UpdatedAt: now, LastECWrite: lastECWrite}
	c.mu.Unlock()
	c.history.Add(HistorySample{Time: now, CPU: copyFloat(cpu), GPU: copyFloat(gpu), TargetTemp: copyFloat(targetTemp), Speed: speed})
}

func (c *FanController) run() {
	defer close(c.done)
	select {
	case <-time.After(time.Second):
	case <-c.ctx.Done():
		return
	}

	state := c.currentMode()
	currentSpeed := initialSpeed(state)
	c.writeSpeed(currentSpeed)
	// 与 Python 原版一致：初始周期起点无条件视为“已写”，避免首个真实周期
	// 因 lastWrite 为零值而误判 heartbeatDue。首周期本就会因 lastCommittedTemp==nil 写入。
	lastWrite := time.Now()
	c.logger.Printf("控制器初始转速写入: speed=%d hex=%s", currentSpeed, toHex(currentSpeed))

	cycleStart := time.Now()
	var lastCommittedTemp *float64
	var lastAppliedVersion uint64
	mode := modeName(state)

	sampleTimer := time.NewTimer(c.pollInterval)
	defer sampleTimer.Stop()

	for {
		cycleTemps := make([]float64, 0, SamplesPerCycle)
		for i := 0; i < SamplesPerCycle; i++ {
			select {
			case <-c.ctx.Done():
				return
			default:
			}
			temps := c.reader.Read()
			state = c.currentMode()
			targetTemp := CombineTemps(state.strategy, temps.CPU, temps.GPU)
			if targetTemp != nil {
				cycleTemps = append(cycleTemps, *targetTemp)
			}
			mode = modeName(state)
			c.setLatest(temps.CPU, temps.GPU, targetTemp, currentSpeed, mode, lastWrite)

			sampleTimer.Reset(c.pollInterval)
			select {
			case <-sampleTimer.C:
			case <-c.nudge:
				if !sampleTimer.Stop() {
					<-sampleTimer.C
				}
				i = SamplesPerCycle
			case <-c.ctx.Done():
				return
			}
		}

		drifted := absDuration(time.Since(cycleStart)-ExpectedCycleDuration()) > LoopDriftTolerance
		heartbeatDue := time.Since(lastWrite) >= HeartbeatInterval
		state = c.currentMode()
		var avgTemp *float64
		var target int

		if state.manualSpeed != nil {
			target = *state.manualSpeed
			lastCommittedTemp = nil
			mode = "manual"
		} else if len(cycleTemps) > 0 {
			avg := average(cycleTemps)
			avgTemp = &avg
			tempSettled := lastCommittedTemp != nil && math.Abs(avg-*lastCommittedTemp) < HysteresisTemp && lastAppliedVersion == state.version
			if tempSettled && !(drifted || heartbeatDue) {
				cycleStart = time.Now()
				continue
			}
			target = ClampSpeed(InterpolateCurve(state.curve, avg))
			mode = "auto:" + state.strategy
		} else {
			cycleStart = time.Now()
			continue
		}

		if target != currentSpeed || drifted || heartbeatDue {
			select {
			case <-c.ctx.Done():
				return
			default:
			}
			if c.writeSpeed(target) {
				currentSpeed = target
				lastWrite = time.Now()
				lastAppliedVersion = state.version
				if avgTemp != nil {
					committed := *avgTemp
					lastCommittedTemp = &committed
				}
				c.logger.Printf("转速已提交: mode=%s speed=%d hex=%s avg_temp=%v drifted=%t heartbeat=%t", mode, target, toHex(target), formatTemp(avgTemp), drifted, heartbeatDue)
			} else {
				// 写失败意味着风扇并未设到该温度对应的转速，不能记为”已提交”，
				// 否则下一周期滞回判断会误以为已稳定而跳过重试。清空已提交温度，强制下一周期重试。
				lastCommittedTemp = nil
				c.logger.Printf("转速提交失败: mode=%s speed=%d hex=%s avg_temp=%v drifted=%t heartbeat=%t", mode, target, toHex(target), formatTemp(avgTemp), drifted, heartbeatDue)
				c.mu.RLock()
				handler := c.onWriteFailure
				c.mu.RUnlock()
				if handler != nil {
					handler()
				}
			}
		}
		cycleStart = time.Now()
	}
}

func (c *FanController) writeSpeed(speed int) bool {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	speedHex := toHex(speed)
	ok1 := c.writer.Write(c.ctx, ECRegFan1, speedHex)
	ok2 := c.writer.Write(c.ctx, ECRegFan2, speedHex)
	return ok1 && ok2
}

func (c *FanController) releaseFans() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.logger.Print("停止控制器，释放 EC 风扇控制")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok1 := c.writer.Write(ctx, ECRegFan1, ECFanRelease)
	ok2 := c.writer.Write(ctx, ECRegFan2, ECFanRelease)
	if !ok1 || !ok2 {
		c.logger.Printf("释放 EC 风扇控制失败: fan1=%t fan2=%t", ok1, ok2)
	}
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
	const digits = "0123456789abcdef"
	if value == 0 {
		return "0x0"
	}
	n := value
	buf := make([]byte, 0, 4)
	for n > 0 {
		buf = append(buf, digits[n&0xf])
		n >>= 4
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return "0x" + string(buf)
}

func formatTemp(temp *float64) any {
	if temp == nil {
		return nil
	}
	return math.Round(*temp*10) / 10
}
