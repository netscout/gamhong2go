package disk

// Switches adapts a *Controller to the bus.Device interface.
// Mapped at $C0E0-$C0EF (slot 6 softswitch window).
// Map AFTER io.SoftSwitches so this device wins the address-decode race
// (bus last-mapping-wins; see bus.go).
type Switches struct{ c *Controller }

// NewSwitches returns a bus.Device shim for the Disk II softswitches.
func NewSwitches(c *Controller) *Switches { return &Switches{c: c} }

// Read triggers the strobe and returns the data register for $C0EC reads.
func (s *Switches) Read(addr uint16) uint8 {
	// $C0ED write in Q7=1 mode loads writeReg; on read Q6H just strobes.
	if addr&0x0F == 0x0D && s.c.q7 {
		s.c.q6 = true
		return 0
	}
	return s.c.Strobe(addr)
}

// Write also triggers the strobe (all 16 addresses are chip-select strobes).
// For $C0ED in write mode, latch the data byte into writeReg.
func (s *Switches) Write(addr uint16, v uint8) {
	if addr&0x0F == 0x0D && s.c.q7 {
		s.c.Q6WriteReg(v)
		s.c.q6 = true
		return
	}
	s.c.Strobe(addr)
}
