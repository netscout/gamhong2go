package cpu

import (
	"fmt"
	"os"
	"testing"
)

// FlatMemory is a simple 64 KB address space for testing.
type FlatMemory struct {
	Data [65536]uint8
}

func (m *FlatMemory) Read(addr uint16) uint8       { return m.Data[addr] }
func (m *FlatMemory) Write(addr uint16, val uint8) { m.Data[addr] = val }

// ---------------------------------------------------------------------------
// Helper: load a small program at an address, set reset vector, and run.
// ---------------------------------------------------------------------------

func setupCPU(program []byte, origin uint16) (*CPU, *FlatMemory) {
	mem := &FlatMemory{}
	copy(mem.Data[origin:], program)
	// Set reset vector to origin.
	mem.Data[0xFFFC] = uint8(origin)
	mem.Data[0xFFFD] = uint8(origin >> 8)
	c := NewCPU(mem)
	c.Reset() // Set PC to reset vector (origin)
	return c, mem
}

func runN(c *CPU, n int) {
	for i := 0; i < n; i++ {
		c.Step()
	}
}

// ---------------------------------------------------------------------------
// Sanity tests — verify core instructions in isolation.
// ---------------------------------------------------------------------------

func TestLDAImmediate(t *testing.T) {
	// LDA #$42; NOP
	c, _ := setupCPU([]byte{0xA9, 0x42, 0xEA}, 0x0400)
	c.Step()
	if c.A != 0x42 {
		t.Fatalf("expected A=0x42, got 0x%02X", c.A)
	}
	if c.getFlag(FlagZ) {
		t.Fatal("Z flag should be clear")
	}
	if c.getFlag(FlagN) {
		t.Fatal("N flag should be clear")
	}
}

