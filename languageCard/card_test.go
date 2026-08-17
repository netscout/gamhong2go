package languageCard

import (
	"fmt"
	"os"
	"testing"

	"github.com/netscout/gamhong2go/bus"
)

// fakeROM is a stub ROMReader for tests. Returns a fixed sentinel byte
// for any address, so assertions can distinguish ROM reads from RAM reads.
type fakeROM struct {
	sentinel uint8
}

func (f *fakeROM) Read(_ uint16) uint8 { return f.sentinel }

// cardState is a helper that snapshots the four observable bits.
type cardState struct {
	ramRead  bool
	ramWrite bool
	bank2Sel bool
	prearm   bool
}

func snap(c *Card) cardState {
	return cardState{c.ramRead, c.ramWrite, c.bank2Sel, c.prearm}
}

// twoReadEnable arms and commits write-enable via two reads of a write-enabling
// address. addr must satisfy (addr & 0x03) == 1 or 3 (arm/commit addresses).
// Resets prearm before arming so the sequence is deterministic regardless of
// prior card state. Leaves the card in ramWrite=true, prearm=true.
func twoReadEnable(c *Card, addr uint8) {
	low := addr & 0x0F
	c.handleSwitch(low, false) // first read  → prearm=true
	c.handleSwitch(low, false) // second read → ramWrite=true
}

// -----------------------------------------------------------------------
// Test 1: TestResetState
// -----------------------------------------------------------------------

// TestResetState verifies the Apple II+ power-on defaults after NewCard.
// Expected per §2 table: ramRead=false, ramWrite=false, bank2Sel=true, prearm=false.
func TestResetState(t *testing.T) {
	rom := &fakeROM{sentinel: 0xEA} // EA = NOP
	c := NewCard(rom)

	if got := snap(c); got != (cardState{false, false, true, false}) {
		t.Fatalf("Reset state: got %+v, want {false false true false}", got)
	}

	// Read from $D000 must return ROM byte (ramRead=false → delegate to ROM).
	if v := c.Read(0xD000); v != 0xEA {
		t.Fatalf("Read $D000 after reset: got %#x, want 0xEA (ROM byte)", v)
	}

	// Write to $D000 must be a no-op (ramWrite=false).
	c.Write(0xD000, 0x42)
	if v := c.Read(0xD000); v != 0xEA {
		t.Fatalf("Write to $D000 when ramWrite=false should be no-op; got %#x after read", v)
	}
}

// -----------------------------------------------------------------------
// Test 2: TestSwitchReadTruthTable
// -----------------------------------------------------------------------

