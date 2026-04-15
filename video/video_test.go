package video

import (
	"testing"
)

func TestTextLineAddresses(t *testing.T) {
	// Verify the non-linear memory layout matches known Apple II addresses.
	expected := []uint16{
		0x0400, 0x0480, 0x0500, 0x0580, 0x0600, 0x0680, 0x0700, 0x0780, // rows 0–7
		0x0428, 0x04A8, 0x0528, 0x05A8, 0x0628, 0x06A8, 0x0728, 0x07A8, // rows 8–15
		0x0450, 0x04D0, 0x0550, 0x05D0, 0x0650, 0x06D0, 0x0750, 0x07D0, // rows 16–23
	}

	for row, exp := range expected {
		got := textLineAddr(row)
		if got != exp {
			t.Errorf("row %d: expected $%04X, got $%04X", row, exp, got)
		}
	}
}

func TestRenderTextBasic(t *testing.T) {
	ram := make([]uint8, 65536)
	v := NewVideo(ram)

	// Put a normal 'A' ($C1 in Apple II encoding) at row 0, col 0.
	ram[0x0400] = 0xC1

	// Put a normal space ($A0) everywhere else — already zero, but let's
	// fill the rest of row 0 explicitly.
	for col := 1; col < 40; col++ {
		ram[0x0400+uint16(col)] = 0xA0
	}

	v.RenderText()

	// The 'A' character should have some lit pixels in the top-left area.
	// Character 'A' row 0 = 0x08 (bit 3 set = pixel at x=3).
	// Check pixel at (3, 0) — should be green.
	offset := (0*ScreenW + 3) * 4
	if v.Pixels[offset] != ColorOn[0] {
		t.Error("expected lit pixel at (3,0) for 'A'")
	}

	// Check pixel at (0, 0) — should be black (bit 0 of 'A' row 0 is 0).
	offset = 0
	if v.Pixels[offset] != ColorOff[0] {
		t.Error("expected dark pixel at (0,0) for 'A'")
	}
}

func TestCharGenInverse(t *testing.T) {
	// Screen byte $01 = inverse 'A' (0x01 + 0x40 = 0x41 = 'A', inverted)
	pixels, inverse := CharGenROM(0x01, 0)
	if !inverse {
		t.Error("$01 should be inverse")
	}
	_ = pixels

	// Screen byte $C1 = normal 'A'
	_, inverse = CharGenROM(0xC1, 0)
	if inverse {
		t.Error("$C1 should be normal (not inverse)")
	}
}

func TestScreenDimensions(t *testing.T) {
	if ScreenW != 280 {
		t.Errorf("expected width 280, got %d", ScreenW)
	}
	if ScreenH != 192 {
		t.Errorf("expected height 192, got %d", ScreenH)
	}
}

func TestHiResLineAddresses(t *testing.T) {
	// Verify known hi-res line addresses (page 1).
	// Layout: addr = $2000 + (line&7)*$400 + ((line>>3)&7)*$80 + (line>>6)*$28
	tests := []struct {
		line int
		addr uint16
	}{
		{0, 0x2000},   // c=0, b=0, a=0
		{1, 0x2400},   // c=1, b=0, a=0
		{7, 0x3C00},   // c=7, b=0, a=0
		{8, 0x2080},   // c=0, b=1, a=0
		{64, 0x2028},  // c=0, b=0, a=1
		{128, 0x2050}, // c=0, b=0, a=2
		{191, 0x3FD0}, // c=7, b=7, a=2
	}

	for _, tc := range tests {
		got := hiResLineAddr(tc.line, false)
		if got != tc.addr {
			t.Errorf("line %d: expected $%04X, got $%04X", tc.line, tc.addr, got)
		}
	}
}

func TestHiResPage2(t *testing.T) {
	// Page 2 starts at $4000 instead of $2000.
	got := hiResLineAddr(0, true)
	if got != 0x4000 {
		t.Errorf("page 2 line 0: expected $4000, got $%04X", got)
	}
}

func TestLoResRendering(t *testing.T) {
	ram := make([]uint8, 65536)
	v := NewVideo(ram)

	// Write a byte to row 0, col 0: low nibble = 1 (magenta top), high nibble = 12 (green bottom)
	ram[0x0400] = 0xC1 // top=1 (magenta), bottom=12 (green)

	v.RenderLoRes(false)

	// Check top-left pixel — should be magenta (color 1)
	offset := 0 // pixel (0,0)
	if v.Pixels[offset] != loResColors[1][0] ||
		v.Pixels[offset+1] != loResColors[1][1] ||
		v.Pixels[offset+2] != loResColors[1][2] {
		t.Errorf("expected magenta at (0,0), got RGB(%d,%d,%d)",
			v.Pixels[offset], v.Pixels[offset+1], v.Pixels[offset+2])
	}

	// Check pixel at row 4 (bottom half of block) — should be green (color 12)
	offset = (4*ScreenW + 0) * 4
	if v.Pixels[offset] != loResColors[12][0] ||
		v.Pixels[offset+1] != loResColors[12][1] ||
		v.Pixels[offset+2] != loResColors[12][2] {
		t.Errorf("expected green at (0,4), got RGB(%d,%d,%d)",
			v.Pixels[offset], v.Pixels[offset+1], v.Pixels[offset+2])
	}
}

func TestRenderDispatch(t *testing.T) {
	ram := make([]uint8, 65536)
	v := NewVideo(ram)

	// Fill text page with spaces ($A0)
	for i := uint16(0x0400); i < 0x0800; i++ {
		ram[i] = 0xA0
	}

	// Should not panic in any mode
	v.Render(true, false, false, false)  // text mode
	v.Render(false, false, false, false) // lo-res
	v.Render(false, true, false, false)  // lo-res mixed
	v.Render(false, false, true, false)  // hi-res
	v.Render(false, true, true, false)   // hi-res mixed
	v.Render(false, false, true, true)   // hi-res page 2
}
