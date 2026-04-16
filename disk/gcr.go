// Package disk emulates the Apple Disk II controller (slot 6).
package disk

import "fmt"

// SectorOrder selects the logical-to-physical sector mapping used when
// encoding/decoding a disk image.
type SectorOrder int

const (
	OrderDOS    SectorOrder = iota // .dsk / .do  -- DOS 3.3 order
	OrderProDOS                    // .po         -- ProDOS order
)

// dosInterleave maps logical sector index -> physical sector on a DOS 3.3 track.
// This is the RWTS sector translation table from the DOS 3.3 source.
var dosInterleave = [16]int{0, 13, 11, 9, 7, 5, 3, 1, 14, 12, 10, 8, 6, 4, 2, 15}

// prodosInterleave maps logical sector index -> physical sector on a ProDOS track.
var prodosInterleave = [16]int{0, 2, 4, 6, 8, 10, 12, 14, 1, 3, 5, 7, 9, 11, 13, 15}

// gcr62 is the 6-and-2 write-translate table. Index is the 6-bit value (0..63);
// value is the on-disk nibble byte (bit 7 always set).
var gcr62 = [64]uint8{
	0x96, 0x97, 0x9A, 0x9B, 0x9D, 0x9E, 0x9F, 0xA6,
	0xA7, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF, 0xB2, 0xB3,
	0xB4, 0xB5, 0xB6, 0xB7, 0xB9, 0xBA, 0xBB, 0xBC,
	0xBD, 0xBE, 0xBF, 0xCB, 0xCD, 0xCE, 0xCF, 0xD3,
	0xD6, 0xD7, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE,
	0xDF, 0xE5, 0xE6, 0xE7, 0xE9, 0xEA, 0xEB, 0xEC,
	0xED, 0xEE, 0xEF, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6,
	0xF7, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF,
}

// gcr62Inv is the reverse table: nibble byte -> 6-bit value, 0xFF = invalid.
var gcr62Inv [256]uint8

func init() {
	for i := range gcr62Inv {
		gcr62Inv[i] = 0xFF
	}
	for i, v := range gcr62 {
		gcr62Inv[v] = uint8(i)
	}
}

const (
	tracksPerDisk   = 35
	sectorsPerTrack = 16
	bytesPerSector  = 256

	// Gap sizes (in $FF self-sync bytes).
	headGap   = 48
	interGap1 = 6
	interGap2 = 27
)

// appendOddEven encodes v as two bytes in Apple II 4+4 odd/even format.
//
//	byte1 = ((v >> 1) & 0x55) | 0xAA   (odd bits)
//	byte2 = (v & 0x55) | 0xAA          (even bits)
func appendOddEven(buf []uint8, v uint8) []uint8 {
	return append(buf,
		((v>>1)&0x55)|0xAA,
		(v&0x55)|0xAA,
	)
}

// decodeOddEven reconstructs a byte from two odd/even encoded bytes.
func decodeOddEven(odd, even uint8) uint8 {
	return ((odd & 0x55) << 1) | (even & 0x55)
}

