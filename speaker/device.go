package speaker

// Device adapts a *Speaker to the bus.Device interface. Mapped at
// $C030-$C03F (the speaker strobe mirrors across that 16-byte window
// on real Apple II hardware). Map AFTER io.SoftSwitches so this
// device wins the address-decode race (see bus.go: last mapping wins).
type Device struct{ s *Speaker }

func NewDevice(s *Speaker) *Device { return &Device{s: s} }

func (d *Device) Read(addr uint16) uint8        { d.s.Toggle(); return 0 }
func (d *Device) Write(addr uint16, v uint8)    { d.s.Toggle() }
