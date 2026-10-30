package controller

import (
	"context"
	"fmt"
	"math"
	"time"
)

// run 是控制循环主体，独立成文件以控制 controller.go 的体量。
//
// 自动模式在拿到首个有效温度之前不写 EC，让固件继续控制风扇；连续
// NoTempFailSafeCycles 个周期没有任何有效温度时，主动释放已接管的 EC 控制权，
// 避免 LHM 故障或传感器掉线时风扇长期停在低速档。
func (c *FanController) run() {
	defer close(c.done)
	select {
	case <-time.After(time.Second):
	case <-c.ctx.Done():
		return
	}

	state := c.currentMode()
	currentSpeed := initialSpeed(state)
	applied := false
	lastWrite := time.Now()

	// 手动模式是用户的显式指令，不依赖温度，启动后立即生效；
	// 自动模式必须等首个有效温度，期间保持固件控制。
	if state.manualSpeed != nil {
		if c.writeSpeed(currentSpeed) {
			applied = true
			lastWrite = time.Now()
			c.logger.Printf("控制器初始转速写入: speed=%d hex=%s", currentSpeed, toHex(currentSpeed))
		}
	}

	cycleStart := time.Now()
	var lastCommittedTemp *float64
	var lastAppliedVersion uint64
	mode := modeName(state)
	noTempCycles := 0
	failsafe := false
	safetyHold := false

	sampleTimer := time.NewTimer(c.pollInterval)
	defer sampleTimer.Stop()

	for {
		cycleTemps := make([]float64, 0, SamplesPerCycle)
		interrupted := false
		maxCPU, maxGPU := -1.0, -1.0
		// 采样循环内复用周期首快照：currentMode 会深拷曲线，逐样本取太贵。
		// 写决策在周期末另行取最新快照，故手动/策略/曲线变化最迟在本周期末生效，
		// 且 SetCurve 等通过 kick() 触发 nudge 提前结束本周期并立即进入写决策。
		state = c.currentMode()
		mode = modeName(state)
		if failsafe || safetyHold {
			mode = "safety"
		}
		for i := 0; i < SamplesPerCycle; i++ {
			select {
			case <-c.ctx.Done():
				return
			default:
			}
			temps := c.reader.Read()
			if temps.CPU != nil && *temps.CPU > maxCPU {
				maxCPU = *temps.CPU
			}
			if temps.GPU != nil && *temps.GPU > maxGPU {
				maxGPU = *temps.GPU
			}
			targetTemp := CombineTemps(state.strategy, temps.CPU, temps.GPU)
			if targetTemp != nil {
				cycleTemps = append(cycleTemps, *targetTemp)
			}
			rpm := c.readRPM()
			c.setLatest(temps.CPU, temps.GPU, targetTemp, currentSpeed, rpm, mode, lastWrite)

			sampleTimer.Reset(c.pollInterval)
			select {
			case <-sampleTimer.C:
			case <-c.nudge:
				sampleTimer.Stop()
				select {
				case <-sampleTimer.C:
				default:
				}
				interrupted = true
				i = SamplesPerCycle
			case <-c.ctx.Done():
				return
			}
		}

		drifted := absDuration(time.Since(cycleStart)-ExpectedCycleDuration(c.pollInterval)) > LoopDriftTolerance
		heartbeatDue := time.Since(lastWrite) >= HeartbeatInterval
		state = c.currentMode()
		var avgTemp *float64
		var target int

		panicNow := state.manualSpeed == nil && panicTempReached(maxCPU, maxGPU)
		if panicNow {
			if !safetyHold {
				c.logger.Printf("高温保护触发: CPU=%.1f GPU=%.1f，释放 EC 上限", maxCPU, maxGPU)
			}
			safetyHold = true
		}

		if safetyHold {
			if panicNow || !panicReleaseReached(maxCPU, maxGPU) {
				// 0xFF 不限制 EC；比写 100 更可靠，且用回差避免立即降档。
				target = SafetyReleaseValue
				lastCommittedTemp = nil
				mode = "safety"
				noTempCycles = 0
				failsafe = false
				if maxCPU >= 0 || maxGPU >= 0 {
					t := math.Max(maxCPU, maxGPU)
					avgTemp = &t
				}
			} else {
				safetyHold = false
				c.logger.Printf("高温保护解除: CPU=%.1f GPU=%.1f", maxCPU, maxGPU)
				if currentSpeed > 100 {
					currentSpeed = 100
				}
			}
		}

		if !safetyHold {
			if state.manualSpeed != nil {
				target = *state.manualSpeed
				lastCommittedTemp = nil
				mode = "manual"
				noTempCycles = 0
				failsafe = false
			} else if state.controlMode == ControlModeTarget && len(cycleTemps) > 0 {
				noTempCycles = 0
				failsafe = false
				// 目标温度模式使用 CPU/GPU 中较高的一路，避免加权平均掩盖热源。
				controlTemp := math.Max(maxCPU, maxGPU)
				if controlTemp < 0 {
					controlTemp = average(cycleTemps)
				}
				avgTemp = &controlTemp
				target = targetControlStep(currentSpeed, controlTemp, state.target, applied)
				lastCommittedTemp = nil
				mode = "target"
			} else if len(cycleTemps) > 0 {
				if noTempCycles >= NoTempFailSafeCycles {
					c.logger.Print("温度数据恢复，重新接管风扇控制")
				}
				noTempCycles = 0
				failsafe = false
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
				// 无任何温度采样：自动模式不能凭旧数据继续写转速。
				// 被配置变更打断的短周期不构成温度故障证据。
				if interrupted {
					cycleStart = time.Now()
					continue
				}
				// 首次达到阈值时释放控制权（若已接管），交还固件兜底。
				noTempCycles++
				if noTempCycles == NoTempFailSafeCycles && !failsafe {
					failsafe = true
					if applied {
						c.releaseFansWith(fmt.Sprintf("连续 %d 个周期无有效温度，释放 EC 控制权交还固件", noTempCycles))
						applied = false
						lastWrite = time.Now()
					} else {
						c.logger.Printf("连续 %d 个周期无有效温度，风扇保持固件控制", noTempCycles)
					}
					c.alert()
				}
				cycleStart = time.Now()
				continue
			}
		}

		versionChanged := lastAppliedVersion != state.version
		if !applied || target != currentSpeed || drifted || heartbeatDue || versionChanged {
			select {
			case <-c.ctx.Done():
				return
			default:
			}
			if c.writeSpeed(target) {
				applied = true
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
				c.alert()
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
	c.releaseFansWith("停止控制器，释放 EC 风扇控制")
}

func (c *FanController) releaseFansWith(reason string) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.logger.Print(reason)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok1 := c.writer.Write(ctx, ECRegFan1, ECFanRelease)
	ok2 := c.writer.Write(ctx, ECRegFan2, ECFanRelease)
	if !ok1 || !ok2 {
		c.logger.Printf("释放 EC 风扇控制失败: fan1=%t fan2=%t", ok1, ok2)
	}
}

// alert 调用外部告警回调（托盘图标闪红）。onWriteFailure 命名沿用历史接口，
// 现在同时用于 EC 写失败与温度数据不可信两种告警。
func (c *FanController) alert() {
	c.mu.RLock()
	handler := c.onWriteFailure
	c.mu.RUnlock()
	if handler != nil {
		handler()
	}
}

// panicTempReached 判断自动模式是否触发高温硬保护。
func panicTempReached(maxCPU, maxGPU float64) bool {
	return maxCPU >= PanicCPUTemp || maxGPU >= PanicGPUTemp
}

// panicReleaseReached 判断高温保护是否可以解除；留 5°C 回差，避免立刻恢复后再次触发。
func panicReleaseReached(maxCPU, maxGPU float64) bool {
	return maxCPU < PanicCPUTemp-PanicReleaseMargin && maxGPU < PanicGPUTemp-PanicReleaseMargin
}
