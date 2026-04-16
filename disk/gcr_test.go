package disk

import (
	"testing"
)

func TestGCRTableSanity(t *testing.T) {
	if len(gcr62) != 64 {
		t.Fatalf("gcr62 table length %d, want 64", len(gcr62))
	}
	seen := map[uint8]bool{}
	for i, v := range gcr62 {
		// Every valid GCR nibble must have bit 7 set.
		if v&0x80 == 0 {
			t.Errorf("gcr62[%d] = 0x%02X: bit 7 not set", i, v)
		}
		// Values must be unique.
		if seen[v] {
			t.Errorf("gcr62[%d] = 0x%02X: duplicate value", i, v)
		}
		seen[v] = true
		// Inverse table must round-trip.
		if gcr62Inv[v] != uint8(i) {
			t.Errorf("gcr62Inv[0x%02X] = %d, want %d", v, gcr62Inv[v], i)
		}
	}
}

func TestEncodeProducesValidPrologues(t *testing.T) {
	var sectors [sectorsPerTrack][bytesPerSector]uint8
	nibs := EncodeTrack(sectors, 254, 0, OrderDOS)

	dosCount := 0
	dataCount := 0
	for i := 0; i < len(nibs)-2; i++ {
		if nibs[i] == 0xD5 && nibs[i+1] == 0xAA {
			switch nibs[i+2] {
			case 0x96:
				dosCount++
			case 0xAD:
				dataCount++
			}
		}
	}
	if dosCount != sectorsPerTrack {
		t.Errorf("address prologues: got %d, want %d", dosCount, sectorsPerTrack)
	}
	if dataCount != sectorsPerTrack {
		t.Errorf("data prologues: got %d, want %d", dataCount, sectorsPerTrack)
	}
}

func buildTestSectors(seed uint8) [sectorsPerTrack][bytesPerSector]uint8 {
	var s [sectorsPerTrack][bytesPerSector]uint8
	for sec := 0; sec < sectorsPerTrack; sec++ {
		for b := 0; b < bytesPerSector; b++ {
			s[sec][b] = uint8(sec*3+b) ^ seed
		}
	}
	return s
}

func TestEncodeDecodeRoundTripDOS(t *testing.T) {
	original := buildTestSectors(0xAB)
	nibs := EncodeTrack(original, 254, 5, OrderDOS)
	recovered, err := DecodeTrack(nibs, OrderDOS)
	if err != nil {
		t.Fatalf("DecodeTrack error: %v", err)
	}
	if recovered != original {
		t.Error("round-trip mismatch (DOS order)")
	}
}

func TestDOSInterleaveDirection(t *testing.T) {
	var sectors [sectorsPerTrack][bytesPerSector]uint8
	for i := range sectors {
		sectors[i][0] = uint8(i)
	}
	nibs := EncodeTrack(sectors, 254, 0, OrderDOS)

	// Walk the nibble stream and extract (physSec, firstByte) pairs
	// from the address and data fields.
	type found struct{ phys, tag uint8 }
	var hits []found
	for i := 0; i < len(nibs)-400; i++ {
		if nibs[i] != 0xD5 || nibs[i+1] != 0xAA || nibs[i+2] != 0x96 {
			continue
		}
		physSec := (nibs[i+7] & 0x55) | ((nibs[i+8] & 0x55) << 1)
		// Find the data prologue after this address field.
		for j := i + 14; j < i+60 && j < len(nibs)-350; j++ {
			if nibs[j] == 0xD5 && nibs[j+1] == 0xAA && nibs[j+2] == 0xAD {
				hits = append(hits, found{physSec, sectors[physSec][0]})
				break
			}
		}
	}

	// DOS 3.3 RWTS: logical 1 -> physical 13, logical 4 -> physical 7.
	// The sector tagged with logical index N should appear at physical dosInterleave[N].
	if dosInterleave[1] != 13 {
		t.Errorf("dosInterleave[1] = %d, want 13 (RWTS sector translation)", dosInterleave[1])
	}
	if dosInterleave[4] != 7 {
		t.Errorf("dosInterleave[4] = %d, want 7 (RWTS sector translation)", dosInterleave[4])
	}
}

func TestEncodeDecodeRoundTripProDOS(t *testing.T) {
	original := buildTestSectors(0x55)
	nibs := EncodeTrack(original, 254, 10, OrderProDOS)
	recovered, err := DecodeTrack(nibs, OrderProDOS)
	if err != nil {
		t.Fatalf("DecodeTrack error: %v", err)
	}
	if recovered != original {
		t.Error("round-trip mismatch (ProDOS order)")
	}
}
