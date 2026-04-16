package disk

// DOS 3.3 image integration tests.
//
// All tests in this file require the real DOS_3_3.dsk file at
// ../../disks/DOS_3_3.dsk (relative to the disk/ package). If the file is
// absent, every test calls t.Skip so CI without the file still passes.
//
// The file is identified by size (143360 bytes) and MD5
// b13de32fd7a97d817744bf2dd71d5479 (standard DOS 3.3 master).

import (
	"crypto/md5"
	"fmt"
	"os"
	"testing"
)

const (
	// dos33ImagePath is relative to the disk/ package directory, which is the
	// working directory when go test runs this package.
	dos33ImagePath = "../disks/DOS_3_3.dsk"
	dos33MD5       = "b13de32fd7a97d817744bf2dd71d5479"
)

// requireDOS33 returns the raw image bytes or calls t.Skip if unavailable.
func requireDOS33(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(dos33ImagePath)
	if err != nil {
		t.Skipf("DOS_3_3.dsk not found at %s (place the standard DOS 3.3 master image there to run this test): %v",
			dos33ImagePath, err)
	}
	return raw
}

// TestDOS33_ImageSize asserts the file is exactly 143 360 bytes
// (35 tracks × 16 sectors × 256 bytes).
func TestDOS33_ImageSize(t *testing.T) {
	raw := requireDOS33(t)
	if len(raw) != imageSize {
		t.Fatalf("DOS_3_3.dsk: size=%d, want %d", len(raw), imageSize)
	}
	// Also verify MD5 so tests are pinned to the canonical image.
	sum := fmt.Sprintf("%x", md5.Sum(raw))
	if sum != dos33MD5 {
		t.Errorf("DOS_3_3.dsk: md5=%s, want %s (wrong image variant?)", sum, dos33MD5)
	}
}

// TestDOS33_T0S0_BootSignature loads the image, reads logical track 0 sector 0,
// and verifies the first three bytes are $01 $A5 $27 (DOS 3.3 boot0 signature).
//
// In a DOS-order .dsk file, logical sector 0 of track 0 is stored at file
// offset 0.  The DISK2 PROM jumps to $0801 after loading; the byte at offset 0
// ($01) is a version marker and is never executed.
func TestDOS33_T0S0_BootSignature(t *testing.T) {
	raw := requireDOS33(t)

	// Logical sector 0 of track 0 is at file offset 0 in DOS order.
	t0s0 := raw[0:bytesPerSector]
	want := [3]uint8{0x01, 0xA5, 0x27}
	got := [3]uint8{t0s0[0], t0s0[1], t0s0[2]}
	if got != want {
		t.Errorf("T0S0 boot signature: got %#v, want %#v", got, want)
	}
}

// TestDOS33_EncodeTrack0_HasNibblePrologues encodes track 0 of the real image
// via EncodeTrack and verifies:
//   - Exactly 16 address-field prologues (D5 AA 96).
//   - Exactly 16 data-field prologues (D5 AA AD).
//   - Each address field decodes to vol=254, track=0, and physical sectors
//     0..15 exactly once (as a set).
//   - Inter-sector gaps contain at least one $FF self-sync byte.
func TestDOS33_EncodeTrack0_HasNibblePrologues(t *testing.T) {
	raw := requireDOS33(t)

	// Build the 16 logical sectors of track 0.
	var sectors [sectorsPerTrack][bytesPerSector]uint8
	for s := 0; s < sectorsPerTrack; s++ {
		offset := s * bytesPerSector // track 0, logical sector s
		copy(sectors[s][:], raw[offset:offset+bytesPerSector])
	}

	nibs := EncodeTrack(sectors, 254, 0, OrderDOS)

	addrCount := 0
	dataCount := 0
	seenSectors := map[uint8]bool{}
	hasFFGap := false

	for i := 0; i < len(nibs)-2; i++ {
		if nibs[i] == 0xFF {
			hasFFGap = true
		}
		if nibs[i] == 0xD5 && nibs[i+1] == 0xAA {
			switch nibs[i+2] {
			case 0x96: // address prologue
				addrCount++
				// Decode address field: 4 odd/even pairs follow at i+3.
				if i+3+8 <= len(nibs) {
					vol := decodeOddEven(nibs[i+3], nibs[i+4])
					track := decodeOddEven(nibs[i+5], nibs[i+6])
					sec := decodeOddEven(nibs[i+7], nibs[i+8])
					chk := decodeOddEven(nibs[i+9], nibs[i+10])
					if vol != 254 {
						t.Errorf("address field %d: vol=%d, want 254", addrCount, vol)
					}
					if track != 0 {
						t.Errorf("address field %d: track=%d, want 0", addrCount, track)
					}
					computedChk := vol ^ track ^ sec
					if chk != computedChk {
						t.Errorf("address field %d (sec=%d): checksum mismatch: got %02X, want %02X",
							addrCount, sec, chk, computedChk)
					}
					seenSectors[sec] = true
				}
			case 0xAD: // data prologue
				dataCount++
			}
		}
	}

	if addrCount != sectorsPerTrack {
		t.Errorf("address prologues (D5 AA 96): got %d, want %d", addrCount, sectorsPerTrack)
	}
	if dataCount != sectorsPerTrack {
		t.Errorf("data prologues (D5 AA AD): got %d, want %d", dataCount, sectorsPerTrack)
	}
	if len(seenSectors) != sectorsPerTrack {
		t.Errorf("unique physical sectors in address fields: got %d, want %d", len(seenSectors), sectorsPerTrack)
	}
	for s := 0; s < sectorsPerTrack; s++ {
		if !seenSectors[uint8(s)] {
			t.Errorf("physical sector %d missing from address fields", s)
		}
	}
	if !hasFFGap {
		t.Error("no $FF self-sync bytes found in encoded track (gaps expected)")
	}
}

