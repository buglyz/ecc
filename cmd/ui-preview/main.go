// ui-preview 是 UI 迁移期间的快速预览入口:
//   - 不请求管理员权限、不做单实例锁、不启动托盘
//   - 使用模拟温度数据 + DryRun EC 写入 (不触碰硬件)
//   - 配置/日志写入临时目录, 不影响真实用户配置
//
// 用法: go run ./cmd/ui-preview
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"log"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/buglyz/ecc/internal/app"
	"github.com/buglyz/ecc/internal/config"
	"github.com/buglyz/ecc/internal/controller"
	"github.com/buglyz/ecc/internal/ec"
	"github.com/buglyz/ecc/internal/logging"
	"github.com/buglyz/ecc/internal/paths"
	"github.com/buglyz/ecc/internal/sensors"
	"github.com/buglyz/ecc/internal/ui"
)

func main() {
	tempDir, err := os.MkdirTemp("", "ecc-ui-preview")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	p := paths.Discover(controller.AppName)
	p.ConfigPath = filepath.Join(tempDir, "config.json")
	p.LogPath = filepath.Join(tempDir, "preview.log")

	logger, closeLog := logging.New(p.LogPath)
	defer closeLog()

	cfg := config.Load(p).Config
	if err := config.Save(p, cfg); err != nil {
		logger.Printf("预览配置保存失败: %v", err)
	}

	reader := sensors.NewSimulatedReader()
	defer reader.Close()

	writer := ec.Writer{ProbePath: p.ECProbe, DryRun: true, Logger: logger}
	fan := controller.NewFanControllerWithRPM(reader, writer, nil, cfg.Curve, cfg.Strategy, time.Second, logger)
	fan.SetManual(cfg.ManualSpeedPtr())
	fan.Start()
	defer fan.Stop()

	go func() {
		log.Println("pprof http on 127.0.0.1:9977")
		http.ListenAndServe("127.0.0.1:9977", nil)
	}()

	service := app.New(p, &cfg, fan, logger)
	exited := make(chan struct{})
	window := ui.New(service, func() { close(exited) }, true)
	if err := window.Start(true); err != nil {
		fmt.Fprintf(os.Stderr, "预览 UI 启动失败: %v\n", err)
		os.Exit(1)
	}
	<-exited
}
