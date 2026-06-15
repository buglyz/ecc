package ec

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/buglyz/ecc/internal/process"
)

const readTimeout = 5 * time.Second

type Reader struct {
	ProbePath string
	Logger    *log.Logger
}

// Read reads a byte value from the specified EC register.
// Returns the value (0-255) and whether the read was successful.
func (r Reader) Read(ctx context.Context, register string) (uint8, bool) {
	if r.Logger == nil {
		r.Logger = log.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		r.Logger.Printf("EC 读取取消: register=%s error=%v", register, err)
		return 0, false
	}
	if _, err := os.Stat(r.ProbePath); err != nil {
		r.Logger.Printf("EC 读取跳过，缺少 ec-probe.exe: %s", r.ProbePath)
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.ProbePath, "read", register)
	cmd.Dir = filepath.Dir(r.ProbePath)
	cmd.SysProcAttr = process.HiddenSysProcAttr()

	output, err := cmd.Output()
	if err != nil {
		r.Logger.Printf("EC 读取命令失败: register=%s error=%v", register, err)
		return 0, false
	}

	// Parse output: ec-probe.exe returns format like "123 (0x7B)"
	// We need to extract just the decimal or hex part
	outputStr := strings.TrimSpace(string(output))
	var value uint64

	// Check for "123 (0x7B)" format - extract the decimal part before space
	if idx := strings.Index(outputStr, " "); idx > 0 {
		outputStr = outputStr[:idx]
	}

	// Try hex format first
	if strings.HasPrefix(outputStr, "0x") || strings.HasPrefix(outputStr, "0X") {
		value, err = strconv.ParseUint(outputStr[2:], 16, 8)
	} else {
		// Try decimal
		value, err = strconv.ParseUint(outputStr, 10, 8)
	}

	if err != nil {
		r.Logger.Printf("EC 读取解析失败: register=%s output=%s error=%v", register, outputStr, err)
		return 0, false
	}

	return uint8(value), true
}

// ReadRPM reads the fan RPM from two consecutive EC registers (low and high byte).
// Returns the RPM value and whether the read was successful.
func (r Reader) ReadRPM(ctx context.Context, registerLow, registerHigh string) (uint16, bool) {
	low, okLow := r.Read(ctx, registerLow)
	if !okLow {
		return 0, false
	}
	high, okHigh := r.Read(ctx, registerHigh)
	if !okHigh {
		return 0, false
	}
	// Combine high and low bytes: RPM = (high << 8) | low
	rpm := (uint16(high) << 8) | uint16(low)
	return rpm, true
}
