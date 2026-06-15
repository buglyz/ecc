package sensors

import (
	"context"
	"fmt"
	"log"

	"github.com/yusufpapurcu/wmi"
)

// GMWMIReader 通过 WMI RW_GMWMI 接口读取风扇 RPM
// 适用于神舟等使用 Gaming WMI 接口的笔记本
type GMWMIReader struct {
	logger *log.Logger
}

// RW_GMWMI WMI 类结构
type RW_GMWMI struct {
	Active       bool
	InstanceName string
	BufferBytes  []byte
}

// NewGMWMIReader 创建 GMWMI RPM 读取器
func NewGMWMIReader(logger *log.Logger) *GMWMIReader {
	return &GMWMIReader{logger: logger}
}

// ReadRPM 从 WMI 读取风扇 RPM
// cpuOffset: CPU 风扇 RPM 在 BufferBytes 中的偏移（通常是 0x0C）
// gpuOffset: GPU 风扇 RPM 在 BufferBytes 中的偏移（通常是 0x10）
// 返回 CPU RPM 和 GPU RPM，如果读取失败返回 0
func (r *GMWMIReader) ReadRPM(ctx context.Context, cpuOffset, gpuOffset int) (cpuRPM, gpuRPM uint16) {
	var dst []RW_GMWMI
	query := "SELECT * FROM RW_GMWMI"

	if err := wmi.QueryNamespace(query, &dst, `root\WMI`); err != nil {
		if r.logger != nil {
			r.logger.Printf("GMWMI 查询失败: %v", err)
		}
		return 0, 0
	}

	if len(dst) == 0 || len(dst[0].BufferBytes) < gpuOffset+2 {
		if r.logger != nil {
			r.logger.Print("GMWMI 返回数据不足")
		}
		return 0, 0
	}

	buf := dst[0].BufferBytes

	// 从 BufferBytes 读取 RPM（小端序 16-bit）
	if cpuOffset >= 0 && cpuOffset+1 < len(buf) {
		cpuRPM = uint16(buf[cpuOffset]) | (uint16(buf[cpuOffset+1]) << 8)
	}

	if gpuOffset >= 0 && gpuOffset+1 < len(buf) {
		gpuRPM = uint16(buf[gpuOffset]) | (uint16(buf[gpuOffset+1]) << 8)
	}

	return cpuRPM, gpuRPM
}

// Close 实现 io.Closer 接口
func (r *GMWMIReader) Close() error {
	return nil
}

// GMWMIFanReader 实现 controller.FanReader 接口
// 将 GMWMI 读取适配到 FanReader 接口
type GMWMIFanReader struct {
	reader *GMWMIReader
}

// NewGMWMIFanReader 创建适配器
func NewGMWMIFanReader(logger *log.Logger) *GMWMIFanReader {
	return &GMWMIFanReader{
		reader: NewGMWMIReader(logger),
	}
}

// ReadRPM 实现 FanReader 接口
// registerLow 和 registerHigh 被解释为十六进制偏移字符串
// 例如 "0x0C" 和 "0x0D" 表示从 BufferBytes[0x0C:0x0E] 读取
// 只使用 registerLow 参数，registerHigh 被忽略（因为总是读取 2 字节）
// 返回 CPU 风扇 RPM
func (r *GMWMIFanReader) ReadRPM(ctx context.Context, registerLow, registerHigh string) (uint16, bool) {
	// 解析偏移量
	var cpuOffset, gpuOffset int
	// registerLow 表示 CPU RPM 偏移，默认 0x0C
	if _, err := fmt.Sscanf(registerLow, "0x%x", &cpuOffset); err != nil {
		cpuOffset = 0x0C
	}
	// GPU 偏移固定为 0x10
	gpuOffset = 0x10

	cpuRPM, _ := r.reader.ReadRPM(ctx, cpuOffset, gpuOffset)

	if cpuRPM == 0 {
		return 0, false
	}

	return cpuRPM, true
}

// Close 关闭读取器
func (r *GMWMIFanReader) Close() error {
	return r.reader.Close()
}
