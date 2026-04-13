package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/apple2emu/bus"
	"github.com/apple2emu/cpu"
	appleio "github.com/apple2emu/io"
	"github.com/apple2emu/memory"
)

func main() {
	romPath := flag.String("rom", "roms/apple2plus.rom", "Path to Apple II ROM image")
	traceCount := flag.Int("trace", 200, "Number of instructions to trace (0 = run until loop)")
	flag.Parse()

	// --- Load ROM -----------------------------------------------------------
	// Apple II+ ROM: 12 KB mapped at $D000–$FFFF
	// Some dumps are 16 KB ($C000–$FFFF, includes slot ROM area)
	rom, err := loadROM(*romPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		fmt.Fprintln(os.Stderr, "To get started, place an Apple II+ ROM in the roms/ directory:")
		fmt.Fprintln(os.Stderr, "  mkdir -p roms")
		fmt.Fprintln(os.Stderr, "  cp /path/to/apple2plus.rom roms/")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Common ROM sizes:")
		fmt.Fprintln(os.Stderr, "  12288 bytes (12 KB) — standard Apple II+ ROM ($D000–$FFFF)")
		fmt.Fprintln(os.Stderr, "  16384 bytes (16 KB) — extended dump ($C000–$FFFF)")
		os.Exit(1)
	}

	// --- Wire the bus -------------------------------------------------------
	ram := memory.NewRAM()
	sw := appleio.NewSoftSwitches()
	b := bus.NewBus()

	// Base layer: RAM across the full space (ROM overlays on top)
	b.Map(0x0000, 0xBFFF, ram)

	// I/O soft switches
	b.Map(0xC000, 0xC0FF, sw)

	// Slot ROM area — empty for now (reads $00)
	// Later iterations will populate slots (e.g., Disk II in slot 6).

	// ROM overlay — sits on top of RAM in the ROM region
	b.Map(rom.Base, rom.End(), rom)

	fmt.Printf("Apple II Emulator — Iteration 2\n")
	fmt.Printf("ROM: %s (%d bytes at $%04X–$%04X)\n", *romPath, rom.Size(), rom.Base, rom.End())
	fmt.Println("Bus map:")
	b.Dump()
	fmt.Println()

	// --- Boot the CPU -------------------------------------------------------
	c := cpu.NewCPU(b)
	c.Reset()

	resetVec := uint16(b.Read(0xFFFC)) | uint16(b.Read(0xFFFD))<<8
	fmt.Printf("Reset vector: $%04X\n", resetVec)
	fmt.Printf("Starting CPU trace (%d instructions)...\n\n", *traceCount)

	// --- Trace execution ----------------------------------------------------
	prevPC := c.PC
	stuckCount := 0

	for i := 0; i < *traceCount || *traceCount == 0; i++ {
		pc := c.PC
		op := b.Read(pc)
		b1 := b.Read(pc + 1)
		b2 := b.Read(pc + 2)

		name := opNames[op]
		size := opSizes[op]

		// Format the raw bytes column
		var bytesStr string
		switch size {
		case 1:
			bytesStr = fmt.Sprintf("%02X      ", op)
		case 2:
			bytesStr = fmt.Sprintf("%02X %02X   ", op, b1)
		case 3:
			bytesStr = fmt.Sprintf("%02X %02X %02X", op, b1, b2)
		default:
			bytesStr = fmt.Sprintf("%02X      ", op)
		}

		fmt.Printf("%04X  %s  %-4s  A:%02X X:%02X Y:%02X SP:%02X P:%02X\n",
			pc, bytesStr, name, c.A, c.X, c.Y, c.SP, c.P)

		c.Step()

		// Detect stuck loops (JMP to self)
		if c.PC == prevPC {
			stuckCount++
			if stuckCount > 2 {
				fmt.Printf("\n--- CPU stuck at $%04X (JMP to self) ---\n", c.PC)
				break
			}
		} else {
			stuckCount = 0
		}
		prevPC = c.PC
	}

	fmt.Printf("\nFinal state: PC:%04X A:%02X X:%02X Y:%02X SP:%02X P:%02X  Cycles:%d\n",
		c.PC, c.A, c.X, c.Y, c.SP, c.P, c.Cycles)

	// Show what's in the text screen area ($0400–$07FF) — preview for iter 3
	fmt.Println("\nText page 1 preview (first 3 rows, raw bytes):")
	for row := 0; row < 3; row++ {
		base := textRowAddr(row)
		fmt.Printf("  $%04X: ", base)
		for col := 0; col < 40; col++ {
			ch := ram.Data[base+uint16(col)]
			fmt.Printf("%02X ", ch)
		}
		fmt.Println()
	}
}

