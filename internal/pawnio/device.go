package pawnio

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// ECDevice 是 PawnIO 的 EC 读写抽象，便于测试和非 Windows 构建。
type ECDevice interface {
	ReadEC(ctx context.Context, reg byte) (byte, error)
	WriteEC(ctx context.Context, reg, val byte) error
	Close() error
}

// Writer 实现 controller.FanWriter：把 ECC 的寄存器写入映射到 EC 0x2C/0x2D。
// 每次写入后回读校验；失败返回 false，让控制循环按既有逻辑重试/告警。
type Writer struct {
	dev    ECDevice
	logger *log.Logger
}

func NewWriter(dev ECDevice, logger *log.Logger) *Writer {
	if logger == nil {
		logger = log.Default()
	}
	return &Writer{dev: dev, logger: logger}
}

func (w *Writer) Write(ctx context.Context, register string, valueHex string) bool {
	if w.dev == nil {
		w.logger.Print("PawnIO 写入失败：设备未初始化")
		return false
	}
	reg, err := parseByte(register)
	if err != nil {
		w.logger.Printf("PawnIO 写入失败：寄存器 %q 无效: %v", register, err)
		return false
	}
	val, err := parseByte(valueHex)
	if err != nil {
		w.logger.Printf("PawnIO 写入失败：值 %q 无效: %v", valueHex, err)
		return false
	}

	if err := w.dev.WriteEC(ctx, reg, val); err != nil {
		w.logger.Printf("PawnIO 写入失败: reg=0x%02X value=0x%02X error=%v", reg, val, err)
		return false
	}

	// 回读校验。EC 写入后约 100–150ms 才稳定，且读取偶发噪声；按 S8D6
	// 的实测节奏等待 150ms，最多重试 3 次。
	for attempt := 0; attempt < 3; attempt++ {
		time.Sleep(150 * time.Millisecond)
		got, err := w.dev.ReadEC(ctx, reg)
		if err != nil {
			w.logger.Printf("PawnIO 回读失败: reg=0x%02X error=%v", reg, err)
			continue
		}
		if got == val {
			return true
		}
		w.logger.Printf("PawnIO 回读不符: reg=0x%02X wrote=0x%02X read=0x%02X (attempt %d)", reg, val, got, attempt+1)
	}
	return false
}

func parseByte(s string) (byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty value")
	}
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	}
	v, err := strconv.ParseUint(s, base, 8)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}
	return byte(v), nil
}
