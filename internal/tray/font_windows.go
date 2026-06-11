//go:build windows

package tray

var glyphs = map[rune][7]uint8{
	'0': {0b01110, 0b10001, 0b10011, 0b10101, 0b11001, 0b10001, 0b01110},
	'1': {0b00100, 0b01100, 0b00100, 0b00100, 0b00100, 0b00100, 0b01110},
	'2': {0b01110, 0b10001, 0b00001, 0b00010, 0b00100, 0b01000, 0b11111},
	'3': {0b11110, 0b00001, 0b00001, 0b01110, 0b00001, 0b00001, 0b11110},
	'4': {0b00010, 0b00110, 0b01010, 0b10010, 0b11111, 0b00010, 0b00010},
	'5': {0b11111, 0b10000, 0b11110, 0b00001, 0b00001, 0b10001, 0b01110},
	'6': {0b00110, 0b01000, 0b10000, 0b11110, 0b10001, 0b10001, 0b01110},
	'7': {0b11111, 0b00001, 0b00010, 0b00100, 0b01000, 0b01000, 0b01000},
	'8': {0b01110, 0b10001, 0b10001, 0b01110, 0b10001, 0b10001, 0b01110},
	'9': {0b01110, 0b10001, 0b10001, 0b01111, 0b00001, 0b00010, 0b01100},
	'-': {0b00000, 0b00000, 0b00000, 0b11111, 0b00000, 0b00000, 0b00000},
}

func renderText16(text string, r, g, b uint8) []byte {
	const size = 16
	pixels := make([]byte, size*size*4)

	runes := []rune(text)
	if len(runes) > 2 {
		runes = runes[:2]
	}
	const glyphW = 5
	const glyphH = 7
	totalW := glyphW*len(runes) + (len(runes)-1)*1
	if totalW < 0 {
		totalW = 0
	}
	startX := (size - totalW) / 2
	startY := (size - glyphH) / 2

	for i, ch := range runes {
		glyph, ok := glyphs[ch]
		if !ok {
			continue
		}
		x0 := startX + i*(glyphW+1)
		for row := 0; row < glyphH; row++ {
			bits := glyph[row]
			for col := 0; col < glyphW; col++ {
				if bits&(1<<(glyphW-1-col)) != 0 {
					px := x0 + col
					py := startY + row
					if px < 0 || px >= size || py < 0 || py >= size {
						continue
					}
					idx := (py*size + px) * 4
					pixels[idx+0] = b
					pixels[idx+1] = g
					pixels[idx+2] = r
					pixels[idx+3] = 255
				}
			}
		}
	}
	return pixels
}