// EncodeTrack converts 16 sectors of 256 bytes (in logical sector order) into
// a GCR 6-and-2 nibble stream. vol is the disk volume byte (typically 254).
func EncodeTrack(sectorData [sectorsPerTrack][bytesPerSector]uint8, vol uint8, trackNum int, order SectorOrder) []uint8 {
	fwd := dosInterleave
	if order == OrderProDOS {
		fwd = prodosInterleave
	}
	// physToLog[physSec] = logical sector; inverse of the forward interleave table.
	var physToLogEnc [sectorsPerTrack]int
	for logSec, phys := range fwd {
		physToLogEnc[phys] = logSec
	}

	buf := make([]uint8, 0, 7000)

	// Head gap.
	for i := 0; i < headGap; i++ {
		buf = append(buf, 0xFF)
	}

	for physSec := 0; physSec < sectorsPerTrack; physSec++ {
		logSec := physToLogEnc[physSec]

		sector := sectorData[logSec]

		// --- Address field ---
		buf = append(buf, 0xD5, 0xAA, 0x96) // prologue
		buf = appendOddEven(buf, vol)
		buf = appendOddEven(buf, uint8(trackNum))
		buf = appendOddEven(buf, uint8(physSec))
		chk := vol ^ uint8(trackNum) ^ uint8(physSec)
		buf = appendOddEven(buf, chk)
		buf = append(buf, 0xDE, 0xAA, 0xEB) // epilogue

		// Gap 1 (between address and data fields).
		for i := 0; i < interGap1; i++ {
			buf = append(buf, 0xFF)
		}

		// --- Data field ---
		buf = append(buf, 0xD5, 0xAA, 0xAD) // prologue

		// 6-and-2 pre-nibble packing (PRENIB16 compatible with Apple Disk II ROM at $C600).
		//
		// The Apple Disk II bootstrap ROM POSTNIB16 loop reconstructs sector bytes as follows:
		//   For sector[k] (k=0..255): load primary nibble (top 6 bits), then extract 2 bits
		//   from pre_buf[85-k%86] via two LSR/ROL operations.
		//   The loop uses X = 85, 84, ..., 0, reset to 85 every 86 bytes.
		//
		// Therefore pre[j] must hold the low-2-bit pairs for sector bytes at offsets
		// (85-j), (171-j), and (257-j):
		//   pre[j] bits [1:0] = rev2(sector[85-j]  & 3)   for j=0..85
		//   pre[j] bits [3:2] = rev2(sector[171-j] & 3)   for j=0..85
		//   pre[j] bits [5:4] = rev2(sector[257-j] & 3)   for j=2..85  (k=172..255)
		// (j=0 and j=1 have no third byte since sector[257] and sector[256] are out of range.)

		pre := [86]uint8{}
		for j := 0; j < 86; j++ {
			b0 := sector[85-j]
			b1 := sector[171-j]
			pre[j] = rev2(b0&0x03) | (rev2(b1&0x03) << 2)
		}
		// Third byte: j=2..85 → sector[255..172].
		for j := 2; j < 86; j++ {
			pre[j] |= rev2(sector[257-j]&0x03) << 4
		}

		// Encode pre-nibbles in reverse order with pairwise XOR (canonical RWTS WRITE16).
		// Each emitted 6-bit value is: v[k] XOR v[k-1] (last = previous raw value).
		// This is algebraically equivalent to RWTS's running-XOR EOR decoder
		// (telescoping property): the PROM recovers v[k] via decoded = nibble XOR last;
		// last = decoded, which correctly undoes the pairwise difference encoding.
		last := uint8(0)
		for i := 85; i >= 0; i-- {
			buf = append(buf, gcr62[pre[i]^last])
			last = pre[i]
		}

		// Encode 256 data nibbles (bits 7:2 of each byte) with same running XOR.
		for i := 0; i < 256; i++ {
			sixBit := sector[i] >> 2
			buf = append(buf, gcr62[sixBit^last])
			last = sixBit
		}

		// Checksum nibble: the final last value, decoded by RWTS as 0 after XOR.
		buf = append(buf, gcr62[last])

		buf = append(buf, 0xDE, 0xAA, 0xEB) // epilogue

		// Gap 2 (inter-sector).
		for i := 0; i < interGap2; i++ {
			buf = append(buf, 0xFF)
		}
	}

	return buf
}

// rev2 reverses the two low bits of v.
func rev2(v uint8) uint8 {
	return ((v & 1) << 1) | ((v >> 1) & 1)
}

