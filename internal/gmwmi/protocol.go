package gmwmi

import "encoding/binary"

const (
	BufferSize = 32

	CmdRead  = 0xFA00
	CmdWrite = 0xFB00

	SubCapabilities = 0x0201
	SubSensors      = 0x0200
	SubSysSensors   = 0x0207
	SubFanSpeed     = 0x0205
	SubFanMode      = 0x0206
)

// Buffer 是 root\wmi:RW_GMWMI 的 32 字节命令/响应缓冲区。
type Buffer [BufferSize]byte

func (b *Buffer) A0() uint16 { return binary.LittleEndian.Uint16(b[0:2]) }
func (b *Buffer) A1() uint16 { return binary.LittleEndian.Uint16(b[2:4]) }
func (b *Buffer) A2() uint32 { return binary.LittleEndian.Uint32(b[4:8]) }
func (b *Buffer) A3() uint32 { return binary.LittleEndian.Uint32(b[8:12]) }
func (b *Buffer) A4() uint32 { return binary.LittleEndian.Uint32(b[12:16]) }
func (b *Buffer) A5() uint32 { return binary.LittleEndian.Uint32(b[16:20]) }

func (b *Buffer) SetA0(v uint16) { binary.LittleEndian.PutUint16(b[0:2], v) }
func (b *Buffer) SetA1(v uint16) { binary.LittleEndian.PutUint16(b[2:4], v) }
func (b *Buffer) SetA2(v uint32) { binary.LittleEndian.PutUint32(b[4:8], v) }
func (b *Buffer) SetA3(v uint32) { binary.LittleEndian.PutUint32(b[8:12], v) }
func (b *Buffer) SetA4(v uint32) { binary.LittleEndian.PutUint32(b[12:16], v) }

func NewRequest(a0, a1 uint16, a2, a3, a4 uint32) Buffer {
	var b Buffer
	b.SetA0(a0)
	b.SetA1(a1)
	b.SetA2(a2)
	b.SetA3(a3)
	b.SetA4(a4)
	return b
}

type Capabilities struct {
	UserFanSupported bool
	Support3Fan      bool
	Model            uint32
}

func ParseCapabilities(b Buffer) Capabilities {
	model := b.A2()
	return Capabilities{
		UserFanSupported: b.A0() >= 0x0200,
		Support3Fan:      model == 52 || model == 53,
		Model:            model,
	}
}

type Sensors struct {
	CPUTemp *float64
	GPUTemp *float64
	SysTemp *float64

	CPURPM *uint16
	GPURPM *uint16
	SysRPM *uint16
}

func tempPtr(v uint32) *float64 {
	if v == 0 {
		return nil
	}
	t := float64(byte(v))
	return &t
}

func rpmPtr(v uint32) *uint16 {
	if v == 0 {
		return nil
	}
	r := uint16(v)
	return &r
}

func ParseSensors(b Buffer) Sensors {
	return Sensors{
		CPUTemp: tempPtr(b.A2()),
		GPUTemp: tempPtr(b.A3()),
		CPURPM:  rpmPtr(b.A4()),
		GPURPM:  rpmPtr(b.A5()),
	}
}

func ParseSysSensors(b Buffer) (temp *float64, rpm *uint16) {
	return tempPtr(b.A2()), rpmPtr(b.A3())
}
