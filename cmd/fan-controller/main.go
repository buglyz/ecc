package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	"github.com/buglyz/ecc/internal/instance"
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
	shutdown := flag.Bool("shutdown", false, "signal a running instance to exit gracefully")
	flag.Parse()

	// 安装器/卸载器的优雅退出入口。必须在提权、互斥体和配置读写之前处理：
	// 第二个进程只负责发信号，不启动任何硬件控制逻辑。
	if *shutdown {
		if err := instance.SignalShutdown(instance.DefaultShutdownEventName); err != nil {
			fmt.Fprintf(os.Stderr, "发送退出信号失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	pollInterval, err := pollIntervalFromMilliseconds(*interval)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --interval: %v\n", err)
		os.Exit(2)
	}
	if err := validateDashboardPort(*port); err != nil {
		fmt.Fprintf(os.Stderr, "invalid --port: %v\n", err)
		os.Exit(2)
	}
	requestedDashboardURL := "http://" + dashboard.BindAddress(*port)
	if *port != 0 && dashboardAlreadyRunning(requestedDashboardURL) {
		if !*noBrowser {
			openBrowser(requestedDashboardURL)
		}
		return
	}

	admin.SetDPIAwareness()
	runtimePaths := paths.Discover(controller.AppName)
	logger, closeLog := logging.New(runtimePaths.LogPath)
	defer closeLog()

	if !admin.RequireAdmin(*skipAdmin || *dryRun || *simulate) {
		logger.Print("需要管理员权限，已尝试重新启动为管理员")
		return
	}

	// 单实例保护必须与端口无关：不同 --port 的两个进程会并发写同一 EC，
	// 先退出的实例还会写 0xFF 释放另一个实例的控制权。在任何硬件初始化前获取
	// 全局命名互斥体，拿不到就退出（并尝试打开已有实例的控制台）。
	guard, acquired, err := instance.Acquire(instance.DefaultMutexName)
	if err != nil {
		logger.Printf("单实例互斥体创建失败: %v", err)
		fmt.Fprintf(os.Stderr, "单实例互斥体创建失败: %v\n", err)
		os.Exit(1)
	}
	if !acquired {
		logger.Print("检测到已有风扇控制实例在运行，本次启动退出")
		if !*noBrowser {
			if runningURL := runningDashboardURL(runtimePaths, requestedDashboardURL); runningURL != "" {
				openBrowser(runningURL)
			}
		}
		return
	}
	defer guard.Release()

	// 监听安装器/卸载器发来的优雅退出事件；Close 时注销句柄。
	shutdownListener, err := instance.NewShutdownListener(instance.DefaultShutdownEventName)
	if err != nil {
		logger.Printf("退出信号监听创建失败: %v", err)
	} else {
		defer shutdownListener.Close()
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

	// 在 Start() 启动 HTTP goroutine 之前快照这些字段：server 持有 &cfg，
	// handleConfig 会在请求中写回 *s.cfg，启动后再裸读 cfg 即构成数据竞争。
	logStrategy, logManualEnabled, logManualSpeed := cfg.Strategy, cfg.ManualEnabled, cfg.ManualSpeed
	minimized := cfg.Minimize == 1

	server := dashboard.New(dashboard.BindAddress(*port), runtimePaths, &cfg, fan, logger)
	if err := server.Start(); err != nil {
		if *port != 0 && dashboardAlreadyRunning(requestedDashboardURL) {
			logger.Printf("检测到已运行实例: %s", requestedDashboardURL)
			if !*noBrowser {
				openBrowser(requestedDashboardURL)
			}
			return
		}
		logger.Printf("Web 控制台启动失败: %v", err)
		return
	}

	// 先成功占用控制台端口，再启动硬件控制循环。重复启动时第二个进程会在上方退出，
	// 不会短暂并发写 EC，也不会在退出清理时释放第一个实例的风扇控制权。
	fan.Start()
	defer fan.Stop()

	dashURL := server.URL()
	// 记录控制台地址，供后续使用不同 --port 启动的实例打开已运行的控制台。
	instanceURLPath := filepath.Join(runtimePaths.StateDir, "instance.url")
	if err := writeInstanceURL(instanceURLPath, dashURL); err != nil {
		logger.Printf("实例地址文件写入失败: %v", err)
	} else {
		defer os.Remove(instanceURLPath)
	}
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

	if shutdownListener != nil {
		go func() {
			select {
			case <-shutdownListener.Done():
				logger.Print("收到外部退出信号，正在关闭")
				triggerExit()
			case <-exitCh:
			}
		}()
	}

	if !*noTray {
		onShow := func() { openBrowser(dashURL) }
		trayIcon := tray.New(onShow, triggerExit)

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
		waitForExit(exitCh)
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

func validateDashboardPort(port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("must be between 0 and 65535")
	}
	return nil
}

func writeInstanceURL(path, url string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(url+"\n"), 0o644)
}

// runningDashboardURL 优先读取上一次实例写入的控制台地址，其次回退到本次请求的
// 端口。只有 /api/health 校验通过才返回，避免崩溃残留的地址文件误导用户。
func runningDashboardURL(runtimePaths paths.Paths, fallback string) string {
	path := filepath.Join(runtimePaths.StateDir, "instance.url")
	if data, err := os.ReadFile(path); err == nil {
		if url := strings.TrimSpace(string(data)); url != "" && dashboardAlreadyRunning(url) {
			return url
		}
	}
	if dashboardAlreadyRunning(fallback) {
		return fallback
	}
	return ""
}

func dashboardAlreadyRunning(baseURL string) bool {
	client := http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		OK   bool      `json:"ok"`
		Time time.Time `json:"time"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4*1024)).Decode(&health); err != nil {
		return false
	}
	return health.OK && !health.Time.IsZero()
}

func waitForExit(exitCh <-chan struct{}) {
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case <-exitCh:
	}
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
