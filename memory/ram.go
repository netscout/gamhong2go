package memory

// RAM represents the Apple II's main memory (48 KB, expandable).
// It stores data at the address directly — the bus handles range checking.
type RAM struct {
	Data [0x10000]uint8 // Full 64 KB backing store for simplicity
}

// NewRAM returns an initialised RAM (all zeroes).
func NewRAM() *RAM {
	return &RAM{}
}

// Read returns the byte at addr.
func (r *RAM) Read(addr uint16) uint8 {
	return r.Data[addr]
}

// Write stores val at addr.
func (r *RAM) Write(addr uint16, val uint8) {
	r.Data[addr] = val
}