// TestSwitchReadTruthTable is table-driven over all 16 $C08X addresses.
// Expected-value table derived exclusively from the §2 truth table in
// plan-language-card.md.
//
// Columns: low nibble, expected ramRead, expected bank2Sel.
// write-enable action on read depends on prearm state (tested separately).
func TestSwitchReadTruthTable(t *testing.T) {
	// Table from §2: READ access.
	// low | bank | ram-read | write-enable action
	//   0 |   2  |  RAM     | disable write (clear prearm, clear ramWrite)
	//   1 |   2  |  ROM     | arm/commit write
	//   2 |   2  |  ROM     | disable write
	//   3 |   2  |  RAM     | arm/commit write
	//   4 |   2  |  RAM     | disable write  (mirror of 0)
	//   5 |   2  |  ROM     | arm/commit     (mirror of 1)
	//   6 |   2  |  ROM     | disable write  (mirror of 2)
	//   7 |   2  |  RAM     | arm/commit     (mirror of 3)
	//   8 |   1  |  RAM     | disable write
	//   9 |   1  |  ROM     | arm/commit
	//   A |   1  |  ROM     | disable write
	//   B |   1  |  RAM     | arm/commit
	//   C |   1  |  RAM     | disable write  (mirror of 8)
	//   D |   1  |  ROM     | arm/commit     (mirror of 9)
	//   E |   1  |  ROM     | disable write  (mirror of A)
	//   F |   1  |  RAM     | arm/commit     (mirror of B)

	type row struct {
		low      uint8
		ramRead  bool
		bank2Sel bool
		disarm   bool // true = disable-write switch (clears prearm+ramWrite); false = arm/commit
	}

	rows := []row{
		{0x0, true, true, true},   // READBSR2
		{0x1, false, true, false},  // WRITEBSR2
		{0x2, false, true, true},   // OFFBSR2
		{0x3, true, true, false},   // RDWRBSR2
		{0x4, true, true, true},    // READBSR2 mirror
		{0x5, false, true, false},  // WRITEBSR2 mirror
		{0x6, false, true, true},   // OFFBSR2 mirror
		{0x7, true, true, false},   // RDWRBSR2 mirror
		{0x8, true, false, true},   // READBSR1
		{0x9, false, false, false}, // WRITEBSR1
		{0xA, false, false, true},  // OFFBSR1
		{0xB, true, false, false},  // RDWRBSR1
		{0xC, true, false, true},   // READBSR1 mirror
		{0xD, false, false, false}, // WRITEBSR1 mirror
		{0xE, false, false, true},  // OFFBSR1 mirror
		{0xF, true, false, false},  // RDWRBSR1 mirror
	}

	for _, r := range rows {
		// Sub-test A: prearm=false starting state.
		t.Run(fmt.Sprintf("C08%X_prearm_false", r.low), func(t *testing.T) {
			rom := &fakeROM{sentinel: 0xFF}
			c := NewCard(rom) // bank2Sel=true, prearm=false
			c.handleSwitch(r.low, false /*read*/)

			if c.ramRead != r.ramRead {
				t.Errorf("low=%X read: ramRead got %v, want %v", r.low, c.ramRead, r.ramRead)
			}
			if c.bank2Sel != r.bank2Sel {
				t.Errorf("low=%X read: bank2Sel got %v, want %v", r.low, c.bank2Sel, r.bank2Sel)
			}
			if r.disarm {
				// disable-write switch: prearm and ramWrite must be cleared.
				if c.prearm {
					t.Errorf("low=%X read (disarm): prearm want false, got true", r.low)
				}
				if c.ramWrite {
					t.Errorf("low=%X read (disarm): ramWrite want false, got true", r.low)
				}
			} else {
				// arm/commit switch: first read (prearm was false) → prearm becomes true,
				// ramWrite stays false (commit requires two reads).
				if !c.prearm {
					t.Errorf("low=%X read (arm, first): prearm want true, got false", r.low)
				}
				if c.ramWrite {
					t.Errorf("low=%X read (arm, first): ramWrite want false, got true", r.low)
				}
			}
		})

		// Sub-test B: prearm=true starting state — arm/commit switches commit ramWrite.
		t.Run(fmt.Sprintf("C08%X_prearm_true", r.low), func(t *testing.T) {
			rom := &fakeROM{sentinel: 0xFF}
			c := NewCard(rom)
			// Force prearm=true by doing a first arm-read at $C081 (low=1).
			c.handleSwitch(0x01, false)
			// Now apply the row's switch as a read.
			c.handleSwitch(r.low, false)

			if c.ramRead != r.ramRead {
				t.Errorf("low=%X read(prearm=true): ramRead got %v, want %v", r.low, c.ramRead, r.ramRead)
			}
			if c.bank2Sel != r.bank2Sel {
				t.Errorf("low=%X read(prearm=true): bank2Sel got %v, want %v", r.low, c.bank2Sel, r.bank2Sel)
			}
			if r.disarm {
				// disable-write clears both, regardless of prior prearm.
				if c.prearm {
					t.Errorf("low=%X read(prearm=true,disarm): prearm want false", r.low)
				}
				if c.ramWrite {
					t.Errorf("low=%X read(prearm=true,disarm): ramWrite want false", r.low)
				}
			} else {
				// arm/commit with prearm=true → ramWrite becomes true, prearm stays true.
				if !c.ramWrite {
					t.Errorf("low=%X read(prearm=true,arm): ramWrite want true (commit)", r.low)
				}
				if !c.prearm {
					t.Errorf("low=%X read(prearm=true,arm): prearm want true", r.low)
				}
			}
		})
	}
}

// -----------------------------------------------------------------------
// Test 3: TestSwitchWriteTruthTable
// -----------------------------------------------------------------------

