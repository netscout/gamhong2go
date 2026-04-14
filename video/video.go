package video

// Screen dimensions for the Apple II display.
const (
	TextCols = 40
	TextRows = 24
	CharW    = 7                // pixels per character width
	CharH    = 8                // pixels per character height
	ScreenW  = TextCols * CharW // 280 pixels
	ScreenH  = TextRows * CharH // 192 pixels
)

// Colors (RGBA) — Apple II phosphor green on black.
var (
	ColorOn  = [4]uint8{0x33, 0xFF, 0x33, 0xFF} // bright green
	ColorOff = [4]uint8{0x00, 0x00, 0x00, 0xFF} // black
)

// Video renders Apple II display modes into a pixel buffer.
type Video struct {
	RAM          []uint8 // Direct reference to main RAM (64 KB)
	Pixels       []uint8 // RGBA pixel buffer (280×192×4 bytes)
	FlashState   bool    // Toggled at ~1.875 Hz for flashing characters
	flashCounter int     // Frame counter for flash timing
}

// NewVideo creates a video renderer attached to the given RAM.
func NewVideo(ram []uint8) *Video {
	return &Video{
		RAM:    ram,
		Pixels: make([]uint8, ScreenW*ScreenH*4),
	}
}

// RenderText renders the 40×24 text screen (page 1 at $0400–$07FF)
// into the Pixels buffer.
func (v *Video) RenderText() {
	// Update flash state: toggle every 16 frames (~1.875 Hz at 60 fps)
	v.flashCounter++
	if v.flashCounter >= 16 {
		v.flashCounter = 0
		v.FlashState = !v.FlashState
	}

	for row := 0; row < TextRows; row++ {
		baseAddr := textLineAddr(row)

		for col := 0; col < TextCols; col++ {
			screenByte := v.RAM[baseAddr+uint16(col)]

			for scanline := 0; scanline < CharH; scanline++ {
				pixels, inverse := CharGenROM(screenByte, scanline)

				// Handle flashing: characters in $40–$7F range
				if screenByte >= 0x40 && screenByte < 0x80 && v.FlashState {
					inverse = true
				}

				// Render 7 pixels for this scanline of this character
				py := row*CharH + scanline
				for px := 0; px < CharW; px++ {
					// Bit 0 = leftmost pixel
					lit := (pixels>>uint(px))&1 != 0
					if inverse {
						lit = !lit
					}

					offset := (py*ScreenW + col*CharW + px) * 4
					if lit {
						v.Pixels[offset+0] = ColorOn[0]
						v.Pixels[offset+1] = ColorOn[1]
						v.Pixels[offset+2] = ColorOn[2]
						v.Pixels[offset+3] = ColorOn[3]
					} else {
						v.Pixels[offset+0] = ColorOff[0]
						v.Pixels[offset+1] = ColorOff[1]
						v.Pixels[offset+2] = ColorOff[2]
						v.Pixels[offset+3] = ColorOff[3]
					}
				}
			}
		}
	}
}

// textLineAddr returns the base address for a given text row (0–23).
// The Apple II text screen has a notoriously non-linear memory layout:
//
//	Row 0 → $0400    Row 8  → $0428    Row 16 → $0450
//	Row 1 → $0480    Row 9  → $04A8    Row 17 → $04D0
//	Row 2 → $0500    Row 10 → $0528    Row 18 → $0550
//	Row 3 → $0580    Row 11 → $05A8    Row 19 → $05D0
//	Row 4 → $0600    Row 12 → $0628    Row 20 → $0650
//	Row 5 → $0680    Row 13 → $06A8    Row 21 → $06D0
//	Row 6 → $0700    Row 14 → $0728    Row 22 → $0750
//	Row 7 → $0780    Row 15 → $07A8    Row 23 → $07D0
func textLineAddr(row int) uint16 {
	return 0x0400 + uint16(row%8)*0x80 + uint16(row/8)*0x28
}
