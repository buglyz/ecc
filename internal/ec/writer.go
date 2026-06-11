package ec

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/buglyz/ecc/internal/process"
)

const writeTimeout = 5 * time.Second

type Writer struct {
	ProbePath string
	DryRun    bool
	Logger    *log.Logger
}

func (w Writer) Write(ctx context.Context, register string, valueHex string) bool {
	if w.Logger == nil {
		w.Logger = log.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		w.Logger.Printf("EC 写入取消: register=%s value=%s error=%v", register, valueHex, err)
		return false
	}
	if w.DryRun {
		w.Logger.Printf("EC 写入预演: register=%s value=%s", register, valueHex)
		return true
	}
	if _, err := os.Stat(w.ProbePath); err != nil {
		w.Logger.Printf("EC 写入跳过，缺少 ec-probe.exe: %s", w.ProbePath)
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.ProbePath, "write", "-v", register, valueHex)
	cmd.Dir = filepath.Dir(w.ProbePath)
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	if err := cmd.Run(); err != nil {
		w.Logger.Printf("EC 写入命令失败: register=%s value=%s error=%v", register, valueHex, err)
		return false
	}
	w.Logger.Printf("EC 写入完成: register=%s value=%s", register, valueHex)
	return true
}