// TestSwitchWriteTruthTable verifies WRITE access to all 16 $C08X addresses.
// Key invariants per §2: no write ever sets ramWrite=true; prearm is always
// cleared; bank and ramRead are updated per the same decode as reads.
func TestSwitchWriteTruthTable(t *testing.T) {
	type row struct {
		low      uint8
		ramRead  bool
		bank2Sel bool
	}

	rows := []row{
		{0x0, true, true},   // READBSR2
		{0x1, false, true},  // WRITEBSR2
		{0x2, false, true},  // OFFBSR2
		{0x3, true, true},   // RDWRBSR2
		{0x4, true, true},   // READBSR2 mirror
		{0x5, false, true},  // WRITEBSR2 mirror
		{0x6, false, true},  // OFFBSR2 mirror
		{0x7, true, true},   // RDWRBSR2 mirror
		{0x8, true, false},  // READBSR1
		{0x9, false, false}, // WRITEBSR1
		{0xA, false, false}, // OFFBSR1
		{0xB, true, false},  // RDWRBSR1
		{0xC, true, false},  // READBSR1 mirror
		{0xD, false, false}, // WRITEBSR1 mirror
		{0xE, false, false}, // OFFBSR1 mirror
		{0xF, true, false},  // RDWRBSR1 mirror
	}

	for _, r := range rows {
		// Prime prearm=true before writing so we can confirm write clears it.
		rom := &fakeROM{sentinel: 0xFF}
		c := NewCard(rom)
		c.prearm = true // force prearm so write must clear it

		c.handleSwitch(r.low, true /*write*/)

		if c.ramRead != r.ramRead {
			t.Errorf("low=%X write: ramRead got %v, want %v", r.low, c.ramRead, r.ramRead)
		}
		if c.bank2Sel != r.bank2Sel {
			t.Errorf("low=%X write: bank2Sel got %v, want %v", r.low, c.bank2Sel, r.bank2Sel)
		}
		// Invariant: write NEVER enables ramWrite.
		if c.ramWrite {
			t.Errorf("low=%X write: ramWrite must be false after any write to $C08X, got true", r.low)
		}
		// Invariant: write always clears prearm.
		if c.prearm {
			t.Errorf("low=%X write: prearm must be false after any write to $C08X, got true", r.low)
		}
	}
}

// -----------------------------------------------------------------------
// Test 4: TestTwoConsecutiveReadsEnableWrite
// -----------------------------------------------------------------------

// TestTwoConsecutiveReadsEnableWrite pins the two-consecutive-reads quirk.
// Uses $C081 (low=1, bank2, ROM read, arm/commit).
func TestTwoConsecutiveReadsEnableWrite(t *testing.T) {
	rom := &fakeROM{sentinel: 0xAD} // AD = LDA abs
	c := NewCard(rom)

	// First read at $C081: arm prearm.
	c.handleSwitch(0x01, false)
	if !c.prearm || c.ramWrite {
		t.Fatalf("after 1st read $C081: want prearm=true ramWrite=false, got prearm=%v ramWrite=%v",
			c.prearm, c.ramWrite)
	}

	// Second read at $C081: commit ramWrite.
	c.handleSwitch(0x01, false)
	if !c.prearm || !c.ramWrite {
		t.Fatalf("after 2nd read $C081: want prearm=true ramWrite=true, got prearm=%v ramWrite=%v",
			c.prearm, c.ramWrite)
	}

	// Now switch to RAM-read to confirm we can write then read back.
	// $C083 read twice enables both ramRead and ramWrite; use it.
	c.handleSwitch(0x03, false) // $C083 read with prearm=true: ramRead=true, ramWrite commit re-asserts.
	if !c.ramRead || !c.ramWrite {
		t.Fatalf("after read $C083 (prearm=true): want ramRead=true ramWrite=true, got ramRead=%v ramWrite=%v",
			c.ramRead, c.ramWrite)
	}

	// Write $D000 = 0x42 (bank2 selected, ramWrite=true).
	c.Write(0xD000, 0x42)

	// Switch to RAM read, disable write ($C080): ramRead=true, ramWrite=false.
	c.handleSwitch(0x00, false)
	if !c.ramRead || c.ramWrite {
		t.Fatalf("after read $C080: want ramRead=true ramWrite=false")
	}

	// Re-read $D000 → should return 0x42 from bank2 RAM.
	if v := c.Read(0xD000); v != 0x42 {
		t.Fatalf("Read $D000 (bank2 RAM): got %#x, want 0x42", v)
	}
}

// -----------------------------------------------------------------------
// Test 5: TestInterveningWriteClearsPrearm
// -----------------------------------------------------------------------

