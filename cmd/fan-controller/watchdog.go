package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/ec"
	"github.com/buglyz/ecc/internal/nl5"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/pawnio"
)

const (
	heartbeatInterval   = 5 * time.Second
	watchdogTimeout     = 30 * time.Second
	heartbeatStaleAfter = 30 * time.Second
	heartbeatStartGrace = 20 * time.Second
)

// heartbeatHealthy 判断控制循环是否还在正常更新：
// 启动初期允许一段宽限期；之后要求最新采样时间在 heartbeatStaleAfter 内。
func heartbeatHealthy(latest, start, now time.Time) bool {
	if latest.IsZero() {
		return now.Sub(start) < heartbeatStartGrace
	}
	return now.Sub(latest) < heartbeatStaleAfter
}

// runHeartbeat 定期刷新心跳文件。主进程被强杀后文件仍在，看门狗据此判断。
// healthy 返回 false 时停止续命：控制循环卡死但进程未退出时，看门狗仍能释放风扇。
func runHeartbeat(path string, stop <-chan struct{}, healthy func() bool, logger *log.Logger) {
	unhealthy := false
	write := func() bool {
		if healthy != nil && !healthy() {
			if !unhealthy {
				logger.Print("控制循环心跳异常，暂停刷新看门狗心跳")
				unhealthy = true
			}
			return false
		}
		if unhealthy {
			logger.Print("控制循环恢复，继续刷新看门狗心跳")
			unhealthy = false
		}
		if err := os.WriteFile(path, []byte(time.Now().Format(time.RFC3339Nano)), 0o644); err != nil {
			logger.Printf("写看门狗心跳失败: %v", err)
		}
		return true
	}
	write()
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			write()
		case <-stop:
			return
		}
	}
}

// runWatchdog 在独立进程中运行：心跳过期则把 EC 上限写回 FF/FF。
func runWatchdog(heartbeatPath string, timeout time.Duration, logger *log.Logger) {
	if heartbeatPath == "" {
		logger.Print("看门狗缺少心跳路径，退出")
		return
	}
	if timeout <= 0 {
		timeout = watchdogTimeout
	}
	start := time.Now()
	for {
		info, err := os.Stat(heartbeatPath)
		if err != nil {
			// 心跳文件从未出现（主进程在接管前就退出）：不写 EC。
			if os.IsNotExist(err) && time.Since(start) > 20*time.Second {
				logger.Print("看门狗：未发现心跳文件，主进程未接管，退出")
				return
			}
			time.Sleep(heartbeatInterval)
			continue
		}
		age := time.Since(info.ModTime())
		if age <= timeout {
			time.Sleep(heartbeatInterval)
			continue
		}

		logger.Printf("看门狗：心跳已过期 %s，释放 EC 风扇上限", age.Round(time.Second))
		releaseFansFF(logger)
		_ = os.Remove(heartbeatPath)
		return
	}
}

// releaseFansFF 优先走 PawnIO，失败时回退 ec-probe。
func releaseFansFF(logger *log.Logger) {
	p := paths.Discover(controller.AppName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if p.PawnIOModule != "" {
		if dev, err := pawnio.Open(p.PawnIOModule, logger); err == nil {
			w := pawnio.NewWriter(dev, logger)
			ok1 := w.Write(ctx, "0x2C", "0xFF")
			ok2 := w.Write(ctx, "0x2D", "0xFF")
			_ = dev.Close()
			if ok1 && ok2 {
				logger.Print("看门狗：PawnIO 已释放风扇上限")
				return
			}
			logger.Printf("看门狗：PawnIO 释放不完整（0x2C=%t 0x2D=%t），尝试 ec-probe", ok1, ok2)
		} else {
			logger.Printf("看门狗：PawnIO 打开失败: %v", err)
		}
	}

	if p.ECProbe == "" {
		logger.Print("看门狗：没有可用的 EC 后端，无法释放")
		return
	}
	w := ec.Writer{ProbePath: p.ECProbe, Logger: logger}
	ok1 := w.Write(ctx, "0x2C", "0xFF")
	ok2 := w.Write(ctx, "0x2D", "0xFF")
	if ok1 && ok2 {
		logger.Print("看门狗：ec-probe 已释放风扇上限")
	} else {
		logger.Printf("看门狗：ec-probe 释放失败（0x2C=%t 0x2D=%t）", ok1, ok2)
	}
}

// monitorOEMOwnership 定期检查原厂 ControlCenter 是否重新启用了风扇控制。
func monitorOEMOwnership(exitCh <-chan struct{}, triggerExit func(), logger *log.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if v, ok := nl5.FanControlStatus(); ok && v == 1 {
				logger.Print("检测到原厂 ControlCenter 启用风扇控制，ECC 退出并释放风扇")
				triggerExit()
				return
			}
		case <-exitCh:
			return
		}
	}
}

func startupOwnership(logger *log.Logger, dryRun, simulate bool) {
	if status, ok := nl5.FanControlStatus(); ok {
		logger.Printf("NL5 FanControlStatus=%d", status)
		if !dryRun && !simulate && status != 0 {
			if nl5.SetFanControlStatus(0) {
				logger.Print("已将 NL5 FanControlStatus 置 0，避免原厂 daemon 接管")
			} else {
				logger.Print("警告：无法写入 NL5 FanControlStatus")
			}
		}
		return
	}
	if !dryRun && !simulate {
		logger.Print("未检测到 NL5 ControlCenter 注册表状态（可能未安装）")
	}
}

func heartbeatPathForPID(dir string) string {
	return fmt.Sprintf("%s%cwatchdog-%d.heartbeat", dir, os.PathSeparator, os.Getpid())
}
