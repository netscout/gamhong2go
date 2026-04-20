package languageCard

// Switches adapts a *Card to the bus.Device interface at $C080-$C08F.
// Map AFTER io.SoftSwitches so this device wins the address decode
// for that range. bus.Bus implements last-mapping-wins in its Read
// and Write loops (bus/bus.go:40-42 walks b.mappings backwards).
type Switches struct{ c *Card }

// NewSwitches returns the softswitch shim. Map at $C080-$C08F.
func NewSwitches(c *Card) *Switches { return &Switches{c: c} }

// Read handles $C080-$C08F read-access side effects.
// Returns 0. Real II+ hardware returns the floating-bus byte (most
// recent video-scan byte); returning 0 is a known divergence — see
// §7 risks in plan-language-card.md.
func (s *Switches) Read(addr uint16) uint8 {
	s.c.handleSwitch(uint8(addr&0x0F), false /*isWrite*/)
	return 0
}

// Write handles $C080-$C08F write-access side effects.
// Value is ignored (the softswitches are address-strobed, not
// data-latched).
func (s *Switches) Write(addr uint16, _ uint8) {
	s.c.handleSwitch(uint8(addr&0x0F), true /*isWrite*/)
}
