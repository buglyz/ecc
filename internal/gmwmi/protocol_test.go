package gmwmi

import "testing"

func TestParseCapabilitiesFromQNLXS(t *testing.T) {
	var b Buffer
	// a0=0x010D, a1=0x0201, a2=48 (from the real SQ131 response)
	b[0], b[1] = 0x0d, 0x01
	b[2], b[3] = 0x01, 0x02
	b[4] = 0x30

	caps := ParseCapabilities(b)
	if caps.UserFanSupported {
		t.Fatal("UserFanSupported=true, want false")
	}
	if caps.Support3Fan {
		t.Fatal("Support3Fan=true, want false")
	}
	if caps.Model != 48 {
		t.Fatalf("Model=%d, want 48", caps.Model)
	}
}

func TestParseCapabilitiesSupports3Fan(t *testing.T) {
	var b Buffer
	b[0], b[1] = 0x00, 0x02 // a0=0x0200
	b[4] = 52
	caps := ParseCapabilities(b)
	if !caps.UserFanSupported {
		t.Fatal("UserFanSupported=false, want true")
	}
	if !caps.Support3Fan {
		t.Fatal("Support3Fan=false, want true")
	}
}

func TestParseSensorsFromQNLXS(t *testing.T) {
	var b Buffer
	copy(b[:], []byte{
		0x00, 0xfa, 0x00, 0x02, // a0/a1
		0x52, 0x00, 0x00, 0x00, // a2 = 82 C
		0x39, 0x00, 0x00, 0x00, // a3 = 57 C
		0x2e, 0x0e, 0x00, 0x00, // a4 = 3630 RPM
		0xa1, 0x0d, 0x00, 0x00, // a5 = 3489 RPM
	})

	s := ParseSensors(b)
	if s.CPUTemp == nil || *s.CPUTemp != 82 {
		t.Fatalf("CPUTemp=%v, want 82", s.CPUTemp)
	}
	if s.GPUTemp == nil || *s.GPUTemp != 57 {
		t.Fatalf("GPUTemp=%v, want 57", s.GPUTemp)
	}
	if s.CPURPM == nil || *s.CPURPM != 3630 {
		t.Fatalf("CPURPM=%v, want 3630", s.CPURPM)
	}
	if s.GPURPM == nil || *s.GPURPM != 3489 {
		t.Fatalf("GPURPM=%v, want 3489", s.GPURPM)
	}
}

func TestParseSysSensors(t *testing.T) {
	var b Buffer
	b[4] = 0x50             // a2 = 80 C
	b[8], b[9] = 0xf3, 0x10 // a3 = 4339 RPM
	temp, rpm := ParseSysSensors(b)
	if temp == nil || *temp != 80 {
		t.Fatalf("SysTemp=%v, want 80", temp)
	}
	if rpm == nil || *rpm != 4339 {
		t.Fatalf("SysRPM=%v, want 4339", rpm)
	}
}

func TestNewRequestLayout(t *testing.T) {
	b := NewRequest(CmdWrite, SubFanSpeed, 30, 40, 255)
	if b.A0() != CmdWrite || b.A1() != SubFanSpeed {
		t.Fatalf("a0/a1 = %#x/%#x", b.A0(), b.A1())
	}
	if b.A2() != 30 || b.A3() != 40 || b.A4() != 255 {
		t.Fatalf("a2/a3/a4 = %d/%d/%d", b.A2(), b.A3(), b.A4())
	}
}
