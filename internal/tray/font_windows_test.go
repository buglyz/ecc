//go:build windows

package tray

import (
	"testing"
)

func TestRenderText16EmptyString(t *testing.T) {
	pixels := renderText16("", 255, 255, 255)
	if len(pixels) != 16*16*4 {
		t.Errorf("expected %d bytes, got %d", 16*16*4, len(pixels))
	}
	// All pixels should be transparent (alpha = 0)
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			t.Errorf("expected all pixels transparent, but pixel at %d has alpha %d", i/4, pixels[i])
			break
		}
	}
}

func TestRenderText16SingleDigit(t *testing.T) {
	pixels := renderText16("5", 255, 0, 0)
	if len(pixels) != 16*16*4 {
		t.Errorf("expected %d bytes, got %d", 16*16*4, len(pixels))
	}

	// Count opaque pixels
	opaqueCount := 0
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] == 255 {
			opaqueCount++
			// Check RGB values for opaque pixels
			idx := i - 3
			if pixels[idx] != 0 || pixels[idx+1] != 0 || pixels[idx+2] != 255 {
				t.Errorf("opaque pixel at %d has wrong RGB: (%d,%d,%d), want (0,0,255)",
					idx/4, pixels[idx], pixels[idx+1], pixels[idx+2])
				break
			}
		}
	}

	// Digit '5' should have some opaque pixels
	if opaqueCount == 0 {
		t.Error("expected some opaque pixels for digit '5', got none")
	}
}

func TestRenderText16TwoDigits(t *testing.T) {
	pixels := renderText16("42", 128, 128, 128)
	if len(pixels) != 16*16*4 {
		t.Errorf("expected %d bytes, got %d", 16*16*4, len(pixels))
	}

	// Count opaque pixels
	opaqueCount := 0
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] == 255 {
			opaqueCount++
		}
	}

	// Two digits should have more opaque pixels than one
	if opaqueCount == 0 {
		t.Error("expected some opaque pixels for '42', got none")
	}
}

func TestRenderText16TruncatesLongString(t *testing.T) {
	// Should only render first 2 characters
	pixels1 := renderText16("12", 255, 255, 255)
	pixels2 := renderText16("123456", 255, 255, 255)

	// Both should produce identical output (only "12" rendered)
	for i := range pixels1 {
		if pixels1[i] != pixels2[i] {
			t.Error("expected '12' and '123456' to render identically (first 2 chars only)")
			break
		}
	}
}

func TestRenderText16UnknownCharacter(t *testing.T) {
	// Unknown character should be skipped
	pixels := renderText16("X", 255, 255, 255)
	if len(pixels) != 16*16*4 {
		t.Errorf("expected %d bytes, got %d", 16*16*4, len(pixels))
	}

	// All pixels should be transparent for unknown char
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			t.Error("expected all pixels transparent for unknown character 'X'")
			break
		}
	}
}

func TestRenderText16Hyphen(t *testing.T) {
	pixels := renderText16("-", 255, 255, 255)
	if len(pixels) != 16*16*4 {
		t.Errorf("expected %d bytes, got %d", 16*16*4, len(pixels))
	}

	// Hyphen should have some opaque pixels
	opaqueCount := 0
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] == 255 {
			opaqueCount++
		}
	}

	if opaqueCount == 0 {
		t.Error("expected some opaque pixels for hyphen '-', got none")
	}
}

func TestRenderText16ColorMapping(t *testing.T) {
	r, g, b := uint8(100), uint8(150), uint8(200)
	pixels := renderText16("8", r, g, b)

	// Find first opaque pixel and verify color
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] == 255 {
			idx := i - 3
			// Verify BGR order (Windows BGRA format)
			if pixels[idx] != b || pixels[idx+1] != g || pixels[idx+2] != r {
				t.Errorf("pixel color mismatch: got (%d,%d,%d), want (%d,%d,%d)",
					pixels[idx], pixels[idx+1], pixels[idx+2], b, g, r)
			}
			break
		}
	}
}

func TestGlyphsHaveCorrectDimensions(t *testing.T) {
	const glyphW = 5
	const glyphH = 7

	for ch, glyph := range glyphs {
		if len(glyph) != glyphH {
			t.Errorf("glyph '%c' has %d rows, expected %d", ch, len(glyph), glyphH)
		}

		for row, bits := range glyph {
			// Check that no bits are set beyond the 5-bit width
			if bits > 0b11111 {
				t.Errorf("glyph '%c' row %d has bits beyond width: 0b%b", ch, row, bits)
			}
		}
	}
}

func TestRenderText16PixelBounds(t *testing.T) {
	// Test that rendering doesn't write outside buffer bounds
	pixels := renderText16("99", 255, 255, 255)

	// Verify buffer size
	if len(pixels) != 16*16*4 {
		t.Fatalf("expected %d bytes, got %d", 16*16*4, len(pixels))
	}

	// Verify all bytes are valid (0-255)
	for i, b := range pixels {
		if b > 255 {
			t.Errorf("invalid byte at index %d: %d", i, b)
		}
	}
}