// TestInterveningWriteClearsPrearm pins the simplified clear-on-$C08X-write
// rule (§2 clear-rule in plan-language-card.md).
func TestInterveningWriteClearsPrearm(t *testing.T) {
	c := NewCard(&fakeROM{})

	// First read of $C081: prearm=true.
	c.handleSwitch(0x01, false)
	if !c.prearm {
		t.Fatal("after 1st read $C081: prearm must be true")
	}

	// Intervening write to $C081: prearm=false, ramWrite=false.
	c.handleSwitch(0x01, true)
	if c.prearm || c.ramWrite {
		t.Fatalf("after write $C081: want prearm=false ramWrite=false, got prearm=%v ramWrite=%v",
			c.prearm, c.ramWrite)
	}

	// Another read of $C081: prearm=true again, but ramWrite still false.
	c.handleSwitch(0x01, false)
	if !c.prearm || c.ramWrite {
		t.Fatalf("after 2nd read $C081 (post-write): want prearm=true ramWrite=false, got prearm=%v ramWrite=%v",
			c.prearm, c.ramWrite)
	}
}

// -----------------------------------------------------------------------
// Test 6: TestDisableWriteClearsBothPrearmAndRamWrite
// -----------------------------------------------------------------------

// TestDisableWriteClearsBothPrearmAndRamWrite verifies that a disable-write
// switch read clears both prearm and ramWrite from a committed state.
func TestDisableWriteClearsBothPrearmAndRamWrite(t *testing.T) {
	c := NewCard(&fakeROM{})

	// Reach prearm=true, ramWrite=true via two reads of $C081.
	twoReadEnable(c, 0x01)
	if !c.prearm || !c.ramWrite {
		t.Fatal("setup: expected prearm=true ramWrite=true after two reads of $C081")
	}

	// Read $C080 (disable-write switch, low=0).
	c.handleSwitch(0x00, false)
	if c.prearm || c.ramWrite {
		t.Fatalf("after read $C080: want prearm=false ramWrite=false, got prearm=%v ramWrite=%v",
			c.prearm, c.ramWrite)
	}
}

// -----------------------------------------------------------------------
// Test 7: TestBank1VsBank2Isolation
// -----------------------------------------------------------------------

// TestBank1VsBank2Isolation verifies that bank1 and bank2 are truly
// independent slices for the $D000-$DFFF window.
func TestBank1VsBank2Isolation(t *testing.T) {
	c := NewCard(&fakeROM{sentinel: 0xFF})

	// Enable write, bank2 selected (low=3 twice → ramRead=true, ramWrite=true, bank2Sel=true).
	twoReadEnable(c, 0x03) // low=3: bank2Sel=true, arm/commit
	if !c.ramWrite || !c.bank2Sel {
		t.Fatal("setup bank2: ramWrite or bank2Sel not set")
	}
	c.Write(0xD000, 0xAA)

	// Switch to bank1, re-enable write (low=0xB: bit3=1→bank1, low&3=3→arm/commit).
	c.handleSwitch(0x08, false) // READBSR1: bank1, ramRead=true, disable write
	twoReadEnable(c, 0x0B)     // low=B: bank1Sel=true, arm/commit × 2
	if !c.ramWrite || c.bank2Sel {
		t.Fatal("setup bank1: ramWrite or bank2Sel wrong")
	}
	c.Write(0xD000, 0x55)

	// Read under bank2: should see 0xAA.
	c.handleSwitch(0x00, false) // READBSR2: bank2, ramRead=true, disable write
	if v := c.Read(0xD000); v != 0xAA {
		t.Fatalf("Read $D000 under bank2: got %#x, want 0xAA", v)
	}

	// Read under bank1: should see 0x55.
	c.handleSwitch(0x08, false) // READBSR1: bank1, ramRead=true, disable write
	if v := c.Read(0xD000); v != 0x55 {
		t.Fatalf("Read $D000 under bank1: got %#x, want 0x55", v)
	}
}

// -----------------------------------------------------------------------
// Test 7b: TestBankBitUpdatedOnEverySwitch
// -----------------------------------------------------------------------

// TestBankBitUpdatedOnEverySwitch pins that bit 3 decoding runs
// unconditionally on every $C08X access (without requiring write-enable dance).
func TestBankBitUpdatedOnEverySwitch(t *testing.T) {
	c := NewCard(&fakeROM{})
	// After reset: bank2Sel=true.

	// Single read of $C088 (low=8, bit3=1 → bank1).
	c.handleSwitch(0x08, false)
	if c.bank2Sel {
		t.Fatal("after read $C088: bank2Sel must be false (bank1 selected)")
	}

	// Single read of $C080 (low=0, bit3=0 → bank2).
	c.handleSwitch(0x00, false)
	if !c.bank2Sel {
		t.Fatal("after read $C080: bank2Sel must be true (bank2 selected)")
	}
}