// loadROM detects the ROM size and sets the correct base address.
func loadROM(path string) (*memory.ROM, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	size := info.Size()
	switch size {
	case 12288: // 12 KB — standard Apple II+ ROM
		return memory.LoadROM(path, 0xD000)
	case 16384: // 16 KB — includes $C000 area
		return memory.LoadROM(path, 0xC000)
	default:
		// Try to guess: if it's close to 12K, use $D000
		if size > 0 && size <= 12288 {
			return memory.LoadROM(path, uint16(0x10000-size))
		}
		return nil, fmt.Errorf("unexpected ROM size %d bytes (expected 12288 or 16384)", size)
	}
}

// textRowAddr returns the base address for a text screen row.
// The Apple II text screen has a non-linear layout.
func textRowAddr(row int) uint16 {
	// Each group of 8 rows is offset by $80; within a group, rows are $28 apart.
	return 0x0400 + uint16(row/8)*0x28 + uint16(row%8)*0x80
}

// ---------------------------------------------------------------------------
// Opcode names and sizes for disassembly trace
// ---------------------------------------------------------------------------

var opNames [256]string
var opSizes [256]uint8

func init() {
	// Default: unknown
	for i := range opNames {
		opNames[i] = "???"
		opSizes[i] = 1
	}

	n := func(op byte, name string, size uint8) {
		opNames[op] = name
		opSizes[op] = size
	}

	// Load/Store
	n(0xA9, "LDA", 2)
	n(0xA5, "LDA", 2)
	n(0xB5, "LDA", 2)
	n(0xAD, "LDA", 3)
	n(0xBD, "LDA", 3)
	n(0xB9, "LDA", 3)
	n(0xA1, "LDA", 2)
	n(0xB1, "LDA", 2)
	n(0xA2, "LDX", 2)
	n(0xA6, "LDX", 2)
	n(0xB6, "LDX", 2)
	n(0xAE, "LDX", 3)
	n(0xBE, "LDX", 3)
	n(0xA0, "LDY", 2)
	n(0xA4, "LDY", 2)
	n(0xB4, "LDY", 2)
	n(0xAC, "LDY", 3)
	n(0xBC, "LDY", 3)
	n(0x85, "STA", 2)
	n(0x95, "STA", 2)
	n(0x8D, "STA", 3)
	n(0x9D, "STA", 3)
	n(0x99, "STA", 3)
	n(0x81, "STA", 2)
	n(0x91, "STA", 2)
	n(0x86, "STX", 2)
	n(0x96, "STX", 2)
	n(0x8E, "STX", 3)
	n(0x84, "STY", 2)
	n(0x94, "STY", 2)
	n(0x8C, "STY", 3)

	// Transfers
	n(0xAA, "TAX", 1)
	n(0xA8, "TAY", 1)
	n(0x8A, "TXA", 1)
	n(0x98, "TYA", 1)
	n(0xBA, "TSX", 1)
	n(0x9A, "TXS", 1)

	// Stack
	n(0x48, "PHA", 1)
	n(0x08, "PHP", 1)
	n(0x68, "PLA", 1)
	n(0x28, "PLP", 1)

	// Arithmetic
	n(0x69, "ADC", 2)
	n(0x65, "ADC", 2)
	n(0x75, "ADC", 2)
	n(0x6D, "ADC", 3)
	n(0x7D, "ADC", 3)
	n(0x79, "ADC", 3)
	n(0x61, "ADC", 2)
	n(0x71, "ADC", 2)
	n(0xE9, "SBC", 2)
	n(0xE5, "SBC", 2)
	n(0xF5, "SBC", 2)
	n(0xED, "SBC", 3)
	n(0xFD, "SBC", 3)
	n(0xF9, "SBC", 3)
	n(0xE1, "SBC", 2)
	n(0xF1, "SBC", 2)

	// Logic
	n(0x29, "AND", 2)
	n(0x25, "AND", 2)
	n(0x35, "AND", 2)
	n(0x2D, "AND", 3)
	n(0x3D, "AND", 3)
	n(0x39, "AND", 3)
	n(0x21, "AND", 2)
	n(0x31, "AND", 2)
	n(0x09, "ORA", 2)
	n(0x05, "ORA", 2)
	n(0x15, "ORA", 2)
	n(0x0D, "ORA", 3)
	n(0x1D, "ORA", 3)
	n(0x19, "ORA", 3)
	n(0x01, "ORA", 2)
	n(0x11, "ORA", 2)
	n(0x49, "EOR", 2)
	n(0x45, "EOR", 2)
	n(0x55, "EOR", 2)
	n(0x4D, "EOR", 3)
	n(0x5D, "EOR", 3)
	n(0x59, "EOR", 3)
	n(0x41, "EOR", 2)
	n(0x51, "EOR", 2)

	// Shift/Rotate
	n(0x0A, "ASL", 1)
	n(0x06, "ASL", 2)
	n(0x16, "ASL", 2)
	n(0x0E, "ASL", 3)
	n(0x1E, "ASL", 3)
	n(0x4A, "LSR", 1)
	n(0x46, "LSR", 2)
	n(0x56, "LSR", 2)
	n(0x4E, "LSR", 3)
	n(0x5E, "LSR", 3)
	n(0x2A, "ROL", 1)
	n(0x26, "ROL", 2)
	n(0x36, "ROL", 2)
	n(0x2E, "ROL", 3)
	n(0x3E, "ROL", 3)
	n(0x6A, "ROR", 1)
	n(0x66, "ROR", 2)
	n(0x76, "ROR", 2)
	n(0x6E, "ROR", 3)
	n(0x7E, "ROR", 3)

	// Inc/Dec
	n(0xE6, "INC", 2)
	n(0xF6, "INC", 2)
	n(0xEE, "INC", 3)
	n(0xFE, "INC", 3)
	n(0xC6, "DEC", 2)
	n(0xD6, "DEC", 2)
	n(0xCE, "DEC", 3)
	n(0xDE, "DEC", 3)
	n(0xE8, "INX", 1)
	n(0xC8, "INY", 1)
	n(0xCA, "DEX", 1)
	n(0x88, "DEY", 1)

	// Compare
	n(0xC9, "CMP", 2)
	n(0xC5, "CMP", 2)
	n(0xD5, "CMP", 2)
	n(0xCD, "CMP", 3)
	n(0xDD, "CMP", 3)
	n(0xD9, "CMP", 3)
	n(0xC1, "CMP", 2)
	n(0xD1, "CMP", 2)
	n(0xE0, "CPX", 2)
	n(0xE4, "CPX", 2)
	n(0xEC, "CPX", 3)
	n(0xC0, "CPY", 2)
	n(0xC4, "CPY", 2)
	n(0xCC, "CPY", 3)

	// Bit
	n(0x24, "BIT", 2)
	n(0x2C, "BIT", 3)

	// Branches
	n(0x90, "BCC", 2)
	n(0xB0, "BCS", 2)
	n(0xF0, "BEQ", 2)
	n(0x30, "BMI", 2)
	n(0xD0, "BNE", 2)
	n(0x10, "BPL", 2)
	n(0x50, "BVC", 2)
	n(0x70, "BVS", 2)

	// Jump
	n(0x4C, "JMP", 3)
	n(0x6C, "JMP", 3)
	n(0x20, "JSR", 3)
	n(0x60, "RTS", 1)
	n(0x40, "RTI", 1)

	// Flags
	n(0x18, "CLC", 1)
	n(0x38, "SEC", 1)
	n(0xD8, "CLD", 1)
	n(0xF8, "SED", 1)
	n(0x58, "CLI", 1)
	n(0x78, "SEI", 1)
	n(0xB8, "CLV", 1)

	// Misc
	n(0xEA, "NOP", 1)
	n(0x00, "BRK", 1)
}