// TestDOS33_EncodeDecodeRoundTrip_AllTracks encodes every track 0..34 and
// decodes each; every 256-byte sector must match the source file bytes at the
// expected file offset (applying DOS interleave to go from logical to physical).
func TestDOS33_EncodeDecodeRoundTrip_AllTracks(t *testing.T) {
	raw := requireDOS33(t)

	for tr := 0; tr < tracksPerDisk; tr++ {
		var sectors [sectorsPerTrack][bytesPerSector]uint8
		for s := 0; s < sectorsPerTrack; s++ {
			offset := (tr*sectorsPerTrack + s) * bytesPerSector
			copy(sectors[s][:], raw[offset:offset+bytesPerSector])
		}

		nibs := EncodeTrack(sectors, 254, tr, OrderDOS)
		recovered, err := DecodeTrack(nibs, OrderDOS)
		if err != nil {
			t.Errorf("track %d: DecodeTrack error: %v", tr, err)
			continue
		}
		if recovered != sectors {
			t.Errorf("track %d: round-trip mismatch", tr)
		}
	}
}

// TestDOS33_NibbleStreamPacing mounts the real image via Controller.Mount,
// turns the motor on, simulates the RWTS byte-read loop (advance *cyclePtr by
// 32, read $C0EC), and asserts that within ~6656 reads the controller emits a
// full D5 AA 96 sequence at least once.
//
// If this test fails it means the GCR encoding is broken or the pacing
// mechanism prevents valid nibbles from being returned.
func TestDOS33_NibbleStreamPacing(t *testing.T) {
	requireDOS33(t)

	c, cyc := newTestController(t)
	if err := c.Mount(0, dos33ImagePath, ""); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	// Motor on, ensure read mode (Q7=0, Q6=0).
	strobe(c, 0x09) // MOTORON
	strobe(c, 0x0E) // Q7L

	// Simulate RWTS-style loop: advance *cyclePtr by 32 before each read.
	// Each read that returns a byte with bit 7 set is a valid nibble.
	const maxReads = 6656 * 3 // three full revolutions to be safe

	// Collect last 3 nibbles to detect D5 AA 96.
	recent := [3]uint8{}

	found := false
	for i := 0; i < maxReads; i++ {
		*cyc += cyclesPerNibble
		v := strobe(c, 0x0C)
		if v&0x80 != 0 {
			// Valid nibble — shift into recent.
			recent[0], recent[1], recent[2] = recent[1], recent[2], v
			if recent[0] == 0xD5 && recent[1] == 0xAA && recent[2] == 0x96 {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("D5 AA 96 address prologue not found within %d reads; "+
			"GCR encoding may be broken or nibble pacing prevents valid bytes from being returned",
			maxReads)
	}
}

// TestDOS33_Bit7PacingNotReady mounts the real image, turns the motor on, reads
// $C0EC twice with NO cycle advance between reads, and asserts the second read
// has bit 7 CLEAR ("not ready").
//
// If this invariant is broken (both reads return bit 7 set), RWTS's
// `BPL *-3` wait-for-byte loop exits immediately without actually waiting for a
// new nibble, causing the address-field search to fail and DOS boot to hang.
func TestDOS33_Bit7PacingNotReady(t *testing.T) {
	requireDOS33(t)

	c, cyc := newTestController(t)
	if err := c.Mount(0, dos33ImagePath, ""); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	strobe(c, 0x09) // MOTORON
	strobe(c, 0x0E) // Q7L (read data mode)

	// Advance by a full nibble time so the first read gets a valid nibble.
	*cyc += cyclesPerNibble
	first := strobe(c, 0x0C)
	if first&0x80 == 0 {
		t.Logf("first read 0x%02X has bit 7 clear; advancing one more nibble time", first)
		*cyc += cyclesPerNibble
		first = strobe(c, 0x0C)
	}
	if first&0x80 == 0 {
		t.Fatalf("cannot get a valid nibble even after 2×cyclesPerNibble; motor/image not working")
	}

	// Second read with NO cycle advance — must have bit 7 CLEAR.
	second := strobe(c, 0x0C)
	if second&0x80 != 0 {
		t.Errorf("second $C0EC read (no cycle advance) returned 0x%02X with bit 7 SET; "+
			"pacing broken — RWTS BPL loop would exit prematurely and DOS boot would fail",
			second)
	}
}

// TestGCR_RunningXOR_Conformance verifies that EncodeTrack produces a nibble
// stream that is decodable by an independent, byte-accurate reimplementation of
// the Apple Disk II bootstrap ROM ($C600) read algorithm. This test does NOT
// call our DecodeTrack — it mirrors the exact ROM loops so that a
// self-referential encoder/decoder pair cannot hide a conformance bug.
//
// The Apple Disk II ROM decodes sector data in three stages:
//  1. Pre-nibble loop (86 iters, positions 85..0): A ^= decode[nibble]; preBuf[Y--] = A
//  2. Primary loop (256 iters, positions 0..255): A ^= decode[nibble]; priiBuf[Y++] = A
//  3. Checksum: A ^= decode[nibble]; if A != 0 -> error
//  4. POSTNIB: sector[k] = (priiBuf[k] << 2) | rev2(bits from preBuf[85-k%86])
func TestGCR_RunningXOR_Conformance(t *testing.T) {
	// Build a test sector: bytes 0x00..0xFF.
	var sector [bytesPerSector]uint8
	for i := range sector {
		sector[i] = uint8(i)
	}

	var sectors [sectorsPerTrack][bytesPerSector]uint8
	sectors[0] = sector // logical sector 0

	nibs := EncodeTrack(sectors, 254, 0, OrderDOS)

	// Find the data-field prologue D5 AA AD for physical sector 0.
	dataStart := -1
	for i := 0; i < len(nibs)-2; i++ {
		if nibs[i] == 0xD5 && nibs[i+1] == 0xAA && nibs[i+2] == 0xAD {
			dataStart = i + 3
			break
		}
	}
	if dataStart < 0 {
		t.Fatal("conformance: D5 AA AD data prologue not found in encoded track")
	}

	// Need 86 pre-nibbles + 256 primary + 1 checksum = 343 nibbles total.
	if dataStart+343 > len(nibs) {
		t.Fatalf("conformance: not enough nibbles after D5 AA AD (have %d, need 343)", len(nibs)-dataStart)
	}

	// Verify epilogue immediately follows the 343 nibbles.
	if nibs[dataStart+343] != 0xDE || nibs[dataStart+344] != 0xAA {
		t.Errorf("conformance: expected DE AA epilogue at offset 343/344, got %02X %02X",
			nibs[dataStart+343], nibs[dataStart+344])
	}

	// --- Stage 1 & 2: running-XOR read (mirrors Apple Disk II ROM at $C6AA-$C6CB) ---
	// A starts at 0 (because the ROM does EOR #$AD when finding the AD prologue byte,
	// making A = $AD ^ $AD = 0 when the prologue matches).
	// For 86 pre-nibbles: A ^= decode[nibble]; preBuf[Y--] = A  (Y goes 85..0)
	// For 256 primary:    A ^= decode[nibble]; priiBuf[Y++] = A (Y goes 0..255)
	preBuf := [86]uint8{}
	priiBuf := [256]uint8{}
	{
		A := uint8(0)
		// Pre-nibble loop: positions 85 down to 0.
		for k := 0; k < 86; k++ {
			nibble := nibs[dataStart+k]
			raw := gcr62Inv[nibble]
			if raw == 0xFF {
				t.Fatalf("conformance: invalid GCR nibble 0x%02X at pre-nibble offset %d", nibble, k)
			}
			A ^= raw
			preBuf[85-k] = A
		}
		// Primary loop: positions 0..255.
		for k := 0; k < 256; k++ {
			nibble := nibs[dataStart+86+k]
			raw := gcr62Inv[nibble]
			if raw == 0xFF {
				t.Fatalf("conformance: invalid GCR nibble 0x%02X at primary offset %d", nibble, k)
			}
			A ^= raw
			priiBuf[k] = A
		}
		// Checksum nibble: A ^= decode[chk]; must yield 0.
		chkNibble := nibs[dataStart+342]
		chkRaw := gcr62Inv[chkNibble]
		if chkRaw == 0xFF {
			t.Fatalf("conformance: invalid GCR checksum nibble 0x%02X", chkNibble)
		}
		A ^= chkRaw
		if A != 0 {
			t.Errorf("conformance: checksum mismatch: A = 0x%02X after checksum nibble, want 0x00", A)
		}
	}

	// --- Stage 3: POSTNIB16 (mirrors Apple Disk II ROM at $C6D5-$C6E9) ---
	// sector[k] = (priiBuf[k] << 2) | rev2(2 bits from preBuf[X])
	// X counts 85..0, resetting to 85 every 86 bytes.
	// Each call takes 2 bits (one LSR+ROL pair each), shifting preBuf[X] right.
	var got [bytesPerSector]uint8
	preBufWork := preBuf // copy so we can shift bits out
	X := 86             // will decrement to 85 on first iteration
	for k := 0; k < 256; k++ {
		X--
		if X < 0 {
			X = 85
		}
		A := priiBuf[k]
		// First LSR+ROL: carry = bit0 of preBuf[X]; A = (A<<1)|carry
		carry := preBufWork[X] & 1
		preBufWork[X] >>= 1
		A = (A<<1 | carry) & 0xFF
		// Second LSR+ROL
		carry = preBufWork[X] & 1
		preBufWork[X] >>= 1
		A = (A<<1 | carry) & 0xFF
		got[k] = A
	}

	if got != sector {
		for i := range sector {
			if got[i] != sector[i] {
				t.Errorf("conformance: first mismatch at byte %d: got 0x%02X, want 0x%02X", i, got[i], sector[i])
				break
			}
		}
		t.Errorf("conformance: Apple Disk II ROM POSTNIB16 could not recover original sector from our encoded nibbles")
	}
}

// captureTracer is a Tracer that records emitted events for assertion in tests.
type captureTracer struct {
	nibbles      []uint8
	phaseSrcEvents int
	modeEvents   []string
	addrFields   []addrField
}

type addrField struct {
	vol, track, sector uint8
}

func (ct *captureTracer) TraceNibbleRead(_ uint64, _, _, _ int, value uint8) {
	ct.nibbles = append(ct.nibbles, value)
}
func (ct *captureTracer) TracePhaseStrobe(_ uint64, _, _ int, _ bool, _, _ int, _ [4]bool) {
	ct.phaseSrcEvents++
}
func (ct *captureTracer) TraceModeChange(_ uint64, _ int, event string) {
	ct.modeEvents = append(ct.modeEvents, event)
}
func (ct *captureTracer) TraceAddressField(_ uint64, _, _, _ int, vol, track, sector uint8) {
	ct.addrFields = append(ct.addrFields, addrField{vol, track, sector})
}

// TestDOS33_TracerAddressFields verifies that when a Tracer is wired, it
// receives TraceAddressField calls for valid D5 AA 96 sequences during a
// simulated read loop.
func TestDOS33_TracerAddressFields(t *testing.T) {
	requireDOS33(t)

	c, cyc := newTestController(t)
	if err := c.Mount(0, dos33ImagePath, ""); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	tr := &captureTracer{}
	c.SetTracer(tr)

	strobe(c, 0x09) // MOTORON
	strobe(c, 0x0E) // Q7L

	const maxReads = 6656 * 3
	for i := 0; i < maxReads; i++ {
		*cyc += cyclesPerNibble
		strobe(c, 0x0C)
		if len(tr.addrFields) >= 2 {
			break
		}
	}

	if len(tr.addrFields) == 0 {
		t.Error("Tracer received no TraceAddressField events; address detection broken")
	} else {
		af := tr.addrFields[0]
		if af.vol != 254 {
			t.Errorf("first address field: vol=%d, want 254", af.vol)
		}
		if af.track != 0 {
			t.Errorf("first address field: track=%d, want 0", af.track)
		}
		t.Logf("first address field: vol=%d trk=%d sec=%d", af.vol, af.track, af.sector)
	}

	if len(tr.modeEvents) == 0 {
		t.Error("Tracer received no TraceModeChange events (MOTORON expected)")
	}
}
