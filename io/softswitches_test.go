package io

import (
	"testing"
)

func TestKeyboardStrobe(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)

	// No key pressed — bit 7 clear
	val := sw.Read(0xC000)
	if val&0x80 != 0 {
		t.Fatal("strobe should be clear before any keypress")
	}

	// Press 'A'
	sw.PressKey('A')
	val = sw.Read(0xC000)
	if val != ('A' | 0x80) {
		t.Fatalf("expected 0x%02X, got 0x%02X", 'A'|0x80, val)
	}

	// Clear strobe by reading $C010
	sw.Read(0xC010)
	val = sw.Read(0xC000)
	if val&0x80 != 0 {
		t.Fatal("strobe should be clear after reading $C010")
	}
}

func TestGraphicsModes(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)

	// Default is text mode
	if !sw.TextMode {
		t.Fatal("should start in text mode")
	}

	// Switch to graphics
	sw.Read(0xC050)
	if sw.TextMode {
		t.Fatal("should be in graphics mode after reading $C050")
	}

	// Switch back to text
	sw.Read(0xC051)
	if !sw.TextMode {
		t.Fatal("should be in text mode after reading $C051")
	}

	// Hi-res on
	sw.Read(0xC057)
	if !sw.HiRes {
		t.Fatal("hi-res should be on")
	}
}

func TestPaddleTimerActive(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)
	sw.SetPaddle(0, 128)
	// Trigger at cyc=0
	cyc = 0
	sw.Read(0xC070)
	// Immediately: bit 7 should be set (timer active)
	if got := sw.Read(0xC064); got != 0x80 {
		t.Fatalf("expected $80 at cyc=0, got $%02X", got)
	}
	// Halfway (11 + 11*128 = 1419, halfway ~710): still active
	cyc = 710
	if got := sw.Read(0xC064); got != 0x80 {
		t.Fatalf("expected $80 at cyc=710, got $%02X", got)
	}
}

func TestPaddleTimerExpires(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)
	sw.SetPaddle(0, 128) // T = 11 + 11*128 = 1419
	cyc = 0
	sw.Read(0xC070)
	// At expiry: bit 7 should be clear
	cyc = 1419
	if got := sw.Read(0xC064); got != 0x00 {
		t.Fatalf("expected $00 at cyc=1419, got $%02X", got)
	}
	cyc = 5000
	if got := sw.Read(0xC064); got != 0x00 {
		t.Fatalf("expected $00 long after expiry, got $%02X", got)
	}
}

func TestPaddleTriggerRearmsAll(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)
	for i := 0; i < 4; i++ {
		sw.SetPaddle(i, uint8(i*64)) // 0, 64, 128, 192
	}
	cyc = 100
	sw.Read(0xC070)
	// Paddle 0: T = 11 + 11*0 = 11; at cyc=100+11=111 it expires
	cyc = 100 + 11 + 11*0
	if got := sw.Read(0xC064); got != 0x00 {
		t.Fatalf("paddle 0: expected $00 at expiry, got $%02X", got)
	}
	// Paddle 1: T = 11 + 11*64 = 715; one cycle before expiry still active
	cyc = 100 + 11 + 11*64 - 1
	if got := sw.Read(0xC065); got != 0x80 {
		t.Fatalf("paddle 1: expected $80 one cycle before expiry, got $%02X", got)
	}
}

func TestPaddleTriggerOnWrite(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)
	sw.SetPaddle(0, 255)
	cyc = 100
	sw.Write(0xC070, 0x00)
	// T = 11 + 11*255 = 2816; one cycle before expiry should still be active
	cyc = 100 + 11 + 11*255 - 1
	if got := sw.Read(0xC064); got != 0x80 {
		t.Fatalf("expected $80 before expiry, got $%02X", got)
	}
}

func TestPaddleDefaultsCentered(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)
	// Without SetPaddle, default pos must be 128 (T = 1419 cycles)
	cyc = 0
	sw.Read(0xC070)
	cyc = 11 + 11*128 - 1
	if got := sw.Read(0xC064); got != 0x80 {
		t.Fatalf("centered paddle should be active near expiry, got $%02X", got)
	}
	cyc = 11 + 11*128
	if got := sw.Read(0xC064); got != 0x00 {
		t.Fatalf("centered paddle should expire at 1419, got $%02X", got)
	}
}

func TestVBLBitIIe(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)

	// cyc = 0: beginning of active scan, bit 7 CLEAR (IIe polarity).
	cyc = 0
	if got := sw.Read(0xC019) & 0x80; got != 0x00 {
		t.Fatalf("at cyc=0 expected bit7=0 (active scan, IIe), got $%02X", got)
	}

	// cyc = 12479 (last visible cycle): still active, bit 7 still 0.
	cyc = 12479
	if got := sw.Read(0xC019) & 0x80; got != 0x00 {
		t.Fatalf("at cyc=12479 expected bit7=0, got $%02X", got)
	}

	// cyc = 12480 (first VBL cycle): bit 7 SET.
	cyc = 12480
	if got := sw.Read(0xC019) & 0x80; got != 0x80 {
		t.Fatalf("at cyc=12480 expected bit7=1 (VBL, IIe), got $%02X", got)
	}

	// cyc = 17029 (last VBL cycle): bit 7 still SET.
	cyc = 17029
	if got := sw.Read(0xC019) & 0x80; got != 0x80 {
		t.Fatalf("at cyc=17029 expected bit7=1, got $%02X", got)
	}

	// cyc = 17030 (second frame start): active again, bit 7 CLEAR.
	cyc = 17030
	if got := sw.Read(0xC019) & 0x80; got != 0x00 {
		t.Fatalf("at cyc=17030 expected bit7=0 (new frame active), got $%02X", got)
	}
}

func TestVBLWraps(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)

	// 1,000,000 frames in. cyc % 17030 = 100 → active region.
	cyc = 1_000_000*17030 + 100
	if got := sw.Read(0xC019) & 0x80; got != 0x00 {
		t.Fatalf("expected active (bit7=0) after 1M frames, got $%02X", got)
	}

	// cyc % 17030 = 13000 → VBL region.
	cyc = 1_000_000*17030 + 13000
	if got := sw.Read(0xC019) & 0x80; got != 0x80 {
		t.Fatalf("expected VBL (bit7=1) after 1M frames, got $%02X", got)
	}
}

func TestButtonPressRelease(t *testing.T) {
	var cyc uint64
	sw := NewSoftSwitches(&cyc)
	if got := sw.Read(0xC061); got != 0x00 {
		t.Fatalf("button 0 should start unpressed, got $%02X", got)
	}
	sw.PressButton(0)
	if got := sw.Read(0xC061); got != 0x80 {
		t.Fatalf("button 0 pressed: expected $80, got $%02X", got)
	}
	sw.ReleaseButton(0)
	if got := sw.Read(0xC061); got != 0x00 {
		t.Fatalf("button 0 released: expected $00, got $%02X", got)
	}
	// Button 0 press must not affect buttons 1 and 2
	sw.PressButton(0)
	if got := sw.Read(0xC062); got != 0x00 {
		t.Fatalf("button 1 should remain unpressed, got $%02X", got)
	}
	if got := sw.Read(0xC063); got != 0x00 {
		t.Fatalf("button 2 should remain unpressed, got $%02X", got)
	}
}
