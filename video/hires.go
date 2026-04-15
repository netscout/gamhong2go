package video

// Hi-res memory layout:
//   Page 1: $2000–$3FFF (8 KB)
//   Page 2: $4000–$5FFF (8 KB)
//
// Each byte controls 7 horizontal pixels.
//   Bits 0–6: pixel data (bit 0 = leftmost)
//   Bit 7:    palette select (shifts colors by half a pixel)
//
// The 192 scanlines are divided into groups with the same interleaved
// pattern as the text screen, but with 8 sections of 8 lines each:
//
//   Line 0   → $2000    Line 64  → $2028    Line 128 → $2050
//   Line 1   → $2080    Line 65  → $20A8    Line 129 → $20D0
//   ...
//   Line 7   → $2380    Line 71  → $23A8    Line 135 → $23D0
//   Line 8   → $2400    Line 72  → $2428    Line 136 → $2450
//   ...

// hresLineAddr returns the memory address for a given scanline (0–191).
func hiResLineAddr(line int, page2 bool) uint16 {
	base := uint16(0x2000)
	if page2 {
		base = 0x4000
	}
	// Same interleaving formula as text but scaled for 192 lines:
	//   section (0–2) = line / 64      → offset by $28 (40 bytes)
	//   group   (0–7) = (line%64) / 8  → offset by $80 (128 bytes)
	//   row     (0–7) = line % 8       → offset by $400 (1024 bytes)
	section := line / 64
	group := (line % 64) / 8
	row := line % 8
	return base + uint16(row)*0x0400 + uint16(group)*0x0080 + uint16(section)*0x0028
}

// NTSC artifact colors for hi-res mode.
// The Apple II generates color through NTSC artifact patterns.
// Adjacent pixel pairs create different colors depending on their
// column position (even/odd) and the palette bit.
var hiResColors = [2][4][3]uint8{
	// Palette 0 (bit 7 = 0): Black, Green, Purple, White
	{
		{0, 0, 0},       // 00 = black
		{17, 221, 0},    // 01 = green (odd pixel lit)
		{221, 34, 221},  // 10 = purple (even pixel lit)
		{255, 255, 255}, // 11 = white
	},
	// Palette 1 (bit 7 = 1): Black, Orange, Blue, White
	{
		{0, 0, 0},       // 00 = black
		{255, 102, 0},   // 01 = orange (odd pixel lit)
		{34, 34, 255},   // 10 = blue (even pixel lit)
		{255, 255, 255}, // 11 = white
	},
}

// RenderHiRes renders the 280×192 hi-res graphics screen.
// If mixed mode is on, the bottom 32 scanlines (4 text rows) show text.
func (v *Video) RenderHiRes(page2, mixed bool) {
	maxLine := 192
	if mixed {
		maxLine = 160 // 192 - 32 (4 text rows × 8 scanlines)
	}

	for line := 0; line < maxLine; line++ {
		addr := hiResLineAddr(line, page2)
		v.renderHiResLine(line, addr)
	}

	if mixed {
		v.renderTextRows(20, 24)
	}
}

// renderHiResLine renders a single scanline of hi-res graphics using
// NTSC artifact color emulation.
func (v *Video) renderHiResLine(line int, addr uint16) {
	for col := 0; col < 40; col++ {
		b := v.RAM[addr+uint16(col)]
		palette := int((b >> 7) & 1)
		pixels := b & 0x7F

		for bit := 0; bit < 7; bit++ {
			px := col*7 + bit
			if px >= ScreenW {
				break
			}

			thisPixel := (pixels >> uint(bit)) & 1

			// Determine color from pixel pair pattern.
			// The "column" parity (even/odd pixel in the full 280-pixel row)
			// determines which artifact color appears.
			var colorIdx int
			if thisPixel == 0 {
				colorIdx = 0 // black
			} else {
				// Check neighboring pixel for white detection
				var neighbor uint8
				if bit < 6 {
					neighbor = (pixels >> uint(bit+1)) & 1
				} else if col < 39 {
					// Next byte's bit 0
					nextByte := v.RAM[addr+uint16(col+1)]
					neighbor = nextByte & 1
				}

				if neighbor == 1 {
					colorIdx = 3 // white (adjacent pixels both lit)
				} else if px%2 == 0 {
					colorIdx = 2 // even column → purple/blue
				} else {
					colorIdx = 1 // odd column → green/orange
				}
			}

			// Also check if the previous pixel makes this one white
			if thisPixel == 1 && colorIdx != 3 {
				var prevPixel uint8
				if bit > 0 {
					prevPixel = (pixels >> uint(bit-1)) & 1
				} else if col > 0 {
					prevByte := v.RAM[addr+uint16(col-1)]
					prevPixel = (prevByte >> 6) & 1
				}
				if prevPixel == 1 {
					colorIdx = 3 // white
				}
			}

			color := hiResColors[palette][colorIdx]
			offset := (line*ScreenW + px) * 4
			v.Pixels[offset+0] = color[0]
			v.Pixels[offset+1] = color[1]
			v.Pixels[offset+2] = color[2]
			v.Pixels[offset+3] = 0xFF
		}
	}
}