// -----------------------------------------------------------------------
// Test 8: TestUpperBank_E000_FFFF_IsShared
// -----------------------------------------------------------------------

// TestUpperBank_E000_FFFF_IsShared verifies the 8 KB shared $E000-$FFFF
// region is not affected by the bank-select bit.
// Writes under bank2, reads back under bank1. Also exercises the $FFFF
// boundary to pin slice arithmetic against off-by-one refactors.
func TestUpperBank_E000_FFFF_IsShared(t *testing.T) {
	c := NewCard(&fakeROM{sentinel: 0xFF})

	// Enable write under bank2 (low=3 twice).
	twoReadEnable(c, 0x03)
	if !c.ramWrite || !c.bank2Sel {
		t.Fatal("setup bank2: ramWrite or bank2Sel not set")
	}
	// Also set ramRead=true.
	if !c.ramRead {
		t.Fatal("setup bank2: ramRead not set after $C083")
	}

	c.Write(0xE000, 0xAA)
	c.Write(0xF000, 0xBB)
	c.Write(0xFFFF, 0xCC)

	// Switch to bank1, re-enable write and read.
	c.handleSwitch(0x08, false) // READBSR1: bank1, ramRead=true, disable write
	twoReadEnable(c, 0x0B)     // RDWRBSR1: bank1, arm/commit ×2
	if c.bank2Sel {
		t.Fatal("after bank1 setup: bank2Sel must be false")
	}

	// Read back through bank1 — should return the shared values written under bank2.
	if v := c.Read(0xE000); v != 0xAA {
		t.Fatalf("Read $E000 under bank1: got %#x, want 0xAA (shared upper bank)", v)
	}
	if v := c.Read(0xF000); v != 0xBB {
		t.Fatalf("Read $F000 under bank1: got %#x, want 0xBB (shared upper bank)", v)
	}
	if v := c.Read(0xFFFF); v != 0xCC {
		t.Fatalf("Read $FFFF under bank1: got %#x, want 0xCC (shared upper bank, boundary)", v)
	}
}

// -----------------------------------------------------------------------
// Test 9: TestRomReadWhileRamWriteEnabled
// -----------------------------------------------------------------------

// TestRomReadWhileRamWriteEnabled pins that ramRead and ramWrite are
// independent flags. Primes bank2 RAM[$D000] = 0x42, then enters the
// "ramRead=false, ramWrite=true" state and confirms reads return ROM.
func TestRomReadWhileRamWriteEnabled(t *testing.T) {
	const romSentinel = 0xAD
	rom := &fakeROM{sentinel: romSentinel}
	c := NewCard(rom)

	// Step 1: Commit a known byte into bank2 RAM at $D000.
	// $C083 twice: ramRead=true, ramWrite=true, bank2Sel=true.
	twoReadEnable(c, 0x03) // low=3: bank2, arm/commit ×2
	c.Write(0xD000, 0x42)

	// Step 2: Clear to ROM-read, write-disabled via $C082 (OFFBSR2).
	c.handleSwitch(0x02, false) // OFFBSR2: ramRead=false, ramWrite=false, prearm=false
	if c.ramRead || c.ramWrite {
		t.Fatal("step 2: expected ramRead=false ramWrite=false after read $C082")
	}

	// Step 3: Enter "ramRead=false, ramWrite=true" via two reads of $C081.
	// $C081: low=1, bank2, ROM read, arm/commit.
	c.handleSwitch(0x01, false) // first read: prearm=true
	c.handleSwitch(0x01, false) // second read: ramWrite=true, ramRead=false
	if c.ramRead || !c.ramWrite {
		t.Fatalf("step 3: expected ramRead=false ramWrite=true, got ramRead=%v ramWrite=%v",
			c.ramRead, c.ramWrite)
	}

	// Step 4: Read $D000 — must return ROM byte, NOT 0x42.
	if v := c.Read(0xD000); v != romSentinel {
		t.Fatalf("Read $D000 (ramRead=false): got %#x, want %#x (ROM byte); bank2 RAM has 0x42",
			v, romSentinel)
	}
}

