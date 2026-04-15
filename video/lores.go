package video

// Apple II lo-res palette — 16 colors.
// Each entry is [R, G, B]. These match the NTSC artifact colors.
var loResColors = [16][3]uint8{
	{0, 0, 0},       // 0  Black
	{221, 0, 51},     // 1  Magenta (Deep Red)
	{0, 0, 153},      // 2  Dark Blue
	{221, 34, 221},   // 3  Purple (Violet)
	{0, 119, 34},     // 4  Dark Green
	{85, 85, 85},     // 5  Grey 1
	{34, 34, 255},    // 6  Medium Blue
	{102, 170, 255},  // 7  Light Blue
	{136, 85, 0},     // 8  Brown
	{255, 102, 0},    // 9  Orange
	{170, 170, 170},  // 10 Grey 2
	{255, 153, 136},  // 11 Pink
	{17, 221, 0},     // 12 Green (Light Green)
	{255, 255, 0},    // 13 Yellow
	{68, 255, 153},   // 14 Aquamarine
	{255, 255, 255},  // 15 White
}

// RenderLoRes renders the 40×48 lo-res graphics screen.
// Each byte in text page memory encodes two vertically stacked blocks:
//   - Low nibble  → top block color
//   - High nibble → bottom block color
// Each block is 7×4 pixels on screen (same char cell width, half height).
//
// If mixed mode is on, the bottom 4 text rows (rows 20–23, lo-res rows 40–47)
// are rendered as text instead.
func (v *Video) RenderLoRes(mixed bool) {
	maxLoResRow := 48
	if mixed {
		maxLoResRow = 40 // stop before the text area
	}

	for row := 0; row < 24; row++ {
		baseAddr := textLineAddr(row)

		for col := 0; col < TextCols; col++ {
			b := v.RAM[baseAddr+uint16(col)]
			topColor := b & 0x0F
			botColor := (b >> 4) & 0x0F

			loResRow := row * 2
			// Top block (4 scanlines)
			if loResRow < maxLoResRow {
				v.fillBlock(col, row*CharH, CharW, 4, loResColors[topColor])
			}
			// Bottom block (4 scanlines)
			if loResRow+1 < maxLoResRow {
				v.fillBlock(col, row*CharH+4, CharW, 4, loResColors[botColor])
			}
		}
	}

	// In mixed mode, render the bottom 4 text rows
	if mixed {
		v.renderTextRows(20, 24)
	}
}

// fillBlock fills a rectangular area with a solid color.
func (v *Video) fillBlock(col, py, w, h int, color [3]uint8) {
	for y := py; y < py+h && y < ScreenH; y++ {
		for x := col * CharW; x < col*CharW+w && x < ScreenW; x++ {
			offset := (y*ScreenW + x) * 4
			v.Pixels[offset+0] = color[0]
			v.Pixels[offset+1] = color[1]
			v.Pixels[offset+2] = color[2]
			v.Pixels[offset+3] = 0xFF
		}
	}
}
