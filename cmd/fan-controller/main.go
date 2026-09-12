package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buglyz/ecc/internal/admin"
	"github.com/buglyz/ecc/internal/app"
	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/ec"
	"github.com/buglyz/ecc/internal/instance"
	"github.com/buglyz/ecc/internal/logging"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/sensors"
	"github.com/buglyz/ecc/internal/tray"
	"github.com/buglyz/ecc/internal/ui"
)

func main() {
	interval := flag.Int("interval", 1000, "polling interval in milliseconds (default 1000)")
	dryRun := flag.Bool("dry-run", false, "log EC writes instead of touching hardware")
	simulate := flag.Bool("simulate", false, "use simulated CPU/GPU temperatures")
	skipAdmin := flag.Bool("skip-admin", false, "do not request administrator elevation")
	noTray := flag.Bool("no-tray", false, "disable system tray icon")
	flag.Parse()

	pollInterval, err := pollIntervalFromMilliseconds(*interval)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --interval: %v\n", err)
		os.Exit(2)
	}
	releaseInstance, alreadyRunning, err := instance.Acquire()
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法建立单实例锁: %v\n", err)
		return
	}
	if alreadyRunning {
		ui.ActivateExisting()
		return
	}
	defer releaseInstance()

	admin.SetDPIAwareness()
	runtimePaths := paths.Discover(controller.AppName)
	logger, closeLog := logging.New(runtimePaths.LogPath)
	defer closeLog()

	if !admin.RequireAdmin(*skipAdmin || *dryRun || *simulate) {
		logger.Print("需要管理员权限，已尝试重新启动为管理员")
		return
	}

	if missing := requiredRuntimeFiles(runtimePaths, *dryRun, *simulate); len(missing) > 0 {
		logger.Printf("缺少必要运行文件: %s", strings.Join(missing, "; "))
		fmt.Fprintf(os.Stderr, "缺少必要运行文件:\n%s\n", strings.Join(missing, "\n"))
		os.Exit(1)
	}

	loadResult := config.Load(runtimePaths)
	cfg := loadResult.Config

	// 如果配置文件损坏，记录警告并稍后通知用户
	var configCorrupted bool
	if len(loadResult.CorruptedFiles) > 0 {
		configCorrupted = true
		for _, path := range loadResult.CorruptedFiles {
			logger.Printf("警告: 配置文件已损坏并备份: %s", path)
		}
		logger.Print("已使用默认配置启动")
	}

	if err := config.Save(runtimePaths, cfg); err != nil {
		logger.Printf("配置保存失败: %v", err)
	}

	var reader controller.SensorReader
	if *simulate {
		reader = sensors.NewSimulatedReader()
		logger.Print("温度读取使用模拟模式")
	} else {
		reader = &sensors.PowerShellReader{DLLPath: runtimePaths.HardwareDLL, Logger: logger}
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Printf("温度读取器关闭失败: %v", err)
		}
	}()

	writer := ec.Writer{ProbePath: runtimePaths.ECProbe, DryRun: *dryRun || *simulate, Logger: logger}
	var fanReader controller.FanReader
	if !*dryRun && !*simulate {
		// 通过 EC 寄存器读取风扇 RPM，走与写入相同的 ec-probe / WinRing0 路径。
		// 转速寄存器 0xB0-0xB3 经真机 dump 交叉验证（见 controller/constants.go）。
		// ec.Reader.ReadRPM 读不到（缺驱动/缺 ec-probe）时优雅降级返回 false。
		fanReader = ec.Reader{ProbePath: runtimePaths.ECProbe, Logger: logger}
		logger.Print("使用 EC 寄存器读取风扇 RPM")
	}
	fan := controller.NewFanControllerWithRPM(reader, writer, fanReader, cfg.Curve, cfg.Strategy, pollInterval, logger)
	fan.SetManual(cfg.ManualSpeedPtr())

	logStrategy, logManualEnabled, logManualSpeed := cfg.Strategy, cfg.ManualEnabled, cfg.ManualSpeed
	minimized := cfg.Minimize == 1
	fan.Start()
	defer fan.Stop()

	exitCh := make(chan struct{})
	var exitOnce sync.Once
	triggerExit := func() {
		exitOnce.Do(func() { close(exitCh) })
	}
	service := app.New(runtimePaths, &cfg, fan, logger)
	window := ui.New(service, triggerExit, !*noTray)
	if err := window.Start(!minimized); err != nil {
		logger.Printf("原生 UI 启动失败: %v", err)
		return
	}
	logger.Printf("应用启动完成: strategy=%s manual_enabled=%t manual_speed=%d dry_run=%t simulate=%t",
		logStrategy, logManualEnabled, logManualSpeed, *dryRun, *simulate)

	go func() {
		signals := make(chan os.Signal, 2)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(signals)
		select {
		case <-signals:
			triggerExit()
		case <-exitCh:
		}
	}()

	if !*noTray {
		trayIcon := tray.New(window.Show, triggerExit)

		// 如果配置损坏，闪动图标提醒用户（日志已记录详细信息）
		if configCorrupted {
			trayIcon.Alert()
		}

		// Set write failure handler to alert via tray icon
		fan.SetWriteFailureHandler(func() {
			trayIcon.Alert()
		})

		go func() {
			ticker := time.NewTicker(pollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					latest := fan.Latest()
					trayIcon.Update(latest.TargetTemp, latest.Speed)
				case <-exitCh:
					return
				}
			}
		}()

		go func() {
			<-exitCh
			trayIcon.Stop()
		}()

		runtime.LockOSThread()
		trayIcon.Run()
	} else {
		<-exitCh
	}
	window.Close()
}

func requiredRuntimeFiles(runtimePaths paths.Paths, dryRun bool, simulate bool) []string {
	if simulate {
		return nil
	}
	missing := make([]string, 0, 2)
	if !dryRun {
		if _, err := os.Stat(runtimePaths.ECProbe); err != nil {
			missing = append(missing, "ec-probe.exe: "+runtimePaths.ECProbe)
		}
	}
	if _, err := os.Stat(runtimePaths.HardwareDLL); err != nil {
		missing = append(missing, "LibreHardwareMonitorLib.dll: "+runtimePaths.HardwareDLL)
	}
	return missing
}

func pollIntervalFromMilliseconds(milliseconds int) (time.Duration, error) {
	const maxDuration = time.Duration(1<<63 - 1)
	if milliseconds <= 0 {
		return 0, fmt.Errorf("must be greater than zero milliseconds")
	}
	if int64(milliseconds) > int64(maxDuration/time.Millisecond) {
		return 0, fmt.Errorf("is too large")
	}
	return time.Duration(milliseconds) * time.Millisecond, nil
}