// -----------------------------------------------------------------------
// Test 10: TestResetClearsAllFlags
// -----------------------------------------------------------------------

// TestResetClearsAllFlags verifies Reset() returns all four flags to the
// Apple II+ power-on defaults, and that RAM contents survive reset.
func TestResetClearsAllFlags(t *testing.T) {
	c := NewCard(&fakeROM{sentinel: 0xFF})

	// Force a non-default state.
	c.ramRead = true
	c.ramWrite = true
	c.bank2Sel = false
	c.prearm = true

	// Write a sentinel into bank1 (bank2Sel=false) at $D000.
	// ramWrite=true, bank2Sel=false → lands in bank1.
	c.Write(0xD000, 0x99)

	c.Reset()

	// Check flags.
	if got := snap(c); got != (cardState{false, false, true, false}) {
		t.Fatalf("after Reset: got %+v, want {false false true false}", got)
	}

	// RAM contents in bank1 must survive reset.
	// To read bank1 we need ramRead=true, bank2Sel=false.
	c.ramRead = true
	c.bank2Sel = false
	if v := c.Read(0xD000); v != 0x99 {
		t.Fatalf("bank1[$D000] after reset: got %#x, want 0x99 (RAM retains across reset)", v)
	}
}

// -----------------------------------------------------------------------
// Test 11: TestWriteToRamDisabledIsNoop
// -----------------------------------------------------------------------

// TestWriteToRamDisabledIsNoop verifies that writes when ramWrite=false
// do not land in RAM and the read-back returns the ROM byte.
func TestWriteToRamDisabledIsNoop(t *testing.T) {
	const romSentinel = 0xEA
	c := NewCard(&fakeROM{sentinel: romSentinel})
	// After reset: ramRead=false, ramWrite=false.

	// Attempt write — must be a no-op.
	c.Write(0xD000, 0xFF)
	// Read $D000 → ROM byte (ramRead=false).
	if v := c.Read(0xD000); v != romSentinel {
		t.Fatalf("Read $D000 after ignored write: got %#x, want %#x (ROM)", v, romSentinel)
	}

	// Now enable write and confirm write lands.
	twoReadEnable(c, 0x03) // RDWRBSR2: ramRead=true, ramWrite=true, bank2Sel=true
	c.Write(0xD000, 0xAA)
	if v := c.Read(0xD000); v != 0xAA {
		t.Fatalf("Read $D000 after write (ramWrite=true): got %#x, want 0xAA", v)
	}
}

// -----------------------------------------------------------------------
// Test 12: TestMirroredAddressesBehaveIdentically
// -----------------------------------------------------------------------

// TestMirroredAddressesBehaveIdentically verifies that $C084–$C087 mirror
// $C080–$C083, and $C08C–$C08F mirror $C088–$C08B. Parameterized across
// all four mirror-pairs per quartet × 2 quartets.
func TestMirroredAddressesBehaveIdentically(t *testing.T) {
	// Mirror pairs: (canonical low, mirror low).
	pairs := [][2]uint8{
		{0x0, 0x4}, {0x1, 0x5}, {0x2, 0x6}, {0x3, 0x7}, // bank2 quartet
		{0x8, 0xC}, {0x9, 0xD}, {0xA, 0xE}, {0xB, 0xF}, // bank1 quartet
	}

	for _, p := range pairs {
		canonical, mirror := p[0], p[1]

		// Apply canonical switch (read), snapshot.
		c1 := NewCard(&fakeROM{})
		c1.handleSwitch(canonical, false)
		s1 := snap(c1)

		// Apply mirror switch (read), snapshot.
		c2 := NewCard(&fakeROM{})
		c2.handleSwitch(mirror, false)
		s2 := snap(c2)

		if s1 != s2 {
			t.Errorf("canonical $C08%X and mirror $C08%X disagree: %+v vs %+v",
				canonical, mirror, s1, s2)
		}
	}
}

// -----------------------------------------------------------------------
// Test 13: TestSwitchReadReturnsZero
// -----------------------------------------------------------------------

// TestSwitchReadReturnsZero pins the API contract that Switches.Read always
// returns 0. Known divergence from floating-bus hardware behavior (§7 risk 4).
func TestSwitchReadReturnsZero(t *testing.T) {
	c := NewCard(&fakeROM{})
	sw := NewSwitches(c)

	for addr := uint16(0xC080); addr <= 0xC08F; addr++ {
		if v := sw.Read(addr); v != 0 {
			t.Errorf("Switches.Read($%04X): got %#x, want 0", addr, v)
		}
	}
}

