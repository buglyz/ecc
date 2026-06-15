package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/buglyz/ecc/internal/admin"
	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/dashboard"
	"github.com/buglyz/ecc/internal/ec"
	"github.com/buglyz/ecc/internal/logging"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/sensors"
	"github.com/buglyz/ecc/internal/tray"
)

func main() {
	port := flag.Int("port", 8765, "local dashboard port")
	interval := flag.Int("interval", 1000, "polling interval in milliseconds (default 1000)")
	dryRun := flag.Bool("dry-run", false, "log EC writes instead of touching hardware")
	simulate := flag.Bool("simulate", false, "use simulated CPU/GPU temperatures")
	skipAdmin := flag.Bool("skip-admin", false, "do not request administrator elevation")
	noTray := flag.Bool("no-tray", false, "disable system tray icon")
	noBrowser := flag.Bool("no-browser", false, "do not auto-open browser")
	flag.Parse()

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

	cfg := config.Load(runtimePaths)
	if err := config.Save(runtimePaths, cfg); err != nil {
		logger.Printf("配置保存失败: %v", err)
	}

	var reader controller.SensorReader
	if *simulate {
		reader = sensors.NewSimulatedReader()
		logger.Print("温度读取使用模拟模式")
	} else {
		reader = &sensors.PowerShellReader{DLLPath: runtimePaths.HardwareDLL, StateDir: runtimePaths.StateDir, Logger: logger}
	}
	defer func() {
		if err := reader.Close(); err != nil {
			logger.Printf("温度读取器关闭失败: %v", err)
		}
	}()

	writer := ec.Writer{ProbePath: runtimePaths.ECProbe, DryRun: *dryRun || *simulate, Logger: logger}
	var fanReader controller.FanReader
	if !*dryRun && !*simulate {
		fanReader = &ec.Reader{ProbePath: runtimePaths.ECProbe, Logger: logger}
	}
	pollInterval := time.Duration(*interval) * time.Millisecond
	fan := controller.NewFanControllerWithRPM(reader, writer, fanReader, cfg.Curve, cfg.Strategy, pollInterval, logger)
	fan.SetManual(cfg.ManualSpeedPtr())
	fan.Start()
	defer fan.Stop()

	// 在 Start() 启动 HTTP goroutine 之前快照这些字段：server 持有 &cfg，
	// handleConfig 会在请求中写回 *s.cfg，启动后再裸读 cfg 即构成数据竞争。
	logStrategy, logManualEnabled, logManualSpeed := cfg.Strategy, cfg.ManualEnabled, cfg.ManualSpeed
	minimized := cfg.Minimize == 1

	server := dashboard.New(dashboard.BindAddress(*port), runtimePaths, &cfg, fan, logger)
	if err := server.Start(); err != nil {
		logger.Printf("Web 控制台启动失败: %v", err)
		return
	}

	dashURL := server.URL()
	logger.Printf("应用启动完成: url=%s strategy=%s manual_enabled=%t manual_speed=%d dry_run=%t simulate=%t",
		dashURL, logStrategy, logManualEnabled, logManualSpeed, *dryRun, *simulate)

	if !*noBrowser && !minimized {
		openBrowser(dashURL)
	}

	exitCh := make(chan struct{})
	var exitOnce sync.Once
	triggerExit := func() {
		exitOnce.Do(func() { close(exitCh) })
	}

	if !*noTray {
		onShow := func() { openBrowser(dashURL) }
		trayIcon := tray.New(onShow, triggerExit)

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
			signals := make(chan os.Signal, 2)
			signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
			select {
			case <-signals:
				triggerExit()
			case <-exitCh:
			}
			signal.Stop(signals)
		}()

		go func() {
			<-exitCh
			trayIcon.Stop()
		}()

		runtime.LockOSThread()
		trayIcon.Run()
	} else {
		waitForExit()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("Web 控制台关闭失败: %v", err)
	}
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

func waitForExit() {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	} else {
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
