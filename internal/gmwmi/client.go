package gmwmi

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/buglyz/ecc/internal/process"
)

const cliTimeout = 5 * time.Second

// Client 调用 GmwmiCli.exe 完成一次 32 字节 WMI 缓冲区往返。
// 协议本身（命令字、字段偏移）在 Go 侧实现，C# 只做薄封装。
type Client struct {
	CliPath string
	DryRun  bool
	Logger  *log.Logger

	run func(ctx context.Context, args ...string) ([]byte, error)
}

func NewClient(cliPath string, dryRun bool, logger *log.Logger) *Client {
	if logger == nil {
		logger = log.Default()
	}
	return &Client{CliPath: cliPath, DryRun: dryRun, Logger: logger}
}

func (c *Client) Transfer(ctx context.Context, req Buffer) (Buffer, error) {
	var resp Buffer
	if c.DryRun {
		c.Logger.Printf("GMWMI 预演: %s", hex.EncodeToString(req[:]))
		return resp, nil
	}
	if c.CliPath == "" {
		return resp, fmt.Errorf("GmwmiCli.exe 路径为空")
	}
	if _, err := os.Stat(c.CliPath); err != nil {
		return resp, fmt.Errorf("缺少 GmwmiCli.exe: %w", err)
	}

	run := c.run
	if run == nil {
		run = c.runDefault
	}
	out, err := run(ctx, "xfer", hex.EncodeToString(req[:]))
	if err != nil {
		return resp, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return resp, fmt.Errorf("解析 GMWMI 返回值失败: %w", err)
	}
	if len(raw) != BufferSize {
		return resp, fmt.Errorf("GMWMI 返回 %d 字节，期望 %d", len(raw), BufferSize)
	}
	copy(resp[:], raw)
	return resp, nil
}

func (c *Client) runDefault(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.CliPath, args...)
	cmd.Dir = filepath.Dir(c.CliPath)
	cmd.SysProcAttr = process.HiddenSysProcAttr()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("GmwmiCli %v 失败: %w (%s)", args, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (c *Client) Detect(ctx context.Context) (Capabilities, error) {
	req := NewRequest(CmdRead, SubCapabilities, 0, 0, 0)
	resp, err := c.Transfer(ctx, req)
	if err != nil {
		return Capabilities{}, err
	}
	return ParseCapabilities(resp), nil
}

func (c *Client) ReadSensors(ctx context.Context, caps Capabilities) (Sensors, error) {
	req := NewRequest(CmdRead, SubSensors, 0, 0, 0)
	resp, err := c.Transfer(ctx, req)
	if err != nil {
		return Sensors{}, err
	}
	s := ParseSensors(resp)
	if caps.Support3Fan {
		sysReq := NewRequest(CmdRead, SubSysSensors, 0, 0, 0)
		if sysResp, err := c.Transfer(ctx, sysReq); err == nil {
			s.SysTemp, s.SysRPM = ParseSysSensors(sysResp)
		}
	}
	return s, nil
}

// SetMode 发送 0xFB00/0x0206：1=用户模式，0=交还 BIOS。
func (c *Client) SetMode(ctx context.Context, mode uint32) error {
	_, err := c.Transfer(ctx, NewRequest(CmdWrite, SubFanMode, mode, 0, 0))
	return err
}

// SetFan 发送 0xFB00/0x0205：CPU/GPU/SYS 百分比；SYS 传 255 表示双风扇机型不接管。
func (c *Client) SetFan(ctx context.Context, cpu, gpu, sys uint8) error {
	_, err := c.Transfer(ctx, NewRequest(CmdWrite, SubFanSpeed, uint32(cpu), uint32(gpu), uint32(sys)))
	return err
}
