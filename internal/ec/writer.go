package ec

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/buglyz/ecc/internal/process"
)

const writeTimeout = 5 * time.Second

// driverLoadFailure 是 ec-probe 在 WinRing0 驱动加载失败时打印的标志串（小写，
// 供大小写不敏感匹配）。判读逻辑见 writeSucceeded。
const driverLoadFailure = "unable to load the winring0 driver"

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
	output, err := cmd.CombinedOutput()
	if err != nil {
		w.Logger.Printf("EC 写入命令失败: register=%s value=%s error=%v output=%s", register, valueHex, err, strings.TrimSpace(string(output)))
		return false
	}
	if !writeSucceeded(string(output)) {
		w.Logger.Printf("EC 写入失败，驱动未加载: register=%s value=%s output=%s", register, valueHex, strings.TrimSpace(string(output)))
		return false
	}
	w.Logger.Printf("EC 写入完成: register=%s value=%s", register, valueHex)
	return true
}

// writeSucceeded 判读 ec-probe write 的输出是否代表写入真正生效。
// ec-probe 在 WinRing0 驱动加载失败时 write 命令仍返回 exit 0（区别于 read/dump
// 返回 1），仅靠退出码会把「驱动没加载、EC 根本没写入」误判为成功。因此在退出码
// 正常的前提下，还需扫描输出：命中驱动加载失败标志即判为未生效，交由控制循环重试。
func writeSucceeded(output string) bool {
	return !strings.Contains(strings.ToLower(output), driverLoadFailure)
}
