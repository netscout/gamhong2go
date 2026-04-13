package memory

import (
	"fmt"
	"os"
)

// ROM is a read-only memory region loaded from a binary file.
// It maps into the address space starting at Base.
type ROM struct {
	Data []uint8
	Base uint16 // start address in the CPU address space
}

// LoadROM reads a binary file and creates a ROM mapped at base.
// The ROM occupies [base, base+len(file)-1].
func LoadROM(path string, base uint16) (*ROM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load ROM %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("load ROM %s: file is empty", path)
	}

	// Reject if the ROM overflows the 64KB address space
	end := int(base) + len(data) - 1
	if end > 0xFFFF {
		return nil, fmt.Errorf("load ROM %s: %d bytes at $%04X overflows address space",
			path, len(data), base)
	}

	return &ROM{Data: data, Base: base}, nil
}

// Read returns the byte at addr. The caller (bus) guarantees addr is in range.
func (r *ROM) Read(addr uint16) uint8 {
	offset := addr - r.Base
	if int(offset) < len(r.Data) {
		return r.Data[offset]
	}
	return 0xFF
}

// Write is a no-op — ROM is read-only.
func (r *ROM) Write(addr uint16, val uint8) {}

// Size returns the number of bytes in the ROM image.
func (r *ROM) Size() int {
	return len(r.Data)
}

// End returns the last address occupied by this ROM (inclusive).
func (r *ROM) End() uint16 {
	return r.Base + uint16(len(r.Data)) - 1
}