// DecodeTrack converts a GCR nibble buffer back to 16 sectors of 256 bytes.
// Returns an error if any sector cannot be found or decoded.
func DecodeTrack(nibbles []uint8, order SectorOrder) ([sectorsPerTrack][bytesPerSector]uint8, error) {
	// physToLog[physSec] = logical sector index.
	// The interleave tables map logical->physical, so we build the inverse here.
	fwdInterleave := dosInterleave
	if order == OrderProDOS {
		fwdInterleave = prodosInterleave
	}
	var physToLog [sectorsPerTrack]int
	for logSec, phys := range fwdInterleave {
		physToLog[phys] = logSec
	}

	var result [sectorsPerTrack][bytesPerSector]uint8
	n := len(nibbles)
	if n == 0 {
		return result, fmt.Errorf("decode: empty nibble buffer")
	}

	pos := 0

	// at returns nibbles[pos+offset] (mod n).
	at := func(offset int) uint8 {
		return nibbles[(pos+offset)%n]
	}

	// skipTo advances pos until nibbles[pos] == target, searching at most n bytes.
	skipTo := func(target uint8) bool {
		for j := 0; j < n; j++ {
			if nibbles[(pos+j)%n] == target {
				pos = (pos + j) % n
				return true
			}
		}
		return false
	}

	sectorsFound := 0
	maxIter := n + sectorsPerTrack*100

	for sectorsFound < sectorsPerTrack && maxIter > 0 {
		maxIter--

		// Find address prologue D5 AA 96.
		if !skipTo(0xD5) {
			break
		}
		if at(1) != 0xAA || at(2) != 0x96 {
			pos = (pos + 1) % n
			continue
		}
		pos = (pos + 3) % n

		// Read address field (4 pairs: vol, track, sector, checksum).
		vol := decodeOddEven(at(0), at(1))
		track := decodeOddEven(at(2), at(3))
		physSec := decodeOddEven(at(4), at(5))
		storedChk := decodeOddEven(at(6), at(7))
		pos = (pos + 8) % n

		computedChk := vol ^ track ^ physSec
		if storedChk != computedChk {
			continue
		}
		if physSec >= sectorsPerTrack {
			continue
		}

		// Find data prologue D5 AA AD (skip epilogue DE AA EB and gaps).
		if !skipTo(0xD5) {
			break
		}
		if at(1) != 0xAA || at(2) != 0xAD {
			pos = (pos + 1) % n
			continue
		}
		pos = (pos + 3) % n

		// Read 343 raw nibbles (86 pre + 256 data + 1 checksum).
		rawNibs := make([]uint8, 343)
		valid := true
		for k := 0; k < 343; k++ {
			dec := gcr62Inv[at(0)]
			if dec == 0xFF {
				valid = false
				break
			}
			rawNibs[k] = dec
			pos = (pos + 1) % n
		}
		if !valid {
			continue
		}

		// Decode XOR chain to recover pre[] and sixBit[] values.
		decoded := make([]uint8, 343)
		last := uint8(0)
		for k := 0; k < 343; k++ {
			decoded[k] = rawNibs[k] ^ last
			last = decoded[k]
		}

		// Reconstruct pre[0..85] from stream positions 0..85 (stored reversed).
		pre := [86]uint8{}
		for k := 0; k < 86; k++ {
			pre[85-k] = decoded[k]
		}

		// Reconstruct sector bytes: combine sixBit (upper 6) and pre bits (lower 2).
		// pre[j] holds bits from sector[85-j], sector[171-j], sector[257-j], so:
		//   sector[k] (k<86):    bits [1:0] of pre[85-k]
		//   sector[k] (k<172):   bits [3:2] of pre[171-k]
		//   sector[k] (k<256):   bits [5:4] of pre[257-k]
		var sector [bytesPerSector]uint8
		for k := 0; k < 256; k++ {
			sixBit := decoded[86+k]
			var low2 uint8
			switch {
			case k < 86:
				low2 = rev2(pre[85-k] & 0x03)
			case k < 172:
				low2 = rev2((pre[171-k] >> 2) & 0x03)
			default:
				low2 = rev2((pre[257-k] >> 4) & 0x03)
			}
			sector[k] = (sixBit << 2) | low2
		}

		// Map physical sector to logical sector and store.
		logSec := physToLog[physSec]
		result[logSec] = sector
		sectorsFound++
	}

	if sectorsFound < sectorsPerTrack {
		return result, fmt.Errorf("decode: only found %d of %d sectors", sectorsFound, sectorsPerTrack)
	}
	return result, nil
}
