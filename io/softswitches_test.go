package io

import (
	"testing"
)

func TestKeyboardStrobe(t *testing.T) {
	sw := NewSoftSwitches()

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
	sw := NewSoftSwitches()

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
