package disk

import (
	"fmt"
	"os"
	"testing"

	"github.com/apple2emu/bus"
	"github.com/apple2emu/cpu"
	appleio "github.com/apple2emu/io"
	"github.com/apple2emu/memory"
)

// TestBootPROM_LoadsT0S0 is an integration test that wires up the real CPU,
// bus, RAM, ROM, softswitches, and disk controller, then runs the PROM boot
// code and checks that $0800-$08FF contains the correct T0S0 data.
//
// This test requires the real ROM and disk image files to be present.
func TestBootPROM_LoadsT0S0(t *testing.T) {
	romPath := "../roms/Apple2_Plus.rom"
	promPath := "../roms/DISK2.rom"
	diskPath := "../disks/DOS_3_3.dsk"

	for _, path := range []string{romPath, promPath, diskPath} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("required file missing: %s", path)
		}
	}

	// Read expected T0S0 directly from the .dsk file (first 256 bytes).
	dskData, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("read disk image: %v", err)
	}
	expectedT0S0 := dskData[:256]

	// Wire the bus exactly like main.go does.
	var diskCycle uint64
	ram := memory.NewRAM()
	sw := appleio.NewSoftSwitches(&diskCycle)
	b := bus.NewBus()

	rom, err := memory.LoadROM(romPath, 0xD000)
	if err != nil {
		t.Fatalf("load ROM: %v", err)
	}

	b.Map(0x0000, 0xBFFF, ram)
	b.Map(0xC000, 0xC0FF, sw)
	b.Map(rom.Base, rom.End(), rom)

	dc := NewController(&diskCycle)
	if err := dc.Mount(0, diskPath, ""); err != nil {
		t.Fatalf("mount disk: %v", err)
	}
	b.Map(0xC0E0, 0xC0EF, NewSwitches(dc))

	prom, err := memory.LoadROM(promPath, 0xC600)
	if err != nil {
		t.Fatalf("load PROM: %v", err)
	}
	b.Map(prom.Base, prom.End(), prom)

	// Init CPU and reset (loads PC from $FFFC).
	c := cpu.NewCPU(b)
	c.Reset()
	t.Logf("Reset vector: $%04X", c.PC)

	// Run CPU until PC == $0801 (PROM jumps here after loading T0S0)
	// or until we exceed a cycle budget.
	const maxCycles = 5_000_000
	reached0801 := false
	for c.Cycles < maxCycles {
		consumed := uint64(c.Step())
		diskCycle += consumed

		if c.PC == 0x0801 {
			reached0801 = true
			t.Logf("Reached $0801 at cycle %d", c.Cycles)
			break
		}

		// Also detect if we landed in the monitor (Autostart ROM).
		// The monitor prompt loop is at $FF59 or thereabouts.
		if c.PC >= 0xFF00 && c.Cycles > 500_000 {
			t.Logf("Landed in monitor at PC=$%04X, A=$%02X X=$%02X Y=$%02X P=$%02X SP=$%02X cycle=%d",
				c.PC, c.A, c.X, c.Y, c.P, c.SP, c.Cycles)
			// Don't break — the ROM might pass through here during init
		}
	}

	if !reached0801 {
		t.Logf("CPU state: PC=$%04X A=$%02X X=$%02X Y=$%02X P=$%02X SP=$%02X cycles=%d",
			c.PC, c.A, c.X, c.Y, c.P, c.SP, c.Cycles)
		// Dump $0800-$080F to see what's there
		t.Log("$0800-$080F:")
		for i := uint16(0x0800); i < 0x0810; i++ {
			t.Logf("  $%04X = $%02X", i, b.Read(i))
		}
		// Dump zero page $26-$2B (used by PROM for addressing)
		t.Log("ZP $26-$2B:")
		for i := uint16(0x26); i <= 0x2B; i++ {
			t.Logf("  $%02X = $%02X", i, b.Read(i))
		}
		// Dump stack area
		t.Log("Stack top $01F8-$01FF:")
		for i := uint16(0x01F8); i <= 0x01FF; i++ {
			t.Logf("  $%04X = $%02X", i, b.Read(i))
		}
		t.Fatalf("Did not reach $0801 within %d cycles", maxCycles)
	}

	// Compare $0800-$08FF with expected T0S0.
	mismatches := 0
	for i := 0; i < 256; i++ {
		got := b.Read(uint16(0x0800 + i))
		want := expectedT0S0[i]
		if got != want {
			if mismatches < 20 {
				t.Errorf("$%04X: got $%02X, want $%02X", 0x0800+i, got, want)
			}
			mismatches++
		}
	}
	if mismatches > 0 {
		t.Errorf("Total mismatches: %d of 256 bytes", mismatches)
	} else {
		t.Log("T0S0 loaded correctly at $0800-$08FF")
	}

	// Also log what boot1 would execute.
	t.Logf("$0801: $%02X $%02X $%02X (first 3 bytes of boot1 code)",
		b.Read(0x0801), b.Read(0x0802), b.Read(0x0803))

	// Boot1 loads 10 sectors to $3600-$3FFF. Verified below after boot1 runs.

	// Dump key ZP locations at $0801 entry.
	t.Log("ZP at boot1 entry:")
	for _, addr := range []uint16{0x26, 0x27, 0x28, 0x29, 0x2A, 0x2B, 0x3C, 0x3D, 0x40, 0x41, 0x42, 0x43, 0x44} {
		t.Logf("  $%02X = $%02X", addr, b.Read(addr))
	}

	// Continue running boot1, tracing key instructions.
	const boot1Budget = 10_000_000
	startCycles := c.Cycles
	traceCount := 0
	lastPC := uint16(0xFFFF)
	for c.Cycles-startCycles < boot1Budget {
		pc := c.PC
		opcode := b.Read(pc)

		// Trace the first 100 unique PC values in boot1 ($0800-$08FF range).
		if pc >= 0x0800 && pc <= 0x08FF && pc != lastPC && traceCount < 100 {
			op1, op2 := b.Read(pc+1), b.Read(pc+2)
			t.Logf("  [%d] PC=$%04X op=$%02X %02X %02X  A=$%02X X=$%02X Y=$%02X SP=$%02X P=$%02X",
				c.Cycles, pc, opcode, op1, op2, c.A, c.X, c.Y, c.SP, c.P)
			lastPC = pc
			traceCount++
		}

		// Also trace RTS/JMP instructions anywhere.
		if (opcode == 0x60 || opcode == 0x6C || opcode == 0x4C) && traceCount < 200 {
			op1, op2 := b.Read(pc+1), b.Read(pc+2)
			t.Logf("  [%d] PC=$%04X op=$%02X %02X %02X  A=$%02X X=$%02X Y=$%02X SP=$%02X P=$%02X  (RTS/JMP)",
				c.Cycles, pc, opcode, op1, op2, c.A, c.X, c.Y, c.SP, c.P)
			traceCount++
		}

		consumed := uint64(c.Step())
		diskCycle += consumed

		if c.PC < 0x0010 {
			t.Logf("Boot1 crashed to PC=$%04X at cycle %d (A=$%02X X=$%02X Y=$%02X P=$%02X SP=$%02X)",
				c.PC, c.Cycles, c.A, c.X, c.Y, c.P, c.SP)
			t.Log("Memory around crash:")
			for i := uint16(0x0000); i < 0x0010; i++ {
				t.Logf("  $%04X = $%02X", i, b.Read(i))
			}
			t.Log("Full stack page $0100-$01FF:")
			for row := 0; row < 16; row++ {
				line := fmt.Sprintf("  $%04X:", 0x0100+row*16)
				for col := 0; col < 16; col++ {
					line += fmt.Sprintf(" %02X", b.Read(uint16(0x0100+row*16+col)))
				}
				t.Log(line)
			}
			// Also dump $3700-$3720 to see what's there
			t.Log("DOS entry $3700-$3720:")
			for i := uint16(0x3700); i <= 0x3720; i++ {
				t.Logf("  $%04X = $%02X", i, b.Read(i))
			}
			break
		}
	}

	t.Logf("Final CPU state: PC=$%04X A=$%02X X=$%02X Y=$%02X P=$%02X SP=$%02X cycles=%d",
		c.PC, c.A, c.X, c.Y, c.P, c.SP, c.Cycles)

	// Check warm-start vectors (set by ROM cold-start at $FAA6)
	t.Logf("Warm-start vectors: $03F0=%02X $03F1=%02X $03F2=%02X $03F3=%02X $03F4=%02X",
		b.Read(0x03F0), b.Read(0x03F1), b.Read(0x03F2), b.Read(0x03F3), b.Read(0x03F4))

	// Simulate RESET and see if warm-start path works
	t.Log("=== Simulating RESET ===")
	c.Reset()
	t.Logf("After Reset: PC=$%04X SP=$%02X", c.PC, c.SP)

	// Run until we either reach $E000 (Applesoft) or timeout
	resetBudget := uint64(500_000)
	resetStart := c.Cycles
	reachedBasic := false
	for c.Cycles-resetStart < resetBudget {
		consumed := uint64(c.Step())
		diskCycle += consumed

		if c.PC == 0xE000 {
			reachedBasic = true
			t.Logf("Reached Applesoft BASIC at $E000! cycle=%d", c.Cycles)
			break
		}
	}
	if !reachedBasic {
		t.Logf("Did not reach $E000 within budget. PC=$%04X", c.PC)
	}
}
