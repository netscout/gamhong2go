package bus

import (
	"testing"
)

// testDevice is a minimal device for testing.
type testDevice struct {
	data [65536]uint8
	name string
}

func (d *testDevice) Read(addr uint16) uint8       { return d.data[addr] }
func (d *testDevice) Write(addr uint16, val uint8) { d.data[addr] = val }

func TestBasicRouting(t *testing.T) {
	b := NewBus()
	ram := &testDevice{name: "ram"}
	rom := &testDevice{name: "rom"}

	b.Map(0x0000, 0xBFFF, ram)
	b.Map(0xD000, 0xFFFF, rom)

	// Write to RAM region
	b.Write(0x0042, 0xAB)
	if ram.data[0x0042] != 0xAB {
		t.Fatal("RAM write failed")
	}

	// Read from ROM region
	rom.data[0xFFFC] = 0x00
	rom.data[0xFFFD] = 0x04
	if b.Read(0xFFFC) != 0x00 || b.Read(0xFFFD) != 0x04 {
		t.Fatal("ROM read failed")
	}
}

func TestPriority(t *testing.T) {
	b := NewBus()
	ram := &testDevice{name: "ram"}
	rom := &testDevice{name: "rom"}

	// Map RAM first, then ROM on top for same region.
	b.Map(0x0000, 0xFFFF, ram)
	b.Map(0xD000, 0xFFFF, rom)

	// Write goes to ROM (higher priority), but ROM ignores it in practice.
	// Read from the overlapping region should come from ROM.
	ram.data[0xD000] = 0x11
	rom.data[0xD000] = 0x22
	val := b.Read(0xD000)
	if val != 0x22 {
		t.Fatalf("expected ROM value 0x22, got 0x%02X", val)
	}

	// Read from RAM-only region still works.
	ram.data[0x0042] = 0x99
	if b.Read(0x0042) != 0x99 {
		t.Fatal("RAM read in non-overlapping region failed")
	}
}

func TestOpenBus(t *testing.T) {
	b := NewBus()
	// No devices mapped — should return $FF (open bus).
	if b.Read(0x1234) != 0xFF {
		t.Fatal("open bus should return 0xFF")
	}
}
