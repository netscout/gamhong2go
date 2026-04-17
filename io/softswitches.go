package io

// Video timing constants used by the $C019 VBL softswitch.
//
// Real NTSC Apple II video frame: 262 scanlines × 65 cycles/line = 17030.
// Visible region: 192 × 65 = 12480. VBL region: cycles [12480, 17030).
//
// NOT to be confused with main.cyclesPerFrame (the CPU slice budget,
// 17050), which drives the speaker's resampling math and is deliberately
// 20 cycles longer than the real NTSC video frame. See main.go.
const (
	cyclesPerVideoFrame = 17030
	visibleCycles       = 12480
)

// SoftSwitches handles the Apple II I/O space at $C000–$C0FF.
// Keyboard input, graphics mode switching, and paddle/joystick 555-timer
// emulation are all handled here.
//
// Paddle emulation: writing or reading $C070 re-arms all four 555 timers.
// $C064–$C067 return bit 7 = 1 while the timer is still counting, 0 after
// expiry. Timer duration = 11 + 11*paddlePos[i] CPU cycles (Apple II spec).
// Default paddle positions are 128 (center) so games start in the dead zone.
type SoftSwitches struct {
	// Keyboard
	KeyData  uint8 // last key pressed (bit 7 = strobe)
	KeyReady bool  // true when a new key is available

	// Graphics mode flags (soft switch state)
	TextMode  bool // true = text, false = graphics
	MixedMode bool // true = mixed (4 lines of text at bottom)
	Page2     bool // true = display page 2
	HiRes     bool // true = hi-res, false = lo-res

	// Paddle / joystick (555-timer emulation)
	// paddlePos[i] = virtual pot position 0..255. Default 128 (center).
	paddlePos [4]uint8
	// paddleExpiry[i] = absolute CPU cycle when the 555 timer expires.
	// While *cyclePtr < paddleExpiry[i], $C064+i returns 0x80 (bit 7 set).
	// Zero means "already expired" (safe boot default).
	paddleExpiry [4]uint64
	// button[i] = true while button i is held down.
	// Maps to $C061 (i=0), $C062 (i=1), $C063 (i=2) bit 7.
	button [3]bool
	// cyclePtr is an injected pointer to a monotonically-increasing CPU cycle
	// counter. Must be the same diskCycle used by the disk controller (NEVER
	// frameCycle, which resets each frame and would make paddle timing wrong).
	cyclePtr *uint64
}

// NewSoftSwitches returns I/O in the default boot state (text mode, paddles
// centered). cyclePtr must point to a monotonically-increasing cycle counter
// (diskCycle in main.go).
func NewSoftSwitches(cyclePtr *uint64) *SoftSwitches {
	return &SoftSwitches{
		TextMode:  true,
		cyclePtr:  cyclePtr,
		paddlePos: [4]uint8{128, 128, 128, 128},
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

	// Push buttons: bit 7 = 1 while button is held
	case 0xC061:
		return buttonByte(s.button[0])
	case 0xC062:
		return buttonByte(s.button[1])
	case 0xC063:
		return buttonByte(s.button[2])

	// Paddle timers: bit 7 = 1 while 555 timer is still counting
	case 0xC064, 0xC065, 0xC066, 0xC067:
		idx := int(addr - 0xC064)
		if s.cyclePtr != nil && *s.cyclePtr < s.paddleExpiry[idx] {
			return 0x80
		}
		return 0x00

	// $C019 — vertical-blanking status, IIe polarity (what games expect):
	//   bit 7 = 1 during VBL          (cycles 12480..17029 of each 17030-cycle frame)
	//   bit 7 = 0 during active scan  (cycles 0..12479)
	//
	// Note: Apple II / II+ has NO $C019 softswitch in the reference manual
	// (reads would return floating bus). We adopt IIe polarity because that's
	// what every game polling VBL expects (Choplifter, Lode Runner, Beagle
	// Bros demos, etc.). IIe polarity works for both models we care about;
	// no code change needed when IIe ROM support lands later.
	//
	// Derived from *cyclePtr (monotonic CPU cycle counter) so turbo mode
	// scales VBL rate with emulated-CPU speed, as real hardware would.
	//
	// IMPORTANT: cyclesPerVideoFrame is 17030 (real NTSC), distinct from
	// main.cyclesPerFrame (17050, the CPU slice budget). See const block
	// comment above; the 20-cycle gap is intentional and documented.
	case 0xC019:
		if s.cyclePtr == nil {
			// Defensive: in main, cyclePtr is always non-nil. Real hardware
			// would return floating bus; 0x00 is safer here than panicking.
			return 0x00
		}
		cyclePhase := *s.cyclePtr % cyclesPerVideoFrame
		if cyclePhase >= visibleCycles {
			return 0x80 // VBL
		}
		return 0x00 // active scan

	// Paddle trigger: re-arm all four 555 timers
	case 0xC070:
		if s.cyclePtr != nil {
			now := *s.cyclePtr
			for i := 0; i < 4; i++ {
				// T = 11 + 11 * position cycles (Apple II 555 timing spec).
				// At center (128): T = 11 + 11*128 = 1419 cycles.
				// At max   (255): T = 11 + 11*255 = 2816 cycles.
				s.paddleExpiry[i] = now + 11 + 11*uint64(s.paddlePos[i])
			}
		}
		return 0x00
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

// SetPaddle sets the virtual pot position for paddle i (0..3).
// Position 0..255 maps to the Apple II pot range.
// Default at boot is 128 (center dead zone).
func (s *SoftSwitches) SetPaddle(i int, pos uint8) {
	if i >= 0 && i < 4 {
		s.paddlePos[i] = pos
	}
}

// PressButton sets button i (0..2) to held. Reflected in $C061/$C062/$C063 bit 7.
func (s *SoftSwitches) PressButton(i int) {
	if i >= 0 && i < 3 {
		s.button[i] = true
	}
}

// ReleaseButton clears button i (0..2).
func (s *SoftSwitches) ReleaseButton(i int) {
	if i >= 0 && i < 3 {
		s.button[i] = false
	}
}

// buttonByte returns 0x80 if pressed, 0x00 otherwise.
// Apple II games read only bit 7 of button registers.
func buttonByte(pressed bool) uint8 {
	if pressed {
		return 0x80
	}
	return 0x00
}
