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
