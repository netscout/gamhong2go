package io

// SoftSwitches handles the Apple II I/O space at $C000–$C0FF.
// For now this is a minimal stub that returns sensible defaults
// so the ROM boot code doesn't hang. Keyboard input and graphics
// mode switching will be filled in during later iterations.
type SoftSwitches struct {
	// Keyboard
	KeyData  uint8 // last key pressed (bit 7 = strobe)
	KeyReady bool  // true when a new key is available

	// Graphics mode flags (soft switch state)
	TextMode  bool // true = text, false = graphics
	MixedMode bool // true = mixed (4 lines of text at bottom)
	Page2     bool // true = display page 2
	HiRes     bool // true = hi-res, false = lo-res
}

// NewSoftSwitches returns I/O in the default boot state (text mode).
func NewSoftSwitches() *SoftSwitches {
	return &SoftSwitches{
		TextMode: true,
	}
}

// Read handles soft switch reads. Many switches are "read-triggered" —
// simply reading the address changes hardware state.
func (s *SoftSwitches) Read(addr uint16) uint8 {
	switch addr {
	// Keyboard (delivers the key data and strobe bit)
	case 0xC000:
		val := s.KeyData
		if s.KeyReady {
			val |= 0x80 // set the strobe bit(10000000 in binary)
		}
		return val
	case 0xC010:
		// Clear keyboard strobe
		s.KeyReady = false
		return s.KeyData & 0x7F // returning only the ASCII data (masking off with 01111111 in binary)

	// Text/Graphics mode switches (accent on read)
	case 0xC050:
		s.TextMode = false
		return 0
	case 0xC051:
		s.TextMode = true
		return 0
	case 0xC052:
		s.MixedMode = false
		return 0
	case 0xC053:
		s.MixedMode = true
		return 0
	case 0xC054:
		s.Page2 = false
		return 0
	case 0xC055:
		s.Page2 = true
		return 0
	case 0xC056:
		s.HiRes = false
		return 0
	case 0xC057:
		s.HiRes = true
		return 0

	// Speaker toggle (stub — no audio yet)
	case 0xC030:
		return 0

	// Annunciator and paddle stubs
	case 0xC061, 0xC062, 0xC063: // push buttons
		return 0 // not pressed
	case 0xC064, 0xC065, 0xC066, 0xC067: // paddle timers
		return 0 // expired
	}

	return 0x00
}

// Write handles soft switch writes. Most switches respond to both
// reads and writes at the same addresses.
func (s *SoftSwitches) Write(addr uint16, val uint8) {
	// The same switch addresses respond to writes too.
	s.Read(addr)
}

// PressKey simulates a keypress. Sets the key data and strobe.
func (s *SoftSwitches) PressKey(key uint8) {
	s.KeyData = key & 0x7F // store raw key; Read() adds strobe from KeyReady
	s.KeyReady = true
}
