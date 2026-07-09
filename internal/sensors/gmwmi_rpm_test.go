package sensors

import "testing"

func TestParseRPMLittleEndian(t *testing.T) {
	// 0x0C=0xE8, 0x0D=0x03 → 0x03E8 = 1000（小端）
	buf := make([]byte, 32)
	buf[0x0C] = 0xE8
	buf[0x0D] = 0x03
	if got := parseRPM(buf, gmwmiCPUFanOffset); got != 1000 {
		t.Fatalf("parseRPM(cpu) = %d, want 1000", got)
	}
}

func TestParseRPMGPUOffset(t *testing.T) {
	buf := make([]byte, 32)
	buf[0x10] = 0x10
	buf[0x11] = 0x27 // 0x2710 = 10000
	if got := parseRPM(buf, gmwmiGPUFanOffset); got != 10000 {
		t.Fatalf("parseRPM(gpu) = %d, want 10000", got)
	}
}

func TestParseRPMOutOfRangeReturnsZero(t *testing.T) {
	buf := make([]byte, 16)
	// 这正是 Bug #1 的场景：把 EC 寄存器地址 0xD0=208 当成数组下标。
	if got := parseRPM(buf, 0xD0); got != 0 {
		t.Fatalf("parseRPM(buf, 208) = %d, want 0 (越界应返回 0)", got)
	}
}

func TestParseRPMNegativeOffsetReturnsZero(t *testing.T) {
	buf := make([]byte, 16)
	if got := parseRPM(buf, -1); got != 0 {
		t.Fatalf("parseRPM(buf, -1) = %d, want 0", got)
	}
}

func TestParseRPMLastPairBoundary(t *testing.T) {
	// 边界：offset 指向最后一对字节，应能读到（此前 off-by-one 会漏读）。
	buf := []byte{0x00, 0x00, 0xD0, 0x07} // len=4, offset=2 → buf[2],buf[3]
	if got := parseRPM(buf, 2); got != 2000 {
		t.Fatalf("parseRPM(last pair) = %d, want 2000 (0x07D0)", got)
	}
	// offset+1 刚好越界应返回 0。
	if got := parseRPM(buf, 3); got != 0 {
		t.Fatalf("parseRPM(buf, 3) = %d, want 0 (offset+1 越界)", got)
	}
}

func TestParseRPMEmptyBuffer(t *testing.T) {
	if got := parseRPM(nil, 0); got != 0 {
		t.Fatalf("parseRPM(nil, 0) = %d, want 0", got)
	}
}