// -----------------------------------------------------------------------
// Test 14: TestCardPluggedIntoBus
// -----------------------------------------------------------------------

// TestCardPluggedIntoBus wires the LC into a real bus.Bus with a fake ROM,
// and verifies that bus-level reads and writes route correctly based on
// softswitch state.
func TestCardPluggedIntoBus(t *testing.T) {
	const romSentinel = 0xBD
	rom := &fakeROM{sentinel: romSentinel}
	c := NewCard(rom)
	sw := NewSwitches(c)

	b := bus.NewBus()
	b.Map(0xD000, 0xFFFF, c)
	b.Map(0xC080, 0xC08F, sw)

	// After reset: ramRead=false → bus read at $D000 returns ROM sentinel.
	if v := b.Read(0xD000); v != romSentinel {
		t.Fatalf("bus Read $D000 (ROM mode): got %#x, want %#x", v, romSentinel)
	}

	// Enable write+read via $C083 twice (through bus softswitches).
	b.Read(0xC083) // first read: prearm=true
	b.Read(0xC083) // second read: ramWrite=true, ramRead=true
	if !c.ramWrite || !c.ramRead {
		t.Fatal("after two bus reads of $C083: ramWrite and ramRead must be true")
	}

	// Write 0x42 to $D000 via bus.
	b.Write(0xD000, 0x42)

	// Read $D000 — must return 0x42 (LC RAM, bank2 selected).
	if v := b.Read(0xD000); v != 0x42 {
		t.Fatalf("bus Read $D000 (RAM mode): got %#x, want 0x42", v)
	}
}

// -----------------------------------------------------------------------
// Test 15: TestBootWithLanguageCardROM (optional smoke test)
// -----------------------------------------------------------------------

// TestBootWithLanguageCardROM is an optional boot smoke test. If
// roms/Apple2_Plus.rom is absent, the test is skipped. It verifies that
// installing the LC over a real ROM doesn't corrupt ROM reads by comparing
// a Read of $D000 before and after LC installation (both should return
// the same ROM byte, since we start in ROM-read mode).
func TestBootWithLanguageCardROM(t *testing.T) {
	const romPath = "../roms/Apple2_Plus.rom"
	data, err := os.ReadFile(romPath)
	if err != nil {
		t.Skip("roms/Apple2_Plus.rom not found — skipping LC boot smoke test")
	}

	// Build a minimal ROMReader from the raw bytes.
	// Apple2_Plus.rom is 12 KB based at $D000.
	const romBase = 0xD000
	if len(data) != 12288 {
		t.Skipf("Apple2_Plus.rom unexpected size %d (want 12288) — skipping", len(data))
	}

	rom := &rawROM{data: data, base: romBase}
	c := NewCard(rom)

	// After reset: ramRead=false → LC delegates to ROM for all $D000-$FFFF reads.
	// Compare ROM byte at $D000 via LC vs direct ROM access.
	want := rom.Read(0xD000)
	if got := c.Read(0xD000); got != want {
		t.Fatalf("LC.Read($D000) in ROM-read mode: got %#x, want %#x (ROM byte)", got, want)
	}

	// Enable RAM read/write, write a sentinel, read back.
	twoReadEnable(c, 0x03) // RDWRBSR2: ramRead=true, ramWrite=true, bank2Sel=true
	c.Write(0xD000, 0x42)
	if v := c.Read(0xD000); v != 0x42 {
		t.Fatalf("LC.Read($D000) after RAM write: got %#x, want 0x42", v)
	}

	// Reset: ROM-read again; bank2 still holds 0x42 but we read ROM now.
	c.Reset()
	if got := c.Read(0xD000); got != want {
		t.Fatalf("LC.Read($D000) after Reset: got %#x, want %#x (ROM byte)", got, want)
	}
}

// rawROM is a minimal ROMReader backed by a raw byte slice.
type rawROM struct {
	data []uint8
	base uint16
}

func (r *rawROM) Read(addr uint16) uint8 {
	offset := int(addr) - int(r.base)
	if offset < 0 || offset >= len(r.data) {
		return 0xFF
	}
	return r.data[offset]
}
