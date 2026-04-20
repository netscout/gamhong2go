package languageCard

// Card is the Apple II Language Card (16 KB RAM, slot 0).
// Overlays the ROM at $D000-$FFFF under softswitch control.
//
// Memory layout: 4 KB bank1 + 4 KB bank2 (both map to $D000-$DFFF;
// one selected at a time) + 8 KB shared for $E000-$FFFF.
// Total 16 KB. Bank1 and bank2 share the same address window, which
// is why the card holds 16 KB of RAM overlaying only 12 KB of address
// space.
type Card struct {
	// RAM backing store. Kept as three distinct slices rather than a
	// single 16 KB buffer so the hot path stays branch-light: the
	// read/write selector only picks bank1 vs bank2 for $D000-$DFFF;
	// $E000-$FFFF is a direct slice.
	bank1 [0x1000]uint8 // $D000-$DFFF bank 1 (4 KB)
	bank2 [0x1000]uint8 // $D000-$DFFF bank 2 (4 KB)
	upper [0x2000]uint8 // $E000-$FFFF shared (8 KB)

	// Softswitch state (see §2 state model in plan-language-card.md).
	ramRead  bool
	ramWrite bool
	bank2Sel bool // true = bank2 selected at $D000-$DFFF; false = bank1
	prearm   bool

	// rom is the ROM device the card overlays. On ROM reads we
	// delegate here, so the LC remains responsible for ROM-vs-RAM
	// mux and the rest of the system doesn't need to know. Keeping
	// a direct pointer (vs re-querying the bus) avoids recursion
	// hazards — the bus maps the LC at the same range the ROM
	// originally held, and the LC owns the fallthrough.
	rom ROMReader
}

// ROMReader is the minimal interface the card needs from the ROM
// device it overlays. memory.ROM implements this (memory/rom.go:37).
type ROMReader interface {
	Read(addr uint16) uint8
}

// NewCard returns a zero-initialised card overlaying rom at
// $D000-$FFFF. Initial state matches Apple II+ power-on (ROM-read,
// bank2, write-disabled). rom must be non-nil.
func NewCard(rom ROMReader) *Card {
	c := &Card{rom: rom}
	c.Reset()
	return c
}

// Reset returns the card to Apple II+ power-on state. Does NOT clear
// RAM contents (matches hardware: RAM retains across reset).
func (c *Card) Reset() {
	c.ramRead = false
	c.ramWrite = false
	c.bank2Sel = true
	c.prearm = false
}

// Read implements bus.Device for $D000-$FFFF.
// Priority: if ramRead, route to LC RAM; else delegate to ROM.
func (c *Card) Read(addr uint16) uint8 {
	if c.ramRead {
		return c.ramSlot(addr)
	}
	return c.rom.Read(addr)
}

// Write implements bus.Device for $D000-$FFFF.
// If ramWrite, route to LC RAM; else ignore (ROM-style no-op).
func (c *Card) Write(addr uint16, val uint8) {
	if !c.ramWrite {
		return
	}
	switch {
	case addr >= 0xE000:
		c.upper[addr-0xE000] = val
	case c.bank2Sel:
		c.bank2[addr-0xD000] = val
	default:
		c.bank1[addr-0xD000] = val
	}
}

// ramSlot picks the correct RAM bank for addr.
func (c *Card) ramSlot(addr uint16) uint8 {
	switch {
	case addr >= 0xE000: // $E000-$FFFF shared
		return c.upper[addr-0xE000]
	case c.bank2Sel:
		return c.bank2[addr-0xD000]
	default:
		return c.bank1[addr-0xD000]
	}
}

// handleSwitch implements the §2 truth table for $C080-$C08F.
// low = addr & 0x0F; isWrite = true when the CPU is writing to $C08X.
//
// Bank select: bit 3 clear = bank2, set = bank1. Applied on every access,
// regardless of low-2 bits and regardless of read vs write.
//
// Read-select + write-arm logic follows low bits 0..1.
// Mapping derived from the §2 truth table (plan-language-card.md):
//
//	case 0: RAM read, disable write  — READBSR{1,2}  per §2 table
//	case 1: ROM read, arm/commit write — WRITEBSR{1,2} per §2 table
//	case 2: ROM read, disable write  — OFFBSR{1,2}   per §2 table
//	case 3: RAM read, arm/commit write — RDWRBSR{1,2} per §2 table
func (c *Card) handleSwitch(low uint8, isWrite bool) {
	// Bit 3: bank select — clear = bank2, set = bank1.
	c.bank2Sel = (low & 0x08) == 0

	switch low & 0x03 {
	case 0: // $C080/$C084/$C088/$C08C — READBSR2/READBSR1 per §2 table: ramRead=true, disable write
		c.ramRead = true
		c.ramWrite = false
		c.prearm = false

	case 1: // $C081/$C085/$C089/$C08D — WRITEBSR2/WRITEBSR1 per §2 table: ramRead=false, arm/commit write
		c.ramRead = false
		if isWrite {
			// Write access: clear prearm and ramWrite unconditionally.
			// Writes to $C080-$C08F never enable write-to-RAM.
			c.ramWrite = false
			c.prearm = false
		} else {
			// Read access: step the prearm/commit dance.
			if c.prearm {
				c.ramWrite = true
			}
			c.prearm = true
		}

	case 2: // $C082/$C086/$C08A/$C08E — OFFBSR2/OFFBSR1 per §2 table: ramRead=false, disable write
		c.ramRead = false
		c.ramWrite = false
		c.prearm = false

	case 3: // $C083/$C087/$C08B/$C08F — RDWRBSR2/RDWRBSR1 per §2 table: ramRead=true, arm/commit write
		c.ramRead = true
		if isWrite {
			// Write access: clear prearm and ramWrite unconditionally.
			c.ramWrite = false
			c.prearm = false
		} else {
			// Read access: step the prearm/commit dance.
			if c.prearm {
				c.ramWrite = true
			}
			c.prearm = true
		}
	}
}
