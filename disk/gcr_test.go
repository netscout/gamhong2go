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