func TestLDAZero(t *testing.T) {
	// LDA #$00
	c, _ := setupCPU([]byte{0xA9, 0x00}, 0x0400)
	c.Step()
	if c.A != 0x00 {
		t.Fatalf("expected A=0x00, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagZ) {
		t.Fatal("Z flag should be set")
	}
}

func TestLDANegative(t *testing.T) {
	// LDA #$FF
	c, _ := setupCPU([]byte{0xA9, 0xFF}, 0x0400)
	c.Step()
	if !c.getFlag(FlagN) {
		t.Fatal("N flag should be set")
	}
}

func TestSTAZeroPage(t *testing.T) {
	// LDA #$99; STA $10
	c, mem := setupCPU([]byte{0xA9, 0x99, 0x85, 0x10}, 0x0400)
	runN(c, 2)
	if mem.Data[0x10] != 0x99 {
		t.Fatalf("expected mem[$10]=0x99, got 0x%02X", mem.Data[0x10])
	}
}

func TestADCSimple(t *testing.T) {
	// CLC; LDA #$10; ADC #$20
	c, _ := setupCPU([]byte{0x18, 0xA9, 0x10, 0x69, 0x20}, 0x0400)
	runN(c, 3)
	if c.A != 0x30 {
		t.Fatalf("expected A=0x30, got 0x%02X", c.A)
	}
	if c.getFlag(FlagC) {
		t.Fatal("carry should be clear")
	}
}

func TestADCOverflow(t *testing.T) {
	// CLC; LDA #$FF; ADC #$01
	c, _ := setupCPU([]byte{0x18, 0xA9, 0xFF, 0x69, 0x01}, 0x0400)
	runN(c, 3)
	if c.A != 0x00 {
		t.Fatalf("expected A=0x00, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagC) {
		t.Fatal("carry should be set")
	}
	if !c.getFlag(FlagZ) {
		t.Fatal("zero should be set")
	}
}

func TestSBCSimple(t *testing.T) {
	// SEC; LDA #$30; SBC #$10
	c, _ := setupCPU([]byte{0x38, 0xA9, 0x30, 0xE9, 0x10}, 0x0400)
	runN(c, 3)
	if c.A != 0x20 {
		t.Fatalf("expected A=0x20, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagC) {
		t.Fatal("carry (no borrow) should be set")
	}
}

func TestJMPAbsolute(t *testing.T) {
	// JMP $0500 (at $0400)
	c, mem := setupCPU([]byte{0x4C, 0x00, 0x05}, 0x0400)
	// Put a NOP at $0500 so there's something to land on.
	mem.Data[0x0500] = 0xEA
	c.Step()
	if c.PC != 0x0500 {
		t.Fatalf("expected PC=0x0500, got 0x%04X", c.PC)
	}
}

func TestJSRAndRTS(t *testing.T) {
	// $0400: JSR $0500
	// $0500: LDA #$77; RTS
	// After RTS, PC should be $0403 (byte after JSR operand).
	c, mem := setupCPU([]byte{0x20, 0x00, 0x05}, 0x0400)
	mem.Data[0x0500] = 0xA9 // LDA #$77
	mem.Data[0x0501] = 0x77
	mem.Data[0x0502] = 0x60 // RTS
	runN(c, 3)              // JSR, LDA, RTS
	if c.A != 0x77 {
		t.Fatalf("expected A=0x77, got 0x%02X", c.A)
	}
	if c.PC != 0x0403 {
		t.Fatalf("expected PC=0x0403, got 0x%04X", c.PC)
	}
}

func TestBranch(t *testing.T) {
	// LDA #$00; BEQ +3; LDA #$FF; NOP (target)
	// $0400: A9 00     LDA #$00
	// $0402: F0 02     BEQ $0406
	// $0404: A9 FF     LDA #$FF (should be skipped)
	// $0406: EA        NOP
	c, _ := setupCPU([]byte{0xA9, 0x00, 0xF0, 0x02, 0xA9, 0xFF, 0xEA}, 0x0400)
	runN(c, 3) // LDA, BEQ (taken), NOP
	if c.A != 0x00 {
		t.Fatalf("branch should have skipped LDA #$FF, got A=0x%02X", c.A)
	}
}

func TestStackPushPull(t *testing.T) {
	// LDA #$AB; PHA; LDA #$00; PLA
	c, _ := setupCPU([]byte{0xA9, 0xAB, 0x48, 0xA9, 0x00, 0x68}, 0x0400)
	runN(c, 4)
	if c.A != 0xAB {
		t.Fatalf("expected A=0xAB after PLA, got 0x%02X", c.A)
	}
}

func TestIndexedIndirect(t *testing.T) {
	// Put the target address $0300 at zero page $20,$21.
	// Put value $42 at $0300.
	// LDX #$10; LDA ($10,X)  — effective pointer at ZP $20.
	c, mem := setupCPU([]byte{0xA2, 0x10, 0xA1, 0x10}, 0x0400)
	mem.Data[0x20] = 0x00
	mem.Data[0x21] = 0x03
	mem.Data[0x0300] = 0x42
	runN(c, 2)
	if c.A != 0x42 {
		t.Fatalf("expected A=0x42, got 0x%02X", c.A)
	}
}

func TestIndirectIndexed(t *testing.T) {
	// ZP $30,$31 = $0200. Y = $05. LDA ($30),Y reads $0205.
	c, mem := setupCPU([]byte{0xA0, 0x05, 0xB1, 0x30}, 0x0400)
	mem.Data[0x30] = 0x00
	mem.Data[0x31] = 0x02
	mem.Data[0x0205] = 0x99
	runN(c, 2)
	if c.A != 0x99 {
		t.Fatalf("expected A=0x99, got 0x%02X", c.A)
	}
}

func TestCycleCount(t *testing.T) {
	// LDA #imm = 2 cycles, STA zp = 3 cycles, NOP = 2 cycles → total 7
	c, _ := setupCPU([]byte{0xA9, 0x01, 0x85, 0x00, 0xEA}, 0x0400)
	total := 0
	for i := 0; i < 3; i++ {
		total += c.Step()
	}
	if total != 7 {
		t.Fatalf("expected 7 cycles, got %d", total)
	}
}

// ---------------------------------------------------------------------------
// Extended edge-case tests
// ---------------------------------------------------------------------------

func TestADCDecimalMode(t *testing.T) {
	// SED; CLC; LDA #$15; ADC #$27 → BCD: 15+27 = 42
	c, _ := setupCPU([]byte{0xF8, 0x18, 0xA9, 0x15, 0x69, 0x27}, 0x0400)
	runN(c, 4)
	if c.A != 0x42 {
		t.Fatalf("BCD 15+27: expected A=0x42, got 0x%02X", c.A)
	}
	if c.getFlag(FlagC) {
		t.Fatal("carry should be clear")
	}
}

func TestADCDecimalCarry(t *testing.T) {
	// SED; CLC; LDA #$99; ADC #$01 → BCD: 99+1 = 00 with carry
	c, _ := setupCPU([]byte{0xF8, 0x18, 0xA9, 0x99, 0x69, 0x01}, 0x0400)
	runN(c, 4)
	if c.A != 0x00 {
		t.Fatalf("BCD 99+01: expected A=0x00, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagC) {
		t.Fatal("carry should be set")
	}
}

func TestSBCDecimalMode(t *testing.T) {
	// SED; SEC; LDA #$42; SBC #$15 → BCD: 42-15 = 27
	c, _ := setupCPU([]byte{0xF8, 0x38, 0xA9, 0x42, 0xE9, 0x15}, 0x0400)
	runN(c, 4)
	if c.A != 0x27 {
		t.Fatalf("BCD 42-15: expected A=0x27, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagC) {
		t.Fatal("carry (no borrow) should be set")
	}
}

func TestJMPIndirectBug(t *testing.T) {
	// The NMOS 6502 JMP ($xxFF) fetches the high byte from $xx00
	// instead of $(xx+1)00. Set up: pointer at $02FF/$0200 (not $0300).
	c, mem := setupCPU([]byte{0x6C, 0xFF, 0x02}, 0x0400)
	mem.Data[0x02FF] = 0x80 // low byte of target
	mem.Data[0x0200] = 0x06 // high byte (bug: wraps to $0200, not $0300)
	mem.Data[0x0300] = 0x99 // this would be read without the bug
	c.Step()
	if c.PC != 0x0680 {
		t.Fatalf("JMP indirect bug: expected PC=0x0680, got 0x%04X", c.PC)
	}
}

func TestAbsoluteXPageCross(t *testing.T) {
	// LDA $10F0,X with X=$20 → address $1110, crosses page.
	// Should take 5 cycles (4 base + 1 page cross).
	c, mem := setupCPU([]byte{0xBD, 0xF0, 0x10}, 0x0400)
	c.X = 0x20
	mem.Data[0x1110] = 0xAB
	cycles := c.Step()
	if c.A != 0xAB {
		t.Fatalf("expected A=0xAB, got 0x%02X", c.A)
	}
	if cycles != 5 {
		t.Fatalf("expected 5 cycles (page cross), got %d", cycles)
	}
}

func TestROLCarryChain(t *testing.T) {
	// SEC; LDA #$80; ROL A → carry in, bit 7 out to carry, result = $01
	c, _ := setupCPU([]byte{0x38, 0xA9, 0x80, 0x2A}, 0x0400)
	runN(c, 3)
	if c.A != 0x01 {
		t.Fatalf("expected A=0x01, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagC) {
		t.Fatal("carry should be set (shifted out from bit 7)")
	}
}

func TestBITFlags(t *testing.T) {
	// BIT sets Z from A&M, N from M bit 7, V from M bit 6.
	// LDA #$00; BIT $10 (where $10 = $C0)
	c, mem := setupCPU([]byte{0xA9, 0x00, 0x24, 0x10}, 0x0400)
	mem.Data[0x10] = 0xC0
	runN(c, 2)
	if !c.getFlag(FlagZ) {
		t.Fatal("Z should be set (A & M = 0)")
	}
	if !c.getFlag(FlagN) {
		t.Fatal("N should be set (bit 7 of M)")
	}
	if !c.getFlag(FlagV) {
		t.Fatal("V should be set (bit 6 of M)")
	}
}

func TestOverflowFlag(t *testing.T) {
	// CLC; LDA #$7F; ADC #$01 → 127 + 1 = 128 → signed overflow
	c, _ := setupCPU([]byte{0x18, 0xA9, 0x7F, 0x69, 0x01}, 0x0400)
	runN(c, 3)
	if c.A != 0x80 {
		t.Fatalf("expected A=0x80, got 0x%02X", c.A)
	}
	if !c.getFlag(FlagV) {
		t.Fatal("V should be set (signed overflow)")
	}
	if !c.getFlag(FlagN) {
		t.Fatal("N should be set")
	}
}

func TestRTI(t *testing.T) {
	// Push a return address and status onto the stack, then RTI.
	c, mem := setupCPU([]byte{0x40}, 0x0400) // RTI at $0400
	// Manually push: status, then PClo, PChi (RTI pulls P, then PC).
	// We want to return to $1234 with carry set.
	c.SP = 0xFD
	mem.Data[0x01FB] = FlagC | FlagU // status (at SP+1 after pull)
	mem.Data[0x01FC] = 0x34          // PC low
	mem.Data[0x01FD] = 0x12          // PC high
	c.SP = 0xFA                      // 3 bytes to pull
	c.Step()
	if c.PC != 0x1234 {
		t.Fatalf("expected PC=0x1234, got 0x%04X", c.PC)
	}
	if !c.getFlag(FlagC) {
		t.Fatal("carry should be set from restored status")
	}
}

func TestZeroPageWrap(t *testing.T) {
	// LDA $FF,X with X=$01 should wrap to $00, not $100.
	c, mem := setupCPU([]byte{0xB5, 0xFF}, 0x0400)
	c.X = 0x01
	mem.Data[0x00] = 0x77   // wrapped address
	mem.Data[0x0100] = 0x99 // non-wrapped (should not be read)
	c.Step()
	if c.A != 0x77 {
		t.Fatalf("expected A=0x77 (zero page wrap), got 0x%02X", c.A)
	}
}

// ---------------------------------------------------------------------------
// Klaus Dormann's 6502 functional test
// ---------------------------------------------------------------------------
//
// This is the gold-standard verification for a 6502 emulator. The test
// binary exercises every official opcode including decimal mode.
//
// To run it:
//   1. Download the binary:
//      curl -L -o cpu/testdata/6502_functional_test.bin \
//        "https://github.com/Klaus2m5/6502_65C02_functional_tests/raw/master/bin_files/6502_functional_test.bin"
//   2. Run: go test ./cpu -v -run TestDormann
//
// The binary is loaded at $0000 (it is exactly 65536 bytes). Execution
// starts at $0400. On success, PC reaches $3469 and loops there forever.
// On failure, PC gets stuck at some other address (a JMP-to-self trap).

const dormannBin = "testdata/6502_functional_test.bin"
const dormannStart uint16 = 0x0400
const dormannSuccess uint16 = 0x3469
const maxDormannCycles = 100_000_000

func TestDormann(t *testing.T) {
	data, err := os.ReadFile(dormannBin)
	if err != nil {
		t.Skipf("Dormann test binary not found at %s — skipping.\n"+
			"Download it with:\n"+
			"  mkdir -p cpu/testdata\n"+
			"  curl -L -o cpu/testdata/6502_functional_test.bin \\\n"+
			"    https://github.com/Klaus2m5/6502_65C02_functional_tests/raw/master/bin_files/6502_functional_test.bin",
			dormannBin)
	}

	mem := &FlatMemory{}
	copy(mem.Data[:], data)

	c := NewCPU(mem)
	c.PC = dormannStart

	prevPC := c.PC
	totalCycles := uint64(0)

	for totalCycles < maxDormannCycles {
		cycles := c.Step()
		totalCycles += uint64(cycles)

		if c.PC == prevPC {
			// CPU is in a trap (JMP to self).
			if c.PC == dormannSuccess {
				t.Logf("PASS — all tests passed at $%04X after %d cycles", c.PC, totalCycles)
				return
			}
			t.Fatalf("TRAP at $%04X after %d cycles — test failed.\n"+
				"Registers: A=%02X X=%02X Y=%02X SP=%02X P=%02X",
				c.PC, totalCycles, c.A, c.X, c.Y, c.SP, c.P)
		}
		prevPC = c.PC
	}

	t.Fatalf("Timed out after %d cycles at PC=$%04X — possible infinite loop",
		maxDormannCycles, c.PC)
}

// ---------------------------------------------------------------------------
// Trace helper (useful for debugging a failing Dormann test)
// ---------------------------------------------------------------------------

func TestDormannTrace(t *testing.T) {
	if os.Getenv("TRACE") == "" {
		t.Skip("Set TRACE=1 to enable the Dormann trace test")
	}

	data, err := os.ReadFile(dormannBin)
	if err != nil {
		t.Skipf("Dormann binary not found: %v", err)
	}

	mem := &FlatMemory{}
	copy(mem.Data[:], data)

	c := NewCPU(mem)
	c.PC = dormannStart

	prevPC := c.PC
	for i := 0; i < 2000; i++ {
		pc := c.PC
		op := mem.Data[pc]
		fmt.Printf("%04X  %02X    A:%02X X:%02X Y:%02X SP:%02X P:%02X\n",
			pc, op, c.A, c.X, c.Y, c.SP, c.P)
		c.Step()
		if c.PC == prevPC {
			fmt.Printf("TRAP at $%04X\n", c.PC)
			break
		}
		prevPC = c.PC
	}
}
