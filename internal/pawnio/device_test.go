package pawnio

import "testing"

func TestParseByte(t *testing.T) {
	cases := map[string]byte{
		"0x2C":   0x2C,
		"0XFF":   0xFF,
		"44":     44,
		"255":    255,
		" 0x64 ": 0x64,
	}
	for in, want := range cases {
		got, err := parseByte(in)
		if err != nil {
			t.Fatalf("parseByte(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("parseByte(%q)=%#x, want %#x", in, got, want)
		}
	}
	for _, in := range []string{"", "0x100", "abc", "0x"} {
		if _, err := parseByte(in); err == nil {
			t.Errorf("parseByte(%q) error=nil, want error", in)
		}
	}
}
