package sensors

import (
	"context"
	"log"

	"github.com/yusufpapurcu/wmi"
)

// GMWMI BufferBytes 中风扇 RPM 的字节偏移（小端 16 位）。
// 这是 WMI RW_GMWMI 返回的数据缓冲里的数组下标，与 EC 寄存器地址是
// 两套完全不同的寻址空间，切勿把 controller 里的 EC 寄存器常量（如
// ECRegFan1RPMLow=0xD0）当作这里的偏移使用——0xD0=208 会直接越界。
const (
	gmwmiCPUFanOffset = 0x0C // BufferBytes[0x0C:0x0E] = CPU 风扇 RPM
	gmwmiGPUFanOffset = 0x10 // BufferBytes[0x10:0x12] = GPU 风扇 RPM
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

	if len(dst) == 0 {
		if r.logger != nil {
			r.logger.Print("GMWMI 返回数据不足")
		}
		return 0, 0
	}

	buf := dst[0].BufferBytes
	cpuRPM = parseRPM(buf, cpuOffset)
	gpuRPM = parseRPM(buf, gpuOffset)
	return cpuRPM, gpuRPM
}

// parseRPM 从 buf 的 offset 处按小端序读取 16 位 RPM。
// offset 越界或为负时返回 0。注意边界条件是 offset+1 < len(buf)（即
// 需要 offset 和 offset+1 两个字节都存在），此前误写成不含等号的判断
// 会漏读缓冲区最后一对字节。
func parseRPM(buf []byte, offset int) uint16 {
	if offset < 0 || offset+1 >= len(buf) {
		return 0
	}
	return uint16(buf[offset]) | (uint16(buf[offset+1]) << 8)
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

// ReadRPM 实现 controller.FanReader 接口，返回 CPU 风扇实际转速。
//
// 注意：接口的 registerLow/registerHigh 参数是为「EC 寄存器地址」寻址方式
// 设计的（ec-probe 那条路径会用到），但 GMWMI 走的是 WMI BufferBytes 数组，
// 两者是完全不同的寻址空间。此前的实现错误地把 EC 寄存器地址（如 "0xD0"=208）
// Sscanf 成数组下标，导致每次都越界、RPM 恒为 0、功能实际从未生效。
// 这里直接忽略这两个参数，使用正确的 GMWMI BufferBytes 偏移常量。
func (r *GMWMIFanReader) ReadRPM(ctx context.Context, registerLow, registerHigh string) (uint16, bool) {
	cpuRPM, _ := r.reader.ReadRPM(ctx, gmwmiCPUFanOffset, gmwmiGPUFanOffset)
	if cpuRPM == 0 {
		return 0, false
	}
	return cpuRPM, true
}

// Close 关闭读取器
func (r *GMWMIFanReader) Close() error {
	return r.reader.Close()
}
