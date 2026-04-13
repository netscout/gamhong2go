package bus

import (
	"fmt"
)

// Device is anything that can be mapped into the address space.
type Device interface {
	Read(addr uint16) uint8
	Write(addr uint16, val uint8)
}

// mapping ties a Device to a contiguous address range.
type mapping struct {
	start  uint16
	end    uint16 // inclusive
	device Device
}

// Bus is the Apple II address decoder. It implements cpu.Memory and routes
// every read/write to the device that owns that address range.
type Bus struct {
	mappings []mapping
}

// NewBus returns an empty bus. Attach devices with Map() before use.
func NewBus() *Bus {
	return &Bus{}
}

// Map registers a device for the address range [start, end] (inclusive).
// Later mappings take priority over earlier ones for overlapping ranges,
// so you can map RAM for the full range first, then overlay ROM on top.
func (b *Bus) Map(start, end uint16, dev Device) {
	b.mappings = append(b.mappings, mapping{start, end, dev})
}

// Read finds the last-registered device whose range contains addr.
func (b *Bus) Read(addr uint16) uint8 {
	// Walk backwards so later mappings (higher priority) win.
	for i := len(b.mappings) - 1; i >= 0; i-- {
		m := &b.mappings[i]
		if addr >= m.start && addr <= m.end {
			return m.device.Read(addr)
		}
	}
	return 0xFF // open bus
}

// Write finds the last-registered device whose range contains addr.
// ROM devices simply ignore writes internally.
func (b *Bus) Write(addr uint16, val uint8) {
	for i := len(b.mappings) - 1; i >= 0; i-- {
		m := &b.mappings[i]
		if addr >= m.start && addr <= m.end {
			m.device.Write(addr, val)
			return
		}
	}
}

// Dump prints the current device map for debugging.
func (b *Bus) Dump() {
	for _, m := range b.mappings {
		fmt.Printf("  $%04X–$%04X  %T\n", m.start, m.end, m.device)
	}
}
